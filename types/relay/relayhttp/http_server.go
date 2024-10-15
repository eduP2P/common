package relayhttp

import (
	"github.com/edup2p/common/types/dial"
	"github.com/edup2p/common/types/relay"
	"net/http"
)

func ServerHandler(s *relay.Server) http.Handler {
	return dial.HTTPHandler(s, relay.UpgradeProtocol)
}
