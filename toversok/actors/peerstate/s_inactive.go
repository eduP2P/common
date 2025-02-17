package peerstate

import (
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
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

func (i *Inactive) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeDirect(i, ap, clearMsg); s != nil {
		return s
	}

	LogDirectMessage(i, ap, clearMsg)

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		if !i.pingDirectValid(ap, clearMsg.Session, m) {
			L(i).Warn("dropping invalid ping", "ap", ap.String())
			return nil
		}

		i.replyWithPongDirect(ap, clearMsg.Session, m)
		return nil
	case *msgsess.Pong:
		i.ackPongDirect(ap, clearMsg.Session, m)
		return nil
	default:
		L(i).Warn("ignoring direct session message",
			"ap", ap,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}

func (i *Inactive) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeRelay(i, relay, peer, clearMsg); s != nil {
		return s
	}

	LogRelayMessage(i, relay, peer, clearMsg)

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		i.replyWithPongRelay(relay, peer, clearMsg.Session, m)
		return nil
	case *msgsess.Pong:
		i.ackPongRelay(relay, peer, clearMsg.Session, m)
		return nil
	case *msgsess.Rendezvous:
		i.tm.Poke()
		return LogTransition(i, &EstRendezGot{
			EstablishingCommon: mkEstComm(i.StateCommon, 0),
			m:                  m,
		})
	default:
		L(i).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}
