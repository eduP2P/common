package peer_state

import (
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
	"time"
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

func (t *Trying) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	if s := cascadeDirect(t, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(t, ap, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !t.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		// TODO(jo): We could start establishing here, possibly.
		t.replyWithPongDirect(ap, clear.Session, m)
		return nil
	case *msg2.Pong:
		t.ackPongDirect(ap, clear.Session, m)
		return nil
	//case *msg.Rendezvous:
	default:
		L(t).Warn("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (t *Trying) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	if s := cascadeRelay(t, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(t, relay, peer, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		t.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg2.Pong:
		t.ackPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg2.Rendezvous:
		return LogTransition(t, &EstRendezGot{
			EstablishingCommon: mkEstComm(t.StateCommon, 0),
			m:                  m,
		})
	default:
		L(t).Warn("ignoring relay session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}
