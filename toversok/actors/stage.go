package actors

import (
	"context"
	"errors"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgactor"
	"github.com/shadowjonathan/edup2p/types/relay"
	"github.com/shadowjonathan/edup2p/types/stage"
	"golang.org/x/exp/maps"
	"log/slog"
	"net/netip"
	"slices"
	"sort"
	"sync"
	"time"
)

type OutConnActor interface {
	ifaces.Actor

	Ctx() context.Context
}

type InConnActor interface {
	ifaces.Actor

	Ctx() context.Context

	ForwardPacket(pkt []byte)
}

//udp, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.IPv4Unspecified(), localPort)))
//if err != nil {
//	panic(fmt.Sprintf("could not create listenUDP: %s", err))
//}

func MakeStage(
	pCtx context.Context,

	nodePriv func() *key.NodePrivate,
	sessPriv func() *key.SessionPrivate,

	bindExt func() types.UDPConn,
	bindLocal func(peer key.NodePublic) types.UDPConn,
	controlSession ifaces.ControlInterface,
) ifaces.Stage {
	ctx := context.WithoutCancel(pCtx)

	s := &Stage{
		Ctx: ctx,

		connMutex: sync.RWMutex{},
		inConn:    make(map[key.NodePublic]InConnActor),
		outConn:   make(map[key.NodePublic]OutConnActor),

		getNodePriv:    nodePriv,
		getSessPriv:    sessPriv,
		localEndpoints: make([]netip.AddrPort, 0),

		peerInfo: make(map[key.NodePublic]*stage.PeerInfo),

		started: false,

		bindExt:   bindExt,
		bindLocal: bindLocal,
		control:   controlSession,
	}

	s.DMan = s.makeDM(bindExt())
	s.DRouter = s.makeDR()

	s.RMan = s.makeRM()
	s.RRouter = s.makeRR()

	s.TMan = s.makeTM()
	s.SMan = s.makeSM(sessPriv)
	s.EMan = s.makeEM()

	return s
}

// Stage for the Actors
type Stage struct {
	// The parent context of the stage that all actors must parent
	Ctx context.Context

	// The DirectManager
	DMan ifaces.DirectManagerActor
	// The DirectRouter
	DRouter ifaces.DirectRouterActor

	// The RelayManager
	RMan ifaces.RelayManagerActor
	// The RelayRouter
	RRouter ifaces.RelayRouterActor

	// The TrafficManager
	TMan ifaces.TrafficManagerActor
	// The SessionManager
	SMan ifaces.SessionManagerActor
	// The EndpointManager
	EMan ifaces.EndpointManagerActor

	connMutex sync.RWMutex
	inConn    map[key.NodePublic]InConnActor
	outConn   map[key.NodePublic]OutConnActor

	getNodePriv    func() *key.NodePrivate
	getSessPriv    func() *key.SessionPrivate
	localEndpoints []netip.AddrPort
	stunEndpoints  []netip.AddrPort

	started bool

	peerInfoMutex sync.RWMutex
	peerInfo      map[key.NodePublic]*stage.PeerInfo

	control ifaces.ControlInterface

	//// A repeatable function to an outside context to acquire a new UDPconn,
	//// once a peer conn has died for whatever reason.
	//reviveOutConn func(peer key.NodePublic) *net.UDPConn
	//
	//makeOutConn func(udp UDPConn, peer key.NodePublic, s *Stage) OutConnActor
	//makeInConn  func(udp UDPConn, peer key.NodePublic, s *Stage) InConnActor

	bindExt   func() types.UDPConn
	bindLocal func(peer key.NodePublic) types.UDPConn
}

// Start kicks off goroutines for the stage and returns
func (s *Stage) Start() {
	if s.started {
		return
	}

	go s.Watchdog()

	go s.TMan.Run()
	go s.SMan.Run()
	go s.EMan.Run()

	go s.DMan.Run()
	go s.DRouter.Run()

	go s.RMan.Run()
	go s.RRouter.Run()

	s.started = true
}

// Watchdog will be run to constantly check for faults on the stage and repair them.
func (s *Stage) Watchdog() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-s.Ctx.Done():
			return
		case <-ticker.C:
			slog.Debug("watchdog tick")
			s.reapConns()
			s.syncConns()
		}
	}
}

// reapConns checks and removes any peer conns that're dead
func (s *Stage) reapConns() {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	peers := make([]key.NodePublic, 0)

	for peer, inconn := range s.inConn {
		if types.IsContextDone(inconn.Ctx()) {
			// Its dead, cancel the outconn and regenerate it

			out, ok := s.outConn[peer]

			if !ok {
				// outconn is gone for some reason, this is fine for now
				// TODO log this?
			} else {
				out.Cancel()
			}

			peers = append(peers, peer)
		}
	}

	for peer, outconn := range s.outConn {
		if types.IsContextDone(outconn.Ctx()) {
			// Conn is dead, add to reaping
			peers = append(peers, peer)
		} else {
			// Conn is still active
			continue
		}

		peers = append(peers, peer)
	}

	for peer, inconn := range s.inConn {
		if types.IsContextDone(inconn.Ctx()) {
			// Conn is dead, add to reaping
			peers = append(peers, peer)
		} else {
			// Conn is still active
			continue
		}
	}

	for _, peer := range peers {
		// we need to remove them here,
		// as else we'll be mutating the maps while going over them.

		if c, ok := s.outConn[peer]; ok {
			c.Cancel()
		}

		if c, ok := s.inConn[peer]; ok {
			c.Cancel()
		}

		delete(s.outConn, peer)
		delete(s.inConn, peer)
	}

	return
}

func (s *Stage) informPeerInfoUpdate(peer key.NodePublic) {
	go SendMessage(s.TMan.Inbox(), &msgactor.SyncPeerInfo{Peer: peer})
}

// OutConnFor Looks up the OutConn for a peer. Returns nil if it doesn't exist.
func (s *Stage) OutConnFor(peer key.NodePublic) OutConnActor {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	return s.outConn[peer]
}

// InConnFor Looks up the InConn for a peer. Returns nil if it doesn't exist.
func (s *Stage) InConnFor(peer key.NodePublic) InConnActor {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	return s.inConn[peer]
}

//// AddConn creates an InConn and OutConn for a specified connection.
//// Starting each Actor'S goroutines as well. It also starts a SockRecv given the
//// udp connection.
//func (s *Stage) AddConn(udp *net.UDPConn, peer key.NodePublic, info *PeerInfo) {
//	s.UpdateSessionKey(peer, session)
//	s.addConn(udp, peer, homeRelay)
//}

// addConnLocked assumes Stage.connMutex and Stage.peerInfoMutex is held by caller.
func (s *Stage) addConnLocked(peer key.NodePublic, udp types.UDPConn) {
	pi := s.peerInfo[peer]

	if pi == nil {
		panic("expecting to have peer information at this point")
	}

	outConn := MakeOutConn(udp, peer, pi.HomeRelay, s)
	inConn := MakeInConn(udp, peer, s)

	s.outConn[peer] = outConn
	s.inConn[peer] = inConn

	go outConn.Run()
	go inConn.Run()
}

func (s *Stage) GetEndpoints() []netip.AddrPort {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	return slices.Concat(s.localEndpoints, s.stunEndpoints)
}

func (s *Stage) setSTUNEndpoints(endpoints []netip.AddrPort) {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].Addr().Less(endpoints[j].Addr())
	})

	if slices.Equal(s.stunEndpoints, endpoints) {
		// no change
		return
	}

	s.stunEndpoints = endpoints

	s.notifyEndpointChanged()
}

func (s *Stage) setLocalEndpoints(endpoints []netip.AddrPort) {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].Addr().Less(endpoints[j].Addr())
	})

	if slices.Equal(s.localEndpoints, endpoints) {
		// no change
		return
	}

	s.localEndpoints = endpoints

	s.notifyEndpointChanged()
}

func (s *Stage) notifyEndpointChanged() {
	if err := s.control.UpdateEndpoints(slices.Concat(s.stunEndpoints, s.localEndpoints)); err != nil {
		slog.Warn("could not update endpoints", "err", err)
	}
}

// syncConns repairs any discrepancies between peerinfo and conns
func (s *Stage) syncConns() {
	var change bool

	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	s.peerInfoMutex.Lock()
	defer s.peerInfoMutex.Unlock()

	piPeers := maps.Keys(s.peerInfo)
	connPeers := types.SetUnion(maps.Keys(s.inConn), maps.Keys(s.outConn))

	deleted := types.SetSubtraction(connPeers, piPeers)
	added := types.SetSubtraction(piPeers, connPeers)

	if len(deleted) > 0 || len(added) > 0 {
		change = true
	}

	for _, peer := range deleted {
		in, inok := s.inConn[peer]
		out, outok := s.outConn[peer]

		if inok {
			in.Cancel()
			delete(s.inConn, peer)
		}

		if outok {
			out.Cancel()
			delete(s.outConn, peer)
		}

		slog.Debug("pruned conns", "peer", peer.Debug())
	}

	for _, peer := range added {
		s.addConnLocked(peer, s.bindLocal(peer))
		slog.Debug("started conns", "peer", peer.Debug())
	}

	if change {
		s.TMan.Poke()
	}
}

func (s *Stage) AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, _ netip.Addr, _ netip.Addr) error {
	s.peerInfoMutex.Lock()

	defer func() {
		// We put these in defer above the below, because syncConns wants peerInfoMutex
		s.syncConns()
		s.informPeerInfoUpdate(peer)
	}()
	defer s.peerInfoMutex.Unlock()

	if _, ok := s.peerInfo[peer]; ok {
		return errors.New("peer already exists")
	}

	s.peerInfo[peer] = &stage.PeerInfo{
		HomeRelay:           homeRelay,
		Endpoints:           endpoints,
		RendezvousEndpoints: make([]netip.AddrPort, 0),
		Session:             session,
	}

	return nil
}

var errNoPeerInfo = errors.New("could not find peer info to update")

func (s *Stage) UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic) error {
	return s.updatePeerInfo(peer, func(info *stage.PeerInfo) {
		if homeRelay != nil {
			info.HomeRelay = *homeRelay
		}
		if endpoints != nil {
			info.Endpoints = endpoints
		}
		if session != nil {
			info.Session = *session
		}
	})
}

func (s *Stage) UpdateHomeRelay(peer key.NodePublic, relay int64) error {
	return s.updatePeerInfo(peer, func(info *stage.PeerInfo) {
		info.HomeRelay = relay
	})
}

// UpdateSessionKey updates the known session key for a particular peer.
func (s *Stage) UpdateSessionKey(peer key.NodePublic, session key.SessionPublic) error {
	return s.updatePeerInfo(peer, func(info *stage.PeerInfo) {
		info.Session = session
	})
}

// SetEndpoints set the known public addresses for a particular peer.
func (s *Stage) SetEndpoints(peer key.NodePublic, endpoints []netip.AddrPort) error {
	return s.updatePeerInfo(peer, func(info *stage.PeerInfo) {
		info.Endpoints = endpoints
	})
}

func (s *Stage) updatePeerInfo(peer key.NodePublic, f func(info *stage.PeerInfo)) error {
	s.peerInfoMutex.Lock()
	defer s.peerInfoMutex.Unlock()

	pi := s.peerInfo[peer]

	if pi == nil {
		return errNoPeerInfo
	}

	f(pi)

	s.informPeerInfoUpdate(peer)
	s.TMan.Poke()

	return nil
}

// GetPeerInfo gets a copy of the peerinfo for peer
func (s *Stage) GetPeerInfo(peer key.NodePublic) *stage.PeerInfo {
	s.peerInfoMutex.RLock()
	defer s.peerInfoMutex.RUnlock()

	return s.peerInfo[peer]
}

func (s *Stage) RemovePeer(peer key.NodePublic) error {
	s.peerInfoMutex.Lock()
	delete(s.peerInfo, peer)
	s.peerInfoMutex.Unlock()

	s.syncConns()
	s.informPeerInfoUpdate(peer)

	return nil
}

func (s *Stage) UpdateRelays(relays []relay.Information) error {
	go SendMessage(s.RMan.Inbox(), &msgactor.UpdateRelayConfiguration{Config: relays})
	go SendMessage(s.EMan.Inbox(), &msgactor.UpdateRelayConfiguration{Config: relays})

	return nil
}

// ControlSTUN returns a set of endpoints pertaining to Control's STUN addrpairs
func (s *Stage) ControlSTUN() []netip.AddrPort {
	// TODO
	return []netip.AddrPort{}
}

//func (s *Stage) RemoveConn(peer key.NodePublic) {
//	s.connMutex.Lock()
//	defer s.connMutex.Unlock()
//
//	in, inok := s.inConn[peer]
//	out, outok := s.inConn[peer]
//
//	if !inok && !outok {
//		// both already removed, we're done here
//		return
//	}
//
//	if inok != outok {
//		// only one of them removed?
//		// we could recover this, but this is a bug, panic.
//		panic(fmt.Sprintf("InConn or OutConn presence on stage was disbalanced: in=%t, out=%t", inok, outok))
//	}
//
//	// Now we know both exist
//
//	delete(s.inConn, peer)
//	delete(s.outConn, peer)
//
//	in.Cancel()
//	out.Cancel()
//
//	// OutConn cancel:
//	//   this closes the outch in SockRecv,
//	//   sends "outconn goodbye" to traffic manager,
//	//
//	// InConn cancel:
//	//   sends "outconn goodbye" to traffic manager.
//
//	// When TM has received both goodbyes:
//	//   removes from internal activity tracking,
//	//   and removes mapping from direct router.
//}
