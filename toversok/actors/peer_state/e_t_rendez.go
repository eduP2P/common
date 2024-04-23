package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

// EstRendezGot is a transient state that immediately transitions to EstRendezAck after the first OnTick
type EstRendezGot struct {
	*EstablishingCommon

	m *msg.Rendezvous
}

func (e *EstRendezGot) Name() string {
	return "rendezvous-got(t)"
}

func (e *EstRendezGot) OnTick() PeerState {
	pi := e.mustPeerInfo()

	pi.RendezvousEndpoints = e.m.MyAddresses

	for _, ep := range e.m.MyAddresses {
		e.tm.SendPingDirect(ep, e.peer, pi.Session)
	}

	e.resetDeadline()

	return LogTransition(e, &EstRendezAck{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstRendezGot) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clear)
}

func (e *EstRendezGot) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clear)
}
