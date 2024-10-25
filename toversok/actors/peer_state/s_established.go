package peer_state

import (
	"context"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
	"slices"
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
	currentOutEndpoint netip.AddrPort

	knownInEndpoints map[netip.AddrPort]bool
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
		})
	}

	if time.Now().After(e.nextPingDeadline) {
		e.tm.SendPingDirect(e.currentOutEndpoint, e.peer, pi.Session)
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

	if !e.canTrustEndpoint(ap) {
		L(e).Log(context.Background(), types.LevelTrace,
			"dropping direct message from addrpair, cannot trust endpoint", "ap", ap.String())
		return nil
	}

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !e.pingDirectValid(ap, clear.Session, m) {
			L(e).Log(context.Background(), types.LevelTrace,
				"dropping invalid ping", "ap", ap.String())
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
		L(e).Debug("ignoring direct session message",
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
		L(e).Debug("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

// canTrustEndpoint returns true if the endpoint that has been given corresponds to the peer.
// this will check the current knownInEndpoints, and if it does not exist, will check peerInfo to see if the peer
// sent this endpoint in the past with rendezvous. If so, adds it to the knownInEndpoints, and sends a SetAKA.
func (e *Established) canTrustEndpoint(ap netip.AddrPort) bool {
	// b.tm.DManSetAKA(b.peer, b.ap)

	nap := types.NormaliseAddrPort(ap)

	if _, ok := e.knownInEndpoints[nap]; ok {
		return true
	}

	pi := e.getPeerInfo()
	if pi == nil {
		// Peer info unavailable
		return false
	}

	if slices.Contains(pi.Endpoints, nap) || slices.Contains(pi.RendezvousEndpoints, nap) {
		// we can trust this endpoint

		e.knownInEndpoints[nap] = true

		e.tm.DManSetAKA(e.peer, nap)

		L(e).Info(
			"adding new aka address to peer, as it is trusted",
			"ap", nap.String(), "peer", e.peer.Debug(),
		)
		return true
	}

	return false
}
