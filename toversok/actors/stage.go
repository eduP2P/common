package actors

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgcontrol"
	"github.com/edup2p/common/types/relay"
	"github.com/edup2p/common/types/relay/relayhttp"
	"github.com/edup2p/common/types/stage"
	"golang.org/x/exp/maps"
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

func MakeStage(
	pCtx context.Context,

	nodePriv func() *key.NodePrivate,
	sessPriv func() *key.SessionPrivate,

	bindExt func() types.UDPConn,
	bindLocal func(peer key.NodePublic) types.UDPConn,
	controlSession ifaces.ControlInterface,

	dialRelayFunc relayhttp.RelayDialFunc,

	wgIf *net.Interface,
) ifaces.Stage {
	// FIXME ??? why the fuck did we ever decide on this
	// ctx := context.WithoutCancel(pCtx)

	if dialRelayFunc == nil {
		dialRelayFunc = relayhttp.Dial
	}

	s := &Stage{
		Ctx: pCtx,

		connMutex: sync.RWMutex{},
		inConn:    make(map[key.NodePublic]InConnActor),
		outConn:   make(map[key.NodePublic]OutConnActor),

		getNodePriv:    nodePriv,
		getSessPriv:    sessPriv,
		localEndpoints: make([]netip.AddrPort, 0),

		peerInfo: make(map[key.NodePublic]*stage.PeerInfo),

		started: false,

		ext:       bindExt(),
		bindLocal: bindLocal,
		control:   controlSession,

		wgIf: wgIf,

		dialRelayFunc: dialRelayFunc,
	}

	s.DMan = s.makeDM(s.ext)
	s.DRouter = s.makeDR()

	s.RMan = s.makeRM()
	s.RRouter = s.makeRR()

	s.TMan = s.makeTM()
	s.SMan = s.makeSM(sessPriv)
	s.EMan = s.makeEM()
	s.MMan = s.makeMM()

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
	// The MDNSManager
	MMan ifaces.MDNSManagerActor

	connMutex sync.RWMutex
	inConn    map[key.NodePublic]InConnActor
	outConn   map[key.NodePublic]OutConnActor

	getNodePriv func() *key.NodePrivate
	getSessPriv func() *key.SessionPrivate

	endpointMutex  sync.RWMutex
	localEndpoints []netip.AddrPort
	stunEndpoints  []netip.AddrPort

	started bool

	peerInfoMutex sync.RWMutex
	peerInfo      map[key.NodePublic]*stage.PeerInfo

	control ifaces.ControlInterface

	wgIf *net.Interface

	//// A repeatable function to an outside context to acquire a new UDPconn,
	//// once a peer conn has died for whatever reason.
	// TODO rework this?
	// reviveOutConn func(peer key.NodePublic) *net.UDPConn
	//
	// makeOutConn func(udp UDPConn, peer key.NodePublic, s *Stage) OutConnActor
	// makeInConn  func(udp UDPConn, peer key.NodePublic, s *Stage) InConnActor

	ext       types.UDPConn
	bindLocal func(peer key.NodePublic) types.UDPConn

	dialRelayFunc relayhttp.RelayDialFunc
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
	go s.MMan.Run()

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
			slog.Log(context.Background(), types.LevelTrace, "watchdog tick")

			if s.shouldReap() {
				slog.Debug("doing reapConns")
				s.reapConns()
			}

			if s.shouldSync() {
				slog.Debug("doing syncConns")
				s.syncConns()
			}
		}
	}
}

func (s *Stage) shouldReap() bool {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	return len(s.reapableConnsLocked()) > 0
}

// reapConns checks and removes any peer conns that're dead
func (s *Stage) reapConns() {
	// FIXME: this causes a lot of contention on active clients, we should look into making this an upgradable lock
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	peers := s.reapableConnsLocked()

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
}

func (s *Stage) reapableConnsLocked() []key.NodePublic {
	peers := make([]key.NodePublic, 0)

	for peer, inconn := range s.inConn {
		if types.IsContextDone(inconn.Ctx()) {
			// Its dead, cancel the outconn and regenerate it

			out, ok := s.outConn[peer]

			if !ok {
				// outconn is gone for some reason, this is fine for now
				slog.Warn("missing outconn pair to inconn, this is fine, but odd", "peer", peer.Debug())
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

	return peers
}

func (s *Stage) syncableConnsLocked() (added, deleted []key.NodePublic) {
	piPeers := maps.Keys(s.peerInfo)
	connPeers := types.SetUnion(maps.Keys(s.inConn), maps.Keys(s.outConn))

	deleted = types.SetSubtraction(connPeers, piPeers)
	added = types.SetSubtraction(piPeers, connPeers)

	return
}

func (s *Stage) shouldSync() bool {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	s.peerInfoMutex.RLock()
	defer s.peerInfoMutex.RUnlock()

	added, deleted := s.syncableConnsLocked()

	return len(added) > 0 || len(deleted) > 0
}

// syncConns repairs any discrepancies between peerinfo and conns
func (s *Stage) syncConns() {
	var change bool

	// FIXME: this causes a lot of contention on active clients, we should look into making this an upgradable lock

	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	s.peerInfoMutex.Lock()
	defer s.peerInfoMutex.Unlock()

	added, deleted := s.syncableConnsLocked()

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
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	return s.inConn[peer]
}

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
	s.endpointMutex.RLock()
	defer s.endpointMutex.RUnlock()

	return slices.Concat(s.localEndpoints, s.stunEndpoints)
}

func (s *Stage) setSTUNEndpoints(endpoints []netip.AddrPort) {
	s.endpointMutex.Lock()
	defer s.endpointMutex.Unlock()

	sortEndpointSlice(endpoints)

	if slices.Equal(s.stunEndpoints, endpoints) {
		// no change
		return
	}

	s.stunEndpoints = endpoints

	s.notifyEndpointChanged()
}

func (s *Stage) setLocalEndpoints(addrs []netip.Addr) {
	s.endpointMutex.Lock()
	defer s.endpointMutex.Unlock()

	localPort := s.getLocalPort()

	if localPort == 0 {
		// TODO this will spam, maybe only have it happen once?
		slog.Warn("could not get local port, disregarding local endpoints change")
		return
	}

	var endpoints []netip.AddrPort

	// Filter own endpoint, and also append localport
	for _, addr := range addrs {
		if s.control.IPv4().Contains(addr) || s.control.IPv6().Contains(addr) {
			continue
		}

		endpoints = append(endpoints, netip.AddrPortFrom(addr, localPort))
	}

	sortEndpointSlice(endpoints)

	if slices.Equal(s.localEndpoints, endpoints) {
		// no change
		return
	}

	slog.Debug("set local endpoints", "endpoints", endpoints)

	s.localEndpoints = endpoints

	s.notifyEndpointChanged()
}

func (s *Stage) getLocalEndpoints() []netip.Addr {
	s.endpointMutex.RLock()
	defer s.endpointMutex.RUnlock()

	return types.Map(s.localEndpoints, func(t netip.AddrPort) netip.Addr {
		return t.Addr()
	})
}

func (s *Stage) getLocalPort() uint16 {
	type HasLocalAddr interface {
		LocalAddr() net.Addr
	}

	ext := s.ext

	if ucc, ok := ext.(*types.UDPConnCloseCatcher); ok {
		ext = ucc.UDPConn
	}

	nc, ok := ext.(HasLocalAddr)
	if !ok {
		// external socket is not able to get a port from?
		slog.Debug("could not get HasLocalAddr from type", "type", reflect.TypeOf(ext))
		return 0
	}

	ap, err := netip.ParseAddrPort(nc.LocalAddr().String())
	if err != nil {
		slog.Warn("parsing addrport from ext failed", "err", err, "addrport", nc.LocalAddr().String())
		return 0
	}

	return ap.Port()
}

func (s *Stage) notifyEndpointChanged() {
	if err := s.control.UpdateEndpoints(slices.Concat(s.stunEndpoints, s.localEndpoints)); err != nil {
		slog.Warn("could not update endpoints", "err", err)
	}
}

func (s *Stage) AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, ip4, ip6 netip.Addr, prop msgcontrol.Properties) error {
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
		Endpoints:           types.NormaliseAddrPortSlice(endpoints),
		RendezvousEndpoints: make([]netip.AddrPort, 0),
		Session:             session,
		IPv4:                ip4,
		IPv6:                ip6,
		MDNS:                prop.MDNS,
	}

	return nil
}

var errNoPeerInfo = errors.New("could not find peer info to update")

func (s *Stage) UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic, prop *msgcontrol.Properties) error {
	return s.updatePeerInfo(peer, func(info *stage.PeerInfo) {
		if homeRelay != nil {
			info.HomeRelay = *homeRelay
		}
		if endpoints != nil {
			info.Endpoints = types.NormaliseAddrPortSlice(endpoints)
		}
		if session != nil {
			info.Session = *session
		}
		if prop != nil {
			info.MDNS = prop.MDNS
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
		info.Endpoints = types.NormaliseAddrPortSlice(endpoints)
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

func (s *Stage) GetPeersWhere(f func(key.NodePublic, *stage.PeerInfo) bool) []key.NodePublic {
	s.peerInfoMutex.RLock()
	defer s.peerInfoMutex.RUnlock()

	var peers []key.NodePublic
	for peer, info := range s.peerInfo {
		if f(peer, info) {
			peers = append(peers, peer)
		}
	}
	return peers
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
