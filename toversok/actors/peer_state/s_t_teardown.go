package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
	"time"
)

type Teardown struct {
	*StateCommon

	// if true, transition to Inactive afterward
	inactive bool

	// if not inactive, set the tryAt parameter for Trying
	tryAt time.Time
}

func (t *Teardown) Name() string {
	return "teardown(t)"
}

func (t *Teardown) OnTick() PeerState {
	t.tm.DManClearAKA(t.peer)
	t.tm.OutConnUseRelay(t.peer, t.mustPeerInfo().HomeRelay)

	if t.inactive {
		return LogTransition(t, &Inactive{
			StateCommon: t.StateCommon,
		})
	} else {
		return LogTransition(t, &Trying{
			StateCommon: t.StateCommon,
			tryAt:       t.tryAt,
		})
	}
}

func (t *Teardown) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(t, ap, clear)
}

func (t *Teardown) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(t, relay, peer, clear)
}
