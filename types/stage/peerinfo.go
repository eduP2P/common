package stage

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type PeerInfo struct {
	HomeRelay           int64
	Endpoints           []netip.AddrPort
	RendezvousEndpoints []netip.AddrPort
	Session             key.SessionPublic
}
