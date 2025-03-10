package actors

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"runtime/debug"
	"slices"
	"time"

	"github.com/edup2p/common/types"
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

func MakeSockRecv(ctx context.Context, udp types.UDPConn) *SockRecv {
	return assureClose(&SockRecv{
		Conn:  udp,
		outCh: make(chan RecvFrame, SockRecvFrameChanBuffer),

		ActorCommon: MakeCommon(ctx, -1),
	})
}

func (r *SockRecv) Run() {
	if !r.running.CheckOrMark() {
		L(r).Warn("tried to run agent, while already running")
		return
	}

	defer r.Cancel()
	defer func() {
		if v := recover(); v != nil {
			L(r).Error("panicked", "err", v, "stack", string(debug.Stack()))
			bail(r.ctx, v)
		}
	}()

	buf := make([]byte, 1<<16)

	for {
		if r.ctx.Err() != nil {
			return
		}

		err := r.Conn.SetReadDeadline(time.Now().Add(SockRecvReadTimeout))
		if err != nil {
			L(r).Error("failed to set read deadline", "err", err)
			return
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
			if r.ctx.Err() != nil {
				// we're closing anyways, just return
				return
			}

			if !errors.Is(err, net.ErrClosed) {
				L(r).Error("failed to read packet", "err", err)
			}

			return
		}

		if n == 0 {
			continue
		}

		pkt := slices.Clone(buf[:n])

		if r.ctx.Err() != nil {
			return
		}

		select {
		case r.outCh <- RecvFrame{
			pkt: pkt,
			ts:  ts,
			src: ap,
		}:
			// fallthrough continue
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *SockRecv) Close() {
	if err := r.Conn.Close(); err != nil {
		slog.Error("failed to close connection for sockrecv", "err", err)
	}
	close(r.outCh)
}
