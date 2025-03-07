package msgsess

import (
	"fmt"
	"slices"
)

type SideBandDataType byte

const (
	MDNSType SideBandDataType = iota
)

type SideBandData struct {
	Type SideBandDataType
	Data []byte
}

func (s *SideBandData) Marshal() []byte {
	b := make([]byte, 0)

	b = append(b, byte(s.Type))
	b = append(b, s.Data...)

	return slices.Concat([]byte{byte(v1), byte(SideBandDataMessage)}, b)
}

func (s *SideBandData) Parse(b []byte) error {
	if len(b) < 1 {
		return errTooSmall
	}

	s.Type = SideBandDataType(b[0])
	s.Data = b[1:]

	return nil
}

func (s *SideBandData) Debug() string {
	return fmt.Sprintf("sidebanddata type=%d data=%x", s.Type, s.Data)
}
