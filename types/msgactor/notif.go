package msgactor

import (
	"net/netip"

	"github.com/edup2p/common/types/key"
)

type PeerState byte

const (
	PeerStateIdle PeerState = iota
	PeerStateRelay
	PeerStateDirect
)

//nolint:unused
type PeerConnStateChangeNotification struct {
	peer key.NodePublic

	state PeerState
}

//nolint:unused
type LocalEndpointsChangeNotification struct {
	endpoints []netip.AddrPort
}

//nolint:unused
type HomeRelayChangeNotification struct {
	homeRelay int64
}
