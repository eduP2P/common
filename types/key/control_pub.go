package key

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go4.org/mem"
	"strings"
)

type ControlPublic NakedKey

func (c ControlPublic) IsZero() bool {
	return c == ControlPublic{}
}

func (c ControlPublic) Debug() string {
	return fmt.Sprintf("%x", c)
}

func (c ControlPublic) HexString() string {
	return hex.EncodeToString(c[:])
}

func (c ControlPublic) AppendText(b []byte) ([]byte, error) {
	return appendHexKey(b, controlPublicHexPrefix, c[:]), nil
}

func (c ControlPublic) MarshalText() (text []byte, err error) {
	return c.AppendText(nil)
}

func (c *ControlPublic) UnmarshalText(text []byte) error {
	return parseHex(c[:], mem.B(text), mem.S(controlPublicHexPrefix))
}

func UnmarshalControlPublic(s string) (*ControlPublic, error) {
	if !strings.HasSuffix(s, "\"") && !strings.HasPrefix(s, "\"") {
		s = fmt.Sprintf("\"%s\"", s)
	}

	pub := new(ControlPublic)

	if err := json.Unmarshal([]byte(s), pub); err != nil {
		return nil, err
	} else {
		return pub, nil
	}
}
