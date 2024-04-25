package relay

import (
	"bufio"
)

type FrameType byte

// TODO consider if putting the byte explicitly will make sure they dont get misaligned in the future
const (
	frameServerKey FrameType = iota
	frameClientInfo
	frameServerInfo

	// packets sent and received from the relay
	frameSendPacket
	frameRecvPacket

	// Pings sent by the client and Ponged (acknowledged) by the server
	framePing
	framePong

	// Keepalive frames sent by the server at an interval
	frameKeepAlive
)

func readFrameHeader(reader *bufio.Reader) (typ FrameType, frameLen uint32, err error) {
	tb, err := reader.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	frameLen, err = readUint32(reader)
	if err != nil {
		return 0, 0, err
	}
	return FrameType(tb), frameLen, nil
}

func writeFrameHeader(bw *bufio.Writer, typ FrameType, frameLen uint32) error {
	if err := bw.WriteByte(byte(typ)); err != nil {
		return err
	}
	return writeUint32(bw, frameLen)
}
