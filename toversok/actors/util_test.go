package actors

import (
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
)

// Test constants
const (
	assertEventuallyTick    = 1 * time.Millisecond
	assertEventuallyTimeout = 10 * assertEventuallyTick
)

// Test variables
var (
	dummyAddr                    = netip.IPv4Unspecified()
	dummyAddrPort                = netip.AddrPortFrom(dummyAddr, 0)
	dummyKey      key.NodePublic = [32]byte{0}
)

// Test session
var (
	testPriv = key.NewSession()
	testPub  = testPriv.Public()
)

func getTestPriv() *key.SessionPrivate {
	return &testPriv
}

func zeroBytes(n int) []byte {
	return make([]byte, n)
}
