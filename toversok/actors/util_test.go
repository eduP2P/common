package actors

import (
	"net/netip"

	"github.com/edup2p/common/types/key"
)

var dummyAddrPort netip.AddrPort = netip.AddrPortFrom(netip.IPv4Unspecified(), 0)
var dummyKey key.NodePublic = [32]byte{0}

func zeroBytes(n int) []byte {
	return make([]byte, n)
}
