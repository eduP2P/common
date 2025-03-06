// Package msgsess contains session message definitions and parsing methods, to be sent over relay or directly.
//
// Session message interface definitions are sealed within this package.
package msgsess

import "github.com/edup2p/common/types/key"

type SessionMessage interface {
	Marshal() []byte

	// todo maybe convert to slog.Group?
	Debug() string

	Parse([]byte) error
}

// ClearMessage represents a full session wire message in decrypted view
type ClearMessage struct {
	Session key.SessionPublic

	Message SessionMessage
}
