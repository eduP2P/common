package actors

import (
	"context"
	"log/slog"
	"net/netip"
	"runtime"
	"runtime/debug"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"golang.org/x/exp/maps"
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

func (s *Stage) makeDM(udpSocket types.UDPConn) *DirectManager {
	c := MakeCommon(s.Ctx, -1)
	return &DirectManager{
		ActorCommon: c,
		sock:        MakeSockRecv(c.ctx, udpSocket),
		s:           s,
		writeCh:     make(chan directWriteRequest, DirectManWriteChLen),
	}
}

func (dm *DirectManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(dm).Error("panicked", "panic", v, "stack", string(debug.Stack()))
			dm.Cancel()
			bail(dm.ctx, v)
		}
	}()

	if !dm.running.CheckOrMark() {
		L(dm).Warn("tried to run agent, while already running")
		return
	}

	go dm.sock.Run()

	runtime.LockOSThread()

	for {
		select {
		case <-dm.ctx.Done():
			dm.Close()
			return
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
				Timestamp:   frame.ts,
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

// TODO: track when we last received a packet from AddrPair?

type DirectRouter struct {
	*ActorCommon

	s *Stage

	// TODO

	aka map[netip.AddrPort]key.NodePublic

	stunEndpoints map[netip.AddrPort]bool

	frameCh chan ifaces.DirectedPeerFrame
}

func (s *Stage) makeDR() *DirectRouter {
	return &DirectRouter{
		ActorCommon:   MakeCommon(s.Ctx, DirectRouterInboxChLen),
		s:             s,
		aka:           make(map[netip.AddrPort]key.NodePublic),
		stunEndpoints: make(map[netip.AddrPort]bool),
		frameCh:       make(chan ifaces.DirectedPeerFrame, DirectRouterFrameChLen),
	}
}

func (dr *DirectRouter) Push(frame ifaces.DirectedPeerFrame) {
	// go func() {
	dr.frameCh <- frame
	// }()
}

func (dr *DirectRouter) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(dr).Error("panicked", "panic", v, "stack", string(debug.Stack()))
			dr.Cancel()
			bail(dr.ctx, v)
		}
	}()

	if !dr.running.CheckOrMark() {
		L(dr).Warn("tried to run agent, while already running")
		return
	}

	runtime.LockOSThread()

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
			dr.handleFrame(frame)
		}
	}
}

func (dr *DirectRouter) handleFrame(frame ifaces.DirectedPeerFrame) {
	if _, ok := dr.stunEndpoints[frame.SrcAddrPort]; ok {
		go SendMessage(dr.s.EMan.Inbox(), &msgactor.EManSTUNResponse{
			Endpoint:  frame.SrcAddrPort,
			Packet:    frame.Pkt,
			Timestamp: frame.Timestamp,
		})
		return
	}

	if msgsess.LooksLikeSessionWireMessage(frame.Pkt) {
		go SendMessage(dr.s.SMan.Inbox(), &msgactor.SManSessionFrameFromAddrPort{
			AddrPort:       frame.SrcAddrPort,
			FrameWithMagic: frame.Pkt,
		})
		return
	}

	peer, ok := dr.peerAKA(frame.SrcAddrPort)

	if !ok {
		// todo log? metric?
		return
	}

	in := dr.s.InConnFor(peer)

	if in == nil {
		// todo log? metric?
		return
	}

	in.ForwardPacket(frame.Pkt)
}

func (dr *DirectRouter) peerAKA(ap netip.AddrPort) (peer key.NodePublic, ok bool) {
	nap := types.NormaliseAddrPort(ap)

	peer, ok = dr.aka[nap]

	slog.Log(context.Background(), types.LevelTrace, "dr: peerAKA", "ap", ap.String(), "nap", nap, "ok", ok)

	return
}

func (dr *DirectRouter) setAKA(ap netip.AddrPort, peer key.NodePublic) {
	nap := types.NormaliseAddrPort(ap)

	slog.Debug("dr: setAKA", "ap", ap.String(), "nap", nap.String(), "peer", peer.Debug())

	dr.aka[nap] = peer
}

func (dr *DirectRouter) removeAKA(peer key.NodePublic) {
	slog.Debug("dr: removeAKA", "peer", peer.Debug())

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
