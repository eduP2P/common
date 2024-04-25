package msg

import (
	"fmt"
	"net/netip"
)

type Pong struct {
	TxID [12]byte

	Src netip.AddrPort // 18 bytes (16+2) on the wire; v4-mapped ipv6 for IPv4
}

func (p *Pong) MarshalSessionMessage() []byte {
	// TODO
	panic("implement me")
}

func (p *Pong) Debug() string {
	return fmt.Sprintf("pong tx=%x", p.TxID)
}
