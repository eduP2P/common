package toversok

import (
	"github.com/LukaGiorgadze/gonull"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
)

type Event interface {
	EventName() string
}

type RelayUpdate struct {
	// Updates relays referenced in this set.
	//
	// Note: Deliberately does not allow for unsetting relays.
	set []types.RelayInformation
}

func (r RelayUpdate) EventName() string {
	return "RelayUpdate"
}

type PeerAddition struct {
	key key.NodePublic

	homeRelayId int64
	sessionKey  key.SessionPublic
	ips         []netip.Addr
}

func (p PeerAddition) EventName() string {
	return "PeerAddition"
}

type PeerRemoval struct {
	key key.NodePublic
}

func (p PeerRemoval) EventName() string {
	return "PeerRemoval"
}

type PeerUpdate struct {
	key key.NodePublic

	homeRelayId gonull.Nullable[int64]
	sessionKey  gonull.Nullable[key.SessionPublic]
	ips         gonull.Nullable[[]netip.Addr]
}

func (p PeerUpdate) EventName() string {
	return "PeerUpdate"
}
