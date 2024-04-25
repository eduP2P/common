package peer_state

import (
	"context"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msg"
	"log/slog"
	"net/netip"
)

// cascadeDirect makes it so that first we call the default "tick" function of a peer's state,
// and if that requests a state transition, call a PeerState.OnDirect with the original arguments,
// and return the requested state change with that one if it returns one.
func cascadeDirect(so PeerState, ap netip.AddrPort, clear *msg.ClearMessage) (s PeerState) {
	if s1 := so.OnTick(); s1 != nil {
		if s2 := s1.OnDirect(ap, clear); s2 != nil {
			s = s2
		} else {
			s = s1
		}
	}

	return
}

// cascadeRelay makes it so that first we call the default "tick" function of a peer's state,
// and if that requests a state transition, call a PeerState.OnRelay with the original arguments,
// and return the requested state change with that one if it returns one.
func cascadeRelay(so PeerState, relay int64, peer key.NodePublic, clear *msg.ClearMessage) (s PeerState) {
	if s1 := so.OnTick(); s1 != nil {
		if s2 := s1.OnRelay(relay, peer, clear); s2 != nil {
			s = s2
		} else {
			s = s1
		}
	}

	return
}

const LevelTrace slog.Level = -8

// L stands for Log
func L(s PeerState) *slog.Logger {
	return slog.With("peer", s.Peer().Debug(), "state", s.Name())
}

func LogTransition(from PeerState, to PeerState) PeerState {
	L(from).Log(context.Background(), LevelTrace, "transitioning state", "to-state", to.Name())

	return to
}

func LogDirectMessage(s PeerState, ap netip.AddrPort, clear *msg.ClearMessage) {
	L(s).Log(context.Background(), LevelTrace, "received relay message",
		slog.Group("from",
			"addrport", ap,
			"session", clear.Session),
		"msg", clear.Message.Debug(),
	)
}

func LogRelayMessage(s PeerState, relay int64, peer key.NodePublic, clear *msg.ClearMessage) {
	L(s).Log(context.Background(), LevelTrace, "received direct message",
		slog.Group("from",
			"relay", relay,
			"peer", peer.Debug(),
			"session", clear.Session),
		"msg", clear.Message.Debug(),
	)
}
