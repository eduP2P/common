package peerstate

import (
	"context"
	"net/netip"
	"slices"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
)

const EstablishedPingInterval = time.Second * 2

type Established struct {
	*StateCommon

	tracker *PingTracker

	lastPingRecv time.Time
	lastPongRecv time.Time

	nextPingDeadline time.Time

	// Set if the current state detects the connection is inctive, and for how long
	inactive      bool
	inactiveSince time.Time

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
		} else if time.Now().After(e.inactiveSince.Add(ConnectionInactivityTimeout)) {
			return LogTransition(e, &Teardown{
				StateCommon: e.StateCommon,
				inactive:    true,
			})
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

func (e *Established) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clearMsg); s != nil {
		return s
	}

	ap = types.NormaliseAddrPort(ap)

	LogDirectMessage(e, ap, clearMsg)

	// TODO check if endpoint is same as current used one
	//  - switch? trusting it blindly is open to replay attacks

	if !e.canTrustEndpoint(ap) {
		L(e).Log(context.Background(), types.LevelTrace,
			"dropping direct message from addrpair, cannot trust endpoint", "ap", ap.String())
		return nil
	}

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		if !e.pingDirectValid(ap, clearMsg.Session, m) {
			L(e).Warn("dropping invalid ping", "ap", ap.String())
			return nil
		}

		e.lastPingRecv = time.Now()
		e.replyWithPongDirect(ap, clearMsg.Session, m)

		if ap != e.currentOutEndpoint && !e.tracker.Has(ap) {
			// We're not sending pings to this, yet we may want to, to prevent asymmetric glare
			pi := e.getPeerInfo()
			if pi == nil {
				// Peer info unavailable
				return nil
			}

			L(e).Log(context.Background(), types.LevelTrace, "sending ping to ping to prevent assymetric glare", "ap", ap.String(), "current", e.currentOutEndpoint.String())

			// Send ping with ID, so that it eventually blackholes
			e.tm.SendPingDirectWithID(ap, e.peer, pi.Session, m.TxID)
		}

		return nil

	case *msgsess.Pong:
		if err := e.pongDirectValid(ap, clearMsg.Session, m); err != nil {
			L(e).Warn("dropping invalid pong", "ap", ap.String(), "err", err)
		} else {
			e.lastPongRecv = time.Now()
			e.tracker.GotPong(ap)
			e.clearPongDirect(ap, clearMsg.Session, m)

			e.checkChangedPreferredEndpoint()
		}

		return nil

	default:
		L(e).Debug("ignoring direct session message",
			"ap", ap,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}

func (e *Established) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeRelay(e, relay, peer, clearMsg); s != nil {
		return s
	}

	LogRelayMessage(e, relay, peer, clearMsg)

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		e.replyWithPongRelay(relay, peer, clearMsg.Session, m)
		return nil

	case *msgsess.Pong:
		e.ackPongRelay(relay, peer, clearMsg.Session, m)
		return nil

	// TODO maybe re-establishment logic?
	// case *msg.Rendezvous:
	default:
		L(e).Debug("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}

func (e *Established) GetEndpoint() netip.AddrPort {
	return e.currentOutEndpoint
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

func (e *Established) checkChangedPreferredEndpoint() {
	bap, err := e.tracker.BestAddrPort()
	if err != nil {
		// this should not happen, at this point we have at least one happy pair
		panic(err)
	}

	if bap != e.currentOutEndpoint {
		L(e).Log(context.Background(), types.LevelTrace, "switching bestaddrport", "bap", bap.String(), "current", e.currentOutEndpoint.String())
		// not the best one, switch
		e.switchToEndpoint(bap)
	}
}

func (e *Established) switchToEndpoint(ep netip.AddrPort) {
	previous := e.currentOutEndpoint

	e.currentOutEndpoint = ep

	e.tm.OutConnUseAddrPort(e.peer, ep)
	e.tm.DManSetAKA(e.peer, ep)

	L(e).Info(
		"SWITCHED direct peer connection to better endpoint",
		"peer", e.peer.Debug(),
		"from", previous.String(),
		"to", ep.String(),
	)
}
