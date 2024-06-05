package relay

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type Information struct {
	ID int64

	// The key that a client can expect from the relay, always set.
	Key key.NodePublic

	// The domain of the relay, to try to connect to.
	//
	// Can be empty ("") with IPs set.
	Domain string `json:",omitempty"`

	// Common Name on expected TLS certificate
	CertCN *string `json:",omitempty"`

	// Forced IPs to try to connect to, bypasses HostName DNS lookup
	IPs []netip.Addr `json:",omitempty"`

	// Optional STUN port override. (Default 3478)
	STUNPort *uint16 `json:",omitempty"`

	// Optional HTTPS/TLS port override. (Default 443)
	HTTPSPort *uint16 `json:",omitempty"`

	// Optional HTTP port override. (Default 80)
	//
	// Also used for captive portal checks.
	HTTPPort *uint16 `json:",omitempty"`

	// Whether to connect to this relay via plain HTTP or not.
	//
	// Used for tests and development environments.
	IsInsecure bool `json:",omitempty"`

	// Whether to use this relay to detect captive portals.
	IsCaptiveBuster *bool `json:",omitempty"`
}
