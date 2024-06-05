package msgsess

import "github.com/shadowjonathan/edup2p/types/key"

type SessionMessage interface {
	MarshalSessionMessage() []byte

	Debug() string
	// todo maybe convert to slog.Group?
}

// ClearMessage represents a full session wire message in decrypted view
type ClearMessage struct {
	Session key.SessionPublic

	Message SessionMessage
}
