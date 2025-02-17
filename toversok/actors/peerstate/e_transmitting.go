package peerstate

import (
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
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

	if e.wantsPing() {
		L(e).Debug("sending periodic ping", "peer", e.peer.Debug())
		e.sendPingsToPeer()
	}

	return nil
}

func (e *EstTransmitting) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clearMsg); s != nil {
		return s
	}

	LogDirectMessage(e, ap, clearMsg)

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		if !e.pingDirectValid(ap, clearMsg.Session, m) {
			L(e).Warn("dropping invalid ping", "ap", ap.String())
			return nil
		}

		e.tm.Poke()
		return LogTransition(e, &EstHalfIng{
			EstablishingCommon: e.EstablishingCommon,
			ap:                 ap,
			sess:               clearMsg.Session,
			ping:               m,
		})
	case *msgsess.Pong:
		e.tm.Poke()
		return LogTransition(e, &Finalizing{
			EstablishingCommon: e.EstablishingCommon,
			ap:                 ap,
			sess:               clearMsg.Session,
			pong:               m,
		})
	default:
		L(e).Warn("ignoring direct session message",
			"ap", ap,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}

func (e *EstTransmitting) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeRelay(e, relay, peer, clearMsg); s != nil {
		return s
	}

	LogRelayMessage(e, relay, peer, clearMsg)

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

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		e.replyWithPongRelay(relay, peer, clearMsg.Session, m)
		return nil
	case *msgsess.Pong:
		e.ackPongRelay(relay, peer, clearMsg.Session, m)
		return nil
	case *msgsess.Rendezvous:
		e.tm.Poke()
		return LogTransition(e, &EstRendezGot{EstablishingCommon: e.EstablishingCommon, m: m})
	default:
		L(e).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}
