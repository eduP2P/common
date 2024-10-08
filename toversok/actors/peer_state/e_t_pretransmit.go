package peer_state

import (
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	msg2 "github.com/edup2p/common/types/msgsess"
	"net/netip"
)

type EstPreTransmit struct {
	*EstablishingCommon
}

func (e *EstPreTransmit) Name() string {
	return "pre-transmit(t)"
}

func (e *EstPreTransmit) OnTick() PeerState {
	pi := e.getPeerInfo()
	if pi == nil {
		// Peer info unavailable
		return nil
	}

	for _, ep := range types.SetUnion(pi.Endpoints, pi.RendezvousEndpoints) {
		e.tm.SendPingDirect(ep, e.peer, pi.Session)
	}

	endpoints := e.tm.Stage().GetEndpoints()

	// Don't send a rendezvous if we don't have any endpoints.
	if len(endpoints) > 0 {
		e.tm.SendMsgToRelay(
			pi.HomeRelay, e.peer, pi.Session,
			&msg2.Rendezvous{MyAddresses: endpoints},
		)
	}

	return LogTransition(e, &EstTransmitting{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstPreTransmit) OnDirect(ap netip.AddrPort, clear *msg2.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clear)
}

func (e *EstPreTransmit) OnRelay(relay int64, peer key.NodePublic, clear *msg2.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clear)
}
