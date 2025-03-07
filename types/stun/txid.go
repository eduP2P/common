package stun

import crand "crypto/rand"

// TxID is a transaction ID.
type TxID [12]byte

// NewTxID returns a new random TxID.
func NewTxID() TxID {
	var tx TxID
	if _, err := crand.Read(tx[:]); err != nil {
		// We expect the randomizer to be available here
		panic(err)
	}
	return tx
}
