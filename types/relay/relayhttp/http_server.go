package relayhttp

import (
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/http"
)

func ServerHandler(s *relay.Server) http.Handler {
	return dial.HTTPHandler(s, relay.UpgradeProtocol)
}
