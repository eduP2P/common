package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/actors"
	"github.com/shadowjonathan/edup2p/types/key"
	msg2 "github.com/shadowjonathan/edup2p/types/msg"
	"net/netip"
	"time"
)

const (
	EstablishmentTimeout        = time.Second * 10
	EstablishmentRetryMax       = time.Minute * 10
	EstablishedPingTimeout      = time.Second * 5
	ConnectionInactivityTimeout = time.Minute
)

type StateCommon struct {
	tm   *actors.TrafficManager
	peer key.NodePublic
}

func (sc *StateCommon) Peer() key.NodePublic {
	return sc.peer
}

func (sc *StateCommon) pingDirectValid(ap netip.AddrPort, sess key.SessionPublic, ping *msg2.Ping) bool {
	return sc.tm.ValidKeys(ping.NodeKey, sess)
}

func (sc *StateCommon) replyWithPongDirect(ap netip.AddrPort, sess key.SessionPublic, ping *msg2.Ping) {
	sc.tm.SendMsgToDirect(ap, sess, &msg2.Pong{
		TxID: ping.TxID,
		Src:  ap,
	})
}

func (sc *StateCommon) pingRelayValid(relay int64, node key.NodePublic, sess key.SessionPublic, ping *msg2.Ping) bool {
	return sc.tm.ValidKeys(ping.NodeKey, sess)
}

func (sc *StateCommon) replyWithPongRelay(relay int64, node key.NodePublic, sess key.SessionPublic, ping *msg2.Ping) {
	sc.tm.SendMsgToRelay(relay, node, sess, &msg2.Pong{
		TxID: ping.TxID,
	})
}

// TODO add bool here and checks by callers
func (sc *StateCommon) ackPongDirect(ap netip.AddrPort, sess key.SessionPublic, pong *msg2.Pong) {
	sent, ok := sc.tm.Pings[pong.TxID]
	if !ok {
		// TODO log: Got pong for unknown ping
		return
	}

	if sent.ToRelay {
		// TODO log: got direct pong to relay ping
		return
	}

	if !sc.tm.ValidKeys(sc.peer, sess) {
		// ?? Somehow the pong is for a valid ping to a node that no longer has this session key?
		// Might happen between restarts, log and ignore.
		// TODO log
		return
	}

	// TODO more checks? (permissive, but log)

	delete(sc.tm.Pings, pong.TxID)
}

// TODO add bool here and checks by callers
func (sc *StateCommon) ackPongRelay(relay int64, node key.NodePublic, sess key.SessionPublic, pong *msg2.Pong) {

	// Relay pongs should come in response to relay pings, note if it is different.
	sent, ok := sc.tm.Pings[pong.TxID]

	if !ok {
		// TODO log: Got pong for unknown ping
		return
	}

	if !sent.ToRelay {
		// TODO log: got relay reply to direct ping
		return
	}

	if !sc.tm.ValidKeys(node, sess) {
		// TODO log
		return
	}

	if !sc.tm.ValidKeys(sent.To, sess) {
		// ?? Somehow the pong is for a valid ping to a node that no longer has this session key?
		// Might happen between restarts, log and ignore.
		// TODO log
		return
	}

	// TODO more checks? (permissive, but log)

	delete(sc.tm.Pings, pong.TxID)

}

func (sc *StateCommon) mustPeerInfo() *actors.PeerInfo {
	pi, ok := sc.tm.PeerInfo[sc.peer]
	if !ok {
		panic("Expected peerinfo at state, but got no peerinfo")
	}
	return pi
}

type EstablishingCommon struct {
	*StateCommon

	deadline time.Time
	attempt  int
}

func mkEstComm(sc *StateCommon, attempts int) *EstablishingCommon {
	ec := &EstablishingCommon{StateCommon: sc, attempt: attempts + 1}
	ec.resetDeadline()
	return ec
}

func (ec *EstablishingCommon) resetDeadline() {
	ec.deadline = time.Now().Add(EstablishmentTimeout)
}

func (ec *EstablishingCommon) expired() bool {
	return time.Now().After(ec.deadline)
}

func (ec *EstablishingCommon) retry() *Trying {
	return &Trying{
		StateCommon: ec.StateCommon,
		attempts:    ec.attempt,
		tryAt:       time.Now().Add(getRetryDelay(ec.attempt)),
	}
}

func getRetryDelay(attempts int) time.Duration {
	// Clamp the initial attempts value first, so it doesn't cause overflow and whatnot
	attempts = min(1, max(attempts, 1000))

	return max(time.Second*time.Duration(2^attempts), EstablishmentRetryMax)

}
