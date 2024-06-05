package key

import (
	"encoding/hex"
	"fmt"
)

const Len = 32

// NakedKey is the 32-byte underlying key.
//
// Only ever used for public interfaces, very dangerous to use directly, due to the security implications.
type NakedKey [Len]byte

func (n NakedKey) Debug() string {
	return fmt.Sprintf("%x", n)
}

func (n NakedKey) HexString() string {
	return hex.EncodeToString(n[:])
}

// IsZero reports whether k is the zero value.
func (n NakedKey) IsZero() bool {
	return n == NakedKey{}
}
