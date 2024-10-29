package peer_state

import (
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"net/netip"
)

type WaitingForInfo struct {
	*StateCommon
}

func (w *WaitingForInfo) Name() string {
	return "waiting"
}

func (w *WaitingForInfo) OnTick() PeerState {
	if pi := w.tm.Stage().GetPeerInfo(w.peer); pi != nil && !pi.Session.IsZero() {
		return LogTransition(w, &Inactive{StateCommon: w.StateCommon})
	}

	return nil
}

func (w *WaitingForInfo) OnDirect(ap netip.AddrPort, clear *msgsess.ClearMessage) PeerState {
	s := cascadeDirect(w, ap, clear)

	if s == nil {
		// The state did not cascade, so we log here.
		LogDirectMessage(w, ap, clear)
	}

	return s
}

func (w *WaitingForInfo) OnRelay(relay int64, peer key.NodePublic, clear *msgsess.ClearMessage) PeerState {
	s := cascadeRelay(w, relay, peer, clear)

	if s == nil {
		// The state did not cascade, so we log here.
		LogRelayMessage(w, relay, peer, clear)
	}

	return s
}

func MakeWaiting(tm ifaces.TrafficManagerActor, peer key.NodePublic) PeerState {
	w := &WaitingForInfo{
		StateCommon: &StateCommon{peer: peer, tm: tm},
	}
	L(w).Info("initialised")

	return w
}
