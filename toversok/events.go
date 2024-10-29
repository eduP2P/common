package toversok

import (
	"github.com/LukaGiorgadze/gonull"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/relay"
	"net/netip"
)

// TODO DEPRECATED, should be refactored into using fake control client and such

type Event interface {
	EventName() string
}

type RelayUpdate struct {
	// Updates relays referenced in this set.
	//
	// Note: Deliberately does not allow for unsetting relays.
	Set []relay.Information
}

func (r RelayUpdate) EventName() string {
	return "RelayUpdate"
}

type PeerAddition struct {
	Key key.NodePublic

	HomeRelayID int64
	SessionKey  key.SessionPublic
	Endpoints   []netip.AddrPort

	VIPs VirtualIPs
}

func (p PeerAddition) EventName() string {
	return "PeerAddition"
}

type PeerUpdate struct {
	Key key.NodePublic

	HomeRelayID gonull.Nullable[int64]
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
