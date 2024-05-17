package relayhttp

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/http"
	"net/netip"
	"strings"
)

func ServerHandler(s *relay.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := strings.ToLower(r.Header.Get("Upgrade"))

		if up != relay.UpgradeProtocol {
			if up != "" {
				s.L().Warn("odd upgrade requested", "upgrade", up, "peer", r.RemoteAddr)
			}
			http.Error(w, "ToverSok relay requires correct protocol upgrade", http.StatusUpgradeRequired)
			return
		}

		h, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "HTTP does not support general TCP support", 500)
			return
		}

		netConn, conn, err := h.Hijack()
		if err != nil {
			s.L().Warn("hijack failed", "error", err, "peer", r.RemoteAddr)
			http.Error(w, "HTTP does not support general TCP support", 500)
			return
		}

		// TODO re-add publickey frontloading?
		//pubKey := s.PublicKey()
		// "Relay-Public-Key: %s\r\n\r\n",pubKey.HexString()

		fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: %s\r\n"+
			"Connection: Upgrade\r\n\r\n",
			up)

		remoteIPPort, _ := netip.ParseAddrPort(netConn.RemoteAddr().String())

		err = s.Accept(r.Context(), netConn, conn, remoteIPPort)

		s.L().Info("client exited", "reason", err)
	})
}
