package key

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
)

type NodePublic NakedKey

type NodePrivate struct {
	_   types.Incomparable
	key NakedKey
}

func (n NodePublic) Debug() string {
	return fmt.Sprintf("%x", n)
}
