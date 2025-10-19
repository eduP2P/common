package peerstate

import (
	"net/netip"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
)

type EstPreTransmit struct {
	*EstablishingCommon
}

func (e *EstPreTransmit) Name() string {
	return "pre-transmit(t)"
}

func (e *EstPreTransmit) OnTick() PeerState {
	pi := e.sendPingsToPeer()
	if pi == nil {
		// Peer info unavailable
		return nil
	}

	endpoints := e.tm.Stage().GetEndpoints()

	// Don't send a rendezvous if we don't have any endpoints.
	if len(endpoints) > 0 {
		e.tm.SendMsgToRelay(
			pi.HomeRelay, e.peer, pi.Session,
			&msgsess.Rendezvous{MyAddresses: endpoints},
		)
	}

	return LogTransition(e, &EstTransmitting{EstablishingCommon: e.EstablishingCommon})
}

func (e *EstPreTransmit) OnDirect(ap netip.AddrPort, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeDirect(e, ap, clearMsg)
}

func (e *EstPreTransmit) OnRelay(relay int64, peer key.NodePublic, clearMsg *msgsess.ClearMessage) PeerState {
	// OnTick will transition into the next state regardless, so just pass it along
	return cascadeRelay(e, relay, peer, clearMsg)
}
