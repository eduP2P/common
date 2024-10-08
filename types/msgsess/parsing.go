package msgsess

import (
	"errors"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"net/netip"
)

// Session Wire header:
//   Magic (8) + Source (32) + Nacl Box Nonce (24) + encrypted user message.

// Session User messages:
//   Version (1) + Type (1) + Data

const wireHeaderLen = len(Magic) + key.Len + NaclBoxNonceLen

func LooksLikeSessionWireMessage(pkt []byte) bool {
	if len(pkt) < wireHeaderLen {
		// too short, cant possibly be a wire message
		return false
	}

	return string(pkt[:len(Magic)]) == Magic
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

var errTooSmall = errors.New("session message too small")

func parsePing(b []byte) (*Ping, error) {
	if len(b) < key.Len+12 {
		return nil, errTooSmall
	}

	txid := [12]byte(b[:12])
	b = b[12:]
	nKey := key.NodePublic(b[:key.Len])

	return &Ping{
		TxID:    txid,
		NodeKey: nKey,
	}, nil
}

func parsePong(b []byte) (*Pong, error) {
	if len(b) < 12+16+2 {
		return nil, errTooSmall
	}

	txid := [12]byte(b[:12])
	b = b[12:]

	ap := types.ParseAddrPort([18]byte(b[:18]))

	return &Pong{TxID: txid, Src: ap}, nil
}

func parseRendezvous(b []byte) (*Rendezvous, error) {
	if len(b)%18 != 0 {
		return nil, errors.New("malformed rendezvous addresses")
	}

	aps := make([]netip.AddrPort, 0)

	for {
		ap := types.ParseAddrPort([18]byte(b[:18]))
		aps = append(aps, ap)
		b = b[18:]

		if len(b) == 0 {
			break
		}
	}

	return &Rendezvous{MyAddresses: aps}, nil
}
