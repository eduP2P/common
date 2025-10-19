package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
)

type ServerSession struct {
	ID   string
	Peer key.NodePublic
	Sess key.SessionPublic

	IPv4 netip.Prefix
	IPv6 netip.Prefix

	HomeRelay int64

	CurrentEndpoints []netip.AddrPort

	Ctx context.Context
	Ccc context.CancelCauseFunc

	greetedMu sync.Mutex
	greeted   map[key.SessionPublic]bool

	getConnChan chan *Conn
	conn        *Conn

	queuedPeerDeltas map[key.NodePublic]PeerDelta

	authChan chan any

	// ServerSessionState
	state ServerSessionState

	server *Server

	Expiry time.Time

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
		greeted:          make(map[key.SessionPublic]bool),
		getConnChan:      make(chan *Conn),
		conn:             cc,
		queuedPeerDeltas: make(map[key.NodePublic]PeerDelta),
		authChan:         make(chan any, 5),
		state:            Authenticate,
		server:           server,
	}
}

func (s *ServerSession) doAuthenticate(resumed bool) error {
	if resumed {
		s.server.callbacks.OnSessionResume(SessID(s.ID), ClientID(s.Peer))
	} else {
		s.server.callbacks.OnSessionCreate(SessID(s.ID), ClientID(s.Peer))
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgChan := make(chan msgcontrol.LogonDeviceKey, 1)
	errChan := make(chan error, 1)

	go func() {
		defer wg.Done()

		for ctx.Err() == nil {
			msg := msgcontrol.LogonDeviceKey{}
			err := s.conn.Expect(&msg, time.Millisecond*100)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}

				slog.Error("got error in devicekey expect goroutine", "err", err)

				errChan <- err

				return
			}

			msgChan <- msg
		}
	}()
	wg.Add(1)

	deviceKeySeen := false
	authURLSent := false

	// TODO build timeout in here somewhere

	for {
		select {
		case err := <-errChan:
			return fmt.Errorf("error when reading device key: %w", err)
		case msg := <-msgChan:
			if deviceKeySeen {
				// device key sent twice, this is an error, we should not continue
				return fmt.Errorf("client sent device key twice")
			}

			deviceKeySeen = true

			s.server.callbacks.OnDeviceKey(SessID(s.ID), msg.DeviceKey)
		case authMsg := <-s.authChan:
			switch msg := authMsg.(type) {
			case RejectAuth:
				err := s.conn.Write(msg.LogonReject)
				if err != nil {
					return fmt.Errorf("error while writing logon reject: %w, %w", err, ErrLogonRejected)
				}

				return ErrLogonRejected
			case AcceptAuth:
				return nil
			case AuthURL:
				if authURLSent {
					// auth url already sent, this is a business logic error, we should error out
					return fmt.Errorf("business logic sent auth url twice")
				}
				authURLSent = true

				err := s.conn.Write(&msgcontrol.LogonAuthenticate{
					AuthenticateURL: msg.url,
				})

				slog.Debug("sent auth url", "url", msg.url, "peer", s.Peer.Debug())

				if err != nil {
					return fmt.Errorf("failed to write LogonAuthenticate with auth url: %w", err)
				}
			default:
				return fmt.Errorf("unknown auth object: %v", msg)
			}
		}
	}

	// TODO after this point, we can get 4 things;
	//  - device key from client, send to OnDeviceKey, lock this out after
	//  - SendAuthURL from business logic, send to client, lock this out (error after)
	//  - AcceptAuthentication from business logic, exits
	//  - RejectAuthentication from business logic, exits

	// todo
}

type RejectAuth struct {
	*msgcontrol.LogonReject
}

type AcceptAuth struct{}

type AuthURL struct {
	url string
}

var ErrLogonRejected = errors.New("authentication resulted in logon rejected")

// Knock asks the session goroutine/connection to "knock" (send ping, await pong) the session,
// to make sure it is still alive.
//
// Will return true if the session is now transitioned to dangling.
func (s *ServerSession) Knock() (dangling bool) {
	// TODO
	panic("implement me")
}

// Greet another session, send PeerAddition
func (s *ServerSession) Greet(otherSess *ServerSession, prop msgcontrol.Properties) {
	s.Slog().Debug("Greet", "from", otherSess.Peer.Debug())

	if err := s.conn.Write(&msgcontrol.PeerAddition{
		PubKey:     otherSess.Peer,
		SessKey:    otherSess.Sess,
		IPv4:       otherSess.IPv4.Addr(),
		IPv6:       otherSess.IPv6.Addr(),
		Endpoints:  otherSess.CurrentEndpoints,
		HomeRelay:  otherSess.HomeRelay,
		Properties: prop,
	}); err != nil {
		slog.Error("error writing peer addition", "err", err)
	}

	s.greetedMu.Lock()
	defer s.greetedMu.Unlock()

	s.greeted[otherSess.Sess] = true
}

func (s *ServerSession) Greeted(sess key.SessionPublic) bool {
	s.greetedMu.Lock()
	defer s.greetedMu.Unlock()

	return s.greeted[sess]
}

func (s *ServerSession) UpdateEndpoints(peer key.NodePublic, endpoints []netip.AddrPort) {
	// TODO mark update delta when dangling

	s.Slog().Debug("UpdateEndpoints", "from", peer.Debug(), "endpoints", endpoints)

	if err := s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:    peer,
		Endpoints: endpoints,
	}); err != nil {
		slog.Error("error writing endpoints peer update", "err", err)
	}
}

func (s *ServerSession) UpdateSessKey(peer key.NodePublic, sessKey key.SessionPublic) {
	// TODO mark update delta when dangling

	s.Slog().Debug("UpdateSessKey", "from", peer.Debug(), "sess-key", sessKey)

	if err := s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:  peer,
		SessKey: &sessKey,
	}); err != nil {
		slog.Error("error writing sess key peer update", "err", err)
	}
}

func (s *ServerSession) UpdateHomeRelay(peer key.NodePublic, homeRelay int64) {
	// TODO mark update delta when dangling

	s.Slog().Debug("UpdateHomeRelay", "from", peer.Debug(), "home-relay", homeRelay)

	if err := s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:    peer,
		HomeRelay: &homeRelay,
	}); err != nil {
		slog.Error("error writing home relay peer update", "err", err)
	}
}

func (s *ServerSession) UpdateProperties(peer key.NodePublic, prop msgcontrol.Properties) {
	s.Slog().Debug("UpdateProperties", "from", peer.Debug(), "prop", prop)

	if err := s.conn.Write(&msgcontrol.PeerUpdate{
		PubKey:     peer,
		Properties: &prop,
	}); err != nil {
		slog.Error("error writing properties peer update", "err", err)
	}
}

// Bye to another session, send PeerRemove
func (s *ServerSession) Bye(peer key.NodePublic) {
	s.Slog().Debug("Bye", "from", peer.Debug())

	if err := s.conn.Write(&msgcontrol.PeerRemove{
		PubKey: peer,
	}); err != nil {
		slog.Error("error writing peer remove message", "err", err)
	}
}

// SendRelays sends all relay information to the client. This is not ran on Resume.
func (s *ServerSession) SendRelays() error {
	s.Slog().Debug("SendRelays")

	return s.conn.Write(&msgcontrol.RelayUpdate{Relays: s.server.relays})
}

func (s *ServerSession) Resume(_ *Conn, _ key.SessionPublic) {
	// TODO: check sessKey == s.key, else send sesskeyupdate

	// TODO we send nothing to the client except queued messages, which are backed up.
	//  we immediately expect a EndpointUpdate and HomeRelayUpdate though,
	//  and wait for that for 10 seconds before sending an update.

	// TODO
	panic("implement me")
}

func (s *ServerSession) AuthenticateAccept() (err error) {
	s.Slog().Debug("AuthenticateAccept")

	if err = s.conn.Write(&msgcontrol.LogonAccept{
		IP4:        s.IPv4,
		IP6:        s.IPv6,
		AuthExpiry: s.Expiry,
		SessionID:  s.ID,
	}); err != nil {
		err = fmt.Errorf("error when sending accept: %w", err)
		return
	}

	return
}

func (s *ServerSession) AuthAndStart() error {
	s.IPv4, s.IPv6, s.Expiry = s.server.callbacks.OnSessionFinalize(SessID(s.ID), ClientID(s.Peer))

	err := s.AuthenticateAccept()
	if err != nil {
		return fmt.Errorf("error while writing logon accept: %w", err)
	}

	go s.Run()

	return nil
}

func (s *ServerSession) Run() {
	// We arrive just after having sent LogonAccept

	var err error

	go func() {
		<-s.Ctx.Done()

		if errors.Is(s.Ctx.Err(), ErrNeedsDisconnect) {
			if err := s.conn.Write(&msgcontrol.Disconnect{
				Reason: "control requested disconnect",
			}); err != nil {
				slog.Error("error writing disconnect message", "err", err)
			}
		}

		s.Slog().Info("session exiting", "err", context.Cause(s.Ctx), "peer", s.Peer.Debug())

		s.server.RemoveSession(s)

		if s.conn != nil {
			if err := s.conn.mc.Close(); err != nil {
				slog.Error("failed to close metaconn", "err", err)
			}
		}
	}()

	defer func() {
		s.Ccc(fmt.Errorf("main run loop exited: %w", err))
	}()

	s.state = Greet

	if err = s.SendRelays(); err != nil {
		err = fmt.Errorf("could not send relays: %w", err)
		return
	}

	// TODO wait here for information?

	err = s.server.sessLockedDoVisibilityPairs(s.Peer, func(m map[ClientID]VisibilityPair) error {
		s.state = Established

		var ops []PairOperation

		for id, pair := range m {
			node := key.NodePublic(id)

			sess, ok := s.server.sessByNode[node]

			if ok && sess.state == Established {
				ops = append(ops, PairOperation{
					A:              s.ID,
					B:              sess.ID,
					AN:             s.Peer,
					BN:             sess.Peer,
					VisibilityPair: &pair,
				})
			}
		}

		s.server.pendingPairs <- ops

		return nil
	})
	if err != nil {
		err = fmt.Errorf("could not send greets: %w", err)
		return
	}

	if s.Expiry != (time.Time{}) {
		go func() {
			select {
			case <-s.Ctx.Done():
			// FIXME on suspend/delay/wallclock change, this won't work properly,
			//  find a time-until api that deals with wall-clock differences
			case <-time.After(time.Until(s.Expiry)):
				s.Ccc(ErrNeedsDisconnect)
			}
		}()
	}

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
			s.Slog().Debug("received pong")
			// TODO
		case *msgcontrol.LogonDeviceKey:
			s.Slog().Debug("received after-logon logon device key, ignoring...")
		default:
			err = fmt.Errorf("received unknown type of message: %#v", msg)
			return
		}
	}
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
