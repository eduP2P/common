package actors

import (
	"context"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgactor"
	"github.com/shadowjonathan/edup2p/types/msgsess"
	"golang.org/x/exp/maps"
	"log/slog"
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
			bail(dm.ctx, v)
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
			L(dm).Log(context.Background(), types.LevelTrace, "direct: writing")
			_, err := dm.sock.Conn.WriteToUDPAddrPort(req.pkt, req.to)
			if err != nil {
				L(dm).Warn("error writing to socket", "error", err)
			}
			L(dm).Log(context.Background(), types.LevelTrace, "direct: written")

		case frame := <-dm.sock.outCh:
			L(dm).Log(context.Background(), types.LevelTrace, "direct: receiving")
			dm.s.DRouter.Push(ifaces.DirectedPeerFrame{
				SrcAddrPort: frame.src,
				Pkt:         frame.pkt,
			})
			L(dm).Log(context.Background(), types.LevelTrace, "direct: received")
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
	dm.writeCh <- directWriteRequest{
		to:  addr,
		pkt: pkt,
	}
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

	stunEndpoints map[netip.AddrPort]bool

	frameCh chan ifaces.DirectedPeerFrame
}

const DRInboxLen = 4
const DRFrameChLen = 4

func (s *Stage) makeDR() *DirectRouter {
	return &DirectRouter{
		ActorCommon:   MakeCommon(s.Ctx, DRInboxLen),
		s:             s,
		aka:           make(map[netip.AddrPort]key.NodePublic),
		stunEndpoints: make(map[netip.AddrPort]bool),
		frameCh:       make(chan ifaces.DirectedPeerFrame, DRFrameChLen),
	}
}

func (dr *DirectRouter) Push(frame ifaces.DirectedPeerFrame) {
	//go func() {
	dr.frameCh <- frame
	//}()
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
			case *msgactor.DRouterPeerAddKnownAs:
				dr.setAKA(m.AddrPort, m.Peer)
			case *msgactor.DRouterPeerClearKnownAs:
				dr.removeAKA(m.Peer)
			case *msgactor.DRouterPushSTUN:
				dr.doSTUN(m.Packets)
			default:
				dr.logUnknownMessage(m)
			}
		case frame := <-dr.frameCh:
			if _, ok := dr.stunEndpoints[frame.SrcAddrPort]; ok {
				go SendMessage(dr.s.EMan.Inbox(), &msgactor.EManSTUNResponse{
					Endpoint: frame.SrcAddrPort,
					Packet:   frame.Pkt,
				})
				continue
			}

			if msgsess.LooksLikeSessionWireMessage(frame.Pkt) {
				go SendMessage(dr.s.SMan.Inbox(), &msgactor.SManSessionFrameFromAddrPort{
					AddrPort:       frame.SrcAddrPort,
					FrameWithMagic: frame.Pkt,
				})
				continue
			}

			peer, ok := dr.peerAKA(frame.SrcAddrPort)

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

func (dr *DirectRouter) peerAKA(ap netip.AddrPort) (peer key.NodePublic, ok bool) {
	nap := types.NormaliseAddrPort(ap)

	peer, ok = dr.aka[nap]

	slog.Debug("dr: peerAKA", "ap", ap.String(), "nap", nap, "ok", ok)

	return
}

func (dr *DirectRouter) setAKA(ap netip.AddrPort, peer key.NodePublic) {
	nap := types.NormaliseAddrPort(ap)

	slog.Info("dr: setAKA", "ap", ap.String(), "nap", nap.String(), "peer", peer.Debug())

	dr.aka[nap] = peer
}

func (dr *DirectRouter) removeAKA(peer key.NodePublic) {
	slog.Info("dr: removeAKA", "peer", peer.Debug())

	maps.DeleteFunc(dr.aka,
		func(_ netip.AddrPort, p key.NodePublic) bool {
			return p == peer
		},
	)
}

func (dr *DirectRouter) doSTUN(p map[netip.AddrPort][]byte) {
	maps.Clear(dr.stunEndpoints)

	for ep, pkt := range p {
		ep = netip.AddrPortFrom(netip.AddrFrom16(ep.Addr().As16()), ep.Port())
		dr.stunEndpoints[ep] = true

		go dr.s.DMan.WriteTo(pkt, ep)
	}
}

func (dr *DirectRouter) Close() {
	close(dr.frameCh)
}
