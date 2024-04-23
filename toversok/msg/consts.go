package msg

// Magic is the 8 byte header of all session messages
// F0 9F AA 84
// F0 9F A7 A6
const Magic = "🪄🧦"

var MagicBytes = []byte(Magic)

type VersionMarker byte

const v1 = VersionMarker(0x1)

type MessageType byte

const (
	PingMessage       = MessageType(0x00)
	PongMessage       = MessageType(0x01)
	RendezvousMessage = MessageType(0xFF)
)

const naclBoxNonceLen = 24
