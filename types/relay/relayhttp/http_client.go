package relayhttp

import (
	"bufio"
	"context"
	"fmt"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/dial"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/relay"
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

type RelayDialFunc func(ctx context.Context, opts dial.Opts, getPriv func() *key.NodePrivate, expectKey key.NodePublic) (relay.Client, error)

func Dial(ctx context.Context, opts dial.Opts, getPriv func() *key.NodePrivate, expectKey key.NodePublic) (relay.Client, error) {
	opts.SetDefaults()

	c, err := dial.HTTP(ctx, opts, makeRelayURL(opts), relay.UpgradeProtocol, func(parentCtx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, opts dial.Opts) (*relay.HTTPClient, error) {
		return relay.EstablishClient(parentCtx, mc, brw, opts.EstablishTimeout, getPriv)
	})
	if err != nil {
		return nil, err
	}

	if !expectKey.IsZero() && c.RelayKey() != expectKey {
		err = fmt.Errorf("relay key did not match expected key")
		c.Cancel(err)

		return nil, err
	}

	return c, nil
}
