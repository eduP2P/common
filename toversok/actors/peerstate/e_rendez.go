package peerstate

import (
	"net/netip"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
)

type EstRendezAck struct {
	*EstablishingCommon
}

func (e *EstRendezAck) Name() string {
	return "rendezvous-acknowledged"
}

func (e *EstRendezAck) OnTick() PeerState {
	if e.expired() {
		return LogTransition(e, e.retry())
	}

	if e.wantsPing() {
		L(e).Debug("sending periodic ping", "peer", e.peer.Debug())
		e.sendPingsToPeer()
	}

	return nil
}

func (e *EstRendezAck) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
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
		if err := e.pongDirectValid(ap, clearMsg.Session, m); err != nil {
			L(e).Warn("dropping invalid pong", "ap", ap.String(), "err", err)
			return nil
		}

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

func (e *EstRendezAck) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
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
	default:
		L(e).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}
