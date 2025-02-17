package msgsess

import (
	"fmt"
	"net/netip"
	"slices"

	"github.com/edup2p/common/types"
)

type Rendezvous struct {
	MyAddresses []netip.AddrPort
}

func (r *Rendezvous) MarshalSessionMessage() []byte {
	b := make([]byte, 0)

	for _, ap := range r.MyAddresses {
		b = append(b, types.PutAddrPort(ap)...)
	}

	return slices.Concat([]byte{byte(v1), byte(RendezvousMessage)}, b)
}

func (r *Rendezvous) Debug() string {
	return fmt.Sprintf("rendezvous addresses=%s", types.PrettyAddrPortSlice(r.MyAddresses))
}
