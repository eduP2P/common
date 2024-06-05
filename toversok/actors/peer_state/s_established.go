package peer_state

import (
	"github.com/shadowjonathan/edup2p/types/key"
	msg2 "github.com/shadowjonathan/edup2p/types/msgsess"
	"net/netip"
	"time"
)

const EstablishedPingInterval = time.Second * 2

type Established struct {
	*StateCommon

	lastPingRecv time.Time
	lastPongRecv time.Time

	nextPingDeadline time.Time

	// Set if the current state detects the connection is inctive, and for how long
	inactive      bool
	inactiveSince time.Time

	// TODO: this can flap,
	//   and basically picks the first best endpoint that the other client responds with,
	//   which may be non-ideal.
	//   Tailscale has logic to pick and switch between different endpoints, and sort them.
	//   We could possibly build this into the state logic.
	currentEndpoint netip.AddrPort
}

func (e *Established) Name() string {
	return "established"
}

func (e *Established) OnTick() PeerState {
	pi := e.getPeerInfo()
	if pi == nil {
		// Peer info unavailable
		return nil
	}

	if e.tm.ActiveIn()[e.peer] || e.tm.ActiveOut()[e.peer] {
		e.inactive = false
	} else {
		if !e.inactive {
			e.inactive = true
			e.inactiveSince = time.Now()
		} else {
			if time.Now().After(e.inactiveSince.Add(ConnectionInactivityTimeout)) {
				return LogTransition(e, &Teardown{
					StateCommon: e.StateCommon,
					inactive:    true,
				})
			}
		}
	}

	if time.Now().After(e.lastPingRecv.Add(EstablishedPingTimeout)) ||
		time.Now().After(e.lastPongRecv.Add(EstablishedPingTimeout)) {
		// Timed out

		return LogTransition(e, &Teardown{
			StateCommon: e.StateCommon,
			inactive:    false,
			tryAt:       time.Now(),
		})
	}

	if time.Now().After(e.nextPingDeadline) {
		e.tm.SendPingDirect(e.currentEndpoint, e.peer, pi.Session)
		e.nextPingDeadline = time.Now().Add(EstablishedPingInterval)
	}

	return nil
}

func (e *Established) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(e, ap, clear)

	// TODO check if endpoint is same as current used one
	//  - switch? trusting it blindly is open to replay attacks

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !e.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		e.lastPingRecv = time.Now()
		e.replyWithPongDirect(ap, clear.Session, m)
		return nil

	case *msg2.Pong:
		e.lastPongRecv = time.Now()
		e.ackPongDirect(ap, clear.Session, m)
		return nil

	//case *msg.Rendezvous:
	default:
		L(e).Warn("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (e *Established) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	if s := cascadeRelay(e, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(e, relay, peer, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		e.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil

	case *msg2.Pong:
		e.ackPongRelay(relay, peer, clear.Session, m)
		return nil

	//case *msg.Rendezvous:
	// TODO maybe re-establishment logic?
	default:
		L(e).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
