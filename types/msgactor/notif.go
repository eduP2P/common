package msgactor

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type PeerState byte

const (
	PeerStateIdle PeerState = iota
	PeerStateRelay
	PeerStateDirect
)

type PeerConnStateChangeNotification struct {
	peer key.NodePublic

	state PeerState
}

type LocalEndpointsChangeNotification struct {
	endpoints []netip.AddrPort
}

type HomeRelayChangeNotification struct {
	homeRelay int64
}
