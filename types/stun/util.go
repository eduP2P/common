package stun

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"net"
)

func foreachAttr(b []byte, fn func(attrType uint16, a []byte) error) error {
	for len(b) > 0 {
		if len(b) < 4 {
			return ErrMalformedAttrs
		}
		attrType := binary.BigEndian.Uint16(b[:2])
		attrLen := int(binary.BigEndian.Uint16(b[2:4]))
		attrLenWithPad := (attrLen + 3) &^ 3
		b = b[4:]
		if attrLenWithPad > len(b) {
			return ErrMalformedAttrs
		}
		if err := fn(attrType, b[:attrLen]); err != nil {
			return err
		}
		b = b[attrLenWithPad:]
	}
	return nil
}

func fingerPrint(b []byte) uint32 { return crc32.ChecksumIEEE(b) ^ 0x5354554e }

func appendU16(b []byte, v uint16) []byte {
	return append(b, byte(v>>8), byte(v))
}

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func xorMappedAddress(tID TxID, b []byte) (addr []byte, port uint16, err error) {
	// XOR-MAPPED-ADDRESS attribute, RFC5389 Section 15.2
	if len(b) < 4 {
		return nil, 0, ErrMalformedAttrs
	}
	xorPort := binary.BigEndian.Uint16(b[2:4])
	addrField := b[4:]
	port = xorPort ^ 0x2112 // first half of magicCookie

	addrLen := familyAddrLen(b[1])
	if addrLen == 0 {
		return nil, 0, ErrMalformedAttrs
	}
	if len(addrField) < addrLen {
		return nil, 0, ErrMalformedAttrs
	}
	xorAddr := addrField[:addrLen]
	addr = make([]byte, addrLen)
	for i := range xorAddr {
		if i < len(magicCookie) {
			addr[i] = xorAddr[i] ^ magicCookie[i]
		} else {
			addr[i] = xorAddr[i] ^ tID[i-len(magicCookie)]
		}
	}
	return addr, port, nil
}

func familyAddrLen(fam byte) int {
	switch fam {
	case 0x01: // IPv4
		return net.IPv4len
	case 0x02: // IPv6
		return net.IPv6len
	default:
		return 0
	}
}

func mappedAddress(b []byte) (addr []byte, port uint16, err error) {
	if len(b) < 4 {
		return nil, 0, ErrMalformedAttrs
	}
	port = uint16(b[2])<<8 | uint16(b[3])
	addrField := b[4:]
	addrLen := familyAddrLen(b[1])
	if addrLen == 0 {
		return nil, 0, ErrMalformedAttrs
	}
	if len(addrField) < addrLen {
		return nil, 0, ErrMalformedAttrs
	}
	return bytes.Clone(addrField[:addrLen]), port, nil
}
