package actors

import (
	"context"
	"maps"
	"net/netip"
	"runtime/debug"
	"time"

	"github.com/edup2p/common/toversok/actors/peerstate"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/stage"
	xmaps "golang.org/x/exp/maps"
)

type TrafficManager struct {
	*ActorCommon
	s *Stage

	ticker *time.Ticker     // 250ms
	poke   chan interface{} // len 1

	peerState map[key.NodePublic]peerstate.PeerState

	pings     map[msgsess.TxID]*stage.SentPing
	activeOut map[key.NodePublic]bool
	activeIn  map[key.NodePublic]bool

	// an opportunistic map that caches session-to-node mapping
	sessMap map[key.SessionPublic]key.NodePublic
}

func (s *Stage) makeTM() *TrafficManager {
	return assureClose(&TrafficManager{
		ActorCommon: MakeCommon(s.Ctx, TrafficManInboxChLen),
		s:           s,

		ticker:    time.NewTicker(TManTickerInterval),
		poke:      make(chan interface{}, 1),
		peerState: make(map[key.NodePublic]peerstate.PeerState),

		pings:     make(map[msgsess.TxID]*stage.SentPing),
		activeOut: make(map[key.NodePublic]bool),
		activeIn:  make(map[key.NodePublic]bool),
		sessMap:   make(map[key.SessionPublic]key.NodePublic),
	})
}

func (tm *TrafficManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(tm).Error("panicked", "error", v, "stack", string(debug.Stack()))
			tm.Cancel()
			bail(tm.ctx, v)
		}
	}()

	if !tm.running.CheckOrMark() {
		L(tm).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {

		case <-tm.ctx.Done():
			return
		case <-tm.ticker.C:
			// Run periodic before inbox, as inbox can get backed up, and ping + path management would get delayed.
			tm.doPingManagement()
			tm.DoStateTick()
		case m := <-tm.inbox:
			tm.Handle(m)
		case <-tm.poke:
			tm.DoStateTick()
		}
	}
}

func (tm *TrafficManager) Handle(m msgactor.ActorMessage) {
	switch m := m.(type) {
	case *msgactor.TManConnActivity:
		var aMap map[key.NodePublic]bool
		if m.IsIn {
			aMap = tm.activeIn
		} else {
			aMap = tm.activeOut
		}
		aMap[m.Peer] = m.IsActive

		tm.onConnActivity(m.Peer)

	case *msgactor.TManConnGoodBye:
		var aMap map[key.NodePublic]bool
		if m.IsIn {
			aMap = tm.activeIn
		} else {
			aMap = tm.activeOut
		}

		// idempotent, as we might receive this multiple times
		delete(aMap, m.Peer)

		tm.onConnRemoval(m.Peer)

	case *msgactor.TManSessionMessageFromDirect:
		n := tm.NodeForSess(m.Msg.Session)

		if n == nil {
			L(tm).Warn("got message from direct for unknown session", "session", m.Msg.Session.Debug())
			return
		}

		node := *n

		if tm.isMDNS(m.Msg) {
			if !tm.mdnsAllowed(node) {
				L(tm).Warn("got direct MDNS packet from peer where it is not allowed", "peer", node.Debug())
				return
			}
			tm.sendMDNS(node, m.Msg)
			return
		}

		tm.forState(node, func(s peerstate.PeerState) peerstate.PeerState {
			return s.OnDirect(types.NormaliseAddrPort(m.AddrPort), m.Msg)
		})
	case *msgactor.TManSessionMessageFromRelay:
		if !tm.ValidKeys(m.Peer, m.Msg.Session) {
			L(tm).Warn("got message from relay for peer with incorrect session",
				"session", m.Msg.Session.Debug(), "peer", m.Peer.Debug(), "relay", m.Relay)
			return
		}

		if tm.isMDNS(m.Msg) {
			if !tm.mdnsAllowed(m.Peer) {
				L(tm).Warn("got relay MDNS packet from peer where it is not allowed", "peer", m.Peer.Debug())
				return
			}
			tm.sendMDNS(m.Peer, m.Msg)
			return
		}

		tm.forState(m.Peer, func(s peerstate.PeerState) peerstate.PeerState {
			return s.OnRelay(m.Relay, m.Peer, m.Msg)
		})
	case *msgactor.SyncPeerInfo:
		oc := tm.s.OutConnFor(m.Peer)

		if oc != nil {
			go SendMessage(oc.Inbox(), m)
		}

		pi := tm.s.GetPeerInfo(m.Peer)

		if pi != nil {
			// Pre-cache
			tm.sessMap[pi.Session] = m.Peer
		} else {
			// Remove any sessions from the cache
			maps.DeleteFunc(tm.sessMap, func(_ key.SessionPublic, key key.NodePublic) bool {
				return key == m.Peer
			})
		}
	case *msgactor.TManSpreadMDNSPacket:
		tm.spreadMDNS(m.Pkt)
	default:
		tm.logUnknownMessage(m)
	}
}

func (tm *TrafficManager) isMDNS(msg *msgsess.ClearMessage) bool {
	sbd, ok := msg.Message.(*msgsess.SideBandData)

	return ok && sbd.Type == msgsess.MDNSType
}

func (tm *TrafficManager) mdnsAllowed(node key.NodePublic) bool {
	pi := tm.s.GetPeerInfo(node)

	if pi == nil {
		return false
	}

	return pi.MDNS
}

func (tm *TrafficManager) sendMDNS(peer key.NodePublic, msg *msgsess.ClearMessage) {
	sbd := msg.Message.(*msgsess.SideBandData)

	go SendMessage(tm.s.MMan.Inbox(), &msgactor.MManReceivedPacket{
		From: peer,
		Data: sbd.Data,
	})
}

func (tm *TrafficManager) spreadMDNS(pkt []byte) {
	peers := tm.s.GetPeersWhere(func(_ key.NodePublic, info *stage.PeerInfo) bool {
		return info.MDNS
	})

	peersDebug := types.Map(peers, func(t key.NodePublic) string {
		return t.Debug()
	})
	L(tm).Log(context.Background(), types.LevelTrace, "sending mdns packet to peers", "peers", peersDebug)
	for _, peer := range peers {
		tm.opportunisticSendTo(peer, &msgsess.SideBandData{
			Type: msgsess.MDNSType,
			Data: pkt,
		})
	}
}

func (tm *TrafficManager) opportunisticSendTo(to key.NodePublic, msg msgsess.SessionMessage) {
	pi := tm.s.GetPeerInfo(to)

	if pi == nil {
		L(tm).Warn("trying to send an opportunistic session message to a node without peerinfo", "to", to.Debug())
		return
	}

	tm.forState(to, func(s peerstate.PeerState) peerstate.PeerState {
		L(tm).Log(context.Background(), types.LevelTrace, "sending opportunistic session message to peer", "peer", to.Debug())

		if e, ok := s.(*peerstate.Established); ok {
			tm.SendMsgToDirect(e.GetEndpoint(), pi.Session, msg)
		} else {
			tm.SendMsgToRelay(pi.HomeRelay, to, pi.Session, msg)
		}

		return nil
	})
}

func (tm *TrafficManager) DoStateTick() {
	// We explicitly range over a slice of the keys we already got,
	// since golang likes to complain when we mutate while we iterate.
	for _, peer := range xmaps.Keys(tm.peerState) {
		tm.forState(peer, func(s peerstate.PeerState) peerstate.PeerState {
			return s.OnTick()
		})
	}
}

func (tm *TrafficManager) NodeForSess(sess key.SessionPublic) *key.NodePublic {
	tm.s.peerInfoMutex.RLock()
	defer tm.s.peerInfoMutex.RUnlock()

	if n, ok := tm.sessMap[sess]; ok {
		if pi, ok := tm.s.peerInfo[n]; ok && pi.Session == sess {
			return &n
		}
	}

	// Not (correctly) cached, look up.

	for n, pi := range tm.s.peerInfo {
		if pi.Session == sess {
			tm.sessMap[sess] = n
			return &n
		}
	}

	// Couldn't find it
	return nil
}

// Poke is a convenience method to have TMan poke OnTick for states ASAP
// (after message queues get cleared).
func (tm *TrafficManager) Poke() {
	// Non-blocking channel send
	select {
	case tm.poke <- nil:
	default:
	}
}

//nolint:unused
func (tm *TrafficManager) isConnActive(peer key.NodePublic) bool {
	return tm.activeOut[peer] || tm.activeIn[peer]
}

func (tm *TrafficManager) isConnKnown(peer key.NodePublic) bool {
	_, ok1 := tm.activeOut[peer]
	_, ok2 := tm.activeIn[peer]

	return ok1 || ok2
}

// onConnActivity is a callback to be fired when a peer changes activity, will handle further logic,
// such as starting or aborting path discovery.
func (tm *TrafficManager) onConnActivity(peer key.NodePublic) {
	tm.ensurePeerState(peer)
}

// onConnRemoval is a callback to be fired when a peer is removed, will handle further logic,
// such as stopping path discovery.
func (tm *TrafficManager) onConnRemoval(peer key.NodePublic) {
	if !tm.isConnKnown(peer) {
		// ensure path discovery stopped, remove references / do cleanup
		delete(tm.peerState, peer)
	}
}

func (tm *TrafficManager) ensurePeerState(peer key.NodePublic) {
	s, ok := tm.peerState[peer]

	if !ok {
		tm.peerState[peer] = peerstate.MakeWaiting(tm, peer)
		tm.Poke()
		return
	}

	if s == nil {
		// !! this should never happen, but we recover regardless
		L(tm).Warn("found nil state for peer, restarting state with Waiting", "peer", peer.Debug())
		tm.peerState[peer] = peerstate.MakeWaiting(tm, peer)
		tm.Poke()
	}
}

func (tm *TrafficManager) Close() {
	tm.ticker.Stop()
}

const PingReapTimeout = 10 * time.Minute

func (tm *TrafficManager) doPingManagement() {
	var oldPings []msgsess.TxID

	for txid, ping := range tm.pings {
		if ping.At.Add(PingReapTimeout).Before(time.Now()) {
			oldPings = append(oldPings, txid)
		}
	}

	for _, txid := range oldPings {
		delete(tm.pings, txid)
	}
}

type StateForState func(state peerstate.PeerState) peerstate.PeerState

func (tm *TrafficManager) forState(peer key.NodePublic, fn StateForState) {
	// A state for a state, perfectly balanced, as all things should be.
	// - Thanos, while writing this code.

	tm.ensurePeerState(peer)

	newState := fn(tm.peerState[peer])

	if newState != nil {
		// state transitions have happened, store the new state
		tm.peerState[peer] = newState
	}
}

func (tm *TrafficManager) DManClearAKA(peer key.NodePublic) {
	SendMessage(tm.s.DRouter.Inbox(), &msgactor.DRouterPeerClearKnownAs{
		Peer: peer,
	})
}

func (tm *TrafficManager) DManSetAKA(peer key.NodePublic, ap netip.AddrPort) {
	SendMessage(tm.s.DRouter.Inbox(), &msgactor.DRouterPeerAddKnownAs{
		Peer:     peer,
		AddrPort: ap,
	})
}

func (tm *TrafficManager) OutConnUseRelay(peer key.NodePublic, relay int64) {
	out := tm.s.OutConnFor(peer)

	if out == nil {
		L(tm).Warn("OutConnUseRelay: could not find outconn for peer", "peer", peer.Debug())
		return
	}

	SendMessage(out.Inbox(), &msgactor.OutConnUse{
		UseRelay:   true,
		RelayToUse: relay,
	})
}

func (tm *TrafficManager) OutConnTrackHome(peer key.NodePublic) {
	out := tm.s.OutConnFor(peer)

	if out == nil {
		L(tm).Warn("OutConnTrackHome: could not find outconn for peer", "peer", peer.Debug())
		return
	}

	SendMessage(out.Inbox(), &msgactor.OutConnUse{
		UseRelay:  true,
		TrackHome: true,
	})
}

func (tm *TrafficManager) OutConnUseAddrPort(peer key.NodePublic, ap netip.AddrPort) {
	out := tm.s.OutConnFor(peer)

	if out == nil {
		L(tm).Warn("OutConnUseAddrPort: could not find outconn for peer", "peer", peer.Debug())
		return
	}

	SendMessage(out.Inbox(), &msgactor.OutConnUse{
		UseRelay:      false,
		AddrPortToUse: ap,
	})
}

func (tm *TrafficManager) ValidKeys(peer key.NodePublic, session key.SessionPublic) bool {
	pi := tm.s.GetPeerInfo(peer)
	return pi != nil && session == pi.Session
}

func (tm *TrafficManager) SendPingDirect(endpoint netip.AddrPort, peer key.NodePublic, session key.SessionPublic) {
	tm.SendPingDirectWithID(endpoint, peer, session, msgsess.NewTxID())
}

func (tm *TrafficManager) SendPingDirectWithID(endpoint netip.AddrPort, peer key.NodePublic, session key.SessionPublic, txid msgsess.TxID) {
	nep := types.NormaliseAddrPort(endpoint)

	tm.SendMsgToDirect(nep, session, &msgsess.Ping{
		TxID:    txid,
		NodeKey: tm.s.getNodePriv().Public(),
	})

	tm.pings[txid] = &stage.SentPing{
		ToRelay:  false,
		AddrPort: nep,
		At:       time.Now(),
		To:       peer,
	}
}

func (tm *TrafficManager) SendPingRelay(relay int64, peer key.NodePublic, session key.SessionPublic) {
	txid := msgsess.NewTxID()

	tm.SendMsgToRelay(relay, peer, session, &msgsess.Ping{
		TxID:    txid,
		NodeKey: tm.s.getNodePriv().Public(),
	})

	tm.pings[txid] = &stage.SentPing{
		ToRelay: true,
		RelayID: relay,
		At:      time.Now(),
		To:      peer,
	}
}

func (tm *TrafficManager) SendMsgToRelay(relay int64, peer key.NodePublic, sess key.SessionPublic, m msgsess.SessionMessage) {
	go SendMessage(tm.s.SMan.Inbox(), &msgactor.SManSendSessionMessageToRelay{
		Relay:     relay,
		Peer:      peer,
		ToSession: sess,
		Msg:       m,
	})
}

func (tm *TrafficManager) SendMsgToDirect(ap netip.AddrPort, sess key.SessionPublic, m msgsess.SessionMessage) {
	go SendMessage(tm.s.SMan.Inbox(), &msgactor.SManSendSessionMessageToDirect{
		AddrPort:  ap,
		ToSession: sess,
		Msg:       m,
	})
}

func (tm *TrafficManager) Pings() map[msgsess.TxID]*stage.SentPing {
	return tm.pings
}

func (tm *TrafficManager) Stage() ifaces.Stage {
	return tm.s
}

func (tm *TrafficManager) ActiveOut() map[key.NodePublic]bool {
	return tm.activeOut
}

func (tm *TrafficManager) ActiveIn() map[key.NodePublic]bool {
	return tm.activeIn
}
