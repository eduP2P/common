package dial

import (
	"bufio"
	"context"
	"fmt"
	"github.com/edup2p/common/types"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
)

type ProtocolServer interface {
	Logger() *slog.Logger
	Accept(ctx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, remoteAddrPort netip.AddrPort) error
}

func HTTPHandler(s ProtocolServer, proto string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := strings.ToLower(r.Header.Get("Upgrade"))

		if up != proto {
			if up != "" {
				s.Logger().Warn("odd upgrade requested", "upgrade", up, "peer", r.RemoteAddr)
			}
			http.Error(w, "ToverSok relay requires correct protocol upgrade", http.StatusUpgradeRequired)
			return
		}

		h, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "HTTP does not support general TCP support", 500)
			return
		}

		netConn, brw, err := h.Hijack()
		if err != nil {
			s.Logger().Warn("hijack failed", "error", err, "peer", r.RemoteAddr)
			http.Error(w, "HTTP does not support general TCP support", 500)
			return
		}

		defer netConn.Close()

		// TODO re-add publickey frontloading?
		//  pubKey := s.PublicKey()
		//  "Relay-Public-Key: %s\r\n\r\n",pubKey.HexString()

		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: %s\r\n"+
			"Connection: Upgrade\r\n\r\n",
			up)

		brw.Flush()

		remoteIPPort, _ := netip.ParseAddrPort(netConn.RemoteAddr().String())

		ctx := context.Background()
		// ctx := r.Context()
		// TODO cannot use request context due to https://github.com/golang/go/issues/32314
		//  it is also expected that the client application takes care of closenotify and the likes,
		//  after the connection has been hijacked

		err = s.Accept(ctx, netConn, brw, remoteIPPort)

		s.Logger().Info("client exited", "reason", err)
	})
}
