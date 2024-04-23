package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
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

	for _, ep := range pi.Endpoints {
		e.tm.SendPingDirect(ep, e.peer, pi.Session)
	}

	e.tm.SendMsgToRelay(
		pi.HomeRelay, e.peer, pi.Session,
		&msg.Rendezvous{MyAddresses: e.tm.S.GetLocalEndpoints()},
	)

	return LogTransition(e, &EstTransmitting{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstPreTransmit) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clear)
}

func (e *EstPreTransmit) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clear)
}
