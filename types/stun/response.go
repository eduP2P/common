package stun

import (
	"encoding/binary"
	"net/netip"
)

// Response generates a binding response.
func Response(txID TxID, addrPort netip.AddrPort) []byte {
	addr := addrPort.Addr()

	var fam byte
	switch {
	case addr.Is4():
		fam = 1
	case addr.Is6():
		fam = 2
	default:
		return nil
	}
	attrsLen := 8 + addr.BitLen()/8
	b := make([]byte, 0, headerLen+attrsLen)

	// Header
	b = append(b, 0x01, 0x01) // success
	b = appendU16(b, uint16(attrsLen))
	b = append(b, magicCookie...)
	b = append(b, txID[:]...)

	// Attributes (well, one)
	b = appendU16(b, attrXorMappedAddress)
	b = appendU16(b, uint16(4+addr.BitLen()/8))
	b = append(b,
		0, // unused byte
		fam)
	b = appendU16(b, addrPort.Port()^0x2112) // first half of magicCookie
	ipa := addr.As16()
	for i, o := range ipa[16-addr.BitLen()/8:] {
		if i < 4 {
			b = append(b, o^magicCookie[i])
		} else {
			b = append(b, o^txID[i-len(magicCookie)])
		}
	}
	return b
}

// ParseResponse parses a successful binding response STUN packet.
// The IP address is extracted from the XOR-MAPPED-ADDRESS attribute.
func ParseResponse(b []byte) (tID TxID, addr netip.AddrPort, err error) {
	if !Is(b) {
		return tID, netip.AddrPort{}, ErrNotSTUN
	}
	copy(tID[:], b[8:8+len(tID)])
	if b[0] != 0x01 || b[1] != 0x01 {
		return tID, netip.AddrPort{}, ErrNotSuccessResponse
	}
	attrsLen := int(binary.BigEndian.Uint16(b[2:4]))
	b = b[headerLen:] // remove STUN header
	if attrsLen > len(b) {
		return tID, netip.AddrPort{}, ErrMalformedAttrs
	} else if len(b) > attrsLen {
		b = b[:attrsLen] // trim trailing packet bytes
	}

	var fallbackAddr netip.AddrPort

	// Read through the attributes.
	// The the addr+port reported by XOR-MAPPED-ADDRESS
	// as the canonical value. If the attribute is not
	// present but the STUN server responds with
	// MAPPED-ADDRESS we fall back to it.
	if err := foreachAttr(b, func(attrType uint16, attr []byte) error {
		switch attrType {
		case attrXorMappedAddress, attrXorMappedAddressAlt:
			ipSlice, port, err := xorMappedAddress(tID, attr)
			if err != nil {
				return err
			}
			if ip, ok := netip.AddrFromSlice(ipSlice); ok {
				addr = netip.AddrPortFrom(ip.Unmap(), port)
			}
		case attrMappedAddress:
			ipSlice, port, err := mappedAddress(attr)
			if err != nil {
				return ErrMalformedAttrs
			}
			if ip, ok := netip.AddrFromSlice(ipSlice); ok {
				fallbackAddr = netip.AddrPortFrom(ip.Unmap(), port)
			}
		}
		return nil
	}); err != nil {
		return TxID{}, netip.AddrPort{}, err
	}

	if addr.IsValid() {
		return tID, addr, nil
	}
	if fallbackAddr.IsValid() {
		return tID, fallbackAddr, nil
	}
	return tID, netip.AddrPort{}, ErrMalformedAttrs
}
