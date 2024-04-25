package relay

import (
	"bufio"
	"encoding/binary"
)

// writeUint32 writes an uint32 in big-endian order to the writer
func writeUint32(writer *bufio.Writer, v uint32) error {
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

// readUint32 reads an uint32 in big-endian order to the reader
func readUint32(reader *bufio.Reader) (uint32, error) {
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
