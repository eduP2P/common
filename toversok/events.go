package toversok

import (
	"github.com/LukaGiorgadze/gonull"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/netip"
)

type Event interface {
	EventName() string
}

type RelayUpdate struct {
	// Updates relays referenced in this set.
	//
	// Note: Deliberately does not allow for unsetting relays.
	Set []relay.RelayInformation
}

func (r RelayUpdate) EventName() string {
	return "RelayUpdate"
}

type PeerAddition struct {
	Key key.NodePublic

	HomeRelayId int64
	SessionKey  key.SessionPublic
	Endpoints   []netip.AddrPort

	VIPs VirtualIPs
}

func (p PeerAddition) EventName() string {
	return "PeerAddition"
}

type PeerUpdate struct {
	Key key.NodePublic

	HomeRelayId gonull.Nullable[int64]
	SessionKey  gonull.Nullable[key.SessionPublic]
	Endpoints   gonull.Nullable[[]netip.AddrPort]
}

func (p PeerUpdate) EventName() string {
	return "PeerUpdate"
}

type PeerRemoval struct {
	Key key.NodePublic
}

func (p PeerRemoval) EventName() string {
	return "PeerRemoval"
}
