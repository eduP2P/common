package msgsess

import (
	"fmt"
	"github.com/edup2p/common/types"
	"net/netip"
	"slices"
)

type Pong struct {
	TxID [12]byte

	Src netip.AddrPort // 18 bytes (16+2) on the wire; v4-mapped ipv6 for IPv4
}

func (p *Pong) MarshalSessionMessage() []byte {
	return slices.Concat([]byte{byte(v1), byte(PongMessage)}, p.TxID[:], types.PutAddrPort(p.Src))
}

func (p *Pong) Debug() string {
	return fmt.Sprintf("pong tx=%x", p.TxID)
}
