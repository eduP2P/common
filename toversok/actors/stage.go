package actors

import (
	"context"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/key"
	"net"
	"net/netip"
	"sync"
	"time"
)

type OutConnActor interface {
	Actor

	Ctx() context.Context
}

type InConnActor interface {
	Actor

	Ctx() context.Context

	ForwardPacket(pkt []byte)
}

// Stage for the Actors
type Stage struct {
	// The DirectManager
	DMan DirectManagerActor
	// The DirectRouter
	DRouter DirectRouterActor

	// The RelayManager
	RMan RelayManagerActor
	// The RelayRouter
	RRouter RelayRouterActor

	// The TrafficManager
	TMan Actor
	// The SessionManager
	SMan Actor

	mu             sync.RWMutex
	inConn         map[key.NodePublic]InConnActor
	outConn        map[key.NodePublic]OutConnActor
	localEndpoints []netip.AddrPort

	localKey key.NodePublic

	// The parent context of the stage that all actors should parent
	ctx context.Context

	// A repeatable function to an outside context to acquire a new UDPconn,
	// once a peer conn has died for whatever reason.
	reviveConn func(peer key.NodePublic) (udp *net.UDPConn, homeRelay int64)

	makeOutConn func(udp UDPConn, peer key.NodePublic, homeRelay int64, s *Stage) OutConnActor
	makeInConn  func(udp UDPConn, peer key.NodePublic, s *Stage) InConnActor
}

// Watchdog will be run to constantly check for faults on the stage and repair them.
func (s *Stage) Watchdog() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(5 * time.Second):
			for _, peer := range s.ReapConns() {
				udp, relay := s.reviveConn(peer)

				s.addConn(udp, peer, relay)
			}
		}
	}
}

// ReapConns checks, removes, and returns the peer conns that need to be regenerated.
func (s *Stage) ReapConns() (peers []key.NodePublic) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peers = make([]key.NodePublic, 0)

	for peer, inconn := range s.inConn {
		if IsContextDone(inconn.Ctx()) {
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
		if IsContextDone(outconn.Ctx()) {
			// Conn is dead, add to reaping
			peers = append(peers, peer)
		} else {
			// Conn is still active
			continue
		}

		peers = append(peers, peer)
	}

	for peer, inconn := range s.inConn {
		if IsContextDone(inconn.Ctx()) {
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

// OutConnFor Looks up the OutConn for a peer. Returns nil if it doesn't exist.
func (s *Stage) OutConnFor(peer key.NodePublic) OutConnActor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.outConn[peer]
}

// InConnFor Looks up the InConn for a peer. Returns nil if it doesn't exist.
func (s *Stage) InConnFor(peer key.NodePublic) InConnActor {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.inConn[peer]
}

// AddConn creates an InConn and OutConn for a specified connection.
// Starting each Actor'S goroutines as well. It also starts a SockRecv given the
// udp connection.
func (s *Stage) AddConn(udp *net.UDPConn, peer key.NodePublic, session key.SessionPublic, endpoints []netip.AddrPort, homeRelay int64) {
	s.UpdateSessionKey(peer, session)
	s.addConn(udp, peer, homeRelay)
}

func (s *Stage) addConn(udp *net.UDPConn, peer key.NodePublic, homeRelay int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	outConn := s.makeOutConn(udp, peer, homeRelay, s)
	inConn := s.makeInConn(udp, peer, s)

	s.outConn[peer] = outConn
	s.inConn[peer] = inConn

	go outConn.Run()
	go inConn.Run()
}

func (s *Stage) GetLocalEndpoints() []netip.AddrPort {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.localEndpoints
}

func (s *Stage) AddPeerInfo(peer key.NodePublic, endpoints []netip.AddrPort, session key.SessionPublic) {
	// TODO
}

// UpdateSessionKey updates the known session key for a particular peer.
func (s *Stage) UpdateSessionKey(peer key.NodePublic, session key.SessionPublic) {
	// TODO
}

// SetEndpoints set the known public addresses for a particular peer.
func (s *Stage) SetEndpoints(peer key.NodePublic, endpoints []netip.AddrPort) {
	// TODO
}

// AddEndpoints adds an endpoint for a particular peer.
func (s *Stage) AddEndpoints(peer key.NodePublic, endpoints []netip.AddrPort) {
	// TODO
}

func (s *Stage) RemoveConn(peer key.NodePublic) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in, inok := s.inConn[peer]
	out, outok := s.inConn[peer]

	if !inok && !outok {
		// both already removed, we're done here
		return
	}

	if inok != outok {
		// only one of them removed?
		// we could recover this, but this is a bug, panic.
		panic(fmt.Sprintf("InConn or OutConn presence on stage was disbalanced: in=%t, out=%t", inok, outok))
	}

	// Now we know both exist

	delete(s.inConn, peer)
	delete(s.outConn, peer)

	in.Cancel()
	out.Cancel()

	// OutConn cancel:
	//   this closes the outch in SockRecv,
	//   sends "outconn goodbye" to traffic manager,
	//
	// InConn cancel:
	//   sends "outconn goodbye" to traffic manager.

	// When TM has received both goodbyes:
	//   removes from internal activity tracking,
	//   and removes mapping from direct router.
}
