package actors

import (
	"context"
	"testing"

	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/stretchr/testify/assert"
)

// Mock Session Message used in this test
type MockSessionMessage struct {
	marshalSessionMessage func() []byte
	debug                 func() string
}

func (m *MockSessionMessage) MarshalSessionMessage() []byte {
	return m.marshalSessionMessage()
}

func (m *MockSessionMessage) Debug() string {
	return m.debug()
}

func assertEncryptedPacket(t *testing.T, pkt []byte, sm *SessionManager, expectedDecryption *msgsess.ClearMessage, failMsg string) {
	// We cannot predict the encryption with a random nonce, so we unpack the packet in receivedReq to test if it is correct
	unpacked, ok := sm.Unpack(pkt)
	assert.Nil(t, ok, "Decryption of packet in received directWriteRequest failed")
	assert.Equal(t, expectedDecryption, unpacked, failMsg)
}

func TestSessionManager(t *testing.T) {
	// SessionManager uses TrafficManager, RelayManager, DirectManager and a UDP socket in this test
	s := &Stage{
		Ctx: context.TODO(),
	}

	tm := s.makeTM()
	s.TMan = tm

	rm := s.makeRM()
	s.RMan = rm

	dm := s.makeDM(&MockUDPConn{})
	s.DMan = dm

	// Generate private key for session, and run SessionManager
	sm := s.makeSM(getTestPriv)
	go sm.Run()

	// Create a test ping message
	txID := [12]byte{42}
	pingBytes := append(txID[:], dummyKey[:]...)
	clearBytes := append([]byte{1, 0}, pingBytes...) // 1 is version nr, 0 is Ping message

	pingMsg := &msgsess.Ping{
		TxID:    txID,
		NodeKey: dummyKey,
	}

	clearMsg := &msgsess.ClearMessage{
		Session: testPub,
		Message: pingMsg,
	}

	// Pack the test ping message
	mockSessionMsg := &MockSessionMessage{
		marshalSessionMessage: func() []byte {
			return clearBytes
		},
	}

	packedBytes := sm.Pack(mockSessionMsg, testPub)

	// Test Handle on frame from relay
	frameFromRelay := &msgactor.SManSessionFrameFromRelay{
		Relay:          0,
		Peer:           dummyKey,
		FrameWithMagic: packedBytes,
	}

	sm.inbox <- frameFromRelay

	expectedFromRelay := &msgactor.TManSessionMessageFromRelay{
		Relay: frameFromRelay.Relay,
		Peer:  frameFromRelay.Peer,
		Msg:   clearMsg,
	}

	receivedFromRelay := <-tm.inbox

	assert.Equal(t, expectedFromRelay, receivedFromRelay, "TrafficManager did not receive expected message when sending frame from an address-port pair to SessionManager")

	// Test Handle on frame from addrport
	frameFromAddrPort := &msgactor.SManSessionFrameFromAddrPort{
		AddrPort:       dummyAddrPort,
		FrameWithMagic: packedBytes,
	}

	sm.inbox <- frameFromAddrPort

	expectedFromDirect := &msgactor.TManSessionMessageFromDirect{
		AddrPort: frameFromAddrPort.AddrPort,
		Msg:      clearMsg,
	}

	receivedFromDirect := <-tm.inbox

	assert.Equal(t, expectedFromDirect, receivedFromDirect, "TrafficManager did not receive expected message when sending frame from relay to SessionManager")

	// Test Handle on message to relay
	msgToRelay := &msgactor.SManSendSessionMessageToRelay{
		Relay:     0,
		Peer:      dummyKey,
		ToSession: testPub,
		Msg:       mockSessionMsg,
	}

	sm.inbox <- msgToRelay
	receivedRelayReq := <-rm.writeCh

	assert.Equal(t, msgToRelay.Relay, receivedRelayReq.toRelay, "RelayManager did not receive expected request when sending message to relay to SessionManager: toRelay field is incorrect")
	assert.Equal(t, msgToRelay.Peer, receivedRelayReq.toPeer, "RelayManager did not receive expected request when sending message to relay to SessionManager: toPeer field is incorrect")
	assertEncryptedPacket(t, receivedRelayReq.pkt, sm, clearMsg, "RelayManager did not receive expected request when sending message to relay to SessionManager: unpacked message is incorrect")

	// Test Handle on message to addrport
	msgToDirect := &msgactor.SManSendSessionMessageToDirect{
		AddrPort:  dummyAddrPort,
		ToSession: testPub,
		Msg:       mockSessionMsg,
	}

	sm.inbox <- msgToDirect
	receivedDirectReq := <-dm.writeCh

	assert.Equal(t, msgToDirect.AddrPort, receivedDirectReq.to, "DirectManager did not receive expected request when sending message to addrport to SessionManager: to field is incorrect")
	assertEncryptedPacket(t, receivedRelayReq.pkt, sm, clearMsg, "DirectManager did not receive expected request when sending message to addrport to SessionManager: unpacked message is incorrect")
}
