package actors

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/relay"
	"github.com/edup2p/common/types/stun"
	"github.com/stretchr/testify/assert"
)

// Mock Control Server used in this test
type MockControl struct {
	endpoints []netip.AddrPort

	controlKey func() key.ControlPublic

	ipv4   func() netip.Prefix
	ipv6   func() netip.Prefix
	expiry func() time.Time

	updateEndpoints func([]netip.AddrPort) error
	updateHomeRelay func(int64) error
}

func (m *MockControl) ControlKey() key.ControlPublic {
	return m.controlKey()
}

func (m *MockControl) IPv4() netip.Prefix {
	return m.ipv4()
}

func (m *MockControl) IPv6() netip.Prefix {
	return m.ipv6()
}

func (m *MockControl) Expiry() time.Time {
	return m.expiry()
}

func (m *MockControl) UpdateEndpoints(endpoints []netip.AddrPort) error {
	m.endpoints = endpoints
	return m.updateEndpoints(endpoints)
}

func (m *MockControl) UpdateHomeRelay(relayID int64) error {
	return m.updateHomeRelay(relayID)
}

func TestEndpointManager(t *testing.T) {
	// EndpointManager uses a DirectRouter and RelayManager in this test
	s := &Stage{
		Ctx: context.TODO(),
	}

	mockControl := &MockControl{
		endpoints:       make([]netip.AddrPort, 0),
		updateEndpoints: func([]netip.AddrPort) error { return nil },
	}

	s.control = mockControl

	dr := s.makeDR()
	s.DRouter = dr

	rm := s.makeRM()
	s.RMan = rm

	// Run EndpointManager
	em := s.makeEM()
	go em.Run()

	// Test startSTUN triggered by UpdateRelayConfiguration message
	relayInfo := relay.Information{
		ID:  0,
		Key: dummyKey,
		IPs: []netip.Addr{dummyAddr},
	}

	updateRelayConfigMsg := &msgactor.UpdateRelayConfiguration{
		Config: []relay.Information{relayInfo},
	}

	em.inbox <- updateRelayConfigMsg
	expectedRelayAddrPort := netip.AddrPortFrom(dummyAddr, stun.DefaultPort)
	receivedMsg := <-dr.inbox
	receivedPushStun, ok := receivedMsg.(*msgactor.DRouterPushSTUN)

	assert.True(t, ok, fmt.Sprintf("Expected DirectRouter to receive a DRouterPushSTUN message, but got: %+v", receivedMsg))
	assert.NotNil(t, receivedPushStun.Packets[expectedRelayAddrPort], "DRouterPushSTUN message does not contain packet from relay endpoint")
	assert.Equal(t, relayInfo, em.relays[relayInfo.ID], "EndpointManager relays map is incorrect")
	assert.Equal(t, relayInfo.ID, em.relayStunEndpoints[expectedRelayAddrPort], "EndpointManager relayStunEndpoints map is incorrect")

	// Test onSTUNResponse triggered by EManSTUNResponse message
	expectedTxID := em.stunRequests[expectedRelayAddrPort].txid
	testSTUNAddr := netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 2, 3, 4}), 5)
	testSTUNPacket := stun.Response(expectedTxID, testSTUNAddr)
	stunResponseMsg := &msgactor.EManSTUNResponse{
		Endpoint:  expectedRelayAddrPort,
		Packet:    testSTUNPacket,
		Timestamp: time.Now(),
	}

	em.inbox <- stunResponseMsg

	receivedMsg = <-rm.inbox
	receivedLatencyResults, ok := receivedMsg.(*msgactor.RManRelayLatencyResults)

	assert.True(t, ok, fmt.Sprintf("Expected RelayManager to receive a RManRelayLatencyResults message, but got: %+v", receivedMsg))
	assert.NotNil(t, receivedLatencyResults.RelayLatency[relayInfo.ID], "RManRelayLatencyResults message does not contain latency from expected relay ID")
	assert.Len(t, mockControl.endpoints, 1, "MockControl has not been updated with the correct amount of endpoints")
	assert.Equal(t, testSTUNAddr, mockControl.endpoints[0], "MockControl has not been updated with the correct endpoint")
}
