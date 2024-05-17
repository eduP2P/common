package peer_state

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msg"
	"net/netip"
)

// This state pattern was inspired by https://refactoring.guru/design-patterns/state/go/example

// PeerState defines an interface with which a PeerState can be driven.
//
// The PeerState return value is effectively a nullable; if its nil, then keep the current state.
// If it's non-nil, replace the state for the peer with the state returned.
type PeerState interface {
	OnTick() PeerState
	OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState
	OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState

	// Name returns a lower-case name to be used in logging.
	Name() string

	// Peer returns the peer for which this state is being managed for.
	Peer() key.NodePublic
}
