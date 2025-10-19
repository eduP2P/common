package msgsess

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"github.com/edup2p/common/types"
)

type Rendezvous struct {
	MyAddresses []netip.AddrPort
}

func (r *Rendezvous) Marshal() []byte {
	b := make([]byte, 0)

	for _, ap := range r.MyAddresses {
		b = append(b, types.PutAddrPort(ap)...)
	}

	return slices.Concat([]byte{byte(v1), byte(RendezvousMessage)}, b)
}

func (r *Rendezvous) Parse(b []byte) error {
	if len(b)%18 != 0 {
		return errors.New("malformed rendezvous addresses")
	}

	aps := make([]netip.AddrPort, 0)

	for {
		ap := types.ParseAddrPort([18]byte(b[:18]))
		aps = append(aps, ap)
		b = b[18:]

		if len(b) == 0 {
			break
		}
	}

	r.MyAddresses = aps

	return nil
}

func (r *Rendezvous) Debug() string {
	return fmt.Sprintf("rendezvous addresses=%s", types.PrettyAddrPortSlice(r.MyAddresses))
}
