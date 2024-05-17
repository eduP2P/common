package relay

type ClientInfo struct {
	// CanAckPings is whether the client wants to receive keepalives over this connection, default true.
	SendKeepalive bool
}

type ServerInfo struct {
	TokenBucketBytesPerSecond int `json:",omitempty"`
	TokenBucketBytesBurst     int `json:",omitempty"`
}
