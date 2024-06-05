package ifaces

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgactor"
	"github.com/shadowjonathan/edup2p/types/msgsess"
	"github.com/shadowjonathan/edup2p/types/stage"
	"net/netip"
)

type Actor interface {
	Run()

	Inbox() chan<- msgactor.ActorMessage

	// Cancel this actor's context.
	Cancel()

	// Close is called by the actor's Run loop when cancelled.
	Close()
}

// ===

type DirectManagerActor interface {
	Actor

	WriteTo(pkt []byte, addr netip.AddrPort)
}

type DirectedPeerFrame struct {
	SrcAddrPort netip.AddrPort

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
