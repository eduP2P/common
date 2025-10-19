package peerstate

import (
	"errors"
	"net/netip"
	"slices"
	"sync"

	"github.com/edup2p/common/types"
)

type PingTracker struct {
	rw      sync.RWMutex
	gotPong map[netip.AddrPort]bool
}

func NewPingTracker() *PingTracker {
	return &PingTracker{
		gotPong: make(map[netip.AddrPort]bool),
	}
}

func (pt *PingTracker) validAPs() []netip.AddrPort {
	var aps []netip.AddrPort

	for ap, gotPong := range pt.gotPong {
		if gotPong {
			aps = append(aps, ap)
		}
	}

	return aps
}

func (pt *PingTracker) GotPong(ap netip.AddrPort) {
	pt.rw.Lock()
	defer pt.rw.Unlock()

	nap := types.NormaliseAddrPort(ap)
	pt.gotPong[nap] = true
}

func (pt *PingTracker) Has(ap netip.AddrPort) bool {
	pt.rw.Lock()
	defer pt.rw.Unlock()

	nap := types.NormaliseAddrPort(ap)
	return pt.gotPong[nap]
}

func (pt *PingTracker) BestAddrPort() (netip.AddrPort, error) {
	pt.rw.RLock()
	defer pt.rw.RUnlock()

	aps := pt.validAPs()
	if len(aps) == 0 {
		return netip.AddrPort{}, errors.New("no valid pings")
	}

	slices.SortFunc(aps, gradeAPs)
	slices.Reverse(aps)

	return aps[0], nil
}

const (
	aBetter = 1
	bBetter = -1
	neither = 0
)

func gradeAPs(a, b netip.AddrPort) int {
	if verCmp := gradeVer(a, b); verCmp != neither {
		return verCmp
	}

	if privCmp := gradePriv(a, b); privCmp != neither {
		return privCmp
	}

	return a.Compare(b)
}

// IPv6 > IPv4
func gradeVer(ap, bp netip.AddrPort) int {
	a := ap.Addr()
	b := bp.Addr()

	if a.Is4() && b.Is6() {
		return bBetter
	} else if a.Is6() && b.Is4() {
		return aBetter
	}

	return neither
}

// Private/Unique Local > Non-Private/Unique Global
func gradePriv(ap, bp netip.AddrPort) int {
	a := ap.Addr()
	b := bp.Addr()

	if a.IsPrivate() && !b.IsPrivate() {
		return aBetter
	} else if !a.IsPrivate() && b.IsPrivate() {
		return bBetter
	}

	return neither
}
