package msgsess

// Magic is the 8 byte header of all session messages
// "🪄🧦"
// F0 9F AA 84
// F0 9F A7 A6
var Magic = string(MagicBytes)

var MagicBytes = []byte{0xF0, 0x9F, 0xAA, 0x84, 0xF0, 0x9F, 0xA7, 0xA6}

type VersionMarker byte

const v1 = VersionMarker(0x1)

type MessageType byte

const (
	PingMessage       = MessageType(0x00)
	PongMessage       = MessageType(0x01)
	RendezvousMessage = MessageType(0xFF)
)

const NaclBoxNonceLen = 24
