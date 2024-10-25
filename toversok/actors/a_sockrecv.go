package actors

import (
	"context"
	"errors"
	"fmt"
	"github.com/edup2p/common/types"
	"net"
	"net/netip"
	"slices"
	"time"
)

type RecvFrame struct {
	pkt []byte

	src netip.AddrPort

	ts time.Time
}

type SockRecv struct {
	*ActorCommon

	Conn types.UDPConn

	outCh chan RecvFrame
}

func MakeSockRecv(udp types.UDPConn, pCtx context.Context) *SockRecv {

	return &SockRecv{
		Conn:  udp,
		outCh: make(chan RecvFrame, SockRecvFrameChanBuffer),

		ActorCommon: MakeCommon(pCtx, -1),
	}
}

func (r *SockRecv) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(r).Error("panicked", "err", v)
			r.Cancel()
			bail(r.ctx, v)
		}
	}()

	if !r.running.CheckOrMark() {
		L(r).Warn("tried to run agent, while already running")
		return
	}

	var buf = make([]byte, 1<<16)

	for {
		if context.Cause(r.ctx) != nil {
			return
		}

		err := r.Conn.SetReadDeadline(time.Now().Add(SockRecvReadTimeout))
		if err != nil {
			panic(fmt.Sprint("Error when setting read deadline:", err))
		}

		n, ap, err := r.Conn.ReadFromUDPAddrPort(buf)

		ts := time.Now()

		var e net.Error
		if err != nil && (!errors.As(err, &e) || !e.Timeout()) {
			// handle error, it's not a timeout
			// TODO
			//  unsure what to do here, as this might be a permanent error of the socket?
			//  would this result in the closing of the channel? if so, wouldnt the corresponding outconn also die?
			//  if so, then who detects the death of the actor and recreates it like that?
			if context.Cause(r.ctx) != nil {
				// we're closing anyways, just return
				return
			}

			if errors.Is(err, net.ErrClosed) {
				r.Cancel()
				return
			}

			panic(err)
		}

		if n == 0 {
			continue
		}

		pkt := slices.Clone(buf[:n])

		if context.Cause(r.ctx) != nil {
			return
		}

		select {
		case <-r.ctx.Done():
			r.Close()
			return
		case r.outCh <- RecvFrame{
			pkt: pkt,
			ts:  ts,
			src: ap,
		}:
			// fallthrough continue
		}
	}
}

func (r *SockRecv) Close() {
	if context.Cause(r.ctx) == nil {
		r.Conn.Close()
		close(r.outCh)
		r.ctxCan()
		return
	}
}
