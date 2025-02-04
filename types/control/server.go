package control

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
	"github.com/edup2p/common/types/relay"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"time"
)

type Server struct {
	// TODO

	privKey key.ControlPrivate

	sessLock   sync.RWMutex
	sessByNode map[key.NodePublic]*ServerSession
	sessByID   map[string]*ServerSession

	callbacks ServerCallbacks

	// TODO a way to allow the server to dynamically update this
	relays []relay.Information

	vGraph *EdgeGraph
	// The intention of this lock is as follows;
	//  - it is held by any session transitioning from authenticating to established, to grab all connections
	//  - when business logic adds pairs, it'll lock when adding to vGraph, and when adding to pending,
	//    to make it one atomic operation
	pendingLock  sync.Mutex
	pendingPairs chan []PairOperation

	// TODO something that remembers/accesses sessions
}

type PairOperation struct {
	// Session IDs
	A, B string

	AN, BN key.NodePublic

	// If nil, then its Bye
	*VisibilityPair
}

func (s *Server) Run() {
	// TODO maybe embedded run with rescue?

	// TODO listen on pendingPairs;
	//  - if Greet, verify both sessions IDs online, then send
	//  - if Bye, send best-case Bye to both session IDs

	for {
		ops := <-s.pendingPairs

		for _, op := range ops {
			s.handleOp(op)
		}
	}
}

func (s *Server) handleOp(op PairOperation) {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	if op.VisibilityPair != nil {
		sessA, okA := s.sessByID[op.A]
		sessB, okB := s.sessByID[op.B]

		if !okA || !okB {
			// Cannot greet non-existent sessions together

			return
		}

		// FIXME this can race? but probably not? (if pendingLock is used for adding all joining sessions)
		// TODO this misses dangling sessions, catch them with rewrite
		if sessA.state != Established || sessB.state != Established {
			// Cannot greet non-established sessions together

			return
		}

		// TODO this doesn't include MDNS and optional visibility concerns, such as quarantine
		sessA.Greet(sessB)
		sessB.Greet(sessA)
	} else {
		sessA, okA := s.sessByID[op.A]
		sessB, okB := s.sessByID[op.B]

		if okA && sessA.state == Established {
			sessA.Bye(op.BN)
		}

		if okB && sessB.state == Established {
			sessB.Bye(op.AN)
		}
	}
}

func (s *Server) Logger() *slog.Logger {
	return slog.With("control", s.privKey.Public().Debug())
}

func (s *Server) Accept(ctx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, remoteAddrPort netip.AddrPort) error {
	cc := NewConn(ctx, mc, brw)

	// TODO this logon segment can be in a different function
	{
		// TODO set deadline on read

		err, clientHello, logon := s.handleLogon(cc)

		if err != nil {
			return fmt.Errorf("handle logon: %w", err)
		}

		sess, resumed, err := s.ReEstablishOrMakeSession(cc, clientHello.ClientNodePub, logon.SessKey, logon.ResumeSessionID)

		if err != nil {
			return s.doReject(cc, sess, err)
		}

		if err := sess.doAuthenticate(resumed); err != nil {
			return fmt.Errorf("authenticate returned with error: %w", err)
		}

		if resumed { // logon.ResumeSessionID != nil
			sess.Resume(cc, logon.SessKey)
		} else {
			if err = sess.AuthAndStart(); err != nil {
				return err
			}
			//go sess.Run()
		}

		// Wait until connection dead
		<-ctx.Done()

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

func (s *Server) handleLogon(cc *Conn) (error, *msgcontrol.ClientHello, *msgcontrol.Logon) {
	// TODO set deadline on read

	var clientHello = new(msgcontrol.ClientHello)
	if err := cc.Expect(clientHello, HandshakeReceiveTimeout); err != nil {
		return fmt.Errorf("error when receiving clienthello: %w", err), nil, nil
	}

	data := randData()

	if err := cc.Write(&msgcontrol.ServerHello{
		ControlNodePub: s.privKey.Public(),
		CheckData:      s.privKey.SealToNode(clientHello.ClientNodePub, data),
	}); err != nil {
		return fmt.Errorf("error when sending serverhello: %w", err), nil, nil
	}

	logon := new(msgcontrol.Logon)
	if err := cc.Expect(logon, HandshakeReceiveTimeout); err != nil {
		return fmt.Errorf("error when receiving logon: %w", err), nil, nil
	}

	// Verify logon
	{
		var nodeData, sessData []byte
		var ok bool

		if nodeData, ok = s.privKey.OpenFromNode(clientHello.ClientNodePub, logon.NodeKeyAttestation); !ok {
			return fmt.Errorf("could not open node attestation"), nil, nil
		}

		if sessData, ok = s.privKey.OpenFromSession(logon.SessKey, logon.SessKeyAttestation); !ok {
			return fmt.Errorf("could not open session attestation"), nil, nil
		}

		// FIXME: we should probably make the below something like constant time, to prevent timing attacks.
		//  It is not now, for development purposes.

		if !slices.Equal(data, nodeData) {
			return fmt.Errorf("node data not equal"), nil, nil
		}

		if !slices.Equal(data, sessData) {
			return fmt.Errorf("sess data not equal"), nil, nil
		}
	}

	return nil, clientHello, logon
}

func (s *Server) doReject(cc *Conn, sess *ServerSession, err error) error {

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

	var peerStr string
	if sess != nil {
		peerStr = sess.Peer.Debug()
	} else {
		peerStr = "<sess nil>"
	}

	slog.Debug("rejected peer", "reason", reject.Reason, "peer", peerStr)

	if err := cc.Write(reject); err != nil {
		return fmt.Errorf("error when sending reject: %w", err)
	} else {
		return nil
	}
}

func randData() []byte {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		panic(fmt.Errorf("could not read rand: %w", err))
	}
	return b
}

func NewServer(privKey key.ControlPrivate, relays []relay.Information) *Server {
	// TODO give caller a way to "deallocate" IPs and such

	s := &Server{
		privKey:    privKey,
		sessLock:   sync.RWMutex{},
		sessByNode: make(map[key.NodePublic]*ServerSession),
		sessByID:   make(map[string]*ServerSession),
		//getIPs:   getIPs,
		relays:       relays,
		vGraph:       NewEdgeGraph(),
		pendingLock:  sync.Mutex{},
		pendingPairs: make(chan []PairOperation, 128),
	}

	go s.Run()

	return s
}

var (
	incorrectState    = errors.New("incorrect state, want nil or Dangling")
	stillEstablished  = errors.New("session is still established or reestablished")
	sessionIdMismatch = errors.New("session ID did not match")
)

func (s *Server) ReEstablishOrMakeSession(cc *Conn, nodeKey key.NodePublic, sessKey key.SessionPublic, sessId *string) (retSess *ServerSession, resumed bool, err error) {
	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	sess, ok := s.sessByNode[nodeKey]

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

		s.sessByNode[nodeKey] = retSess
		s.sessByID[retSess.ID] = retSess

		return
	}

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
	}

	// Session is dangling, we can grab it
	if sessId != nil && sess.ID != *sessId {
		// Cant resume, the client expects a different session ID

		err = sessionIdMismatch
		return
	}

	retSess = sess

	slog.Info("RESUME session", "peer", sess.Peer.Debug())
	resumed = true

	return
}

func (s *Server) RemoveSession(sess *ServerSession) {
	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	mappedSess, ok := s.sessByNode[sess.Peer]

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

		err := s.atomicGetVisibilityPairs(sess.Peer, func(m map[ClientID]VisibilityPair) error {
			var ops []PairOperation

			for id2 := range m {
				sess2, ok2 := s.sessByNode[key.NodePublic(id2)]

				if ok2 {
					ops = append(ops, PairOperation{
						A:              sess.ID,
						B:              sess2.ID,
						AN:             sess.Peer,
						BN:             sess2.Peer,
						VisibilityPair: nil,
					})
				}
			}

			s.pendingPairs <- ops

			return nil
		})

		if err != nil {
			slog.Error("failed to remove sessions", "err", err)
		}

		//s.ForVisibleLocked(sess, func(session *ServerSession) {
		//	session.Bye(sess.Peer)
		//})
	}

	slog.Info("REMOVE session", "peer", sess.Peer.Debug())

	delete(s.sessByNode, sess.Peer)
	delete(s.sessByID, sess.ID)
}

func (s *Server) RegisterSession(sess *ServerSession) {
	// TODO resume support

	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	for _, oSess := range s.sessByNode {
		oSess.Greet(sess)
		sess.Greet(oSess)
	}

	s.sessByNode[sess.Peer] = sess
	s.sessByID[sess.ID] = sess
}

// ForVisible is called by fromSess' Run goroutine, to inform all other sessions it can see of a change (and the likes)
func (s *Server) ForVisible(fromSess *ServerSession, f func(session *ServerSession)) {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	for cid := range s.vGraph.GetEdges(ClientID(fromSess.Peer)) {
		oSess, ok := s.sessByNode[key.NodePublic(cid)]

		if !ok || oSess.state != Established {
			continue
		}

		f(oSess)
	}
}
