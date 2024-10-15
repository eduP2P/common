package peer_state

import (
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
)

type EstHalfIng struct {
	*EstablishingCommon

	ap   netip.AddrPort
	sess key.SessionPublic
	ping *msg2.Ping
}

func (e *EstHalfIng) Name() string {
	return "half-establishing(t)"
}

func (e *EstHalfIng) OnTick() PeerState {
	e.replyWithPongDirect(e.ap, e.sess, e.ping)

	e.tm.SendPingDirect(e.ap, e.peer, e.sess)

	return LogTransition(e, &EstHalf{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstHalfIng) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clear)
}

func (e *EstHalfIng) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clear)
}
