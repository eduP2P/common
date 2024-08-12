package ifaces

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/netip"
)

// ControlCallbacks are the possible updates that the control server wishes to inform the client about.
type ControlCallbacks interface {
	// AddPeer has the server inform the client that it should observe another peer, with the following details.
	AddPeer(
		peer key.NodePublic,
		homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic,
		ip4 netip.Addr, ip6 netip.Addr,
	) error

	// UpdatePeer has the server inform of one of more updates to the client. All parameters other than peer are nullable.
	UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic) error

	// RemovePeer has the server inform the client to stop observing another peer.
	RemovePeer(peer key.NodePublic) error

	// UpdateRelays has the server inform the client of relay information, or updates.
	//
	// This is a set-add/update operation. (The client should not remove relays from its internal cache,
	// if it is not present in this list.)
	UpdateRelays(relay []relay.Information) error
}

// ControlInterface are the methods that should be present on a control session,
// to inform the control server of updates, and get information about the current logged-in node.
type ControlInterface interface {
	// ControlKey gets the public key of the current control server. Used for TOFU and debugging.
	ControlKey() key.ControlPublic
	// IPv4 gets the node's ipv4 address as assigned by the control server.
	//
	// As it is a netip.Prefix, it also includes the expected ipv4 range that all peers will be on.
	IPv4() netip.Prefix
	// IPv6 gets the node's ipv6 address as assigned by the control server.
	//
	// As it is a netip.Prefix, it also includes the expected ipv6 range that all peers will be on.
	IPv6() netip.Prefix

	// UpdateEndpoints informs the server of any changes in STUN-resolved endpoints. This is a set-replace operation.
	UpdateEndpoints([]netip.AddrPort) error
}

// ControlSession is an interface representing an active control session.
type ControlSession interface {
	ControlInterface

	// InstallCallbacks installs the current session's callbacks to another interface.
	//
	// This interface will be informed of updates from the control server.
	InstallCallbacks(ControlCallbacks)
}
