package dial

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

// WithTLS does a "full" dial, including TLS wrapping and CN checking
func WithTLS(ctx context.Context, opts Opts) (net.Conn, error) {
	netConn, err := TCP(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("tcp dial failed: %w", err)
	}

	if opts.TLS {
		netConn = TLS(netConn, opts)
	}

	return netConn, nil
}

func TLS(conn net.Conn, opts Opts) *tls.Conn {
	cfg := new(tls.Config)

	switch {
	case opts.ExpectCertCN != "":
		cfg.ServerName = opts.ExpectCertCN
	case opts.Domain != "":
		cfg.ServerName = opts.Domain
	default:
		// We assume this is sane, else some upstream provider of the opt isn't proper with what it gives
		panic("TLS defined, but no domain provided")
	}

	return tls.Client(conn, cfg)
}

func TCP(ctx context.Context, opts Opts) (net.Conn, error) {
	opts.SetDefaults()

	var err error

	if len(opts.Addrs) == 0 {
		opts.Addrs, err = net.DefaultResolver.LookupNetIP(ctx, "ip", opts.Domain)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup %s: %w", opts.Domain, err)
		}

		if len(opts.Addrs) == 0 {
			return nil, fmt.Errorf("DNS for %s returned no IP addresses", opts.Domain)
		}
	}
	// At this point, opts.Addrs has at least 1 IP we can try.

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
			conn, err := dialOneTCP(dialCtx, ap)

			select {
			case results <- dialResult{c: conn, e: err}:
			case <-returned:
				if conn != nil {
					if err := conn.Close(); err != nil {
						slog.Error("failed to close tcp connection while multi-dialing", "err", err)
					}
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
	d.KeepAlive = time.Second * 10

	return d.DialContext(ctx, "tcp", ap.String())
}
