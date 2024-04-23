package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
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

	return nil
}

func (e *EstRendezAck) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(e, ap, clear)

	switch m := clear.Message.(type) {
	case *msg.Ping:
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
	case *msg.Pong:
		e.tm.Poke()
		return LogTransition(e, &Finalizing{
			EstablishingCommon: e.EstablishingCommon,
			ap:                 ap,
			sess:               clear.Session,
			pong:               m,
		})
	//case *msg.Rendezvous:
	default:
		L(e).Info("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (e *EstRendezAck) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	if s := cascadeRelay(e, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(e, relay, peer, clear)

	switch m := clear.Message.(type) {
	case *msg.Ping:
		e.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg.Pong:
		e.ackPongRelay(relay, peer, clear.Session, m)
		return nil
	//case *msg.Rendezvous:
	default:
		L(e).Info("ignoring direct session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
