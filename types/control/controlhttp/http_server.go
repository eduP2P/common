package controlhttp

import (
	"github.com/shadowjonathan/edup2p/types/control"
	"github.com/shadowjonathan/edup2p/types/dial"
	"net/http"
)

func ServerHandler(s *control.Server) http.Handler {
	return dial.HTTPHandler(s, control.UpgradeProtocol)
}
