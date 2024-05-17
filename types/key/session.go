package key

import (
	"crypto/subtle"
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

type SessionPublic NakedKey

func (n SessionPublic) Debug() string {
	return fmt.Sprintf("%x", n)
}

// MakeSessionPublic parses a 32-byte raw value as a SessionPublic.
//
// This should be used only when deserializing a SessionPublic from a
// binary protocol.
func MakeSessionPublic(raw [32]byte) SessionPublic {
	return raw
}

// IsZero reports whether k is the zero value.
func (k SessionPublic) IsZero() bool {
	return k == SessionPublic{}
}

func (k SessionPublic) ToByteSlice() []byte {
	return k[:]
}

type SessionPrivate struct {
	_   types.Incomparable
	key NakedKey
}

// NewSession creates and returns a new session private key.
func NewSession() SessionPrivate {
	var ret SessionPrivate
	rand(ret.key[:])
	// Key used for nacl seal/open, so needs to be clamped.
	clamp25519Private(ret.key[:])
	return ret
}

// DevNewSessionFromPrivate creates a new SessionPrivate by copying a NodePrivate
//
// Deprecated: Must only be used for development.
func DevNewSessionFromPrivate(priv NodePrivate) SessionPrivate {
	return SessionPrivate{key: priv.key}
}

// IsZero reports whether k is the zero value.
func (k SessionPrivate) IsZero() bool {
	return k.Equal(SessionPrivate{})
}

// Equal reports whether k and other are the same key.
func (k SessionPrivate) Equal(other SessionPrivate) bool {
	return subtle.ConstantTimeCompare(k.key[:], other.key[:]) == 1
}

// Public returns the SessionPublic for k.
// Panics if SessionPrivate is zero.
func (k SessionPrivate) Public() SessionPublic {
	if k.IsZero() {
		panic("can't take the public key of a zero SessionPrivate")
	}
	var ret SessionPublic
	curve25519.ScalarBaseMult((*[32]byte)(&ret), (*[32]byte)(&k.key))
	return ret
}

// Shared returns the SessionShared for communication between k and p.
func (k SessionPrivate) Shared(p SessionPublic) SessionShared {
	if k.IsZero() || p.IsZero() {
		panic("can't compute shared secret with zero keys")
	}
	var ret SessionShared
	box.Precompute((*[32]byte)(&ret.key), (*[32]byte)(&p), (*[32]byte)(&k.key))
	return ret
}

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
