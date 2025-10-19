package ifaces

import "net/netip"

type Injectable interface {
	Available() bool

	InjectPacket(from, to netip.AddrPort, pkt []byte) error
}
