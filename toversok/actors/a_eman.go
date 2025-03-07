package actors

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"runtime/debug"
	"slices"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/relay"
	"github.com/edup2p/common/types/stun"
	"golang.org/x/exp/maps"
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

// TODO future:
//  - UPnP? Other stuff?

func (em *EndpointManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(em).Error("panicked", "panic", v, "stack", string(debug.Stack()))
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
			em.getLocalEndpoints()
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
					em.getLocalEndpoints()
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

	stunReq := &msgactor.DRouterPushSTUN{Packets: make(map[netip.AddrPort][]byte)}

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
		return fmt.Errorf("received different TXID from raddr %s than expected: expected %s, got %s", from, em.stunRequests[from].txid, tid)
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

func (em *EndpointManager) getLocalEndpoints() {
	localEndpoints := em.collectLocalEndpoints()

	L(em).Debug("local endpoints collected", "endpoints", localEndpoints)

	if len(localEndpoints) > 0 {
		em.s.setLocalEndpoints(localEndpoints)
	}
}

func (em *EndpointManager) collectLocalEndpoints() []netip.Addr {
	ifaces, err := net.Interfaces()
	if err != nil {
		L(em).Error("collectLocalEndpoints: failed to list interfaces", "error", err)
		return nil
	}

	var ips []netip.Addr

	// handle err
	for _, i := range ifaces {

		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagPointToPoint != 0 {
			// Skip interfaces that are down, or are also PPP (such as tailscale)
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			L(em).Warn("collectLocalEndpoints: could not get addresses from interface", "error", err, "iface", i.Name)
			continue
		}
		// handle err
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip.IsLoopback() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
				continue
			}

			nip, ok := netip.AddrFromSlice(ip)
			if !ok {
				L(em).Warn("collectLocalEndpoints: could not get addr from slice", "ip", ip)
				continue
			}

			ips = append(ips, types.NormaliseAddr(nip))
		}
	}

	return ips
}

func (em *EndpointManager) Close() {
	em.ticker.Stop()
	em.stunTimeout.Stop()
}
