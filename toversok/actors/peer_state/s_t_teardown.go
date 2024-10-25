package peer_state

import (
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"net/netip"
	"time"
)

type Teardown struct {
	*StateCommon

	// if true, transition to Inactive afterward
	inactive bool
}

func (t *Teardown) Name() string {
	return "teardown(t)"
}

func (t *Teardown) OnTick() PeerState {
	t.tm.DManClearAKA(t.peer)
	t.tm.OutConnTrackHome(t.peer)

	if t.inactive {
		L(t).Info("DROPPED direct peer connection (due to inactivity)", "peer", t.peer.Debug())

		return LogTransition(t, &Inactive{
			StateCommon: t.StateCommon,
		})
	} else {
		L(t).Info("LOST direct peer connection", "peer", t.peer.Debug())

		return LogTransition(t, &Trying{
			StateCommon: t.StateCommon,
			tryAt:       time.Now(),
		})
	}
}

func (t *Teardown) OnDirect(ap netip.AddrPort, clear *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(t, ap, clear)
}

func (t *Teardown) OnRelay(relay int64, peer key.NodePublic, clear *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(t, relay, peer, clear)
}
