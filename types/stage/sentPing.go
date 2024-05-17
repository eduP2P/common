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
