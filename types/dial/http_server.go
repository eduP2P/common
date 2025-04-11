package dial

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/edup2p/common/types"
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

		if tcpConn, ok := netConn.(*net.TCPConn); ok {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				s.Logger().Warn("set keep alive failed", "error", err, "peer", r.RemoteAddr)
			}

			if err := tcpConn.SetKeepAlivePeriod(11 * time.Second); err != nil {
				s.Logger().Warn("set keep alive period failed", "error", err, "peer", r.RemoteAddr)
			}
		} else {
			s.Logger().Warn("could not get *net.TCPConn, to set keepalive", "peer", r.RemoteAddr)
		}

		defer func() {
			if err := netConn.Close(); err != nil {
				slog.Error("error when closing netconn", "err", err)
			}
		}()

		// TODO re-add publickey frontloading?
		//  pubKey := s.PublicKey()
		//  "Relay-Public-Key: %s\r\n\r\n",pubKey.HexString()

		if _, err := fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: %s\r\n"+
			"Connection: Upgrade\r\n\r\n",
			up); err != nil {
			slog.Error("error when writing 101 response", "err", err)
		}

		if err := brw.Flush(); err != nil {
			slog.Error("error when flushing 101 response", "err", err)
		}

		remoteIPPort, _ := netip.ParseAddrPort(netConn.RemoteAddr().String())

		ctx := context.TODO()
		// ctx := r.Context()
		// TODO cannot use request context due to https://github.com/golang/go/issues/32314
		//  it is also expected that the client application takes care of closenotify and the likes,
		//  after the connection has been hijacked

		err = s.Accept(ctx, netConn, brw, remoteIPPort)

		s.Logger().Info("client exited", "reason", err)
	})
}
