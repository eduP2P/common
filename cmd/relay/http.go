package main

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/server/relay"
	"net/http"
	"net/netip"
	"strings"
)

func Handler(s *relay.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := strings.ToLower(r.Header.Get("Upgrade"))

		if up != relay.UpgradeProtocolV0 {
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

		pubKey := s.PublicKey()
		fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: %s\r\n"+
			"Connection: Upgrade\r\n"+
			"Relay-Public-Key: %s\r\n\r\n",
			up,
			pubKey.HexString())

		remoteIPPort, _ := netip.ParseAddrPort(netConn.RemoteAddr().String())

		err = s.Accept(r.Context(), netConn, conn, remoteIPPort)

		s.L().Info("client exited", "reason", err)
	})
}
