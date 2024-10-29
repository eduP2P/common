// Package stage contains some miscellaneous types required in both toversok and types/ifaces.
package stage

import (
	"github.com/edup2p/common/types/key"
	"net/netip"
	"time"
)

type SentPing struct {
	ToRelay  bool
	RelayID  int64
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
