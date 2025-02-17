package actors

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/relay"
	"github.com/stretchr/testify/assert"
)

func TestRelayManager(t *testing.T) {
	// RelayManager uses RelayRouter and a RestartableRelayConn for this test
	rr := &RelayRouter{
		frameCh: make(chan ifaces.RelayedPeerFrame, RelayRouterFrameChLen),
	}

	const RelayID int64 = 0

	homeRelay := &RestartableRelayConn{
		config:   relay.Information{ID: RelayID},
		bufferCh: make(chan relay.SendPacket),
	}

	s := &Stage{
		Ctx:     context.TODO(),
		RRouter: rr,
	}

	// Make and run RelayManager
	rm := s.makeRM()
	rm.relays[RelayID] = homeRelay
	go rm.Run()

	// Message that should be sent to the relay
	relayReq := relayWriteRequest{
		toRelay: 0,
		toPeer:  dummyKey,
		pkt:     []byte{37},
	}

	rm.writeCh <- relayReq
	req := <-homeRelay.bufferCh
	assert.Equal(t, relay.SendPacket{Dst: relayReq.toPeer, Data: relayReq.pkt}, req, "Relay did not receive the expected write request")

	// Message that should be sent to RelayRouter
	rrFrame := ifaces.RelayedPeerFrame{
		SrcRelay: 0,
		SrcPeer:  dummyKey,
		Pkt:      []byte{42},
	}

	rm.inCh <- rrFrame
	frame := <-rr.frameCh
	assert.Equal(t, rrFrame.Pkt, frame.Pkt, "RelayRouter did not receive the expected message")
}

func TestRelayRouter(t *testing.T) {
	// RelayRouter uses SessionManager and two peer InConns in this test
	sm := &SessionManager{
		ActorCommon: MakeCommon(context.TODO(), 0),
	}

	var ics []*InConn

	for _, b := range []byte{1, 2} {
		peerKey := dummyKey
		peerKey[31] = b

		ic := &InConn{
			pktCh: make(chan []byte),
			peer:  peerKey,
		}

		ics = append(ics, ic)
	}

	s := &Stage{
		Ctx:    context.TODO(),
		SMan:   sm,
		inConn: make(map[key.NodePublic]InConnActor),
	}

	// Make and run RelayRouter
	rr := s.makeRR()
	go rr.Run()

	// Message that should be sent to SessionManager
	sessionPkt := slices.Concat(msgsess.MagicBytes, zeroBytes(56))

	frameSession := ifaces.RelayedPeerFrame{
		SrcRelay: 0,
		SrcPeer:  dummyKey,
		Pkt:      sessionPkt,
	}

	rr.Push(frameSession)
	msgSM := <-sm.inbox
	assert.Equal(t, msgSM, &msgactor.SManSessionFrameFromRelay{Relay: frameSession.SrcRelay, Peer: frameSession.SrcPeer, FrameWithMagic: frameSession.Pkt}, "SessionManager did not receive the expected message")

	// For each peer: register peer in RelayRouter and Stage, and then send a message to their inConn
	for i, b := range []byte{1, 2} {
		ic := ics[i]
		peer := ic.peer

		s.inConn[peer] = ic

		frame := ifaces.RelayedPeerFrame{
			SrcRelay: 0,
			SrcPeer:  peer,
			Pkt:      []byte{b},
		}

		rr.Push(frame)
		pkt := <-ic.pktCh
		assert.Equal(t, pkt, frame.Pkt, fmt.Sprintf("Peer %d did not receive the expected message", int(b)))
	}
}
