package controlhttp

import (
	"github.com/edup2p/common/types/control"
	"github.com/edup2p/common/types/dial"
	"net/http"
)

func ServerHandler(s *control.Server) http.Handler {
	return dial.HTTPHandler(s, control.UpgradeProtocol)
}
