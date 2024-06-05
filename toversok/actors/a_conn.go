package actors

import (
	"context"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgactor"
	"net/netip"
	"time"
)

type OutConn struct {
	*ActorCommon

	sock *SockRecv
	s    *Stage

	// If true, conn will send packets to the relay manager with toRelay,
	// else, conn will send packets to the direct manager with toAddrPort.
	useRelay  bool
	trackHome bool

	toRelay    int64
	toAddrPort netip.AddrPort

	peer key.NodePublic

	activityTimer *time.Timer
	isActive      bool
}

func MakeOutConn(udp types.UDPConn, peer key.NodePublic, homeRelay int64, s *Stage) *OutConn {
	t := time.NewTimer(60 * time.Second)
	t.Stop()

	common := MakeCommon(s.Ctx, OutConnInboxChanBuffer)

	return &OutConn{
		ActorCommon: common,

		sock: MakeSockRecv(udp, common.ctx),
		s:    s,

		peer:     peer,
		useRelay: true,
		toRelay:  homeRelay,

		activityTimer: t,
		isActive:      false,
	}
}

func (oc *OutConn) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(oc).Error("panicked", "panic", v)
			oc.Cancel()
		}
	}()

	if !oc.running.CheckOrMark() {
		L(oc).Warn("tried to run agent, while already running")
		return
	}

	go oc.sock.Run()

	for {
		select {
		case <-oc.ctx.Done():
			oc.Close()
			return
		case <-oc.sock.ctx.Done():
			oc.Cancel()
		case <-oc.activityTimer.C:
			oc.UnBump()
		case msg := <-oc.inbox:
			switch m := msg.(type) {
			case *msgactor.OutConnUse:
				oc.useRelay = m.UseRelay
				oc.trackHome = m.TrackHome
				oc.toRelay = m.RelayToUse
				oc.toAddrPort = m.AddrPortToUse

				oc.doTrackHome()
			case *msgactor.SyncPeerInfo:
				oc.doTrackHome()
			default:
				oc.logUnknownMessage(m)
			}
		case frame, ok := <-oc.sock.outCh:
			if !ok {
				// sock closed, the peer is dead
				// TODO:
				//   trigger some kind of healing logic elsewhere?
				oc.Cancel()
				continue
			}

			if oc.useRelay {
				oc.s.RMan.WriteTo(frame.pkt, oc.toRelay, oc.peer)
			} else {
				oc.s.DMan.WriteTo(frame.pkt, oc.toAddrPort)
			}
			oc.Bump()
		}
	}
}

// Bump the activity timer.
func (oc *OutConn) Bump() {
	if !oc.activityTimer.Stop() {
		select {
		case <-oc.activityTimer.C:
		default:
		}
	}
	oc.activityTimer.Reset(ConnActivityInterval)

	if !oc.isActive {
		oc.SendActivity(true)
	}

	oc.isActive = true
}

// UnBump is called when the activity timer fires in the main loop
func (oc *OutConn) UnBump() {
	oc.isActive = false

	oc.SendActivity(false)
}

func (oc *OutConn) SendActivity(isActive bool) {
	oc.s.TMan.Inbox() <- &msgactor.TManConnActivity{
		Peer:     oc.peer,
		IsIn:     false,
		IsActive: isActive,
	}
}

func (oc *OutConn) doTrackHome() {
	if oc.trackHome {
		pi := oc.s.GetPeerInfo(oc.peer)

		if pi == nil {
			L(oc).Warn("tried to update home relay, peerinfo is gone", "peer", oc.peer.Debug())
		}

		oc.toRelay = pi.HomeRelay
	}
}

func (oc *OutConn) Inbox() chan<- msgactor.ActorMessage {
	return oc.inbox
}

func (oc *OutConn) Close() {
	close(oc.sock.outCh)
	close(oc.inbox)

	oc.activityTimer.Stop()

	oc.s.TMan.Inbox() <- &msgactor.TManConnGoodBye{
		Peer: oc.peer,
		IsIn: false,
	}

	L(oc).Debug("closed outconn", "peer", oc.peer)
}

func (oc *OutConn) Ctx() context.Context {
	return oc.ctx
}

// ==================================

type InConn struct {
	*ActorCommon

	s *Stage

	udp types.UDPConn

	pktCh chan []byte

	peer key.NodePublic

	activityTimer *time.Timer
	isActive      bool
}

func MakeInConn(udp types.UDPConn, peer key.NodePublic, s *Stage) *InConn {
	t := time.NewTimer(60 * time.Second)
	t.Stop()

	return &InConn{
		ActorCommon: MakeCommon(s.Ctx, -1),

		s: s,

		udp: udp,

		activityTimer: t,
		isActive:      false,

		pktCh: make(chan []byte, InConnFrameChanBuffer),
		peer:  peer,
	}
}

func (ic *InConn) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(ic).Error("panicked", "panic", v)
			ic.Cancel()
			ic.Close()
		}
	}()

	if !ic.running.CheckOrMark() {
		L(ic).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-ic.ctx.Done():
			ic.Close()
			return
		case <-ic.activityTimer.C:
			ic.UnBump()
		case frame := <-ic.pktCh:
			n, err := ic.udp.Write(frame)
			if err != nil {
				// TODO failsafe logic
				panic(err)
			}

			if n != len(frame) {
				L(ic).Warn("unexpected short write on udp", "expected", len(frame), "got", n)
			}

			ic.Bump()
		}
	}
}

func (ic *InConn) Close() {
	ic.activityTimer.Stop()

	ic.s.TMan.Inbox() <- &msgactor.TManConnGoodBye{
		Peer: ic.peer,
		IsIn: false,
	}
}

func (ic *InConn) Ctx() context.Context {
	return ic.ctx
}

// ForwardPacket does a non-blocking packet forward.
//
// This prevents routers from blocking when the conn is shutting down,
// or if its blocked otherwise.
func (ic *InConn) ForwardPacket(pkt []byte) {
	select {
	case ic.pktCh <- pkt:
	default:
		// TODO maybe convert dropping to timeout?
		//  making lots of timers would be costly though
		// TODO log? metric?
	}
}

// Bump the activity timer.
func (ic *InConn) Bump() {
	if !ic.activityTimer.Stop() {
		select {
		case <-ic.activityTimer.C:
		default:
		}
	}
	ic.activityTimer.Reset(ConnActivityInterval)

	if !ic.isActive {
		ic.SendActivity(true)
	}

	ic.isActive = true
}

// UnBump is called when the activity timer fires in the main loop
func (ic *InConn) UnBump() {
	ic.isActive = false

	ic.SendActivity(false)
}

func (ic *InConn) SendActivity(isActive bool) {
	ic.s.TMan.Inbox() <- &msgactor.TManConnActivity{
		Peer:     ic.peer,
		IsIn:     true,
		IsActive: isActive,
	}
}
