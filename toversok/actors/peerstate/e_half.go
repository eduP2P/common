package peerstate

import (
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"net/netip"
	"time"
)

type EstHalf struct {
	*EstablishingCommon
}

func (e *EstHalf) Name() string {
	return "half-established"
}

func (e *EstHalf) OnTick() PeerState {
	if e.expired() {
		return LogTransition(e, e.retry())
	}

	if e.wantsPing() {
		L(e).Debug("sending periodic ping", "peer", e.peer.Debug())
		e.sendPingsToPeer()
	}

	return nil
}

func (e *EstHalf) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
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

		e.replyWithPongDirect(ap, clearMsg.Session, m)

		// Send one as a hail-mary, for if another got lost
		e.tm.SendPingDirect(ap, e.peer, clearMsg.Session)
		e.lastPing = time.Now()
		return nil
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

func (e *EstHalf) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
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
