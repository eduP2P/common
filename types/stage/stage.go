// Package stage contains some miscellaneous types required in both toversok and types/ifaces.
package stage

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
	"time"
)

type SentPing struct {
	ToRelay  bool
	RelayId  int64
	AddrPort netip.AddrPort
	At       time.Time
	To       key.NodePublic
}

type PeerInfo struct {
	HomeRelay           int64
	Endpoints           []netip.AddrPort
	RendezvousEndpoints []netip.AddrPort
	Session             key.SessionPublic
}
