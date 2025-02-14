package actors

import (
	"context"
	"testing"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/stage"
	"github.com/stretchr/testify/assert"
)

func TestTrafficManager(t *testing.T) {
	// TrafficManager uses an OutConn in this test
	s := &Stage{
		Ctx: context.TODO(),
	}

	wgConn := &MockUDPConn{}

	oc := MakeOutConn(wgConn, dummyKey, 0, s)
	s.outConn = make(map[key.NodePublic]OutConnActor, 0)
	s.outConn[dummyKey] = oc
	s.peerInfo = make(map[key.NodePublic]*stage.PeerInfo, 0)
	s.peerInfo[dummyKey] = &stage.PeerInfo{Session: testPub}

	// Run TrafficManager
	tm := s.makeTM()
	go tm.Run()

	// Test Handle on TManConnActivity message
	activityMsg := &msgactor.TManConnActivity{
		Peer:     dummyKey,
		IsIn:     true,
		IsActive: true,
	}

	assert.False(t, tm.activeIn[activityMsg.Peer], "TrafficManager should not have active incoming connections upon initialisation")
	assert.False(t, tm.activeOut[activityMsg.Peer], "TrafficManager should not have active outgoing connections upon initialisation")
	tm.inbox <- activityMsg

	assert.Eventually(t, func() bool { return tm.activeIn[activityMsg.Peer] }, assertEventuallyTimeout, assertEventuallyTick, "TrafficManager's activeIn map was not updated after it received a message indicating ingoing activity")
	assert.False(t, tm.activeOut[activityMsg.Peer], "TrafficManager's activeOut map was updated while it received a message indicating ingoing activity")

	// Test Handle on TManConnGoodBye message
	goodbyeMsg := &msgactor.TManConnGoodBye{
		Peer: dummyKey,
		IsIn: true,
	}

	tm.inbox <- goodbyeMsg
	assert.Eventually(t, func() bool { return !tm.activeOut[goodbyeMsg.Peer] }, assertEventuallyTimeout, assertEventuallyTick, "TrafficManager's activeIn map was not updated after it received a message indicating peer left")

	// Test Handle on SyncPeerInfo message
	syncMsg := &msgactor.SyncPeerInfo{
		Peer: dummyKey,
	}

	tm.inbox <- syncMsg
	receivedSyncMsg := <-oc.inbox
	assert.Equal(t, syncMsg, receivedSyncMsg, "TrafficManager did not correctly forward SyncPeerInfo message to OutConn")
	assert.Eventually(t, func() bool { return tm.sessMap[testPub] == syncMsg.Peer }, assertEventuallyTimeout, assertEventuallyTick, "TrafficManager's cachedMap is incorrect")
}
