package relayhttp

import (
	"net/http"

	"github.com/edup2p/common/types/dial"
	"github.com/edup2p/common/types/relay"
)

func ServerHandler(s *relay.Server) http.Handler {
	return dial.HTTPHandler(s, relay.UpgradeProtocol)
}
