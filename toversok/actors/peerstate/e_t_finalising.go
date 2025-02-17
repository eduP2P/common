package peerstate

import (
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"net/netip"
)

type Finalizing struct {
	*EstablishingCommon

	ap   netip.AddrPort
	sess key.SessionPublic
	pong *msgsess.Pong
}

func (f *Finalizing) Name() string {
	return "finalizing(t)"
}

func (f *Finalizing) OnTick() PeerState {
	f.ackPongDirect(f.ap, f.sess, f.pong)

	return LogTransition(f, &Booting{
		StateCommon: f.StateCommon,
		ap:          f.ap,
	})
}

func (f *Finalizing) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(f, ap, clearMsg)
}

func (f *Finalizing) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(f, relay, peer, clearMsg)
}
