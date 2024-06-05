package peer_state

import (
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgsess"
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

	return LogTransition(b, &Established{
		StateCommon:      b.StateCommon,
		lastPingRecv:     time.Now(),
		lastPongRecv:     time.Now(),
		nextPingDeadline: time.Now(),
		inactive:         false,
		currentEndpoint:  b.ap,
	})
}

func (b *Booting) OnDirect(ap netip.AddrPort, clear *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(b, ap, clear)
}

func (b *Booting) OnRelay(relay int64, peer key.NodePublic, clear *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(b, relay, peer, clear)
}
