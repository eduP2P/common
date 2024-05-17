package peer_state

import (
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/key"
	msg2 "github.com/shadowjonathan/edup2p/types/msg"
	"net/netip"
)

type EstPreTransmit struct {
	*EstablishingCommon
}

func (e *EstPreTransmit) Name() string {
	return "pre-transmit(t)"
}

func (e *EstPreTransmit) OnTick() PeerState {
	pi := e.mustPeerInfo()

	for _, ep := range types.SetUnion(pi.Endpoints, pi.RendezvousEndpoints) {
		e.tm.SendPingDirect(ep, e.peer, pi.Session)
	}

	localEndpoints := e.tm.Stage().GetLocalEndpoints()

	// Don't send a rendezvous if we don't have any endpoints.
	if len(localEndpoints) > 0 {
		e.tm.SendMsgToRelay(
			pi.HomeRelay, e.peer, pi.Session,
			&msg2.Rendezvous{MyAddresses: localEndpoints},
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
