package stun

import (
	"encoding/binary"
)

const (
	attrNumSoftware      = 0x8022
	attrNumFingerprint   = 0x8028
	attrMappedAddress    = 0x0001
	attrXorMappedAddress = 0x0020
	// This alternative attribute type is not
	// mentioned in the RFC, but the shift into
	// the "comprehension-optional" range seems
	// like an easy mistake for a server to make.
	// And servers appear to send it.
	attrXorMappedAddressAlt = 0x8020

	headerLen      = 20
	bindingRequest = "\x00\x01"

	// STUN magic cookie as defined by RFC 5389:
	// "The magic cookie field MUST contain the fixed value 0x2112A442 in network byte order."
	magicCookie = "\x21\x12\xa4\x42"

	lenFingerprint = 8 // 2+byte header + 2-byte length + 4-byte crc32

	thisSoftware = "toversok" // 8 bytes
)

// Is reports whether b is a STUN message.
func Is(b []byte) bool {
	return len(b) >= headerLen &&
		b[0]&0b11000000 == 0 && // top two bits must be zero
		string(b[4:8]) == magicCookie
}

// ParseBindingRequest parses a STUN binding request.
//
// It returns an error unless it advertises that it came from
// Tailscale.
func ParseBindingRequest(b []byte) (TxID, error) {
	if !Is(b) {
		return TxID{}, ErrNotSTUN
	}
	if string(b[:len(bindingRequest)]) != bindingRequest {
		return TxID{}, ErrNotBindingRequest
	}
	var txID TxID
	copy(txID[:], b[8:8+len(txID)])
	var softwareOK bool
	var lastAttr uint16
	var gotFP uint32
	if err := foreachAttr(b[headerLen:], func(attrType uint16, a []byte) error {
		lastAttr = attrType
		if attrType == attrNumSoftware && string(a) == thisSoftware {
			softwareOK = true
		}
		if attrType == attrNumFingerprint && len(a) == 4 {
			gotFP = binary.BigEndian.Uint32(a)
		}
		return nil
	}); err != nil {
		return TxID{}, err
	}
	if !softwareOK {
		return TxID{}, ErrWrongSoftware
	}
	if lastAttr != attrNumFingerprint {
		return TxID{}, ErrNoFingerprint
	}
	wantFP := fingerPrint(b[:len(b)-lenFingerprint])
	if gotFP != wantFP {
		return TxID{}, ErrWrongFingerprint
	}
	return txID, nil
}
