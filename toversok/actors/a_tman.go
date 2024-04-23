package actors

import (
	"github.com/shadowjonathan/edup2p/toversok/actors/peer_state"
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	maps2 "golang.org/x/exp/maps"
	"net/netip"
	"time"
)

type TrafficManager struct {
	*ActorCommon
	S *Stage

	//// Node to Session
	//n2s bimap.BiMap[key.NodePublic, key.SessionPublic]

	ticker *time.Ticker     // 1s
	poke   chan interface{} // len 1

	peerState map[key.NodePublic]peer_state.PeerState

	Pings     map[msg.TxID]*sentPing
	PeerInfo  map[key.NodePublic]*PeerInfo
	OutActive map[key.NodePublic]bool
	InActive  map[key.NodePublic]bool

	// an opportunistic map that caches session-to-node mapping
	sessMap map[key.SessionPublic]key.NodePublic
}

type PeerInfo struct {
	HomeRelay           int64
	Endpoints           []netip.AddrPort
	RendezvousEndpoints []netip.AddrPort
	Session             key.SessionPublic
}

func (tm *TrafficManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			// TODO logging
			tm.Cancel()
			tm.Close()
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

func (tm *TrafficManager) Handle(m ActorMessage) {
	switch m := m.(type) {
	case *TManConnActivity:
		var aMap map[key.NodePublic]bool
		if m.isIn {
			aMap = tm.InActive
		} else {
			aMap = tm.OutActive
		}
		aMap[m.peer] = m.isActive

		tm.onConnActivity(m.peer)

	case *TManConnGoodBye:
		var aMap map[key.NodePublic]bool
		if m.isIn {
			aMap = tm.InActive
		} else {
			aMap = tm.OutActive
		}

		// idempotent, as we might receive this multiple times
		delete(aMap, m.peer)

		tm.onConnRemoval(m.peer)

	case *TManSessionMessageFromDirect:
		n := tm.NodeForSess(m.msg.Session)

		if n != nil {
			tm.forState(*n, func(s peer_state.PeerState) peer_state.PeerState {
				return s.OnDirect(m.addrPort, m.msg)
			})
		} else {
			// todo log?
		}
	case *TManSessionMessageFromRelay:
		if !tm.ValidKeys(m.peer, m.msg.Session) {
			// todo log
			return
		}

		tm.forState(m.peer, func(s peer_state.PeerState) peer_state.PeerState {
			return s.OnRelay(m.relay, m.peer, m.msg)
		})
	case *TManSetPeerInfo:
		tm.PeerInfo[m.peer] = &PeerInfo{
			HomeRelay: m.homeRelay,
			Endpoints: m.endpoints,
			Session:   m.session,
		}
		tm.Poke()
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
	if n, ok := tm.sessMap[sess]; ok {
		if pi, ok := tm.PeerInfo[n]; ok && pi.Session == sess {
			return &n
		}
	}

	// Not (correctly) cached, look up.

	for n, pi := range tm.PeerInfo {
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
	return tm.OutActive[peer] || tm.InActive[peer]
}

func (tm *TrafficManager) isConnKnown(peer key.NodePublic) bool {
	_, ok1 := tm.OutActive[peer]
	_, ok2 := tm.InActive[peer]

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
		// TODO
		// ensure path discovery stopped, remove references / do cleanup
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
		// TODO log
		tm.peerState[peer] = peer_state.MakeWaiting(tm, peer)
		tm.Poke()
	}

	return
}

func (tm *TrafficManager) Close() {
	// TODO?
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
		// TODO log error
		return
	}

	newState := fn(state)

	if newState != nil {
		// state transitions have happened, store the new state
		tm.peerState[peer] = state
	}
}

//const EstablishmentTimeout = time.Second * 10
//const EstablishmentRetry = time.Second * 40
//
//const EstablishedPingTimeout = time.Second * 5

//// runPathDiscovery goes through path discovery peer_state and updates it when required, doing related actions.
//func (tm *TrafficManager) runPathDiscovery() {
//	for peer, state := range tm.peerState {
//
//		// These cases use reverse if, because "fallthrough" can't be placed in if statements
//
//		switch state.rootState {
//		case WaitingForSession:
//			if _, ok := tm.PeerInfo[peer]; !ok {
//				continue
//			}
//			state.rootState = Inactive
//			fallthrough
//		case Inactive:
//			if !(tm.InActive[peer] || tm.OutActive[peer]) {
//				continue
//			}
//			state.rootState = Trying
//			state.retryEsting = time.Now()
//			fallthrough
//		case Trying:
//			if time.Now().Unix() < state.retryEsting.Unix() {
//				continue
//			}
//
//			state.rootState = Establishing
//			state.estState = PreTransmit
//			state.estingDeadline = time.Now().Add(EstablishmentTimeout)
//			fallthrough
//		case Establishing:
//			if tm.runEstablishing(peer, state) {
//				state.rootState = Trying
//				state.retryEsting = time.Now().Add(EstablishmentRetry)
//			}
//		case Established:
//			if time.Now().After(state.lastPingRecv.Add(EstablishedPingTimeout)) ||
//				time.Now().After(state.lastPongRecv.Add(EstablishedPingTimeout)) {
//				// Timed out
//
//				state.rootState = Trying
//				state.retryEsting = time.Now()
//
//				tm.Poke()
//
//				go SendMessage(tm.S.DMan.Inbox(), &DManPeerClearKnownAs{
//					peer: peer,
//				})
//
//				out := tm.S.OutConnFor(peer)
//
//				if out == nil {
//					// TODO log
//					return
//				}
//
//				go SendMessage(out.Inbox(), &OutConnUse{
//					useRelay:   true,
//					relayToUse: tm.PeerInfo[peer].HomeRelay,
//				})
//			}
//
//		}
//	}
//}

func (tm *TrafficManager) DManClearAKA(peer key.NodePublic) {
	go SendMessage(tm.S.DRouter.Inbox(), &DRouterPeerClearKnownAs{
		peer: peer,
	})
}

func (tm *TrafficManager) DManSetAKA(peer key.NodePublic, ap netip.AddrPort) {
	go SendMessage(tm.S.DRouter.Inbox(), &DRouterPeerAddKnownAs{
		peer:     peer,
		addrPort: ap,
	})
}

func (tm *TrafficManager) OutConnUseRelay(peer key.NodePublic, relay int64) {
	out := tm.S.OutConnFor(peer)

	if out == nil {
		// TODO log
		return
	}

	go SendMessage(out.Inbox(), &OutConnUse{
		useRelay:   true,
		relayToUse: relay,
	})
}

func (tm *TrafficManager) OutConnUseAddrPort(peer key.NodePublic, ap netip.AddrPort) {
	out := tm.S.OutConnFor(peer)

	if out == nil {
		// TODO log
		return
	}

	go SendMessage(out.Inbox(), &OutConnUse{
		useRelay:      false,
		addrPortToUse: ap,
	})
}

//// runEstablishing runs through establishing logic and returns a boolean, signalling whether the timeout has expired.
//func (tm *TrafficManager) runEstablishing(peer key.NodePublic, state *PDState) (downgrade bool) {
//	if state.estState == PreTransmit {
//		pi, ok := tm.PeerInfo[peer]
//		if !ok {
//			panic("Got no peerinfo in establishing peer_state")
//		}
//		for _, e := range pi.Endpoints {
//			tm.SendPingDirect(e, peer, pi.Session)
//		}
//
//		go SendMessage(tm.S.SMan.Inbox(), &SManSendSessionMessageToRelay{
//			relay:     pi.HomeRelay,
//			peer:      peer,
//			toSession: pi.Session,
//			msg:       &msg.Rendezvous{MyAddresses: tm.S.GetLocalEndpoints()},
//		})
//
//		state.estState = Transmitting
//	}
//
//	return time.Now().After(state.estingDeadline)
//}

func (tm *TrafficManager) ValidKeys(peer key.NodePublic, session key.SessionPublic) bool {
	pi, ok := tm.PeerInfo[peer]
	return ok && session == pi.Session
}

//func (tm *TrafficManager) onRelayPing(iMsg *TManSessionMessageFromRelay, sMsg *msg.Ping) {
//	// We just reply, we're not really interested in this from a path discovery standpoint.
//	go SendMessage(tm.S.SMan.Inbox(), &SManSendSessionMessageToRelay{
//		relay:     iMsg.relay,
//		peer:      iMsg.peer,
//		toSession: iMsg.msg.Session,
//		msg: &msg.Pong{
//			TxID: sMsg.TxID,
//			Src:  netip.AddrPort{},
//		},
//	})
//}

//func (tm *TrafficManager) onRelayPong(iMsg *TManSessionMessageFromRelay, sMsg *msg.Pong) {
//	// Relay pongs should come in response to relay pings, note if its different.
//	sent, ok := tm.Pings[sMsg.TxID]
//
//	if !ok {
//		// Pong was for a ping that'S unknown.
//		// TODO log
//		return
//	}
//
//	if !sent.ToRelay {
//		// Pong was for a ping that wasn't to a relay.
//		// TODO log
//		return
//	}
//
//	if !tm.ValidKeys(sent.To, iMsg.msg.Session) {
//		// ?? Somehow the pong is for a valid ping to a node that no longer has this session key?
//		// Might happen between restarts, log and ignore.
//		// TODO log
//		return
//	}
//
//	delete(tm.Pings, sMsg.TxID)
//}

//func (tm *TrafficManager) onRelayRendezvous(iMsg *TManSessionMessageFromRelay, sMsg *msg.Rendezvous) {
//	state, ok := tm.peerState[iMsg.peer]
//
//	if !ok {
//		// Can't do much, peerState hasn't been established
//		// TODO log
//		return
//	}
//
//	// Check peer_state transition
//	switch state.rootState {
//	case Inactive:
//		fallthrough
//	case Trying:
//		fallthrough
//	case Establishing:
//		// double-check if we want to check here, because we fallthrough
//		if state.rootState == Establishing && state.estState != Transmitting {
//			// When in Establishing, we don't care about the rendezvous if we're in any other peer_state
//			return
//		}
//
//		state.rootState = Establishing
//		state.estState = GotRendezvous
//		state.retryEsting = time.Now()
//		state.estingDeadline = time.Now().Add(EstablishmentTimeout)
//	default:
//		// We're not interested in hearing rendezvous in the other states
//		// TODO log
//	}
//
//	tm.PeerInfo[iMsg.peer].RendezvousEndpoints = sMsg.MyAddresses
//
//	// Do actual replies
//	for _, ep := range sMsg.MyAddresses {
//		tm.SendPingDirect(ep, iMsg.peer, iMsg.msg.Session)
//	}
//}

//func (tm *TrafficManager) onDirectPing(iMsg *TManSessionMessageFromDirect, sMsg *msg.Ping) {
//	// ValidKeys has already been done by the callee
//
//	// First, we send a pong, because we're nice.
//	go SendMessage(tm.S.SMan.Inbox(), &SManSendSessionMessageToDirect{
//		addrPort:  iMsg.addrPort,
//		toSession: iMsg.msg.Session,
//		msg: &msg.Pong{
//			TxID: sMsg.TxID,
//			Src:  iMsg.addrPort,
//		},
//	})
//
//	// Note activity and do peer_state handling
//	if state, ok := tm.peerState[sMsg.NodeKey]; ok {
//		state.lastPingRecv = time.Now()
//
//		tm.pdOnDirectPing(sMsg.NodeKey, iMsg.addrPort, state)
//	}
//}

//func (tm *TrafficManager) onDirectPong(iMsg *TManSessionMessageFromDirect, sMsg *msg.Pong) {
//	// Relay pongs should come in response to relay pings, note if its different.
//	sent, ok := tm.Pings[sMsg.TxID]
//
//	if !ok {
//		// Pong was for a ping that'S unknown.
//		// TODO log
//		return
//	}
//
//	if sent.ToRelay {
//		// Pong was for a ping that was sent to a relay.
//		// TODO log
//		return
//	}
//
//	if !tm.ValidKeys(sent.To, iMsg.msg.Session) {
//		// ?? Somehow the pong is for a valid ping to a node that no longer has this session key?
//		// Might happen between restarts, log and ignore.
//		// TODO log
//		return
//	}
//
//	delete(tm.Pings, sMsg.TxID)
//
//	// Note activity and do peer_state handling
//	if state, ok := tm.peerState[sent.To]; ok {
//		state.lastPongRecv = time.Now()
//
//		tm.pdOnDirectPong(sent.To, iMsg.addrPort, state)
//	}
//}

//func (tm *TrafficManager) pdOnDirectPing(peer key.NodePublic, endpoint netip.AddrPort, state *PDState) {
//	if state.rootState == Establishing {
//		switch state.estState {
//		case PreTransmit:
//			panic("Should never receive ping in pretransmit peer_state")
//		case Transmitting:
//			fallthrough
//		case GotRendezvous:
//			// Go to half-established
//			state.estState = HalfEstablished
//			pi, ok := tm.PeerInfo[peer]
//			if !ok {
//				panic("Did not find a session in the middle of path discovery")
//			}
//			tm.SendPingDirect(endpoint, peer, pi.Session)
//		default:
//			// ignore
//		}
//	}
//}

//func (tm *TrafficManager) pdOnDirectPong(peer key.NodePublic, endpoint netip.AddrPort, state *PDState) {
//	if state.rootState == Establishing {
//		state.rootState = Established
//		state.currentEndpoint = endpoint
//
//		go SendMessage(tm.S.DMan.Inbox(), &DManPeerSetKnownAs{
//			peer:     peer,
//			addrPort: state.currentEndpoint,
//		})
//
//		out := tm.S.OutConnFor(peer)
//
//		if out == nil {
//			// TODO log
//			return
//		}
//
//		go SendMessage(out.Inbox(), &OutConnUse{
//			useRelay:      false,
//			addrPortToUse: state.currentEndpoint,
//		})
//	}
//}

func (tm *TrafficManager) SendPingDirect(endpoint netip.AddrPort, peer key.NodePublic, session key.SessionPublic) {
	txid := msg.NewTxID()

	tm.SendMsgToDirect(endpoint, session, &msg.Ping{
		TxID:    txid,
		NodeKey: tm.S.localKey,
	})

	tm.Pings[txid] = &sentPing{
		ToRelay:  false,
		AddrPort: endpoint,
		At:       time.Now(),
		To:       peer,
	}
}

func (tm *TrafficManager) sendPingRelay(endpoint netip.AddrPort, peer key.NodePublic) {
	// TODO
	panic("Not implemented")
}

func (tm *TrafficManager) SendMsgToRelay(relay int64, peer key.NodePublic, sess key.SessionPublic, m msg.SessionMessage) {
	go SendMessage(tm.S.SMan.Inbox(), &SManSendSessionMessageToRelay{
		relay:     relay,
		peer:      peer,
		toSession: sess,
		msg:       m,
	})
}

func (tm *TrafficManager) SendMsgToDirect(ap netip.AddrPort, sess key.SessionPublic, m msg.SessionMessage) {
	go SendMessage(tm.S.SMan.Inbox(), &SManSendSessionMessageToDirect{
		addrPort:  ap,
		toSession: sess,
		msg:       m,
	})
}

//// PDState (Path Discovery State) stores the information known about a particular path discovery.
//type PDState struct {
//	// "esting" is "establishing"
//
//	rootState pdRootState
//	estState  pdEstState
//
//	estingDeadline time.Time
//	retryEsting    time.Time
//
//	lastPingRecv    time.Time
//	lastPongRecv    time.Time
//	currentEndpoint netip.AddrPort
//
//	lastSentPing time.Time
//}

type sentPing struct {
	ToRelay  bool
	RelayId  int64
	AddrPort netip.AddrPort
	At       time.Time
	To       key.NodePublic
}

//type pdRootState int
//
//const (
//	WaitingForSession pdRootState = iota
//	Inactive
//	Trying
//	Establishing
//	Established
//)
//
//// Establishing State
//type pdEstState int
//
//const (
//	PreTransmit pdEstState = iota
//	Transmitting
//	GotRendezvous
//	HalfEstablished
//)
