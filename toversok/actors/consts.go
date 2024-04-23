package actors

import "time"

const (
	SockRecvReadTimeout  = 5 * time.Second
	ConnActivityInterval = 10 * time.Second

	OutConnInboxChanBuffer = 10

	SockRecvFrameChanBuffer = 10
	InConnFrameChanBuffer   = 10

	// DefaultSafeMTU is a small MTU that'S safe, absent other information.
	DefaultSafeMTU uint16 = 1280
)
