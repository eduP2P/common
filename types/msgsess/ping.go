package msgsess

import (
	crand "crypto/rand"
	"fmt"
	"slices"

	"github.com/edup2p/common/types/key"
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

func (p *Ping) Marshal() []byte {
	// TODO add padding
	return slices.Concat([]byte{byte(v1), byte(PingMessage)}, p.TxID[:], p.NodeKey[:])
}

func (p *Ping) Parse(b []byte) error {
	if len(b) < key.Len+12 {
		return errTooSmall
	}

	p.TxID = [12]byte(b[:12])
	b = b[12:]
	p.NodeKey = key.NodePublic(b[:key.Len])

	// TODO count remaining bytes as padding

	return nil
}

func (p *Ping) Debug() string {
	return fmt.Sprintf("ping tx=%x nodekey=%s padding=%v", p.TxID, p.NodeKey.Debug(), p.Padding)
}
