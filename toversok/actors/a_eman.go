package actors

import (
	"context"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/relay"
	"github.com/edup2p/common/types/stun"
	"golang.org/x/exp/maps"
	"net/netip"
	"slices"
	"time"
)

func (s *Stage) makeEM() *EndpointManager {
	em := &EndpointManager{
		ActorCommon: MakeCommon(s.Ctx, SessManInboxChLen),
		s:           s,

		ticker:      time.NewTicker(EManTickerInterval),
		stunTimeout: time.NewTimer(EManStunTimeout),
		relays:      make(map[int64]relay.Information),
	}

	em.stunTimeout.Stop()

	return em
}

type EndpointManager struct {
	*ActorCommon
	s *Stage

	ticker *time.Ticker // 60s

	didStartup bool

	// will be nil if not currently doing STUN
	collectedResponse  []stunResponse
	stunRequests       map[netip.AddrPort]stunRequest
	relayStunEndpoints map[netip.AddrPort]int64
	stunTimeout        *time.Timer

	relays map[int64]relay.Information
}

type stunRequest struct {
	txid stun.TxID

	sendTimestamp time.Time
}

type stunResponse struct {
	respondedAddrPort netip.AddrPort

	fromAddrPort netip.AddrPort

	latency time.Duration
}

// TODO
//  - receive relay info
//  - do STUN requests to each and resolve remote endpoints
//    - maybe determine when symmetric nat / "varies" is happening
//  - do latency determination
//    - inform relay manager that results are ready
//    - relay manager switches home relay and informs stage of that decision
//  - collect local endpoints

// TODO future:
//  - UPnP? Other stuff?

func (em *EndpointManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(em).Error("panicked", "panic", v)
			em.Cancel()
			bail(em.ctx, v)
		}
	}()

	if !em.running.CheckOrMark() {
		L(em).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-em.ctx.Done():
			em.Close()
			return
		case <-em.ticker.C:
			em.startSTUN()
		case <-em.stunTimeout.C:
			em.onSTUNTimeout()
		case m := <-em.inbox:
			switch m := m.(type) {
			case *msgactor.UpdateRelayConfiguration:
				for _, c := range m.Config {
					em.relays[c.ID] = c
				}

				// Quickly update endpoints now that we have STUN for the first time
				if !em.didStartup {
					em.startSTUN()
					em.didStartup = true
				}

			case *msgactor.EManSTUNResponse:
				if err := em.onSTUNResponse(m.Endpoint, m.Packet, m.Timestamp); err != nil {
					L(em).Error("error when processing STUN response", "endpoint", m.Endpoint, "error", err)
				}

			default:
				em.logUnknownMessage(m)
			}
		}
	}
}

func (em *EndpointManager) startSTUN() {
	if em.collectedResponse != nil {
		L(em).Error("tried to start STUN while it was already underway")
		return
	}

	em.collectedResponse = make([]stunResponse, 0)

	var stunReq = &msgactor.DRouterPushSTUN{Packets: make(map[netip.AddrPort][]byte)}

	em.stunRequests = make(map[netip.AddrPort]stunRequest)

	ts := time.Now()

	// FIXME clean up this mess,
	//  make the ongoing STUN process a pointer to a struct or something

	em.relayStunEndpoints = em.collectRelaySTUNEndpoints()

	stunEndpoints := slices.Concat(em.s.ControlSTUN(), maps.Keys(em.relayStunEndpoints))

	for _, ep := range stunEndpoints {
		txID := stun.NewTxID()
		req := stun.Request(txID)

		stunReq.Packets[ep] = req
		em.stunRequests[ep] = stunRequest{txID, ts}
	}

	if len(em.stunRequests) == 0 {
		// We're sending no packets, abort
		L(em).Warn("aborted STUN due to no endpoints")
		em.collectedResponse = nil
		return
	}

	SendMessage(em.s.DRouter.Inbox(), stunReq)
	em.stunTimeout.Reset(EManStunTimeout)
}

func (em *EndpointManager) onSTUNResponse(from netip.AddrPort, pkt []byte, ts time.Time) error {
	if em.collectedResponse == nil {
		return fmt.Errorf("STUN is not active")
	}

	from = types.NormaliseAddrPort(from)

	if _, ok := em.stunRequests[from]; !ok {
		return fmt.Errorf("got response from unexpected raddr while doing STUN: %s", from)
	}

	req := em.stunRequests[from]

	tid, saddr, err := stun.ParseResponse(pkt)
	if err != nil {
		return fmt.Errorf("got error when parsing STUN response from %s: %w", from, err)
	}
	if tid != req.txid {
		return fmt.Errorf("received different TXID from raddr %s than expected: expected %s, got %s", from, em.stunRequests[from], tid)
	}

	latency := ts.Sub(req.sendTimestamp)

	em.collectedResponse = append(em.collectedResponse, stunResponse{
		respondedAddrPort: saddr,
		fromAddrPort:      from,
		latency:           latency,
	})
	delete(em.stunRequests, from)

	if len(em.stunRequests) == 0 {
		em.finaliseSTUN(false)
	}

	return nil
}

func (em *EndpointManager) onSTUNTimeout() {
	if em.collectedResponse == nil {
		L(em).Warn("got timeout notice while not performing STUN")
		return
	}

	em.finaliseSTUN(true)
}

func (em *EndpointManager) finaliseSTUN(timeout bool) {
	ep := em.collectSTUNResponses()

	sortEndpointSlice(ep)

	// Logging
	if timeout {
		if len(ep) > 0 {
			L(em).Warn("STUN completed with non-responsive servers", "endpoints", ep, "stun-not-responded", maps.Keys(em.stunRequests))
		} else {
			L(em).Warn("STUN failed, timed out with no endpoints")
		}
	} else {
		L(em).Debug("STUN completed", "endpoints", ep)
	}

	if len(ep) > 0 {
		em.s.setSTUNEndpoints(ep)
	}

	em.collectedResponse = nil
	em.stunTimeout.Stop()
}

func (em *EndpointManager) collectSTUNResponses() []netip.AddrPort {
	collected := make(map[netip.AddrPort]bool)
	relayLatency := make(map[int64]time.Duration)

	for _, r := range em.collectedResponse {
		collected[r.respondedAddrPort] = true

		if rid := em.endpointToRelay(r.fromAddrPort); rid != nil {
			rid := *rid

			if latency, ok := relayLatency[rid]; (ok && latency > r.latency) || !ok {
				relayLatency[rid] = r.latency
			}
		} else {
			L(em).Log(context.Background(), types.LevelTrace, "collectSTUNResponses: could not match ap to relay", "ap", r.respondedAddrPort.String())
		}
	}

	go SendMessage(em.s.RMan.Inbox(), &msgactor.RManRelayLatencyResults{RelayLatency: relayLatency})

	em.collectedResponse = nil

	return maps.Keys(collected)
}

func (em *EndpointManager) endpointToRelay(ap netip.AddrPort) *int64 {
	if i, ok := em.relayStunEndpoints[ap]; ok {
		return &i
	}

	return nil
}

//func (em *EndpointManager) updateEndpoints() {
//	ep, err := em.doSTUN(EManStunTimeout)
//	if err != nil {
//		if ep != nil && len(ep) > 1 {
//			L(em).Warn("STUN completed with error", "endpoints", ep, "err", err)
//		} else {
//			L(em).Warn("STUN failed with error", "err", err)
//		}
//	}
//	if ep != nil && len(ep) > 1 {
//		em.s.setSTUNEndpoints(ep)
//		L(em).Info("STUN completed", "endpoints", ep)
//	} else {
//		L(em).Warn("STUN completed with no endpoints")
//	}
//}

//// Performs STUN on all known servers, returns all (deduplicated) results, and any error (if there is one).
//func (em *EndpointManager) doSTUN(timeout time.Duration) (responses []netip.AddrPort, err error) {
//	var c *net.UDPConn
//
//	c, err = net.ListenUDP("udp", nil)
//	if err != nil {
//		return nil, fmt.Errorf("failed to open UDP socket: %w", err)
//	}
//
//	requests := make(map[netip.AddrPort]stun.TxID)
//
//	for _, ep := range em.collectSTUNEndpoints() {
//		txID := stun.NewTxID()
//		req := stun.Request(txID)
//
//		_, err = c.WriteToUDP(req, net.UDPAddrFromAddrPort(ep))
//		if err != nil {
//			return nil, fmt.Errorf("failed to write to %s: %w", ep, err)
//		}
//
//		requests[ep] = txID
//	}
//
//	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
//		return nil, fmt.Errorf("failed to set read deadline: %w", err)
//	}
//
//	var responseMap = make(map[netip.AddrPort]bool)
//
//	for {
//		if len(requests) == 0 {
//			break
//		}
//
//		var buf [1024]byte
//		var n int
//		var raddr netip.AddrPort
//
//		n, raddr, err = c.ReadFromUDPAddrPort(buf[:])
//		if err != nil {
//			break
//		}
//
//		if raddr.Addr().Is4In6() {
//			raddr = netip.AddrPortFrom(netip.AddrFrom4(raddr.Addr().As4()), raddr.Port())
//		}
//
//		if _, ok := requests[raddr]; !ok {
//			L(em).Warn("got response from unexpected raddr while doing STUN", "raddr", raddr)
//			continue
//		}
//
//		tid, saddr, err := stun.ParseResponse(buf[:n])
//		if err != nil {
//			L(em).Warn("got error when parsing STUN response from raddr", "raddr", raddr, "err", err)
//			continue
//		}
//		if tid != requests[raddr] {
//			L(em).Warn("received different TXID from raddr than expected", "raddr", raddr, "txid.expected", requests[raddr], "txid.got", tid)
//			continue
//		}
//
//		responseMap[saddr] = true
//		delete(requests, raddr)
//	}
//
//	for ep := range responseMap {
//		responses = append(responses, ep)
//	}
//
//	return responses, err
//}

// Collects STUN endpoints from known relay definitions and Control itself
func (em *EndpointManager) collectRelaySTUNEndpoints() map[netip.AddrPort]int64 {
	relayEndpoints := make(map[netip.AddrPort]int64)

	for _, ri := range em.relays {
		for _, ip := range types.SliceOrEmpty(ri.IPs) {
			relayEndpoints[netip.AddrPortFrom(ip, types.PtrOr(ri.STUNPort, stun.DefaultPort))] = ri.ID
		}
	}

	return relayEndpoints
}

func (em *EndpointManager) Close() {
	//TODO implement me
	panic("implement me")
}
