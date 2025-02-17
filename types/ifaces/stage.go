package ifaces

import (
	"net/netip"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/stage"
)

// Stage documents/iterates the functions a Stage should expose
type Stage interface {
	Start()

	ControlCallbacks

	UpdateHomeRelay(peer key.NodePublic, relay int64) error
	UpdateSessionKey(peer key.NodePublic, session key.SessionPublic) error
	SetEndpoints(peer key.NodePublic, endpoints []netip.AddrPort) error

	GetPeerInfo(peer key.NodePublic) *stage.PeerInfo
	GetEndpoints() []netip.AddrPort
}
