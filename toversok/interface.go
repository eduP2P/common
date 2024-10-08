package toversok

import (
	"context"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"net/netip"
	"time"
)

// PeerCfg isa a peer config update struct, all values are nullable through being pointers.
type PeerCfg struct {
	// The IPs the node is addressable by, allowedIPs in wireguard terms.
	VIPs VirtualIPs

	// The persistent keepalive interval, per peer.
	KeepAliveInterval *time.Duration
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

type ControlHost interface {
	// CreateClient creates a new control session, given a parent (cancellable) context,
	// a way to get the node's private key, and a way to get the current session's key.
	//
	// Will possibly block for a while, considering network conditions, timeouts, etc.
	//
	// Returns an interface with methods that can be called to inform the control server of updates,
	// get logged-in node information, and install callbacks to a particular other interface,
	// to have it be informed of updates from the control server.
	CreateClient(
		parentCtx context.Context,
		getNode func() *key.NodePrivate,
		getSess func() *key.SessionPrivate,
	) (ifaces.ControlSession, error)
}

type WireGuardHost interface {
	// Reset disables the generated wireguard controller, and all its state.
	Reset() error

	// Controller initialises the wireguard interface, and make it ready for configuration changes.
	//
	// Only one controller is expected to exist at any time.
	Controller(privateKey key.NodePrivate, addr4, addr6 netip.Prefix) (WireGuardController, error)
}

type WireGuardController interface {
	// UpdatePeer updates a peer with certain values, mapped by public key.
	UpdatePeer(publicKey key.NodePublic, cfg PeerCfg) error

	// RemovePeer removes a peer's config entirely from the wireguard interface.
	RemovePeer(publicKey key.NodePublic) error

	// GetStats gets basic statistics on a certain peer.
	//
	// Returns (nil, nil) when peer couldn't be found, or stats aren't implemented.
	GetStats(publicKey key.NodePublic) (*WGStats, error)

	// ConnFor returns an (internal or external) UDP-like connection for a particular peer.
	//
	// Can possibly return nil, when the peer has been removed, or not yet known to the controller.
	ConnFor(node key.NodePublic) types.UDPConn
}

type FirewallHost interface {
	Reset() error

	// Controller initialises the controller and returns it
	Controller() (FirewallController, error)
}

type FirewallController interface {
	// QuarantineNodes configures the firewall to block incoming connections from these IPs.
	//
	// Replaces an existing firewall configuration.
	QuarantineNodes(ips []netip.Addr) error
}
