package peer_state

import (
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
)

type Inactive struct {
	*StateCommon
}

func (i *Inactive) Name() string {
	return "inactive"
}

func (i *Inactive) OnTick() PeerState {
	if pi := i.tm.Stage().GetPeerInfo(i.peer); (i.tm.ActiveIn()[i.peer] || i.tm.ActiveOut()[i.peer]) &&
		// We should wait for endpoints if we have none ourselves.
		((len(pi.Endpoints) > 0 || len(pi.RendezvousEndpoints) > 0) ||
			len(i.tm.Stage().GetEndpoints()) != 0) {
		return LogTransition(i, &Trying{StateCommon: i.StateCommon})
	}

	return nil
}

func (i *Inactive) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	if s := cascadeDirect(i, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(i, ap, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !i.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		i.replyWithPongDirect(ap, clear.Session, m)
		return nil
	case *msg2.Pong:
		i.ackPongDirect(ap, clear.Session, m)
		return nil
	default:
		L(i).Warn("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (i *Inactive) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	if s := cascadeRelay(i, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(i, relay, peer, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		i.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg2.Pong:
		i.ackPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg2.Rendezvous:
		i.tm.Poke()
		return LogTransition(i, &EstRendezGot{
			EstablishingCommon: mkEstComm(i.StateCommon, 0),
			m:                  m,
		})
	default:
		L(i).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
