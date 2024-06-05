package key

import (
	"encoding/hex"
	"fmt"
	"go4.org/mem"
)

type SessionPublic NakedKey

func (s SessionPublic) Debug() string {
	return fmt.Sprintf("%x", s)
}

func (s SessionPublic) HexString() string {
	return hex.EncodeToString(s[:])
}

// IsZero reports whether k is the zero value.
func (s SessionPublic) IsZero() bool {
	return s == SessionPublic{}
}

func (s SessionPublic) AppendText(b []byte) ([]byte, error) {
	return appendHexKey(b, sessPublicHexPrefix, s[:]), nil
}

func (s SessionPublic) MarshalText() (text []byte, err error) {
	return s.AppendText(nil)
}

func (s *SessionPublic) UnmarshalText(text []byte) error {
	return parseHex(s[:], mem.B(text), mem.S(sessPublicHexPrefix))
}

// MakeSessionPublic parses a 32-byte raw value as a SessionPublic.
//
// This should be used only when deserializing a SessionPublic from a
// binary protocol.
func MakeSessionPublic(raw [32]byte) SessionPublic {
	return raw
}

func (s SessionPublic) ToByteSlice() []byte {
	return s[:]
}
