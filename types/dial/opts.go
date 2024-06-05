package dial

import (
	"net/netip"
	"time"
)

const (
	DefaultConnectTimeout   = time.Second * 30
	DefaultEstablishTimeout = time.Second * 15
)

type Opts struct {
	Domain string

	// If non-empty, overrides DNS lookup from Domain
	Addrs []netip.Addr

	// If not set, will use 80 for not TLS, and 443 for TLS
	Port uint16

	// Establish the connection with TLS, turns HTTP into HTTPS.
	TLS bool

	// If non-empty, sends this string in SNI, and checks the certificate common name against it.
	//
	// Only works if TLS is true.
	ExpectCertCN string

	// If nil, uses default of 30 seconds
	ConnectTimeout time.Duration

	// If nil, uses default of 15 seconds
	EstablishTimeout time.Duration
}

func (opts *Opts) SetDefaults() {
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = DefaultConnectTimeout
	}

	if opts.EstablishTimeout == 0 {
		opts.EstablishTimeout = DefaultEstablishTimeout
	}

	if opts.Port == 0 {
		if opts.TLS {
			opts.Port = 443
		} else {
			opts.Port = 80
		}
	}
}
