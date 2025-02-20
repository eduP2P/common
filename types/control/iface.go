package control

import (
	"errors"
	"net/netip"
	"time"

	"github.com/edup2p/common/types/key"
)

type (
	ClientID key.NodePublic
	SessID   string
)

var (
	ErrSessionDoesNotExist        = errors.New("session does not exist")
	ErrSessionIsNotAuthenticating = errors.New("session is not authenticating")
	ErrNeedsDisconnect            = errors.New("session needs disconnect")
	ErrClientNotConnected         = errors.New("client is not connected")
)

// ServerLogic denotes exposed functions that a control server must provide for any business logic to interface with it.
type ServerLogic interface {
	// RegisterCallbacks with custom business logic
	RegisterCallbacks(ServerCallbacks)

	// GetClientID will get the client ID corresponding to a particular session ID.
	// Will error if session does not exist.
	GetClientID(SessID) (ClientID, error)

	// GetConnectedClients returns all connected session and their client IDs.
	GetConnectedClients() (map[SessID]ClientID, error)

	/// The following functions pertain to the authentication flow.

	// SendAuthURL will send the authentication URL to the indicated session ID.
	// Must only be called once, will error on a second time.
	// Will error if the session is not pending authentication.
	SendAuthURL(id SessID, url string) error
	// AcceptAuthentication will accept the pending authentication of the indicated session ID.
	// Must be called, or RejectAuthentication must be called.
	// Second time argument dictates for how long the
	// Will error if the session is not pending authentication.
	AcceptAuthentication(SessID) error
	// RejectAuthentication will reject the pending authentication of the indicated session ID.
	// Must be called, or AcceptAuthentication must be called.
	// Will error if the session is not pending authentication.
	RejectAuthentication(id SessID, reason string) error

	// DisconnectSession will disconnect a running client session (and invalidate its ID), if it exists.
	// Will error if session does not exist.
	DisconnectSession(id SessID) error
	// DisconnectClient will disconnect a running session per client (and invalidate its ID), if its connected.
	// Will error if client is not connected.
	DisconnectClient(id ClientID) error

	/// The following functions pertain to client-client networking visibility.

	// GetVisibilityPairs gets all pairs of a particular ClientID.
	// Will error if client ID does not have any pairs, or if client ID is unknown.
	GetVisibilityPairs(ClientID) (map[ClientID]VisibilityPair, error)
	// UpsertVisibilityPair will insert or update a VisibilityPair for a pair of clients.
	UpsertVisibilityPair(ClientID, ClientID, VisibilityPair) error
	// UpsertMultiVisibilityPair will insert or update multiple VisibilityPair's for pairs of clients.
	UpsertMultiVisibilityPair(ClientID, map[ClientID]VisibilityPair) error
	// RemoveVisibilityPair will delete a VisibilityPair between clients.
	// Idempotent, will not error if no pair exists.
	RemoveVisibilityPair(ClientID, ClientID) error
}

// ServerCallbacks denotes all the functions the corresponding business logic to the control server must implement,
// to notify the business logic of certain changes and events. See ServerLogic.RegisterCallbacks.
//
// Calling back into any ServerLogic function while being called back is safe.
// TODO double-check above statement
type ServerCallbacks interface {
	// OnSessionCreate is called when a new session is created.
	// Indicates a pending authentication state, which will hang unless resolved;
	// Call ServerLogic.AcceptAuthentication or ServerLogic.RejectAuthentication to end this state.
	OnSessionCreate(SessID, ClientID)
	// OnSessionResume is called when a session is resumed.
	OnSessionResume(SessID, ClientID)

	// OnDeviceKey is called when a device key has been received.
	// Will only happen when session is pending authentication.
	OnDeviceKey(id SessID, key string)

	// OnSessionFinalize is called right after ServerLogic.AcceptAuthentication, but before that message is sent to the client.
	// The client needs to known which virtual IPs it can use, and the expiry time of the authentication,
	// and this function will provide it to the control server.
	OnSessionFinalize(SessID, ClientID) (netip.Prefix, netip.Prefix, time.Time)

	// OnSessionDestroy is called after the client has been disconnected.
	OnSessionDestroy(SessID, ClientID)
}
