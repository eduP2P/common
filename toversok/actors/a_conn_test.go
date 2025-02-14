package actors

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/edup2p/common/types/msgactor"
	"github.com/stretchr/testify/assert"
)

func TestOutConn(t *testing.T) {
	// OutConn uses TrafficManager, RelayManager, DirectManager and a UDP socket in this test
	s := &Stage{
		Ctx: context.TODO(),
	}

	tm := s.makeTM()
	s.TMan = tm

	rm := s.makeRM()
	s.RMan = rm

	dm := s.makeDM(&MockUDPConn{})
	s.DMan = dm

	wgConn := &MockUDPConn{
		writeCh: make(chan []byte),
		setReadDeadline: func(t time.Time) error {
			return nil
		},
		readFromUDPAddrPort: func(b []byte) (n int, addr netip.AddrPort, err error) {
			return 0, dummyAddrPort, nil
		},
	}

	// Make and run OutConn
	oc := MakeOutConn(wgConn, dummyKey, 0, s)
	go oc.Run()

	// Frame that should be sent to RelayManager
	relayedFrame := RecvFrame{
		pkt: []byte{37},
		src: dummyAddrPort,
	}

	oc.sock.outCh <- relayedFrame
	expectedRelayReq := relayWriteRequest{
		toRelay: oc.toRelay,
		toPeer:  oc.peer,
		pkt:     relayedFrame.pkt,
	}

	relayReq := <-rm.writeCh
	assert.Equal(t, expectedRelayReq, relayReq, "RelayManager did not receive the expected message")

	// Message to make OutConn switch to direct connection
	switchDirectMsg := msgactor.OutConnUse{
		UseRelay:      false,
		TrackHome:     false,
		RelayToUse:    oc.toRelay,
		AddrPortToUse: dummyAddrPort,
	}

	oc.inbox <- &switchDirectMsg

	// Frame that should be sent to DirectManager
	directFrame := RecvFrame{
		pkt: []byte{42},
		src: dummyAddrPort,
	}

	oc.sock.outCh <- directFrame
	expectedDirectReq := directWriteRequest{
		to:  dummyAddrPort,
		pkt: directFrame.pkt,
	}

	directReq := <-dm.writeCh
	assert.Equal(t, expectedDirectReq, directReq, "DirectManager did not receive the expected message")
}

func TestInConn(t *testing.T) {
	// InConn uses TrafficManager and a UDP socket in this test
	s := &Stage{
		Ctx: context.TODO(),
	}

	tm := s.makeTM()
	s.TMan = tm

	wgConn := &MockUDPConn{
		writeCh: make(chan []byte),
		write: func(b []byte) (int, error) {
			return len(b), nil
		},
	}

	// Make and run InConn
	ic := MakeInConn(wgConn, dummyKey, s)
	go ic.Run()

	// Message that should be written to WireGuard connection
	wgPkt := []byte{37}
	ic.ForwardPacket(wgPkt)
	pkt := <-wgConn.writeCh
	assert.Equal(t, wgPkt, pkt, "WireGuard Connection did not receive the expected message")

	// Verify that connection is active
	expectedMsg := &msgactor.TManConnActivity{
		Peer:     ic.peer,
		IsIn:     true,
		IsActive: true,
	}

	msg := <-tm.inbox
	assert.Equal(t, msg, expectedMsg, "Traffic Manager did not receive message indicating InConn became active")

	// Change timer duration for faster test
	ic.activityTimer.Reset(5 * time.Millisecond)
	expectedMsg.IsActive = false

	msg = <-tm.inbox
	assert.Equal(t, msg, expectedMsg, "Traffic Manager did not receive message indicating InConn became inactive")
}
