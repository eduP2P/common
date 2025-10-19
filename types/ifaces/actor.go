package ifaces

import (
	"context"
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/stage"
)

type Actor interface {
	Run()

	Inbox() chan<- msgactor.ActorMessage

	Ctx() context.Context

	// Cancel this actor's context.
	Cancel()

	// Close is called by AfterFunc to clean up
	Close()
}

// ===

type DirectManagerActor interface {
	Actor

	WriteTo(pkt []byte, addr netip.AddrPort)
}

type DirectedPeerFrame struct {
	SrcAddrPort netip.AddrPort

	Timestamp time.Time

	Pkt []byte
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
	SrcRelay int64

	SrcPeer key.NodePublic

	Pkt []byte
}

type RelayRouterActor interface {
	Actor

	Push(frame RelayedPeerFrame)
}

// ===

type TrafficManagerActor interface {
	Actor

	Poke()

	ValidKeys(nodeKey key.NodePublic, sess key.SessionPublic) bool
	SendMsgToDirect(ap netip.AddrPort, sess key.SessionPublic, m msgsess.SessionMessage)
	SendMsgToRelay(relay int64, node key.NodePublic, sess key.SessionPublic, m msgsess.SessionMessage)
	SendPingDirect(ap netip.AddrPort, peer key.NodePublic, session key.SessionPublic)
	SendPingDirectWithID(ap netip.AddrPort, peer key.NodePublic, session key.SessionPublic, txid msgsess.TxID)

	OutConnUseAddrPort(peer key.NodePublic, ap netip.AddrPort)
	OutConnTrackHome(peer key.NodePublic)

	DManSetAKA(peer key.NodePublic, ap netip.AddrPort)
	DManClearAKA(peer key.NodePublic)

	Stage() Stage
	Pings() map[msgsess.TxID]*stage.SentPing

	ActiveIn() map[key.NodePublic]bool
	ActiveOut() map[key.NodePublic]bool
}

// ===

type SessionManagerActor interface {
	Actor

	Session() key.SessionPublic
}

// ===

type EndpointManagerActor interface {
	Actor
}

// ===

type MDNSManagerActor interface {
	Actor
}
