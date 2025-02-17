package peerstate

import (
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"net/netip"
	"time"
)

// EstRendezGot is a transient state that immediately transitions to EstRendezAck after the first OnTick
type EstRendezGot struct {
	*EstablishingCommon

	m *msgsess.Rendezvous
}

func (e *EstRendezGot) Name() string {
	return "rendezvous-got(t)"
}

func (e *EstRendezGot) OnTick() PeerState {
	pi := e.getPeerInfo()
	if pi == nil {
		// Peer info unavailable
		return nil
	}

	pi.RendezvousEndpoints = types.NormaliseAddrPortSlice(e.m.MyAddresses)

	for _, ep := range e.m.MyAddresses {
		e.tm.SendPingDirect(ep, e.peer, pi.Session)
	}

	e.lastPing = time.Now()

	e.resetDeadline()

	return LogTransition(e, &EstRendezAck{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstRendezGot) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clearMsg)
}

func (e *EstRendezGot) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clearMsg)
}
