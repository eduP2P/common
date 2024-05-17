package actor_msg

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msg"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/netip"
)

// Messages

// ======================================================================================================
// TrafficManager msgs

type TManConnActivity struct {
	Peer key.NodePublic

	// else its out
	IsIn bool

	IsActive bool
}

type TManConnGoodBye struct {
	Peer key.NodePublic

	// else its out
	IsIn bool
}

type TManSessionMessageFromRelay struct {
	Relay int64

	Peer key.NodePublic

	// session key from the session message
	Msg *msg.ClearMessage
}

type TManSessionMessageFromDirect struct {
	AddrPort netip.AddrPort

	Msg *msg.ClearMessage
}

// ======================================================================================================
// SessionManager msgs

type SManSessionFrameFromRelay struct {
	Relay int64

	Peer key.NodePublic

	FrameWithMagic []byte
}

type SManSessionFrameFromAddrPort struct {
	AddrPort netip.AddrPort

	FrameWithMagic []byte
}

type SManSendSessionMessageToRelay struct {
	Relay int64

	Peer key.NodePublic

	ToSession key.SessionPublic

	Msg msg.SessionMessage
}

type SManSendSessionMessageToDirect struct {
	AddrPort netip.AddrPort

	ToSession key.SessionPublic

	Msg msg.SessionMessage
}

// ======================================================================================================
// OutConn msgs

type OutConnUse struct {
	UseRelay  bool
	TrackHome bool

	RelayToUse    int64
	AddrPortToUse netip.AddrPort
}

// ======================================================================================================
// DirectManager msgs

//type DManSendSessionMessage struct {
//	AddrPort netip.AddrPort
//
//	rawFrame []byte
//}

type DManSetMTU struct {
	ForAddrPort netip.AddrPort

	MTU uint16
}

// ======================================================================================================
// RelayManager msgs

type RManRelayLatencyResults struct {
	// TODO
}

//type RManSendSessionMessage struct {
//	relay int64
//
//	dst key.NodePublic
//
//	rawFrame []byte
//}

// ======================================================================================================
// DirectRouter msgs

type DRouterPeerClearKnownAs struct {
	Peer key.NodePublic
}

type DRouterPeerAddKnownAs struct {
	Peer key.NodePublic

	AddrPort netip.AddrPort
}

// ====

type SyncPeerInfo struct {
	Peer key.NodePublic
}

type UpdateRelayConfiguration struct {
	Config []relay.RelayInformation
}
