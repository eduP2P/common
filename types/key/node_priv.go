package key

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/edup2p/common/types"
	"go4.org/mem"
	"golang.org/x/crypto/curve25519"
	"strings"
)

type NodePrivate struct {
	_   types.Incomparable
	key NakedKey
}

// NewNode creates and returns a new node private key.
func NewNode() NodePrivate {
	var ret NodePrivate
	rand(ret.key[:])
	clamp25519Private(ret.key[:])
	return ret
}

func NodePrivateFrom(key NakedKey) NodePrivate {
	return NodePrivate{key: key}
}

// Equal reports whether k and other are the same key.
func (n NodePrivate) Equal(other NodePrivate) bool {
	return subtle.ConstantTimeCompare(n.key[:], other.key[:]) == 1
}

// IsZero reports whether k is the zero value.
func (n NodePrivate) IsZero() bool {
	return n.Equal(NodePrivate{})
}

// OpenFrom opens the NaCl box ciphertext, which must be a value
// created by SealTo, and returns the inner cleartext if ciphertext is
// a valid box from p to k.
func (n NodePrivate) OpenFrom(p NodePublic, ciphertext []byte) (cleartext []byte, ok bool) {
	if n.IsZero() || p.IsZero() {
		panic("can't open with zero keys")
	}
	return openFrom(n.key, NakedKey(p), ciphertext)
}

func (n NodePrivate) OpenFromControl(p ControlPublic, ciphertext []byte) (cleartext []byte, ok bool) {
	if n.IsZero() || p.IsZero() {
		panic("can't open with zero keys")
	}
	return openFrom(n.key, NakedKey(p), ciphertext)
}

// SealTo wraps cleartext into a NaCl box (see
// golang.org/x/crypto/nacl) to p, authenticated from k, using a
// random nonce.
//
// The returned ciphertext is a 24-byte nonce concatenated with the
// box value.
func (n NodePrivate) SealTo(p NodePublic, cleartext []byte) (ciphertext []byte) {
	if n.IsZero() || p.IsZero() {
		panic("can't seal with zero keys")
	}
	return sealTo(n.key, NakedKey(p), cleartext)
}

func (n NodePrivate) SealToControl(p ControlPublic, cleartext []byte) (ciphertext []byte) {
	if n.IsZero() || p.IsZero() {
		panic("can't seal with zero keys")
	}
	return sealTo(n.key, NakedKey(p), cleartext)
}

func (n NodePrivate) Public() NodePublic {
	if n.IsZero() {
		panic("can't take the public key of a zero NodePrivate")
	}

	var ret NodePublic
	curve25519.ScalarBaseMult((*[32]byte)(&ret), (*[32]byte)(&n.key))
	return ret
}

// AppendText implements encoding.TextAppender.
func (n NodePrivate) AppendText(b []byte) ([]byte, error) {
	return appendHexKey(b, nodePrivateHexPrefix, n.key[:]), nil
}

// MarshalText implements encoding.TextMarshaler.
func (n NodePrivate) MarshalText() ([]byte, error) {
	return n.AppendText(nil)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *NodePrivate) UnmarshalText(b []byte) error {
	return parseHex(n.key[:], mem.B(b), mem.S(nodePrivateHexPrefix))
}

// UnveilPrivate is a function to get a NakedKey from a NodePrivate.
//
// //Deprecated: nobody should be using this
func UnveilPrivate(private NodePrivate) NakedKey {
	return private.key
}

func UnmarshalPrivate(s string) (*NodePrivate, error) {
	if !strings.HasSuffix(s, "\"") && !strings.HasPrefix(s, "\"") {
		s = fmt.Sprintf("\"%s\"", s)
	}

	pub := new(NodePrivate)

	if err := json.Unmarshal([]byte(s), pub); err != nil {
		return nil, err
	}

	return pub, nil
}

func (n NodePrivate) Marshal() string {
	b, _ := json.Marshal(n)
	return string(b)
}
