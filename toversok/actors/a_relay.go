package actors

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/key"
)

// RestartableRelayConn is a Relay connection that will automatically reconnect,
// as long as there are pending packets.
type RestartableRelayConn struct {
	*ActorCommon

	config types.RelayInformation
	// TODO
}

func (rrc *RestartableRelayConn) Run() {
	//TODO implement me
	panic("implement me")
}

func (rrc *RestartableRelayConn) Close() {
	//TODO implement me
	panic("implement me")
}

func (rrc *RestartableRelayConn) Queue(pkt []byte, peer key.NodePublic) {
	// TODO
	panic("not implemented")
}

func (rrc *RestartableRelayConn) Update(info types.RelayInformation) {
	// TODO
	panic("not implemented")
}

type RelayConnActor interface {
	Actor

	Queue(pkt []byte, peer key.NodePublic)
	Update(info types.RelayInformation)
}

type relayWriteRequest struct {
	toRelay int64
	toPeer  key.NodePublic
	pkt     []byte
}

type RelayManager struct {
	*ActorCommon

	s *Stage

	// TODO

	relays map[int64]RelayConnActor

	inCh chan RelayedPeerFrame

	writeCh chan relayWriteRequest
}

func (rm *RelayManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(rm).Error("panicked", "panic", v)
			rm.Cancel()
		}
	}()

	if !rm.running.CheckOrMark() {
		L(rm).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-rm.ctx.Done():
			rm.Close()
			return
		case m := <-rm.inbox:
			if update, ok := m.(*RManUpdateRelayConfiguration); ok {
				for _, c := range update.config {
					rm.upsert(c)
				}
			} else {
				rm.logUnknownMessage(m)
			}
		case req := <-rm.writeCh:
			conn := rm.getConn(req.toRelay)

			if conn == nil {
				// This only happens if we don't know the relay definition for this relay.
				// Either;
				//  - some home relay got changed to an arbitrary number
				//  - new relay information is still being pushed to us
				//  - there is a bug
				// In any case, log it.

				L(rm).Warn("cannot forward to relay; unknown relay",
					"to_relay", req.toRelay,
					"to_peer", req.toPeer,
				)

				continue
			}

			conn.Queue(req.pkt, req.toPeer)

		case frame := <-rm.inCh:
			rm.s.RRouter.Push(frame)
		}
	}

}

func (rm *RelayManager) Close() {
	// TODO
	//  - close relay connections
	panic("not implemented")
}

func (rm *RelayManager) getConn(int int64) RelayConnActor {
	// TODO needs relay mapping and such
	panic("not implemented")
}

func (rm *RelayManager) upsert(info types.RelayInformation) {
	// TODO
	panic("not implemented")
}

// WriteTo queues a packet relay request to a relay ID, for a certain public key.
//
// Will be called by other actors.
func (rm *RelayManager) WriteTo(pkt []byte, relay int64, dst key.NodePublic) {
	go func() {
		rm.writeCh <- relayWriteRequest{
			toRelay: relay,
			toPeer:  dst,
			pkt:     pkt,
		}
	}()
}

type RelayRouter struct {
	*ActorCommon

	s *Stage

	frameCh chan RelayedPeerFrame
}

func (rr *RelayRouter) Push(frame RelayedPeerFrame) {
	go func() {
		rr.frameCh <- frame
	}()
}

func (rr *RelayRouter) Run() {
	defer func() {
		if v := recover(); v != nil {
			// TODO logging
			rr.Cancel()
		}
	}()

	if !rr.running.CheckOrMark() {
		L(rr).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-rr.ctx.Done():
			rr.Close()
			return
		case frame := <-rr.frameCh:
			if msg.LooksLikeSessionWireMessage(frame.pkt) {
				SendMessage(rr.s.SMan.Inbox(), &SManSessionFrameFromRelay{
					relay:          frame.srcRelay,
					peer:           frame.srcPeer,
					frameWithMagic: frame.pkt,
				})

				continue
			}

			in := rr.s.InConnFor(frame.srcPeer)

			if in == nil {
				// todo log? metric?
				continue
			}

			in.ForwardPacket(frame.pkt)
		}
	}
}

func (rr *RelayRouter) Close() {
	//TODO implement me
	panic("implement me")
}
