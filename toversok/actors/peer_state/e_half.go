package peer_state

import (
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
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

	return nil
}

func (e *EstHalf) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(e, ap, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !e.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		e.replyWithPongDirect(ap, clear.Session, m)

		// Send one as a hail-mary, for if another got lost
		e.tm.SendPingDirect(ap, e.peer, clear.Session)
		return nil
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

func (e *EstHalf) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
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
	default:
		L(e).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
