package key

import (
	"crypto/subtle"
	"github.com/edup2p/common/types"
	"golang.org/x/crypto/nacl/box"
)

type SessionShared struct {
	_   types.Incomparable
	key NakedKey
}

// Equal reports whether k and other are the same key.
func (k SessionShared) Equal(other SessionShared) bool {
	return subtle.ConstantTimeCompare(k.key[:], other.key[:]) == 1
}

func (k SessionShared) IsZero() bool {
	return k.Equal(SessionShared{})
}

// Seal wraps cleartext into a NaCl box (see
// golang.org/x/crypto/nacl), using k as the shared secret and a
// random nonce.
func (k SessionShared) Seal(cleartext []byte) (ciphertext []byte) {
	if k.IsZero() {
		panic("can't seal with zero key")
	}
	var nonce [24]byte
	rand(nonce[:])
	return box.SealAfterPrecomputation(nonce[:], cleartext, &nonce, (*[32]byte)(&k.key))
}

// Open opens the NaCl box ciphertext, which must be a value created
// by Seal, and returns the inner cleartext if ciphertext is a valid
// box using shared secret k.
func (k SessionShared) Open(ciphertext []byte) (cleartext []byte, ok bool) {
	if k.IsZero() {
		panic("can't open with zero key")
	}
	if len(ciphertext) < 24 {
		return nil, false
	}
	nonce := (*[24]byte)(ciphertext)
	return box.OpenAfterPrecomputation(nil, ciphertext[24:], nonce, (*[32]byte)(&k.key))
}
