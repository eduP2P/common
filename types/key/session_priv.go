package key

import (
	"crypto/subtle"
	"github.com/edup2p/common/types"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

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
func (s SessionPrivate) IsZero() bool {
	return s.Equal(SessionPrivate{})
}

// Equal reports whether k and other are the same key.
func (s SessionPrivate) Equal(other SessionPrivate) bool {
	return subtle.ConstantTimeCompare(s.key[:], other.key[:]) == 1
}

// Public returns the SessionPublic for k.
// Panics if SessionPrivate is zero.
func (s SessionPrivate) Public() SessionPublic {
	if s.IsZero() {
		panic("can't take the public key of a zero SessionPrivate")
	}
	var ret SessionPublic
	curve25519.ScalarBaseMult((*[32]byte)(&ret), (*[32]byte)(&s.key))
	return ret
}

// Shared returns the SessionShared for communication between k and p.
func (s SessionPrivate) Shared(p SessionPublic) SessionShared {
	if s.IsZero() || p.IsZero() {
		panic("can't compute shared secret with zero keys")
	}
	var ret SessionShared
	box.Precompute((*[32]byte)(&ret.key), (*[32]byte)(&p), (*[32]byte)(&s.key))
	return ret
}

func (s SessionPrivate) SealToControl(p ControlPublic, cleartext []byte) (ciphertext []byte) {
	if s.IsZero() || p.IsZero() {
		panic("can't seal with zero keys")
	}
	return sealTo(s.key, NakedKey(p), cleartext)
}
