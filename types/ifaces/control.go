package ifaces

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/netip"
)

type ControlCallbacks interface {
	AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, ip4 netip.Addr, ip6 netip.Addr) error
	UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic) error
	RemovePeer(peer key.NodePublic) error

	UpdateRelays(relay []relay.Information) error
}

type ControlInterface interface {
	ControlKey() key.ControlPublic
	IPv4() netip.Prefix
	IPv6() netip.Prefix

	UpdateEndpoints([]netip.AddrPort) error
}

type FullControlInterface interface {
	ControlInterface

	InstallCallbacks(ControlCallbacks)
}
