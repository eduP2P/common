package peerstate

import (
	"net/netip"

	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
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

func (w *WaitingForInfo) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	s := cascadeDirect(w, ap, clearMsg)

	if s == nil {
		// The state did not cascade, so we log here.
		LogDirectMessage(w, ap, clearMsg)
	}

	return s
}

func (w *WaitingForInfo) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	s := cascadeRelay(w, relay, peer, clearMsg)

	if s == nil {
		// The state did not cascade, so we log here.
		LogRelayMessage(w, relay, peer, clearMsg)
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
