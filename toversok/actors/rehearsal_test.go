package actors

import (
	"context"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type MockActor struct {
	ctx context.Context

	s *Stage

	run    func()
	inbox  func() chan<- ActorMessage
	cancel func()
	close  func()
}

func (m *MockActor) Run() {
	m.run()
}

func (m *MockActor) Inbox() chan<- ActorMessage {
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

	push func(frame DirectedPeerFrame)
}

func (m *MockDirectRouter) Push(frame DirectedPeerFrame) {
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

	push func(frame DirectedPeerFrame)
}

func (m *MockRelayRouter) Push(frame DirectedPeerFrame) {
	m.push(frame)
}
