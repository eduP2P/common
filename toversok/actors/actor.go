package actors

import (
	"context"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type Actor interface {
	Run()

	Inbox() chan<- ActorMessage

	// Cancel this actor'S context.
	Cancel()

	// Close is called by the actor'S Run loop when cancelled.
	Close()
}

type ActorCommon struct {
	inbox   chan ActorMessage
	ctx     context.Context
	ctxCan  context.CancelFunc
	running RunCheck
}

func MakeCommon(pCtx context.Context, chLen int) *ActorCommon {
	ctx, ctxCan := context.WithCancel(pCtx)

	var inbox chan ActorMessage = nil

	if chLen >= 0 {
		inbox = make(chan ActorMessage, chLen)
	}

	return &ActorCommon{
		inbox:   inbox,
		ctx:     ctx,
		ctxCan:  ctxCan,
		running: MakeRunCheck(),
	}
}

func (ac *ActorCommon) Inbox() chan<- ActorMessage {
	return ac.inbox
}

func (ac *ActorCommon) Cancel() {
	ac.ctxCan()
}

func (ac *ActorCommon) logUnknownMessage(am ActorMessage) {
	// TODO
}

// ===

type DirectManagerActor interface {
	Actor

	WriteTo(pkt []byte, addr netip.AddrPort)
}

type DirectedPeerFrame struct {
	srcAddrPort netip.AddrPort

	pkt []byte
}

type DirectRouterActor interface {
	Actor

	Push(frame DirectedPeerFrame)
}

// ===

type RelayManagerActor interface {
	Actor

	WriteTo(pkt []byte, relay int64, dst key.NodePublic)
}

type RelayedPeerFrame struct {
	srcRelay int64

	srcPeer key.NodePublic

	pkt []byte
}

type RelayRouterActor interface {
	Actor

	Push(frame RelayedPeerFrame)
}
