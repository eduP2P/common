package relay

import (
	"bufio"

	"github.com/edup2p/common/types"
)

type FrameType byte

// TODO consider if putting the byte explicitly will make sure they dont get misaligned in the future
const (
	frameServerKey  FrameType = iota // 32B public key
	frameClientInfo                  // 32B pub key + naclbox(24B nonce + json)
	frameServerInfo                  // naclbox(24B nonce + json)

	// packets sent and received from the relay
	frameSendPacket // 32B dest pub key + packet bytes
	frameRecvPacket // 32B src pub key + packet bytes

	// Pings sent by the client and Ponged (acknowledged) by the server
	framePing // 8B payload
	framePong // 8B payload

	// Keepalive frames sent by the server at an interval
	frameKeepAlive // 0B
)

func readFrameHeader(reader *bufio.Reader) (typ FrameType, frameLen uint32, err error) {
	tb, err := reader.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	frameLen, err = types.ReadUint32(reader)
	if err != nil {
		return 0, 0, err
	}
	return FrameType(tb), frameLen, nil
}

func writeFrameHeader(bw *bufio.Writer, typ FrameType, frameLen uint32) error {
	if err := bw.WriteByte(byte(typ)); err != nil {
		return err
	}
	return types.WriteUint32(bw, frameLen)
}
