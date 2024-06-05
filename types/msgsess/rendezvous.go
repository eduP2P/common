package msgsess

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/types/bin"
	"net/netip"
	"slices"
)

type Rendezvous struct {
	MyAddresses []netip.AddrPort
}

func (r *Rendezvous) MarshalSessionMessage() []byte {
	b := make([]byte, 0)

	for _, ap := range r.MyAddresses {
		b = append(b, bin.PutAddrPort(ap)...)
	}

	return slices.Concat([]byte{byte(v1), byte(RendezvousMessage)}, b)
}

func (r *Rendezvous) Debug() string {
	return fmt.Sprintf("rendezvous addresses=%#v", r.MyAddresses)
}
