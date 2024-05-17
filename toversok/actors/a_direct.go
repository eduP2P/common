package actors

import (
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/actor_msg"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msg"
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

const DMChanWriteRequestLen = 4

func (s *Stage) makeDM(udpSocket types.UDPConn) *DirectManager {
	c := MakeCommon(s.Ctx, -1)
	return &DirectManager{
		ActorCommon: c,
		sock:        MakeSockRecv(udpSocket, c.ctx),
		s:           s,
		writeCh:     make(chan directWriteRequest, DMChanWriteRequestLen),
	}
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

	go dm.sock.Run()

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
			dm.s.DRouter.Push(ifaces.DirectedPeerFrame{
				SrcAddrPort: frame.src,
				Pkt:         frame.pkt,
			})
		}
	}
}

func (dm *DirectManager) Close() {
	close(dm.writeCh)
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

	frameCh chan ifaces.DirectedPeerFrame
}

const DRInboxLen = 4
const DRFrameChLen = 4

func (s *Stage) makeDR() *DirectRouter {
	return &DirectRouter{
		ActorCommon: MakeCommon(s.Ctx, DRInboxLen),
		s:           s,
		aka:         make(map[netip.AddrPort]key.NodePublic),
		frameCh:     make(chan ifaces.DirectedPeerFrame, DRFrameChLen),
	}
}

func (dr *DirectRouter) Push(frame ifaces.DirectedPeerFrame) {
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
			case *actor_msg.DRouterPeerAddKnownAs:
				dr.aka[m.AddrPort] = m.Peer
			case *actor_msg.DRouterPeerClearKnownAs:
				maps.DeleteFunc(dr.aka,
					func(_ netip.AddrPort, peer key.NodePublic) bool {
						return peer == m.Peer
					},
				)
			default:
				dr.logUnknownMessage(m)
			}
		case frame := <-dr.frameCh:
			if msg.LooksLikeSessionWireMessage(frame.Pkt) {
				go SendMessage(dr.s.SMan.Inbox(), &actor_msg.SManSessionFrameFromAddrPort{
					AddrPort:       frame.SrcAddrPort,
					FrameWithMagic: frame.Pkt,
				})
				continue
			}

			peer, ok := dr.aka[frame.SrcAddrPort]

			if !ok {
				// todo log? metric?
				continue
			}

			in := dr.s.InConnFor(peer)

			if in == nil {
				// todo log? metric?
				continue
			}

			in.ForwardPacket(frame.Pkt)
		}
	}
}

func (dr *DirectRouter) Close() {
	close(dr.frameCh)
}
