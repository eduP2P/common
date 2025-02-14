package actors

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/stretchr/testify/assert"
)

func TestDirectManager(t *testing.T) {
	// DirectManager uses DirectRouter and a UDP socket in this test
	dr := &DirectRouter{
		frameCh: make(chan ifaces.DirectedPeerFrame, DirectRouterFrameChLen),
	}

	mockUDPConn := &MockUDPConn{
		writeCh: make(chan []byte),
		setReadDeadline: func(t time.Time) error {
			return nil
		},
		readFromUDPAddrPort: func(b []byte) (n int, addr netip.AddrPort, err error) {
			return 0, dummyAddrPort, nil
		},
		writeToUDPAddrPort: func(b []byte, addr netip.AddrPort) (int, error) {
			return 0, nil
		},
	}

	s := &Stage{
		Ctx:     context.TODO(),
		DRouter: dr,
	}

	// Make and run DirectManager
	dm := s.makeDM(mockUDPConn)
	go dm.Run()

	// Message that should be written to UDP socket
	udpPkt := []byte{37}
	dm.WriteTo(udpPkt, dummyAddrPort)
	pkt := <-mockUDPConn.writeCh
	assert.Equal(t, udpPkt, pkt, "UDP Socket did not receive the expected message")

	// Message that should be sent to DirectRouter
	drFrame := RecvFrame{
		pkt: []byte{42},
		src: dummyAddrPort,
	}

	dm.sock.outCh <- drFrame
	frame := <-dr.frameCh
	assert.Equal(t, drFrame.pkt, frame.Pkt, "DirectRouter did not receive the expected message")
}

func TestDirectRouter(t *testing.T) {
	// DirectRouter uses EndpointManager, SessionManager and two peer InConns in this test
	em := &EndpointManager{
		ActorCommon: MakeCommon(context.TODO(), 0),
	}

	sm := &SessionManager{
		ActorCommon: MakeCommon(context.TODO(), 0),
	}

	var ics []*InConn
	var peerEndpoints []netip.AddrPort

	for _, b := range []byte{1, 2} {
		peerKey := dummyKey
		peerKey[31] = b
		peerEndpoint := netip.AddrPortFrom(netip.AddrFrom4([4]byte{b, b, b, b}), uint16(b))

		ic := &InConn{
			pktCh: make(chan []byte),
			peer:  peerKey,
		}

		ics = append(ics, ic)
		peerEndpoints = append(peerEndpoints, peerEndpoint)
	}

	s := &Stage{
		Ctx:    context.TODO(),
		EMan:   em,
		SMan:   sm,
		inConn: make(map[key.NodePublic]InConnActor),
	}

	// Make and run DirectRouter
	dr := s.makeDR()
	go dr.Run()

	// Message that should be sent to EndpointManager
	stunAddrPort := netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 2, 3, 4}), 1234)
	dr.stunEndpoints[stunAddrPort] = true

	frameEndpoint := ifaces.DirectedPeerFrame{
		SrcAddrPort: stunAddrPort,
		Pkt:         nil,
	}

	dr.Push(frameEndpoint)
	msgEM := <-em.inbox
	assert.Equal(t, msgEM, &msgactor.EManSTUNResponse{Endpoint: frameEndpoint.SrcAddrPort, Packet: frameEndpoint.Pkt}, "EndpointManager did not receive the expected message")

	// Message that should be sent to SessionManager
	sessionPkt := append(msgsess.MagicBytes, zeroBytes(56)...)

	frameSession := ifaces.DirectedPeerFrame{
		SrcAddrPort: dummyAddrPort,
		Pkt:         sessionPkt,
	}

	dr.Push(frameSession)
	msgSM := <-sm.inbox
	assert.Equal(t, msgSM, &msgactor.SManSessionFrameFromAddrPort{AddrPort: frameSession.SrcAddrPort, FrameWithMagic: frameSession.Pkt}, "SessionManager did not receive the expected message")

	// For each peer: register peer in DirectRouter and Stage, and then send a message to their inConn
	for i, b := range []byte{1, 2} {
		ic := ics[i]
		key := ic.peer
		endpoint := peerEndpoints[i]

		dr.setAKA(endpoint, key)
		s.inConn[key] = ic

		frame := ifaces.DirectedPeerFrame{
			SrcAddrPort: endpoint,
			Pkt:         []byte{b},
		}

		dr.Push(frame)
		pkt := <-ic.pktCh
		assert.Equal(t, pkt, frame.Pkt, fmt.Sprintf("Peer %d did not receive the expected message", int(b)))
	}
}
