package key

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go4.org/mem"
	"strings"
)

type NodePublic NakedKey

func (n NodePublic) Debug() string {
	return fmt.Sprintf("%x", n)
}

func (n NodePublic) HexString() string {
	return hex.EncodeToString(n[:])
}

func (n NodePublic) IsZero() bool {
	return n == NodePublic{}
}

// AppendText implements encoding.TextAppender. It appends a typed prefix
// followed by hex encoded represtation of k to b.
func (n NodePublic) AppendText(b []byte) ([]byte, error) {
	return appendHexKey(b, nodePublicHexPrefix, n[:]), nil
}

// MarshalText implements encoding.TextMarshaler. It returns a typed prefix
// followed by a hex encoded representation of k.
func (n NodePublic) MarshalText() ([]byte, error) {
	return n.AppendText(nil)
}

// UnmarshalText implements encoding.TextUnmarshaler. It expects a typed prefix
// followed by a hex encoded representation of k.
func (n *NodePublic) UnmarshalText(b []byte) error {
	return parseHex(n[:], mem.B(b), mem.S(nodePublicHexPrefix))
}

func UnmarshalPublic(s string) (*NodePublic, error) {
	if !strings.HasSuffix(s, "\"") && !strings.HasPrefix(s, "\"") {
		s = fmt.Sprintf("\"%s\"", s)
	}

	pub := new(NodePublic)

	if err := json.Unmarshal([]byte(s), pub); err != nil {
		return nil, err
	}

	return pub, nil
}

func (n NodePublic) Marshal() string {
	b, _ := json.Marshal(n)
	return string(b)
}
