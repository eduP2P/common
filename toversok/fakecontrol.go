package toversok

import (
	"context"
	"net/netip"
	"time"

	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
)

type FakeControl struct {
	controlKey key.ControlPublic
	ipv4       netip.Prefix
	ipv6       netip.Prefix
}

func (f *FakeControl) ControlKey() key.ControlPublic {
	return f.controlKey
}

func (f *FakeControl) IPv4() netip.Prefix {
	return f.ipv4
}

func (f *FakeControl) IPv6() netip.Prefix {
	return f.ipv6
}

func (f *FakeControl) Expiry() time.Time {
	return time.Time{}
}

func (f *FakeControl) Context() context.Context {
	return context.Background()
}

func (f *FakeControl) InstallCallbacks(ifaces.ControlCallbacks) {
	// NOP
}

func (f *FakeControl) UpdateEndpoints([]netip.AddrPort) error {
	// NOP
	return nil
}

func (f *FakeControl) UpdateHomeRelay(int64) error {
	// NOP
	return nil
}
