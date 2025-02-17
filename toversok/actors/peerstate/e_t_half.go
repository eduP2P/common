package peerstate

import (
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
)

type EstHalfIng struct {
	*EstablishingCommon

	ap   netip.AddrPort
	sess key.SessionPublic
	ping *msgsess.Ping
}

func (e *EstHalfIng) Name() string {
	return "half-establishing(t)"
}

func (e *EstHalfIng) OnTick() PeerState {
	e.replyWithPongDirect(e.ap, e.sess, e.ping)

	e.tm.SendPingDirect(e.ap, e.peer, e.sess)
	e.lastPing = time.Now()

	return LogTransition(e, &EstHalf{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstHalfIng) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clearMsg)
}

func (e *EstHalfIng) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clearMsg)
}
