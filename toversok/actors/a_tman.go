package actors

import (
	"github.com/edup2p/common/toversok/actors/peer_state"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/stage"
	maps2 "golang.org/x/exp/maps"
	"maps"
	"net/netip"
	"time"
)

type TrafficManager struct {
	*ActorCommon
	s *Stage

	ticker *time.Ticker     // 1s
	poke   chan interface{} // len 1

	peerState map[key.NodePublic]peer_state.PeerState

	pings     map[msgsess.TxID]*stage.SentPing
	activeOut map[key.NodePublic]bool
	activeIn  map[key.NodePublic]bool

	// an opportunistic map that caches session-to-node mapping
	sessMap map[key.SessionPublic]key.NodePublic
}

func (s *Stage) makeTM() *TrafficManager {
	return &TrafficManager{
		ActorCommon: MakeCommon(s.Ctx, TrafficManInboxChLen),
		s:           s,

		ticker:    time.NewTicker(TManTickerInterval),
		poke:      make(chan interface{}, 1),
		peerState: make(map[key.NodePublic]peer_state.PeerState),

		pings:     make(map[msgsess.TxID]*stage.SentPing),
		activeOut: make(map[key.NodePublic]bool),
		activeIn:  make(map[key.NodePublic]bool),
		sessMap:   make(map[key.SessionPublic]key.NodePublic),
	}
}

func (tm *TrafficManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(tm).Error("panicked", "error", v)
			tm.Cancel()
			tm.Close()
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
			tm.Close()
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

		if n != nil {
			tm.forState(*n, func(s peer_state.PeerState) peer_state.PeerState {
				return s.OnDirect(m.AddrPort, m.Msg)
			})
		} else {
			L(tm).Warn("got message from direct for unknown session", "session", m.Msg.Session.Debug())
		}
	case *msgactor.TManSessionMessageFromRelay:
		if !tm.ValidKeys(m.Peer, m.Msg.Session) {
			L(tm).Warn("got message from relay for peer with incorrect session",
				"session", m.Msg.Session.Debug(), "peer", m.Peer.Debug(), "relay", m.Relay)
			return
		}

		tm.forState(m.Peer, func(s peer_state.PeerState) peer_state.PeerState {
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
	default:
		tm.logUnknownMessage(m)
	}
}

func (tm *TrafficManager) DoStateTick() {
	// We explicitly range over a slice of the keys we already got,
	// since golang likes to complain when we mutate while we iterate.
	for _, peer := range maps2.Keys(tm.peerState) {
		tm.forState(peer, func(s peer_state.PeerState) peer_state.PeerState {
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
		tm.peerState[peer] = peer_state.MakeWaiting(tm, peer)
		tm.Poke()
		return
	}

	if s == nil {
		// !! this should never happen, but we recover regardless
		L(tm).Warn("found nil state for peer, restarting state with Waiting", "peer", peer.Debug())
		tm.peerState[peer] = peer_state.MakeWaiting(tm, peer)
		tm.Poke()
	}

	return
}

func (tm *TrafficManager) Close() {
	tm.ticker.Stop()
}

func (tm *TrafficManager) doPingManagement() {
	// TODO
	//  - expire old pings
}

type StateForState func(state peer_state.PeerState) peer_state.PeerState

func (tm *TrafficManager) forState(peer key.NodePublic, fn StateForState) {
	// A state for a state, perfectly balanced, as all things should be.
	// - Thanos, while writing this code.

	state, ok := tm.peerState[peer]

	if !ok {
		return
	}

	if state == nil {
		L(tm).Error("found nil state when running update for peer, recovering...", "peer", peer.Debug())
		tm.ensurePeerState(peer)
		state = tm.peerState[peer]
	}

	newState := fn(state)

	if newState != nil {
		// state transitions have happened, store the new state
		tm.peerState[peer] = newState
	}
}

// TODO see if these correspond in peer_state package
//const EstablishmentTimeout = time.Second * 10
//const EstablishmentRetry = time.Second * 40
//
//const EstablishedPingTimeout = time.Second * 5

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
	txid := msgsess.NewTxID()

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
		RelayId: relay,
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
