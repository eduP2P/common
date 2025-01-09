package actors

import "time"

const (
	SockRecvReadTimeout  = 5 * time.Second
	ConnActivityInterval = 10 * time.Second

	// DefaultSafeMTU is a small MTU that's safe, absent other information.
	DefaultSafeMTU uint16 = 1280

	// Inbox
	OutConnInboxChanBuffer = 10
	SessManInboxChLen      = 32
	TrafficManInboxChLen   = 16
	RelayManInboxChLen     = 4
	DirectRouterInboxChLen = 4

	// Frame
	SockRecvFrameChanBuffer = 256
	InConnFrameChanBuffer   = 512

	RelayManFrameChLen      = 8
	RelayManWriteChLen      = 8
	RelayRouterFrameChLen   = 4
	RelayConnSendBufferSize = 32

	DirectManWriteChLen    = 4 * 16
	DirectRouterFrameChLen = 4 * 16

	// Misc

	TManTickerInterval = time.Millisecond * 250

	EManTickerInterval = time.Second * 60

	EManStunTimeout = time.Second * 10

	RelayConnectionRetryInterval = time.Second * 5

	RelayConnectionIdleAfter = time.Minute * 1
)
