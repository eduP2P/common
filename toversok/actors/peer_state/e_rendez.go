package peer_state

import (
	"context"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
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

	if e.wantsPing() {
		L(e).Log(context.Background(), types.LevelTrace, "sending periodic ping", "peer", e.peer.Debug())
		e.sendPingsToPeer()
	}

	return nil
}

func (e *EstRendezAck) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	if s := cascadeDirect(e, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(e, ap, clear)

	switch m := clear.Message.(type) {
	case *msg2.Ping:
		if !e.pingDirectValid(ap, clear.Session, m) {
			L(e).Warn("dropping invalid ping", "ap", ap.String())
			return nil
		}

		e.tm.Poke()
		return LogTransition(e, &EstHalfIng{
			EstablishingCommon: e.EstablishingCommon,
			ap:                 ap,
			sess:               clear.Session,
			ping:               m,
		})
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

func (e *EstRendezAck) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
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
