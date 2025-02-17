package key

import (
	"crypto/subtle"

	"github.com/edup2p/common/types"
	"go4.org/mem"
	"golang.org/x/crypto/curve25519"
)

type ControlPrivate struct {
	_   types.Incomparable
	key NakedKey
}

func (c ControlPrivate) Equal(other ControlPrivate) bool {
	return subtle.ConstantTimeCompare(c.key[:], other.key[:]) == 1
}

// IsZero reports whether k is the zero value.
func (c ControlPrivate) IsZero() bool {
	return c.Equal(ControlPrivate{})
}

func (c ControlPrivate) Public() ControlPublic {
	if c.IsZero() {
		panic("can't take the public key of a zero ControlPrivate")
	}

	var ret ControlPublic
	curve25519.ScalarBaseMult((*[32]byte)(&ret), (*[32]byte)(&c.key))
	return ret
}

func (c ControlPrivate) OpenFromNode(p NodePublic, ciphertext []byte) (cleartext []byte, ok bool) {
	if c.IsZero() || p.IsZero() {
		panic("can't open with zero keys")
	}
	return openFrom(c.key, NakedKey(p), ciphertext)
}

func (c ControlPrivate) OpenFromSession(p SessionPublic, ciphertext []byte) (cleartext []byte, ok bool) {
	if c.IsZero() || p.IsZero() {
		panic("can't open with zero keys")
	}
	return openFrom(c.key, NakedKey(p), ciphertext)
}

func (c ControlPrivate) SealToNode(p NodePublic, cleartext []byte) (ciphertext []byte) {
	if c.IsZero() || p.IsZero() {
		panic("can't seal with zero keys")
	}
	return sealTo(c.key, NakedKey(p), cleartext)
}

// AppendText implements encoding.TextAppender.
func (c ControlPrivate) AppendText(b []byte) ([]byte, error) {
	return appendHexKey(b, controlPrivateHexPrefix, c.key[:]), nil
}

// MarshalText implements encoding.TextMarshaler.
func (c ControlPrivate) MarshalText() ([]byte, error) {
	return c.AppendText(nil)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (c *ControlPrivate) UnmarshalText(b []byte) error {
	return parseHex(c.key[:], mem.B(b), mem.S(controlPrivateHexPrefix))
}

// NewControlPrivate creates and returns a new control private key.
func NewControlPrivate() ControlPrivate {
	var ret ControlPrivate
	rand(ret.key[:])
	clamp25519Private(ret.key[:])
	return ret
}
