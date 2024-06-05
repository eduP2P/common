package relayhttp

import (
	"bufio"
	"context"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/conn"
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
)

func makeRelayURL(opts dial.Opts) string {
	proto := "http"
	domain := opts.Domain
	if opts.TLS {
		proto = "https"
	}
	if domain == "" {
		domain = "relay.ts"
	}
	return fmt.Sprintf("%s://%s/relay", proto, domain)
}

func Dial(ctx context.Context, opts dial.Opts, getPriv func() *key.NodePrivate, expectKey key.NodePublic) (*relay.Client, error) {
	opts.SetDefaults()

	c, err := dial.HTTP(ctx, opts, makeRelayURL(opts), relay.UpgradeProtocol, func(parentCtx context.Context, mc conn.MetaConn, brw *bufio.ReadWriter, opts dial.Opts) (*relay.Client, error) {
		return relay.EstablishClient(ctx, mc, brw, opts.EstablishTimeout, getPriv)
	})
	if err != nil {
		return nil, err
	}

	if !expectKey.IsZero() && c.RelayKey() != expectKey {
		c.Close()

		return nil, fmt.Errorf("relay key did not match expected key")
	}

	return c, nil
}
