package controlhttp

import (
	"bufio"
	"context"
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/control"
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/key"
)

func makeControlURL(opts dial.Opts) string {
	proto := "http"
	domain := opts.Domain
	if opts.TLS {
		proto = "https"
	}
	if domain == "" {
		domain = "control.ts"
	}
	return fmt.Sprintf("%s://%s/control", proto, domain)
}

func Dial(ctx context.Context, opts dial.Opts, getPriv func() *key.NodePrivate, getSess func() *key.SessionPrivate, controlKey key.ControlPublic, session *string) (*control.Client, error) {
	opts.SetDefaults()

	return dial.HTTP(ctx, opts, makeControlURL(opts), control.UpgradeProtocol, func(parentCtx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, opts dial.Opts) (*control.Client, error) {
		return control.EstablishClient(ctx, mc, brw, opts.EstablishTimeout, getPriv, getSess, controlKey, session)
	})
}
