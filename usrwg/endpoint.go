package usrwg

import (
	"github.com/edup2p/common/types/key"
	"net/netip"
	"slices"
)

type endpoint struct {
	k key.NodePublic
}

func (e *endpoint) DstToString() string {
	return e.k.HexString()
}

func (e *endpoint) DstToBytes() []byte {
	// FIXME: we don't yet know if this conflicts with how wggo does things;
	// 	its immutable, so it should probably go right
	return slices.Clone(e.k[:])
}

func (e *endpoint) DstIP() netip.Addr {
	return netip.AddrFrom16([16]byte(e.k[:16]))
}

func (e *endpoint) ClearSrc()           {}
func (e *endpoint) SrcToString() string { panic("unused") } // unused by wireguard-go
func (e *endpoint) SrcIP() netip.Addr   { panic("unused") } // unused by wireguard-go
