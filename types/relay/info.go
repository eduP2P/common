package relay

import (
	"github.com/LukaGiorgadze/gonull"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type RelayInformation struct {
	ID int64

	// The key that a client can expect from the relay, always set.
	Key key.NodePublic

	// The domain of the relay, to try to connect to.
	//
	// Can be empty ("") with IPs set.
	Domain string

	// Common Name on expected TLS certificate
	CertCN gonull.Nullable[string]

	// Forced IPs to try to connect to, bypasses HostName DNS lookup
	IPs gonull.Nullable[[]netip.Addr]

	// Optional STUN port override. (Default 3478)
	STUNPort gonull.Nullable[uint16]

	// Optional HTTPS/TLS port override. (Default 443)
	HTTPSPort gonull.Nullable[uint16]

	// Optional HTTP port override. (Default 80)
	//
	// Also used for captive portal checks.
	HTTPPort gonull.Nullable[uint16]

	// Whether to connect to this relay via plain HTTP or not.
	//
	// Used for tests and development environments.
	IsInsecure bool

	// Whether to use this relay to detect captive portals.
	IsCaptiveBuster gonull.Nullable[bool]
}
