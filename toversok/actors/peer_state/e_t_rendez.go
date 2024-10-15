package peer_state

import (
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
)

// EstRendezGot is a transient state that immediately transitions to EstRendezAck after the first OnTick
type EstRendezGot struct {
	*EstablishingCommon

	m *msg2.Rendezvous
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

	e.resetDeadline()

	return LogTransition(e, &EstRendezAck{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstRendezGot) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clear)
}

func (e *EstRendezGot) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clear)
}
