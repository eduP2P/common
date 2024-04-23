package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/actors"
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type WaitingForInfo struct {
	*StateCommon
}

func (w *WaitingForInfo) Name() string {
	return "waiting"
}

func (w *WaitingForInfo) OnTick() PeerState {
	if pi, ok := w.tm.PeerInfo[w.peer]; ok && !pi.Session.IsZero() {
		return LogTransition(w, &Inactive{StateCommon: w.StateCommon})
	} else {
		return nil
	}
}

func (w *WaitingForInfo) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	s := cascadeDirect(w, ap, clear)

	if s == nil {
		// The state did not cascade, so we log here.
		LogDirectMessage(w, ap, clear)
	}

	return s
}

func (w *WaitingForInfo) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	s := cascadeRelay(w, relay, peer, clear)

	if s == nil {
		// The state did not cascade, so we log here.
		LogRelayMessage(w, relay, peer, clear)
	}

	return s
}

func MakeWaiting(tm *actors.TrafficManager, peer key.NodePublic) PeerState {
	w := &WaitingForInfo{
		StateCommon: &StateCommon{peer: peer, tm: tm},
	}
	L(w).Info("initialised")

	return w
}
