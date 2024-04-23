package msg

import (
	"fmt"
	"net/netip"
)

type Rendezvous struct {
	MyAddresses []netip.AddrPort
}

func (r *Rendezvous) MarshalSessionMessage() []byte {
	// TODO
	panic("implement me")
}

func (r *Rendezvous) Debug() string {
	return fmt.Sprintf("rendezvous addresses=%#v", r.MyAddresses)
}
