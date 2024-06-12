package types

// Contains types and structs pertaining to wrapping and identifying Conn-like types.

import (
	"io"
	"net/netip"
	"time"
)

// UDPConn interface for actors.Stage to more easily deal with.
type UDPConn interface {
	SetReadDeadline(t time.Time) error

	ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error)

	Write(b []byte) (int, error)
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)

	Close() error
}

type UDPConnCloseCatcher struct {
	UDPConn

	Closed bool
}

func (c *UDPConnCloseCatcher) Close() error {
	c.Closed = true

	return c.UDPConn.Close()
}

// TODO add test dummy type

type MetaConn interface {
	io.Closer
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}
