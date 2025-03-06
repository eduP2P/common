package msgsess

import (
	"errors"
	"fmt"
	"github.com/edup2p/common/types/key"
)

// Session Wire header:
//   Magic (8) + Source (32) + Nacl Box Nonce (24) + encrypted user message.

// Session User messages:
//   Version (1) + Type (1) + Data

var wireHeaderLen = len(Magic) + key.Len + NaclBoxNonceLen

func LooksLikeSessionWireMessage(pkt []byte) bool {
	if len(pkt) < wireHeaderLen {
		// too short, cant possibly be a wire message
		return false
	}

	return string(pkt[:len(Magic)]) == Magic
}

func ParseSessionMessage(usrMsg []byte) (SessionMessage, error) {
	version := usrMsg[0]
	msgType := usrMsg[1]

	specificMsg := usrMsg[2:]

	if VersionMarker(version) != v1 {
		return nil, fmt.Errorf("invalid version: %x", version)
	}

	var msg SessionMessage

	switch MessageType(msgType) {
	case PingMessage:
		msg = new(Ping)
	case PongMessage:
		msg = new(Pong)
	case RendezvousMessage:
		msg = new(Rendezvous)
	case SideBandDataMessage:
		msg = new(SideBandData)
	default:
		return nil, fmt.Errorf("invalid message type: %x", msgType)
	}

	if err := msg.Parse(specificMsg); err != nil {
		return nil, fmt.Errorf("failed to parse message of type %d: %w", msgType, err)
	}

	return msg, nil
}

var errTooSmall = errors.New("session message too small")
