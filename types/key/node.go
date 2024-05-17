package key

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"go4.org/mem"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
	"strings"
)

type NodePublic NakedKey

func (n NodePublic) Debug() string {
	return fmt.Sprintf("%x", n)
}

func (n NodePublic) HexString() string {
	return hex.EncodeToString(n[:])
}

// IsZero reports whether k is the zero value.
func (n NodePublic) IsZero() bool {
	return n == NodePublic{}
}

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
	if len(ciphertext) < 24 {
		return nil, false
	}
	nonce := (*[24]byte)(ciphertext)
	return box.Open(nil, ciphertext[len(nonce):], nonce, (*[32]byte)(&p), (*[32]byte)(&n.key))
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
	var nonce [24]byte
	rand(nonce[:])
	return box.Seal(nonce[:], cleartext, &nonce, (*[32]byte)(&p), (*[32]byte)(&n.key))
}

func (n NodePrivate) Public() NodePublic {
	if n.IsZero() {
		panic("can't take the public key of a zero NodePrivate")
	}

	var ret NodePublic
	curve25519.ScalarBaseMult((*[32]byte)(&ret), (*[32]byte)(&n.key))
	return ret
}

const (
	// nodePrivateHexPrefix is the prefix used to identify a
	// hex-encoded node private key.
	//
	// This prefix name is a little unfortunate, in that it comes from
	// WireGuard's own key types, and we've used it for both key types
	// we persist to disk (machine and node keys). But we're stuck
	// with it for now, barring another round of tricky migration.
	nodePrivateHexPrefix = "privkey:"

	// nodePublicHexPrefix is the prefix used to identify a
	// hex-encoded node public key.
	//
	// This prefix is used in the control protocol, so cannot be
	// changed.
	nodePublicHexPrefix = "pubkey:"
)

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

// AppendText implements encoding.TextAppender. It appends a typed prefix
// followed by hex encoded represtation of k to b.
func (k NodePublic) AppendText(b []byte) ([]byte, error) {
	return appendHexKey(b, nodePublicHexPrefix, k[:]), nil
}

// MarshalText implements encoding.TextMarshaler. It returns a typed prefix
// followed by a hex encoded representation of k.
func (k NodePublic) MarshalText() ([]byte, error) {
	return k.AppendText(nil)
}

// UnmarshalText implements encoding.TextUnmarshaler. It expects a typed prefix
// followed by a hex encoded representation of k.
func (k *NodePublic) UnmarshalText(b []byte) error {
	return parseHex(k[:], mem.B(b), mem.S(nodePublicHexPrefix))
}

// UnveilPrivate is a function to get a NakedKey from a NodePrivate.
//
// Deprecated: nobody should be using this
func UnveilPrivate(private NodePrivate) NakedKey {
	return private.key
}

func UnmarshalPublic(s string) (*NodePublic, error) {
	if !strings.HasSuffix(s, "\"") && !strings.HasPrefix(s, "\"") {
		s = fmt.Sprintf("\"%s\"", s)
	}

	pub := new(NodePublic)

	if err := json.Unmarshal([]byte(s), pub); err != nil {
		return nil, err
	} else {
		return pub, nil
	}
}

func (k NodePublic) Marshal() string {
	b, _ := json.Marshal(k)
	return string(b)
}
func UnmarshalPrivate(s string) (*NodePrivate, error) {
	if !strings.HasSuffix(s, "\"") && !strings.HasPrefix(s, "\"") {
		s = fmt.Sprintf("\"%s\"", s)
	}

	pub := new(NodePrivate)

	if err := json.Unmarshal([]byte(s), pub); err != nil {
		return nil, err
	} else {
		return pub, nil
	}
}

func (k NodePrivate) Marshal() string {
	b, _ := json.Marshal(k)
	return string(b)
}
