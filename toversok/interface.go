package toversok

import (
	"net"
	"time"
)

// TODO replace
type WGKey [32]byte

// WGAddress is a struct around IPNet that preserves the original bits, as IPNet masks them
type WGAddress struct {
	// Our IP
	IP net.IP
	// The network the wireguard interface operates in
	Network net.IPNet
}

// A peer config update struct, all values are nullable and thus pointers.
type PeerCfg struct {
	// The IP the node is addressable by, allowedIPs in wireguard terms.
	VirtualIP *net.IP

	KeepAliveInterval *time.Duration

	Endpoint *net.UDPAddr

	// (extra)
	PreSharedKey *WGKey
}

type WGStats struct {
	LastHandshake time.Time
	TxBytes       int64
	RxBytes       int64
}

type WireGuardConfigurator interface {
	// Init the wireguard interface, and make it ready for configuration changes.
	Init(privateKey WGKey, address WGAddress) (port int, err error)

	// UpdatePeer updates a peer with certain values, mapped by public key
	UpdatePeer(publicKey WGKey, cfg PeerCfg) error

	// RemovePeer removes a peer's config entirely from the wireguard interface
	RemovePeer(publicKey WGKey) error

	// GetStats gets basic statistics on a certain peer.
	//
	// (extra)
	GetStats(publicKey WGKey) (WGStats, error)
}

type FirewallConfigurator interface {
	// QuarantineNodes configures the firewall to block incoming connections from these IPs.
	//
	// Replaces an existing firewall configuration.
	QuarantineNodes(ips []net.IP) error
}
