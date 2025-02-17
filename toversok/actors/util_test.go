package actors

import (
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
)

// Test constants
const assertEventuallyTick = 1 * time.Millisecond
const assertEventuallyTimeout = 10 * assertEventuallyTick

// Test variables
var dummyAddr = netip.IPv4Unspecified()
var dummyAddrPort = netip.AddrPortFrom(dummyAddr, 0)
var dummyKey key.NodePublic = [32]byte{0}

// Test session
var testPriv = key.NewSession()
var testPub = testPriv.Public()

func getTestPriv() *key.SessionPrivate {
	return &testPriv
}

func zeroBytes(n int) []byte {
	return make([]byte, n)
}
