package peerstate

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	pub4Addr = netip.MustParseAddrPort("8.0.0.1:1337")
	pub6Addr = netip.MustParseAddrPort("[2000::1]:1337")

	priv4Addr = netip.MustParseAddrPort("10.0.0.1:1337")
	priv6Addr = netip.MustParseAddrPort("[fd00::1]:1337")
)

func TestPingTracker_FullSelection(t *testing.T) {
	pt := NewPingTracker()

	for _, ip := range []netip.AddrPort{
		pub4Addr,
		pub6Addr,

		priv4Addr,
		priv6Addr,
	} {
		pt.GotPong(ip)
	}

	bap, err := pt.BestAddrPort()

	assert.NoError(t, err)
	assert.Equal(t, bap, priv6Addr)
}

func TestPingTracker_NoPings(t *testing.T) {
	pt := NewPingTracker()

	_, err := pt.BestAddrPort()

	assert.Error(t, err)
}

func TestPingTracker_BestAddrPort(t *testing.T) {
	pt := NewPingTracker()

	var bap netip.AddrPort
	var err error

	// First add private ip4
	pt.GotPong(priv4Addr)
	bap, err = pt.BestAddrPort()
	assert.NoError(t, err)
	assert.Equal(t, bap, priv4Addr)

	// Then add public ip4,
	// this changes nothing
	pt.GotPong(pub4Addr)
	bap, err = pt.BestAddrPort()
	assert.NoError(t, err)
	assert.Equal(t, bap, priv4Addr)

	// Then add private ip6
	pt.GotPong(priv6Addr)
	bap, err = pt.BestAddrPort()
	assert.NoError(t, err)
	assert.Equal(t, bap, priv6Addr)

	// Then add public ip6,
	// this changes nothing
	pt.GotPong(pub6Addr)
	bap, err = pt.BestAddrPort()
	assert.NoError(t, err)
	assert.Equal(t, bap, priv6Addr)
}
