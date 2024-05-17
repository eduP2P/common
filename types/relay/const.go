package relay

import "time"

const (
	UpgradeProtocol = "toversok-relay"
)

type ProtocolVersion byte

const (
	relayProtocolV0 ProtocolVersion = 0
)

const (
	MaxPacketSize              = 64 << 10
	ServerClientKeepAlive      = 15 * time.Second
	ServerClientWriteTimeout   = 5 * time.Second
	ServerClientSendQueueDepth = 32 // packets buffered for sending
)
