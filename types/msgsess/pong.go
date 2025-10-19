package msgsess

import (
	"fmt"
	"net/netip"
	"slices"

	"github.com/edup2p/common/types"
)

type Pong struct {
	TxID [12]byte

	Src netip.AddrPort // 18 bytes (16+2) on the wire; v4-mapped ipv6 for IPv4
}

func (p *Pong) Marshal() []byte {
	return slices.Concat([]byte{byte(v1), byte(PongMessage)}, p.TxID[:], types.PutAddrPort(p.Src))
}

func (p *Pong) Parse(b []byte) error {
	if len(b) < 12+16+2 {
		return errTooSmall
	}

	p.TxID = [12]byte(b[:12])
	b = b[12:]

	p.Src = types.ParseAddrPort([18]byte(b[:18]))

	return nil
}

func (p *Pong) Debug() string {
	return fmt.Sprintf("pong tx=%x src=%s", p.TxID, p.Src.String())
}
