package bin

import (
	"bufio"
	"encoding/binary"
	"net/netip"
	"slices"
)

// WriteUint32 writes an uint32 in big-endian order to the writer
func WriteUint32(writer *bufio.Writer, v uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	// Writing a byte at a time is a bit silly,
	// but it causes b not to escape,
	// which more than pays for the silliness.
	for _, c := range &b {
		err := writer.WriteByte(c)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadUint32 reads an uint32 in big-endian order to the reader
func ReadUint32(reader *bufio.Reader) (uint32, error) {
	var b [4]byte
	// Reading a byte at a time is a bit silly,
	// but it causes b not to escape,
	// which more than pays for the silliness.
	for i := range &b {
		c, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		b[i] = c
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

func ParseAddrPort(b [18]byte) netip.AddrPort {
	addr := netip.AddrFrom16([16]byte(b[:16])).Unmap()

	port := binary.BigEndian.Uint16(b[16:])

	return netip.AddrPortFrom(addr, port)
}

func PutAddrPort(ap netip.AddrPort) []byte {
	port := make([]byte, 2)

	as16 := ap.Addr().As16()
	binary.BigEndian.PutUint16(port, ap.Port())

	return slices.Concat(as16[:], port[:])
}
