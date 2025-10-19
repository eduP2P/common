package controlhttp

import (
	"net/http"

	"github.com/edup2p/common/types/control"
	"github.com/edup2p/common/types/dial"
)

func ServerHandler(s *control.Server) http.Handler {
	return dial.HTTPHandler(s, control.UpgradeProtocol)
}
