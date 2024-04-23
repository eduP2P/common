package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type Finalizing struct {
	*EstablishingCommon

	ap   netip.AddrPort
	sess key.SessionPublic
	pong *msg.Pong
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

func (f *Finalizing) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(f, ap, clear)
}

func (f *Finalizing) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(f, relay, peer, clear)
}
