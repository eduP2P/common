package msgcontrol

import (
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/relay"
)

type ControlMessageType byte

const (
	ClientHelloType ControlMessageType = iota
	ServerHelloType
	LogonType
	LogonAuthenticateType
	LogonDeviceKeyType
	LogonAcceptType
	LogonRejectType
	LogoutType
	PingType
	PongType
)

const (
	EndpointUpdateType ControlMessageType = (1 << 8) - (iota + 1)
	HomeRelayUpdateType
	PeerAdditionType
	PeerUpdateType
	PeerRemoveType
	RelayUpdateType
)

// === handshake phase

type ClientHello struct {
	ClientNodePub key.NodePublic
}

type ServerHello struct {
	ControlNodePub key.ControlPublic

	// random data encrypted with shared key (control priv x client pub)
	// to be signed with shared key of nodekey and sesskey
	CheckData []byte
}

type Logon struct {
	SessKey key.SessionPublic

	// check data encrypted with sharedkey (control pub x nodekey priv)
	NodeKeyAttestation []byte
	// check data encrypted with sharedkey (control pub x sesskey priv)
	SessKeyAttestation []byte

	ResumeSessionID *string `json:",omitempty"`
}

type LogonAuthenticate struct {
	AuthenticateURL string

	// is held in suspense until URL is authenticated, then LogonAccept
}

type LogonDeviceKey struct {
	DeviceKey string

	// Device key that's sent to the server after LogonAuthenticate
}

type LogonAccept struct {
	IP4 netip.Prefix
	IP6 netip.Prefix

	SessionID string
}

type RetryStrategyType byte

const (
	// Cannot retry, abort
	NoRetryStrategy RetryStrategyType = iota
	// Regenerate session Key
	RegenerateSessionKey
	// Retry without session ID
	RecreateSession
)

//goland:noinspection GoDirectComparisonOfErrors
func (r RetryStrategyType) Error() string {
	switch r {
	case NoRetryStrategy:
		return "cannot retry"
	case RegenerateSessionKey:
		return "retry by regenerating session key"
	case RecreateSession:
		return "retry by recreating session"
	default:
		panic("unknown retry strategy type")
	}
}

type LogonReject struct {
	Reason string

	RetryStrategy RetryStrategyType `json:",omitempty"`

	RetryAfter time.Duration `json:",omitempty"`
}

type Logout struct{}

type Ping struct {
	// random data encrypted with shared key (control priv x client pub)
	// to be signed with shared key of nodekey and sesskey
	CheckData []byte
}

type Pong struct {
	// check data encrypted with sharedkey (control pub x nodekey priv)
	NodeKeyAttestation []byte
	// check data encrypted with sharedkey (control pub x sesskey priv)
	SessKeyAttestation []byte
}

// === during session

// -> control
type EndpointUpdate struct {
	Endpoints []netip.AddrPort
}

// -> control
type HomeRelayUpdate struct {
	HomeRelay int64
}

// -> client
type PeerAddition struct {
	PubKey  key.NodePublic
	SessKey key.SessionPublic

	IPv4 netip.Addr
	IPv6 netip.Addr

	Endpoints []netip.AddrPort
	HomeRelay int64

	Properties Properties
}

type Properties struct {
	Quarantine bool
	MDNS       bool
}

// -> client
type PeerUpdate struct {
	PubKey key.NodePublic

	SessKey   *key.SessionPublic `json:",omitempty"`
	Endpoints []netip.AddrPort   `json:",omitempty"`
	HomeRelay *int64             `json:",omitempty"`

	Properties *Properties `json:",omitempty"`
}

// -> client
type PeerRemove struct {
	PubKey key.NodePublic
}

// -> client
type RelayUpdate struct {
	Relays []relay.Information
}
