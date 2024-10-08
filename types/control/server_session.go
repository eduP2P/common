package control

import (
	"context"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
	"log/slog"
	"net/netip"
	"time"
)

type ServerSession struct {
	ID   string
	Peer key.NodePublic
	Sess key.SessionPublic

	IPv4 netip.Addr
	IPv6 netip.Addr

	HomeRelay int64

	CurrentEndpoints []netip.AddrPort

	Ctx context.Context
	Ccc context.CancelCauseFunc

	getConnChan chan *Conn
	conn        *Conn

	queuedPeerDeltas map[key.NodePublic]PeerDelta

	// ServerSessionState
	state ServerSessionState

	server *Server

	// TODO
	//  all synced state, known changes, queued changes, etc.
}

func NewSession(cc *Conn, nodeKey key.NodePublic, sessKey key.SessionPublic, server *Server) *ServerSession {
	id := types.RandStringBytesMaskImprSrc(32)

	ctx, ccc := context.WithCancelCause(context.Background())

	return &ServerSession{
		ID:               id,
		Peer:             nodeKey,
		Sess:             sessKey,
		CurrentEndpoints: make([]netip.AddrPort, 0),
		Ctx:              ctx,
		Ccc:              ccc,
		getConnChan:      make(chan *Conn),
		conn:             cc,
		queuedPeerDeltas: make(map[key.NodePublic]PeerDelta),
		state:            Authenticate,
		server:           server,
	}
}

// Knock asks the session goroutine/connection to "knock" (send ping, await pong) the session,
// to make sure it is still alive.
//
// Will return true if the session is now transitioned to dangling.
func (s *ServerSession) Knock() (dangling bool) {
	// TODO
	panic("implement me")
}

// Greet another session, send PeerAddition
func (s *ServerSession) Greet(otherSess *ServerSession) {
	s.Slog().Debug("Greet", "from", otherSess.Peer.Debug())

	s.conn.Write(&msgcontrol.PeerAddition{
		PubKey:    otherSess.Peer,
		SessKey:   otherSess.Sess,
		IPv4:      otherSess.IPv4,
		IPv6:      otherSess.IPv6,
		Endpoints: otherSess.CurrentEndpoints,
		HomeRelay: otherSess.HomeRelay,
	})
}

func (s *ServerSession) UpdateEndpoints(peer key.NodePublic, endpoints []netip.AddrPort) {
	// TODO mark update delta when dangling

	s.Slog().Debug("UpdateEndpoints", "from", peer.Debug(), "endpoints", endpoints)

	s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:    peer,
		Endpoints: endpoints,
	})
}

func (s *ServerSession) UpdateSessKey(peer key.NodePublic, sessKey key.SessionPublic) {
	// TODO mark update delta when dangling

	s.Slog().Debug("UpdateSessKey", "from", peer.Debug(), "sess-key", sessKey)

	s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:  peer,
		SessKey: &sessKey,
	})
}

func (s *ServerSession) UpdateHomeRelay(peer key.NodePublic, homeRelay int64) {
	// TODO mark update delta when dangling

	s.Slog().Debug("UpdateHomeRelay", "from", peer.Debug(), "home-relay", homeRelay)

	s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:    peer,
		HomeRelay: &homeRelay,
	})
}

// Bye to another session, send PeerRemove
func (s *ServerSession) Bye(peer key.NodePublic) {
	s.Slog().Debug("Bye", "from", peer.Debug())

	s.conn.Write(&msgcontrol.PeerRemove{
		PubKey: peer,
	})
}

// SendRelays sends all relay information to the client. This is not ran on Resume.
func (s *ServerSession) SendRelays() error {
	s.Slog().Debug("SendRelays")

	return s.conn.Write(&msgcontrol.RelayUpdate{Relays: s.server.relays})
}

func (s *ServerSession) Resume(cc *Conn, sessKey key.SessionPublic) {
	// TODO: check sessKey == s.key, else send sesskeyupdate

	// TODO we send nothing to the client except queued messages, which are backed up.
	//  we immediately expect a EndpointUpdate and HomeRelayUpdate though,
	//  and wait for that for 10 seconds before sending an update.

	// TODO
	panic("implement me")
}

func (s *ServerSession) AuthenticateAccept() (accepted bool, err error) {
	// TODO add authenticate logic

	s.Slog().Debug("AuthenticateAccept")

	ip4, ip6 := s.server.getIPs(s.Peer)

	if err = s.conn.Write(&msgcontrol.LogonAccept{
		IP4:       ip4,
		IP6:       ip6,
		SessionID: s.ID,
	}); err != nil {
		err = fmt.Errorf("error when sending accept: %w", err)
		return
	}

	s.IPv4 = ip4.Addr()
	s.IPv6 = ip6.Addr()

	accepted = true

	return
}

func (s *ServerSession) Run() {
	// We arrive just after Logon, next message can be Accept or Authenticate

	var err error

	go func() {
		<-s.Ctx.Done()

		s.Slog().Info("session exiting", "err", context.Cause(s.Ctx), "peer", s.Peer.Debug())

		s.server.RemoveSession(s)

		if s.conn != nil {
			s.conn.mc.Close()
		}
	}()

	defer func() {
		s.Ccc(fmt.Errorf("main run loop exited: %w", err))
	}()

	var accepted bool

	if accepted, err = s.AuthenticateAccept(); !accepted || err != nil {
		return
	}

	s.state = Greet

	if err = s.SendRelays(); err != nil {
		err = fmt.Errorf("could not send relays: %w", err)
		return
	}

	// TODO wait here for information?

	s.server.ForVisible(s, func(session *ServerSession) {
		// TODO this currently blocks and holds the lock, we should make Greet async as well

		// TODO there is no bubbling of errors, ignore? log?

		session.Greet(s)

		s.Greet(session)
	})

	s.state = Established

	s.Slog().Info("established session")

	for {
		var m msgcontrol.ControlMessage

		m, err = s.conn.Read(0)

		if err != nil {
			// TODO this currently removes the session on connection break; no resuming

			return
		}

		switch msg := m.(type) {
		case *msgcontrol.EndpointUpdate:
			if msg.Endpoints == nil {
				s.Slog().Warn("received nil endpoints")

				continue
			}

			s.CurrentEndpoints = msg.Endpoints

			s.Slog().Debug("received endpoints", "endpoints", msg.Endpoints)

			s.server.ForVisible(s, func(session *ServerSession) {
				session.UpdateEndpoints(s.Peer, msg.Endpoints)
			})
		case *msgcontrol.HomeRelayUpdate:
			s.HomeRelay = msg.HomeRelay

			s.Slog().Debug("received home relay", "home-relay", msg.HomeRelay)

			s.server.ForVisible(s, func(session *ServerSession) {
				session.UpdateHomeRelay(s.Peer, msg.HomeRelay)
			})
		case *msgcontrol.Pong:
			// TODO
		default:
			err = fmt.Errorf("received unknown type of message: %#v", msg)
			return
		}
	}

	time.Sleep(30 * time.Second)

	// TODO make other peers aware

	// for now, send a reject
	//if err = s.conn.Write(&msgcontrol.LogonReject{
	//	Reason:        "dev: reject unambiguously",
	//	RetryStrategy: 0,
	//}); err != nil {
	//	err = fmt.Errorf("error when sending reject: %w", err)
	//	return
	//}

	return

	// TODO after Accept, we send the client peer and relay definitions,
	//  but we need to wait for the client to send their home relay and endpoints,
	//  before we'd (ideally) send a complete peer info to other clients.
	//  We will wait 10 seconds for this, before timing out and sending incomplete information.

	// TODO
	panic("implement me")
}

func (s *ServerSession) Slog() *slog.Logger {
	return slog.With("peer", s.Peer.Debug())
}

// TODO needs a notion of "who is it allowed to see"

type PeerDelta struct {
	add    bool
	remove bool

	endpoints bool
	session   bool
	relay     bool
}

func (p PeerDelta) Merge(o PeerDelta) PeerDelta {
	if o.add || o.remove {
		return o
	}

	if p.add || p.remove {
		return p
	}

	return PeerDelta{
		endpoints: p.endpoints || o.endpoints,
		session:   p.session || o.session,
		relay:     p.relay || o.relay,
	}
}

type ServerSessionState byte

const (
	Authenticate ServerSessionState = iota
	Greet
	Established
	Dangling
	ReEstablishing
	Deconstructing
)
