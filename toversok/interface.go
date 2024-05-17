package toversok

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
	"time"
)

// PeerCfg isa a peer config update struct, all values are nullable through being pointers.
type PeerCfg struct {
	Set bool

	// The IPs the node is addressable by, allowedIPs in wireguard terms.
	VIPs *VirtualIPs

	KeepAliveInterval *time.Duration

	LocalEndpointPort *uint16
}

type VirtualIPs struct {
	IPv4 netip.Addr
	IPv6 netip.Addr
}

type WGStats struct {
	LastHandshake time.Time
	TxBytes       int64
	RxBytes       int64
}

type WireGuardConfigurator interface {
	// Init the wireguard interface, and make it ready for configuration changes.
	Init(privateKey key.NakedKey, addr4, add6 netip.Prefix) (port uint16, err error)

	// UpdatePeer updates a peer with certain values, mapped by public key
	UpdatePeer(publicKey key.NodePublic, cfg PeerCfg) error

	// RemovePeer removes a peer's config entirely from the wireguard interface
	RemovePeer(publicKey key.NodePublic) error

	// GetStats gets basic statistics on a certain peer.
	//
	// Returns (nil, nil) when peer couldn't be found.
	GetStats(publicKey key.NodePublic) (*WGStats, error)
}

type FirewallConfigurator interface {
	// QuarantineNodes configures the firewall to block incoming connections from these IPs.
	//
	// Replaces an existing firewall configuration.
	QuarantineNodes(ips []netip.Addr) error
}
