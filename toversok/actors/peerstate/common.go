package peerstate

import (
	"context"
	"log/slog"
	"net/netip"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/stage"
)

const (
	EstablishmentTimeout             = time.Second * 10
	EstablishmentRetryMax            = time.Minute * 10
	EstablishedPingTimeout           = time.Second * 5
	ConnectionInactivityTimeout      = time.Minute
	EstablishingPingMinInterval      = time.Millisecond * 900
	BurstEstablishingPingMinInterval = time.Millisecond * 200
)

type StateCommon struct {
	tm   ifaces.TrafficManagerActor
	peer key.NodePublic
}

func (sc *StateCommon) Peer() key.NodePublic {
	return sc.peer
}

func (sc *StateCommon) pingDirectValid(_ netip.AddrPort, sess key.SessionPublic, ping *msgsess.Ping) bool {
	return sc.tm.ValidKeys(ping.NodeKey, sess)
}

func (sc *StateCommon) replyWithPongDirect(ap netip.AddrPort, sess key.SessionPublic, ping *msgsess.Ping) {
	sc.tm.SendMsgToDirect(ap, sess, &msgsess.Pong{
		TxID: ping.TxID,
		Src:  ap,
	})
}

//nolint:unused
func (sc *StateCommon) pingRelayValid(_ int64, _ key.NodePublic, sess key.SessionPublic, ping *msgsess.Ping) bool {
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
		slog.Warn(
			"got pong for unknown ping",
			"from-ap", ap,
			"txid", pong.TxID,
			"sess", sess,
		)
		return
	}

	if sent.ToRelay {
		slog.Warn(
			"got direct pong to relay ping",
			"from-ap", ap,
			"txid", pong.TxID,
			"ping-to", sent.To.Debug(),
			"to-relay", sent.RelayID,
			"sess", sess,
		)
		return
	}

	if !sc.tm.ValidKeys(sc.peer, sess) {
		// ?? Somehow the pong is for a valid ping to a node that no longer has this session key?
		// Might happen between restarts, log and ignore.
		slog.Warn(
			"received valid pong for unexpected remote session",
			"from-ap", ap,
			"txid", pong.TxID,
			"sess", sess,
		)
		return
	}

	// TODO more checks? (permissive, but log)

	delete(sc.tm.Pings(), pong.TxID)
}

// TODO add bool here and checks by callers
func (sc *StateCommon) ackPongRelay(relayID int64, node key.NodePublic, sess key.SessionPublic, pong *msgsess.Pong) {
	// Relay pongs should come in response to relay pings, note if it is different.
	sent, ok := sc.tm.Pings()[pong.TxID]

	if !ok {
		slog.Warn(
			"got pong for unknown ping",
			"from-relay", relayID,
			"txid", pong.TxID,
			"sess", sess,
		)
		return
	}

	if !sent.ToRelay {
		slog.Warn(
			"got relay pong to direct ping",
			"from-relay", relayID,
			"txid", pong.TxID,
			"ping-to", sent.To.Debug(),
			"to-relay", sent.RelayID,
			"sess", sess,
		)
		return
	}

	if node != sent.To {
		slog.Warn(
			"received pong to ping (with same TXID) from a different peer than we sent it to, possible collision",
			"to-peer", sent.To.Debug(),
			"from-peer", node.Debug(),
			"from-relay", relayID,
			"txid", pong.TxID,
			"sess", sess,
		)
		return
	}

	if !sc.tm.ValidKeys(sent.To, sess) {
		// ?? Somehow the pong is for a valid ping to a node that no longer has this session key?
		// Might happen between restarts, log and ignore.
		slog.Warn(
			"received valid pong for unexpected remote session",
			"from-relay", relayID,
			"txid", pong.TxID,
			"sess", sess,
		)
		return
	}

	if sent.RelayID != relayID {
		slog.Debug(
			"received relay pong to relay ping from other relay, ignoring...",
			"to-relay", sent.RelayID,
			"from-relay", relayID,
			"txid", pong.TxID,
		)
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

	lastPing  time.Time
	pingCount uint
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
	interval := EstablishingPingMinInterval

	if ec.pingCount < 4 {
		interval = BurstEstablishingPingMinInterval
	}

	return ec.lastPing.Add(interval).Before(time.Now())
}

func (ec *EstablishingCommon) sendPingsToPeer() *stage.PeerInfo {
	pi := ec.getPeerInfo()
	if pi == nil {
		// Peer info unavailable
		return nil
	}

	endpoints := types.SetUnion(pi.Endpoints, pi.RendezvousEndpoints)

	for _, ep := range endpoints {
		ec.tm.SendPingDirect(ep, ec.peer, pi.Session)
	}

	slog.Log(context.Background(), types.LevelTrace, "fanning direct pings to peer", "peer", ec.peer.Debug(), "via-endpoints", types.PrettyAddrPortSlice(endpoints))

	ec.lastPing = time.Now()
	ec.pingCount++

	return pi
}
