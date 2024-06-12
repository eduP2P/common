package control

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgcontrol"
	"github.com/shadowjonathan/edup2p/types/relay"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"time"
)

type Server struct {
	// TODO

	privKey key.ControlPrivate

	sessLock sync.RWMutex
	sessions map[key.NodePublic]*ServerSession

	getIPs func(public key.NodePublic) (netip.Prefix, netip.Prefix)

	// TODO a way to allow the server to dynamically update this
	relays []relay.Information

	// TODO something that remembers/accesses sessions
}

func (s *Server) Logger() *slog.Logger {
	return slog.With("control", s.privKey.Public().Debug())
}

func (s *Server) Accept(ctx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, remoteAddrPort netip.AddrPort) error {
	cc := NewConn(ctx, mc, brw)

	// TODO this logon segment can be in a different function
	{
		// TODO set deadline on read

		var clientHello = new(msgcontrol.ClientHello)
		if err := cc.Expect(clientHello, HandshakeReceiveTimeout); err != nil {
			return fmt.Errorf("error when receiving clienthello: %w", err)
		}

		data := randData()

		if err := cc.Write(&msgcontrol.ServerHello{
			ControlNodePub: s.privKey.Public(),
			CheckData:      s.privKey.SealToNode(clientHello.ClientNodePub, data),
		}); err != nil {
			return fmt.Errorf("error when sending serverhello: %w", err)
		}

		logon := new(msgcontrol.Logon)
		if err := cc.Expect(logon, HandshakeReceiveTimeout); err != nil {
			return fmt.Errorf("error when receiving logon: %w", err)
		}

		// Verify logon
		{
			var nodeData, sessData []byte
			var ok bool

			if nodeData, ok = s.privKey.OpenFromNode(clientHello.ClientNodePub, logon.NodeKeyAttestation); !ok {
				return fmt.Errorf("could not open node attestation")
			}

			if sessData, ok = s.privKey.OpenFromSession(logon.SessKey, logon.SessKeyAttestation); !ok {
				return fmt.Errorf("could not open session attestation")
			}

			// FIXME: we should probably make the below something like constant time, to prevent timing attacks.
			// It is not now, for development purposes.

			if !slices.Equal(data, nodeData) {
				return fmt.Errorf("node data not equal")
			}

			if !slices.Equal(data, sessData) {
				return fmt.Errorf("sess data not equal")
			}
		}

		sess, err := s.ReEstablishOrMakeSession(cc, clientHello.ClientNodePub, logon.SessKey, logon.ResumeSessionID)

		if err != nil {
			reject := &msgcontrol.LogonReject{}

			if errors.Is(err, stillEstablished) {
				if errors.Is(err, stillEstablished) {
					// TODO we need to replace this with knocking-and-acquiring

					reject.RetryStrategy = msgcontrol.RegenerateSessionKey
					reject.RetryAfter = time.Second * 15
					reject.Reason = "other client session still active, please retry"
				} else {
					reject.Reason = "cannot log in at the moment, please retry in the future"
				}
			} else if errors.Is(err, sessionIdMismatch) {
				reject.RetryStrategy = msgcontrol.RecreateSession
				reject.Reason = "session ID mismatch, please try without"
			} else {
				reject.Reason = "could not acquire session"
				slog.Warn("rejected session with unknown error", "err", err)
			}

			if err := cc.Write(reject); err != nil {
				return fmt.Errorf("error when sending reject: %w", err)
			} else {
				return nil
			}
		}

		if logon.ResumeSessionID != nil {
			sess.Resume(cc, logon.SessKey)
		} else {
			go sess.Run()
		}

		select {
		case <-ctx.Done():
			// connection dead, exit
		}

		return ctx.Err()

		//// for now, send a reject
		//if err := cc.Write(&msgcontrol.LogonReject{
		//	Reason:        "dev: reject unambiguously",
		//	RetryStrategy: 0,
		//}); err != nil {
		//	return fmt.Errorf("error when sending reject: %w", err)
		//}

		// TODO send authenticate (then wait, or expect devicekey), accept, or reject

		// TODO resume

		// TODO
		//  1. expect ClientHello
		//  2. send ServerHello
		//  3. expect Logon
		//  4. when reauth required, send LogonAuthenticate
		//      - expect LogonDeviceKey
		//  5. send LogonAccept|LogonReject

		// TODO (here) add to sessions?

		// TODO (here) run session

		// TODO (here) mark session as latent
	}

	//TODO implement me
	panic("implement me")
}

func randData() []byte {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		panic(fmt.Errorf("could not read rand: %w", err))
	}
	return b
}

func NewServer(privKey key.ControlPrivate, getIPs func(public key.NodePublic) (netip.Prefix, netip.Prefix), relays []relay.Information) *Server {
	// TODO give caller a way to "deallocate" IPs and such

	return &Server{
		privKey:  privKey,
		sessions: make(map[key.NodePublic]*ServerSession),
		getIPs:   getIPs,
		relays:   relays,
	}
}

var (
	incorrectState    = errors.New("incorrect state, want nil or Dangling")
	stillEstablished  = errors.New("session is still established or reestablished")
	sessionIdMismatch = errors.New("session ID did not match")
)

func (s *Server) ReEstablishOrMakeSession(cc *Conn, nodeKey key.NodePublic, sessKey key.SessionPublic, sessId *string) (retSess *ServerSession, err error) {
	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	sess, ok := s.sessions[nodeKey]

	if !ok {
		if sessId != nil {
			// There's no session ID to match if its empty.
			// The client requested resume, so we need to tell it to try again without the session ID,
			// kicking internal logic to regenerate session keys and clearing state.
			err = sessionIdMismatch
			return
		}

		// Simple path, we make, store, and return

		retSess = NewSession(cc, nodeKey, sessKey, s)

		slog.Info("CREATE session", "peer", retSess.Peer.Debug())

		s.sessions[nodeKey] = retSess

		return
	} else {
		// less simple path: we have a session in state for this nodekey

		if sess.state != Dangling {
			// We only accept resuming dangling sessions, everything else is incorrect.
			err = incorrectState

			if sess.state == Established || sess.state == ReEstablishing {
				// The server may lag behind for a second, so if we wrap this error and return the session,
				// the caller could knock that session to force it to Dangling.

				err = fmt.Errorf("established state (%w): %w", err, stillEstablished)
				retSess = sess
			}

			return
		} else {
			// Session is dangling, we can grab it

			if sessId != nil && sess.ID != *sessId {
				// Cant resume, the client expects a different session ID

				err = sessionIdMismatch
				return
			}

			retSess = sess

			slog.Info("RESUME session", "peer", sess.Peer.Debug())

			return
		}
	}
}

func (s *Server) RemoveSession(sess *ServerSession) {
	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	mappedSess, ok := s.sessions[sess.Peer]

	if !ok {
		// already removed?
		return
	}

	if sess != mappedSess {
		// not the same session
		return
	}

	if sess.state != Authenticate {
		// others peers know of this session, send remove

		s.ForVisibleLocked(sess, func(session *ServerSession) {
			session.Bye(sess.Peer)
		})
	}

	slog.Info("REMOVE session", "peer", sess.Peer.Debug())

	delete(s.sessions, sess.Peer)
}

func (s *Server) RegisterSession(sess *ServerSession) {
	// TODO resume support

	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	for _, oSess := range s.sessions {
		oSess.Greet(sess)
		sess.Greet(oSess)
	}

	s.sessions[sess.Peer] = sess
}

// ForVisible is called by fromSess' Run goroutine, to inform all other sessions it can see of a change (and the likes)
func (s *Server) ForVisible(fromSess *ServerSession, f func(session *ServerSession)) {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	s.ForVisibleLocked(fromSess, f)
}

func (s *Server) ForVisibleLocked(fromSess *ServerSession, f func(session *ServerSession)) {
	// TODO filtering and such

	for _, oSess := range s.sessions {
		if oSess == fromSess || oSess.state != Established {
			continue
		}

		f(oSess)
	}
}
