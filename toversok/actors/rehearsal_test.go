package actors

import (
	"context"
	"net/netip"
	"time"

	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
)

type MockActor struct {
	ctx context.Context

	s *Stage

	run    func()
	inbox  func() chan<- msgactor.ActorMessage
	cancel func()
	close  func()
}

func (m *MockActor) Run() {
	m.run()
}

func (m *MockActor) Inbox() chan<- msgactor.ActorMessage {
	return m.inbox()
}

func (m *MockActor) Cancel() {
	m.cancel()
}

func (m *MockActor) Close() {
	m.close()
}

type MockDirectManager struct {
	*MockActor

	writeTo func(pkt []byte, addr netip.AddrPort)
}

func (m *MockDirectManager) WriteTo(pkt []byte, addr netip.AddrPort) {
	m.writeTo(pkt, addr)
}

type MockDirectRouter struct {
	*MockActor

	push func(frame ifaces.DirectedPeerFrame)
}

func (m *MockDirectRouter) Push(frame ifaces.DirectedPeerFrame) {
	m.push(frame)
}

type MockRelayManager struct {
	*MockActor

	writeTo func(pkt []byte, relay int64, dst key.NodePublic)
}

func (m *MockRelayManager) WriteTo(pkt []byte, relay int64, dst key.NodePublic) {
	m.writeTo(pkt, relay, dst)
}

type MockRelayRouter struct {
	*MockActor

	push func(frame ifaces.DirectedPeerFrame)
}

func (m *MockRelayRouter) Push(frame ifaces.DirectedPeerFrame) {
	m.push(frame)
}

type MockUDPConn struct {
	writeCh chan []byte

	setReadDeadline     func(t time.Time) error
	readFromUDPAddrPort func(b []byte) (n int, addr netip.AddrPort, err error)

	write              func(b []byte) (int, error)
	writeToUDPAddrPort func(b []byte, addr netip.AddrPort) (int, error)

	close func() error
}

func (m *MockUDPConn) SetReadDeadline(t time.Time) error {
	return m.setReadDeadline(t)
}

func (m *MockUDPConn) ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error) {
	return m.readFromUDPAddrPort(b)
}

func (m *MockUDPConn) Write(b []byte) (n int, err error) {
	m.writeCh <- b
	return m.write(b)
}

func (m *MockUDPConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	m.writeCh <- b
	return m.writeToUDPAddrPort(b, addr)
}

func (m *MockUDPConn) Close() error {
	return m.close()
}
