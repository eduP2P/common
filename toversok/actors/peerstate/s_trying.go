package peerstate

import (
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
)

type Trying struct {
	*StateCommon

	tryAt    time.Time
	attempts int
}

func (t *Trying) Name() string {
	return "trying"
}

func (t *Trying) OnTick() PeerState {
	if time.Now().After(t.tryAt) {
		return LogTransition(t, &EstPreTransmit{
			EstablishingCommon: mkEstComm(t.StateCommon, t.attempts),
		})
	}

	return nil
}

func (t *Trying) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeDirect(t, ap, clearMsg); s != nil {
		return s
	}

	LogDirectMessage(t, ap, clearMsg)

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		if !t.pingDirectValid(ap, clearMsg.Session, m) {
			L(t).Warn("dropping invalid ping", "ap", ap.String())
			return nil
		}

		// TODO(jo): We could start establishing here, possibly.
		t.replyWithPongDirect(ap, clearMsg.Session, m)
		return nil
	case *msgsess.Pong:
		t.ackPongDirect(ap, clearMsg.Session, m)
		return nil
	default:
		L(t).Warn("ignoring direct session message",
			"ap", ap,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}

func (t *Trying) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	if s := cascadeRelay(t, relay, peer, clearMsg); s != nil {
		return s
	}

	LogRelayMessage(t, relay, peer, clearMsg)

	switch m := clearMsg.Message.(type) {
	case *msgsess.Ping:
		t.replyWithPongRelay(relay, peer, clearMsg.Session, m)
		return nil
	case *msgsess.Pong:
		t.ackPongRelay(relay, peer, clearMsg.Session, m)
		return nil
	case *msgsess.Rendezvous:
		return LogTransition(t, &EstRendezGot{
			EstablishingCommon: mkEstComm(t.StateCommon, 0),
			m:                  m,
		})
	default:
		L(t).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clearMsg.Session,
			"msg", m.Debug())
		return nil
	}
}
