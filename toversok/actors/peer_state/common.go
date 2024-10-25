package peer_state

import (
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/stage"
	"net/netip"
	"time"
)

const (
	EstablishmentTimeout        = time.Second * 10
	EstablishmentRetryMax       = time.Minute * 10
	EstablishedPingTimeout      = time.Second * 5
	ConnectionInactivityTimeout = time.Minute
	EstablishingPingMinInterval = time.Millisecond * 900
)

type StateCommon struct {
	tm   ifaces.TrafficManagerActor
	peer key.NodePublic
}

func (sc *StateCommon) Peer() key.NodePublic {
	return sc.peer
}

func (sc *StateCommon) pingDirectValid(ap netip.AddrPort, sess key.SessionPublic, ping *msgsess.Ping) bool {
	return sc.tm.ValidKeys(ping.NodeKey, sess)
}

func (sc *StateCommon) replyWithPongDirect(ap netip.AddrPort, sess key.SessionPublic, ping *msgsess.Ping) {
	sc.tm.SendMsgToDirect(ap, sess, &msgsess.Pong{
		TxID: ping.TxID,
		Src:  ap,
	})
}

func (sc *StateCommon) pingRelayValid(relay int64, node key.NodePublic, sess key.SessionPublic, ping *msgsess.Ping) bool {
	return sc.tm.ValidKeys(ping.NodeKey, sess)
}

func (sc *StateCommon) replyWithPongRelay(relay int64, node key.NodePublic, sess key.SessionPublic, ping *msgsess.Ping) {
	sc.tm.SendMsgToRelay(relay, node, sess, &msgsess.Pong{
		TxID: ping.TxID,
	})
}

// TODO add bool here and checks by callers
func (sc *StateCommon) ackPongDirect(ap netip.AddrPort, sess key.SessionPublic, pong *msgsess.Pong) {
	sent, ok := sc.tm.Pings()[pong.TxID]
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

	delete(sc.tm.Pings(), pong.TxID)
}

// TODO add bool here and checks by callers
func (sc *StateCommon) ackPongRelay(relay int64, node key.NodePublic, sess key.SessionPublic, pong *msgsess.Pong) {

	// Relay pongs should come in response to relay pings, note if it is different.
	sent, ok := sc.tm.Pings()[pong.TxID]

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

	delete(sc.tm.Pings(), pong.TxID)

}

func (sc *StateCommon) getPeerInfo() *stage.PeerInfo {
	return sc.tm.Stage().GetPeerInfo(sc.peer)
}

type EstablishingCommon struct {
	*StateCommon

	deadline time.Time
	attempt  int

	lastPing time.Time
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

	return min(time.Second*time.Duration(2^attempts), EstablishmentRetryMax)
}

func (ec *EstablishingCommon) wantsPing() bool {
	return ec.lastPing.Add(EstablishingPingMinInterval).Before(time.Now())
}

func (ec *EstablishingCommon) sendPingsToPeer() *stage.PeerInfo {
	pi := ec.getPeerInfo()
	if pi == nil {
		// Peer info unavailable
		return nil
	}

	for _, ep := range types.SetUnion(pi.Endpoints, pi.RendezvousEndpoints) {
		ec.tm.SendPingDirect(ep, ec.peer, pi.Session)
	}

	ec.lastPing = time.Now()

	return pi
}
