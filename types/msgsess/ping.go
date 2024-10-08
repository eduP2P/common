package msgsess

import (
	crand "crypto/rand"
	"fmt"
	"github.com/edup2p/common/types/key"
	"slices"
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

	// TODO implement padding
	Padding int
}

func (p *Ping) MarshalSessionMessage() []byte {
	return slices.Concat([]byte{byte(v1), byte(PingMessage)}, p.TxID[:], p.NodeKey[:])
}

func (p *Ping) Debug() string {
	return fmt.Sprintf("ping tx=%x padding=%v", p.TxID, p.Padding)
}
