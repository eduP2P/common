package relayhttp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"
)

const (
	DefaultConnectTimeout   = time.Second * 30
	DefaultEstablishTimeout = time.Second * 15
)

type DialOpts struct {
	Domain string

	// If non-empty, overrides DNS lookup from Domain
	Addrs []netip.Addr

	// If not set, will use 80 for not TLS, and 443 for TLS
	Port uint16

	// Establish the connection with TLS, turns HTTP into HTTPS.
	TLS bool

	// If non-empty, returns an error if the connected relay server public key is not this one.
	ExpectKey key.NodePublic

	// If non-empty, sends this string in SNI, and checks the certificate common name against it.
	//
	// Only works if TLS is true.
	ExpectCertCN string

	// If nil, uses default of 30 seconds
	ConnectTimeout time.Duration

	// If nil, uses default of 15 seconds
	EstablishTimeout time.Duration
}

func (opts *DialOpts) makeURL() string {
	proto := "http"
	if opts.TLS {
		proto = "https"
	}
	return fmt.Sprintf("%s://%s/relay", proto, opts.Domain)
}

func (opts *DialOpts) setDefaults() {
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

func Dial(ctx context.Context, opts DialOpts, getPriv func() *key.NodePrivate) (*relay.Client, error) {
	opts.setDefaults()

	var err error

	if opts.Addrs == nil || len(opts.Addrs) == 0 {
		opts.Addrs, err = net.DefaultResolver.LookupNetIP(ctx, "ip", opts.Domain)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup %s: %w", opts.Domain, err)
		}

		if len(opts.Addrs) == 0 {
			return nil, fmt.Errorf("DNS for %s returned no IP addresses", opts.Domain)
		}
	}
	// At this point, opts.Addrs has at least 1 IP we can try.

	var netConn net.Conn

	netConn, err = dialTCP(ctx, opts)

	if err != nil {
		return nil, fmt.Errorf("tcp dial failed: %w", err)
	}

	if opts.TLS {
		// TODO do TLS handshake, expect opts.Domain as cert CN, and replace netConn with it
		panic("todo")
	}

	brw := bufio.NewReadWriter(bufio.NewReader(netConn), bufio.NewWriter(netConn))

	req, err := http.NewRequest("GET", opts.makeURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create http request: %w", err)
	}
	req.Header.Set("Upgrade", relay.UpgradeProtocol)
	req.Header.Set("Connection", "Upgrade")

	if err := req.Write(brw); err != nil {
		return nil, fmt.Errorf("could not write http request: %w", err)
	}
	if err := brw.Flush(); err != nil {
		return nil, fmt.Errorf("could not flush http request: %w", err)
	}

	netConn.SetReadDeadline(time.Now().Add(time.Second * 5))
	resp, err := http.ReadResponse(brw.Reader, req)
	if err != nil {
		return nil, fmt.Errorf("could not read http response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("GET did not result in 101 response code: %w: %d \"%s\"", err, resp.StatusCode, b)
	}

	// At this point, we're speaking the relay protocol with the server.

	c, err := relay.EstablishClient(ctx, netConn, brw, opts.EstablishTimeout, getPriv)

	if err != nil {
		return nil, fmt.Errorf("failed to establish client: %w", err)
	}

	if !opts.ExpectKey.IsZero() {
		if opts.ExpectKey != c.RelayKey() {
			return nil, errors.New("expected relay server key did not match received key")
		}
	}

	return c, nil
}

func dialTCP(ctx context.Context, opts DialOpts) (net.Conn, error) {
	type dialResult struct {
		c net.Conn
		e error
	}

	dialCtx, dialCancel := context.WithCancel(ctx)
	defer dialCancel()

	results := make(chan dialResult)

	returned := make(chan struct{})
	defer close(returned)

	for _, addr := range opts.Addrs {
		ap := netip.AddrPortFrom(addr, opts.Port)
		go func() {
			c, e := dialOneTCP(dialCtx, ap)

			select {
			case results <- dialResult{c: c, e: e}:
			case <-returned:
				if c != nil {
					c.Close()
				}
			}
		}()
	}

	// Start the timer for the fallback racer.
	timer := time.NewTimer(opts.ConnectTimeout)
	defer timer.Stop()

	var errs []error

	for {
		select {
		case <-timer.C:
			// Timeout
			return nil, fmt.Errorf("dial timeout: %w", errors.Join(errs...))
		case res := <-results:
			if res.e == nil {
				return res.c, nil
			} else {
				errs = append(errs, res.e)

				if len(errs) >= len(opts.Addrs) {
					return nil, fmt.Errorf("dial failure: %w", errors.Join(errs...))
				}
			}
		}
	}
}

func dialOneTCP(ctx context.Context, ap netip.AddrPort) (net.Conn, error) {
	// For some reason, DialTCP does not have a *Context variant.
	// So for now we put the AddrPort back into a string and pass it to our dialer.
	// see: https://github.com/golang/go/issues/49097

	var d net.Dialer
	d.LocalAddr = nil

	return d.DialContext(ctx, "tcp", ap.String())
}
