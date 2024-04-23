package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type Inactive struct {
	*StateCommon
}

func (i *Inactive) Name() string {
	return "inactive"
}

func (i *Inactive) OnTick() PeerState {
	if i.tm.InActive[i.peer] || i.tm.OutActive[i.peer] {
		return LogTransition(i, &Trying{StateCommon: i.StateCommon})
	}

	return nil
}

func (i *Inactive) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	if s := cascadeDirect(i, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(i, ap, clear)

	switch m := clear.Message.(type) {
	case *msg.Ping:
		if !i.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		i.replyWithPongDirect(ap, clear.Session, m)
		return nil
	case *msg.Pong:
		i.ackPongDirect(ap, clear.Session, m)
		return nil
	default:
		L(i).Info("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (i *Inactive) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	if s := cascadeRelay(i, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(i, relay, peer, clear)

	switch m := clear.Message.(type) {
	case *msg.Ping:
		i.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg.Pong:
		i.ackPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg.Rendezvous:
		i.tm.Poke()
		return LogTransition(i, &EstRendezGot{
			EstablishingCommon: mkEstComm(i.StateCommon, 0),
			m:                  m,
		})
	default:
		L(i).Info("ignoring direct session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
