package stun

import crand "crypto/rand"

// TxID is a transaction ID.
type TxID [12]byte

// NewTxID returns a new random TxID.
func NewTxID() TxID {
	var tx TxID
	if _, err := crand.Read(tx[:]); err != nil {
		panic(err)
	}
	return tx
}
