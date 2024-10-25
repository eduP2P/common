package msgactor

import (
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/relay"
	"net/netip"
	"time"
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
	Msg *msgsess.ClearMessage
}

type TManSessionMessageFromDirect struct {
	AddrPort netip.AddrPort

	Msg *msgsess.ClearMessage
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

	Msg msgsess.SessionMessage
}

type SManSendSessionMessageToDirect struct {
	AddrPort netip.AddrPort

	ToSession key.SessionPublic

	Msg msgsess.SessionMessage
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
	RelayLatency map[int64]time.Duration
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

type DRouterPushSTUN struct {
	Packets map[netip.AddrPort][]byte
}

// ====

type EManSTUNResponse struct {
	Endpoint netip.AddrPort

	Packet []byte

	Timestamp time.Time
}

// ====

type SyncPeerInfo struct {
	Peer key.NodePublic
}

type UpdateRelayConfiguration struct {
	Config []relay.Information
}
