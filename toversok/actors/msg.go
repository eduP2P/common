package actors

import (
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msg"
	"net/netip"
)

// Messages

// ======================================================================================================
// TrafficManager msgs

type TManConnActivity struct {
	peer key.NodePublic

	// else its out
	isIn bool

	isActive bool
}

type TManConnGoodBye struct {
	peer key.NodePublic

	// else its out
	isIn bool
}

type TManSessionMessageFromRelay struct {
	relay int64

	peer key.NodePublic

	// session key from the session message
	msg *msg.ClearMessage
}

type TManSessionMessageFromDirect struct {
	addrPort netip.AddrPort

	msg *msg.ClearMessage
}

type TManSetPeerInfo struct {
	peer key.NodePublic

	session   key.SessionPublic
	homeRelay int64
	endpoints []netip.AddrPort
}

// ======================================================================================================
// SessionManager msgs

type SManSessionFrameFromRelay struct {
	relay int64

	peer key.NodePublic

	frameWithMagic []byte
}

type SManSessionFrameFromAddrPort struct {
	addrPort netip.AddrPort

	frameWithMagic []byte
}

type SManSendSessionMessageToRelay struct {
	relay int64

	peer key.NodePublic

	toSession key.SessionPublic

	msg msg.SessionMessage
}

type SManSendSessionMessageToDirect struct {
	addrPort netip.AddrPort

	toSession key.SessionPublic

	msg msg.SessionMessage
}

// ======================================================================================================
// OutConn msgs

type OutConnUse struct {
	useRelay bool

	relayToUse    int64
	addrPortToUse netip.AddrPort
}

// ======================================================================================================
// DirectManager msgs

//type DManSendSessionMessage struct {
//	addrPort netip.AddrPort
//
//	rawFrame []byte
//}

type DManSetMTU struct {
	forAddrPort netip.AddrPort

	mtu uint16
}

// ======================================================================================================
// RelayManager msgs

type RManUpdateRelayConfiguration struct {
	config []types.RelayInformation
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
	peer key.NodePublic
}

type DRouterPeerAddKnownAs struct {
	peer key.NodePublic

	addrPort netip.AddrPort
}
