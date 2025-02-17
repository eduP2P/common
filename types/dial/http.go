package dial

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/edup2p/common/types"
)

// getPriv func() *key.NodePrivate, getSess func() *key.SessionPrivate, controlKey key.NodePublic

func HTTP[T any](ctx context.Context, opts Opts, url, protocol string, makeClient func(parentCtx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, opts Opts) (*T, error)) (*T, error) {
	netConn, err := WithTLS(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	brw := bufio.NewReadWriter(bufio.NewReader(netConn), bufio.NewWriter(netConn))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create http request: %w", err)
	}
	req.Header.Set("Upgrade", protocol)
	req.Header.Set("Connection", "Upgrade")

	closeNetConn := func() {
		if err := netConn.Close(); err != nil {
			slog.Error("error when closing netconn", "err", err)
		}
	}

	if err := req.Write(brw); err != nil {
		closeNetConn()
		return nil, fmt.Errorf("could not write http request: %w", err)
	}
	if err := brw.Flush(); err != nil {
		closeNetConn()
		return nil, fmt.Errorf("could not flush http request: %w", err)
	}

	if err := netConn.SetReadDeadline(time.Now().Add(time.Second * 5)); err != nil {
		closeNetConn()
		return nil, fmt.Errorf("could not set read deadline: %w", err)
	}
	resp, err := http.ReadResponse(brw.Reader, req)
	if err != nil {
		return nil, fmt.Errorf("could not read http response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		b, _ := io.ReadAll(resp.Body)
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("could not close http response body: %w", err)
		}
		return nil, fmt.Errorf("GET did not result in 101 response code: %w: %d \"%s\"", err, resp.StatusCode, b)
	}

	// At this point, we're speaking the protocol with the server.

	c, err := makeClient(ctx, netConn, brw, opts)
	if err != nil {
		closeNetConn()
		return nil, fmt.Errorf("failed to establish client: %w", err)
	}

	return c, nil
}
