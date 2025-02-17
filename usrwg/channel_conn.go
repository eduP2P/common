package usrwg

import (
	"context"
	"net"
	"net/netip"
	"time"
)

// ChannelConn is a types.UDPConn based on two internal channels.
//
// On the "frontend" (types.UDPConn) it supports SetReadDeadLine, as normal.
type ChannelConn struct {
	// Packets to be read by the frontend
	incoming chan []byte

	// Packets written by the frontend
	outgoing chan []byte

	currentReadDeadline time.Time
}

const ChannelConnBufferSize = 16

func makeChannelConn() *ChannelConn {
	return &ChannelConn{
		incoming:            make(chan []byte, ChannelConnBufferSize),
		outgoing:            make(chan []byte, ChannelConnBufferSize),
		currentReadDeadline: time.Time{},
	}
}

/// The "Frontend"; types.UDPConn

func (cc *ChannelConn) SetReadDeadline(t time.Time) error {
	cc.currentReadDeadline = t

	return nil
}

func (cc *ChannelConn) ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error) {
	var val []byte

	if cc.currentReadDeadline == (time.Time{}) {
		// Block until value received.
		val = <-cc.incoming
	} else {
		// Block until value or timeout.
		select {
		case val = <-cc.incoming:
		case <-time.After(time.Until(cc.currentReadDeadline)):
			return 0, netip.AddrPort{}, context.DeadlineExceeded
		}
	}

	return copy(b, val), netip.AddrPort{}, nil
}

func (cc *ChannelConn) Write(b []byte) (int, error) {
	// We don't have SetWriteDeadline, so we just block.

	cc.outgoing <- b

	return len(b), nil
}

func (cc *ChannelConn) WriteToUDPAddrPort(_ []byte, _ netip.AddrPort) (int, error) {
	return 0, net.ErrWriteToConnected
}

func (cc *ChannelConn) Close() error {
	// TODO boolean to check if is already closed?

	close(cc.outgoing)
	close(cc.incoming)

	return nil
}

/// The "Backend"

// Tries to read a packet from the outgoing channel, returns nil if there's none waiting.
func (cc *ChannelConn) tryGetOut() (pkt []byte) {
	select {
	case pkt = <-cc.outgoing:
	default:
	}

	return
}

// Reads a packet from the outgoing channel, and waits.
//
// Returns nil on timeout.
func (cc *ChannelConn) getOut(d time.Duration) (pkt []byte) { // nolint:unused
	select {
	case pkt = <-cc.outgoing:
	case <-time.After(d):
	}

	return
}

// Puts a packet in the incoming channel, and waits.
//
// Will return false on timeout.
func (cc *ChannelConn) putIn(pkt []byte, d time.Duration) (ok bool) {

	select {
	case cc.incoming <- pkt:
		ok = true
	case <-time.After(d):
		ok = false
	}

	return
}
