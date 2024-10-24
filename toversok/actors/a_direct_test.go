package actors

import (
	"context"
	"fmt"
	"net/netip"
	"testing"

	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/stretchr/testify/assert"
)

func zeroBytes(n int) []byte {
	return make([]byte, n)
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
		peerKey := [32]byte{0}
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

	// Run DirectRouter
	dr := DirectRouter{
		ActorCommon:   MakeCommon(context.TODO(), 0),
		s:             s,
		aka:           make(map[netip.AddrPort]key.NodePublic),
		stunEndpoints: make(map[netip.AddrPort]bool),
		frameCh:       make(chan ifaces.DirectedPeerFrame, DirectRouterFrameChLen),
	}

	go dr.Run()

	// Message that should be sent to Endpoint Manager
	stunAddrPort := netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 2, 3, 4}), 1234)
	dr.stunEndpoints[stunAddrPort] = true

	frameEndpoint := ifaces.DirectedPeerFrame{
		SrcAddrPort: stunAddrPort,
		Pkt:         nil,
	}

	dr.Push(frameEndpoint)
	msgEM := <-em.inbox
	assert.Equal(t, msgEM, &msgactor.EManSTUNResponse{Endpoint: frameEndpoint.SrcAddrPort, Packet: frameEndpoint.Pkt}, "Endpoint Manager did not receive message")

	// Message that should be sent to Session Manager
	sessionPkt := append(msgsess.MagicBytes, zeroBytes(56)...)

	frameSession := ifaces.DirectedPeerFrame{
		SrcAddrPort: netip.AddrPortFrom(netip.IPv4Unspecified(), 0),
		Pkt:         sessionPkt,
	}

	dr.Push(frameSession)
	msgSM := <-sm.inbox
	assert.Equal(t, msgSM, &msgactor.SManSessionFrameFromAddrPort{AddrPort: frameSession.SrcAddrPort, FrameWithMagic: frameSession.Pkt}, "Session Manager did not receive message")

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
		assert.Equal(t, pkt, frame.Pkt, fmt.Sprintf("Peer %d did not receive message", int(b)))
	}
}
