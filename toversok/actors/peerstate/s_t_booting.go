package peerstate

import (
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"net/netip"
	"time"
)

type Booting struct {
	*StateCommon

	ap netip.AddrPort
}

func (b *Booting) Name() string {
	return "booting(t)"
}

func (b *Booting) OnTick() PeerState {
	b.tm.OutConnUseAddrPort(b.peer, b.ap)
	b.tm.DManSetAKA(b.peer, b.ap)

	L(b).Info("ESTABLISHED direct peer connection", "peer", b.peer.Debug(), "via", b.ap.String())

	return LogTransition(b, &Established{
		StateCommon:        b.StateCommon,
		lastPingRecv:       time.Now(),
		lastPongRecv:       time.Now(),
		nextPingDeadline:   time.Now(),
		inactive:           false,
		currentOutEndpoint: b.ap,
		knownInEndpoints:   map[netip.AddrPort]bool{types.NormaliseAddrPort(b.ap): true},
	})
}

func (b *Booting) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(b, ap, clearMsg)
}

func (b *Booting) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(b, relay, peer, clearMsg)
}
