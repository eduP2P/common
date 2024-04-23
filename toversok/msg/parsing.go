package msg

import "fmt"

// Session Wire header:
//   Magic (8) + Source (32) + Nacl Box Nonce (24) + encrypted user message.

// Session User messages:
//   Version (1) + Type (1) + Data

const wireHeaderLen = len(Magic) + 32 + naclBoxNonceLen

func LooksLikeSessionWireMessage(pkt []byte) bool {
	if len(pkt) < wireHeaderLen {
		// too short, cant possibly be a wire message
		return false
	}

	return string(pkt[len(Magic)]) == Magic
}

func ParseSessionMessage(usrMsg []byte) (SessionMessage, error) {
	version := usrMsg[0]
	msgType := usrMsg[1]

	specificMsg := usrMsg[2:]

	if VersionMarker(version) != v1 {
		return nil, fmt.Errorf("invalid version: %x", version)
	}

	switch MessageType(msgType) {
	case PingMessage:
		return parsePing(specificMsg)
	case PongMessage:
		return parsePong(specificMsg)
	case RendezvousMessage:
		return parseRendezvous(specificMsg)
	default:
		return nil, fmt.Errorf("invalid message type: %x", msgType)
	}
}

func parsePing(b []byte) (*Ping, error) {
	// TODO
	panic("implement me")
}

func parsePong(b []byte) (*Pong, error) {
	// TODO
	panic("implement me")
}

func parseRendezvous(b []byte) (*Rendezvous, error) {
	// TODO
	panic("implement me")
}
