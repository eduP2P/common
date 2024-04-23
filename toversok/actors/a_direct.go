package actors

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"golang.org/x/exp/maps"
	"net/netip"
)

type directWriteRequest struct {
	to  netip.AddrPort
	pkt []byte
}

type DirectManager struct {
	*ActorCommon

	sock *SockRecv
	s    *Stage

	writeCh chan directWriteRequest
}

func (dm *DirectManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(dm).Error("panicked", "panic", v)
			dm.Cancel()
		}
	}()

	if !dm.running.CheckOrMark() {
		L(dm).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-dm.ctx.Done():
			dm.Close()
			return
		//case msg := <-dm.inbox:
		//	switch m := msg.(type) {
		//	case *DManSetMTU:
		//		dm.SetMTUFor(m.forAddrPort, m.mtu)
		//	default:
		//		dm.logUnknownMessage(m)
		//	}
		case req := <-dm.writeCh:
			_, err := dm.sock.Conn.WriteToUDPAddrPort(req.pkt, req.to)
			if err != nil {
				L(dm).Warn("error writing to socket", "error", err)
			}

		case frame := <-dm.sock.outCh:
			dm.s.DRouter.Push(DirectedPeerFrame{
				srcAddrPort: frame.src,
				pkt:         frame.pkt,
			})
		}
	}
}

func (dm *DirectManager) Close() {
	//TODO implement me
	panic("implement me")
}

// WriteTo queues a UDP write request to a certain addr-port pair.
//
// Will be called by other actors.
//
// If the Packet is larger than the current MTU, it will be broken up.
func (dm *DirectManager) WriteTo(pkt []byte, addr netip.AddrPort) {
	go func() {
		dm.writeCh <- directWriteRequest{
			to:  addr,
			pkt: pkt,
		}
	}()
}

//// MTUFor gets the MTU for a netip.AddrPort pair, or default.
//func (dm *DirectManager) MTUFor(ap netip.AddrPort) uint16 {
//	// TODO(jo): there is a small possibility that internal representation in
//	//   netip.AddrPort can differ, even though they'd be the same IP+Port pair.
//	//   I haven't found such a case, but it'S nagging in the back of my mind,
//	//   which is why this is a separate function,
//	//   so we can do any canonisation later.
//	mtu, ok := dm.mtuFor[ap]
//	if !ok {
//		return DefaultSafeMTU
//	} else {
//		return mtu
//	}
//}
//
//// SetMTUFor sets the MTU for a netip.AddrPort pair.
//func (dm *DirectManager) SetMTUFor(ap netip.AddrPort, mtu uint16) {
//	dm.mtuFor[ap] = mtu
//}

type DirectRouter struct {
	*ActorCommon

	s *Stage

	// TODO

	aka map[netip.AddrPort]key.NodePublic

	frameCh chan DirectedPeerFrame
}

func (dr *DirectRouter) Push(frame DirectedPeerFrame) {
	go func() {
		dr.frameCh <- frame
	}()
}

func (dr *DirectRouter) Run() {
	defer func() {
		if v := recover(); v != nil {
			// TODO logging
			dr.Cancel()
		}
	}()

	if !dr.running.CheckOrMark() {
		L(dr).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-dr.ctx.Done():
			dr.Close()
			return
		case m := <-dr.inbox:
			switch m := m.(type) {
			case *DRouterPeerAddKnownAs:
				dr.aka[m.addrPort] = m.peer
			case *DRouterPeerClearKnownAs:
				maps.DeleteFunc(dr.aka,
					func(_ netip.AddrPort, peer key.NodePublic) bool {
						return peer == m.peer
					},
				)
			default:
				dr.logUnknownMessage(m)
			}
		case frame := <-dr.frameCh:
			if msg.LooksLikeSessionWireMessage(frame.pkt) {
				SendMessage(dr.s.SMan.Inbox(), &SManSessionFrameFromAddrPort{
					addrPort:       frame.srcAddrPort,
					frameWithMagic: frame.pkt,
				})
			}

			peer, ok := dr.aka[frame.srcAddrPort]

			if !ok {
				// todo log? metric?
				continue
			}

			in := dr.s.InConnFor(peer)

			if in == nil {
				// todo log? metric?
				continue
			}

			in.ForwardPacket(frame.pkt)
		}
	}
}

func (dr *DirectRouter) Close() {
	//TODO implement me
	panic("implement me")
}
