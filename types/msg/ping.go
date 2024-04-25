package msg

import (
	crand "crypto/rand"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/key"
)

type TxID [12]byte

func NewTxID() TxID {
	var tx TxID
	if _, err := crand.Read(tx[:]); err != nil {
		panic(err)
	}
	return tx
}

type Ping struct {
	TxID TxID

	// Allegedly the sender's nodekey address
	NodeKey key.NodePublic

	Padding int
}

func (p *Ping) MarshalSessionMessage() []byte {
	// TODO
	panic("implement me")
}

func (p *Ping) Debug() string {
	return fmt.Sprintf("ping tx=%x padding=%v", p.TxID, p.Padding)
}
