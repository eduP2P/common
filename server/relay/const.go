package relay

import "time"

const (
	UpgradeProtocolV0 = "toversok-relay-v0"
)

const (
	MaxPacketSize              = 64 << 10
	ServerClientKeepAlive      = 60 * time.Second
	ServerClientWriteTimeout   = 5 * time.Second
	ServerClientSendQueueDepth = 32 // packets buffered for sending
)
