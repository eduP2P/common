package peer_state

import (
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
)

type EstTransmitting struct {
	*EstablishingCommon
}

func (e *EstTransmitting) Name() string {
	return "transmitting"
}

func (e *EstTransmitting) OnTick() PeerState {
	if e.expired() {
		return LogTransition(e, e.retry())
	}

	return nil
}

func (e *EstTransmitting) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(e, ap, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !e.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		e.tm.Poke()
		return LogTransition(e, &EstHalfIng{
			EstablishingCommon: e.EstablishingCommon,
			ap:                 ap,
			sess:               clear.Session,
			ping:               m,
		})
	case *msg2.Pong:
		e.tm.Poke()
		return LogTransition(e, &Finalizing{
			EstablishingCommon: e.EstablishingCommon,
			ap:                 ap,
			sess:               clear.Session,
			pong:               m,
		})
	//case *msg.Rendezvous:
	default:
		L(e).Warn("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (e *EstTransmitting) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	if s := cascadeRelay(e, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(e, relay, peer, clear)

	// NOTE: There an edgecase that can happen here:
	//
	// Due to the way OnTick is called and cascaded every time,
	// there is a direct cascadeRelay {OnRelay->OnTick+OnRelay->...} transition chain through;
	//  - waiting
	//  - inactive
	//  - trying
	//  - pre-transmit
	// That can be triggered by a single OnRelay call to waiting,
	//  if all conditions to transition in the above have been met.
	//
	// If this OnRelay call is for an incoming Rendezvous, this means that:
	// We'd be processing the rendezvous here,
	//  while we have already sent one immediately just before in pre-transmit.
	//
	// We would then be sending double pings, one to endpoints defined via Control,
	//  and then once again to the endpoints sent in Rendezvous, which would possibly overlap.
	//
	// This is harmless, as the state diagram permits for it, but its worth noting.

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		e.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg2.Pong:
		e.ackPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg2.Rendezvous:
		e.tm.Poke()
		return LogTransition(e, &EstRendezGot{EstablishingCommon: e.EstablishingCommon, m: m})
	default:
		L(e).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
