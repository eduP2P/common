package control

import (
	"errors"

	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
)

var nilClientID = ClientID{}

func (s *Server) RegisterCallbacks(cb ServerCallbacks) {
	s.callbacks = cb
}

func (s *Server) whenSessAuthenticating(id SessID, f func(*ServerSession) error) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	sid := string(id)

	sess, ok := s.sessByID[sid]

	if !ok {
		return ErrSessionDoesNotExist
	}

	if sess.state != Authenticate {
		return ErrSessionIsNotAuthenticating
	}

	return f(sess)
}

func (s *Server) SendAuthURL(id SessID, url string) error {
	return s.whenSessAuthenticating(id, func(sess *ServerSession) error {
		sess.authChan <- AuthURL{url: url}

		return nil
	})
}

func (s *Server) AcceptAuthentication(id SessID) error {
	return s.whenSessAuthenticating(id, func(sess *ServerSession) error {
		sess.authChan <- AcceptAuth{}

		return nil
	})
}

func (s *Server) RejectAuthentication(id SessID, reason string) error {
	return s.whenSessAuthenticating(id, func(sess *ServerSession) error {
		sess.authChan <- RejectAuth{
			&msgcontrol.LogonReject{
				Reason: reason,
			},
		}

		return nil
	})
}

func (s *Server) GetClientID(id SessID) (ClientID, error) {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	sid := string(id)

	sess, ok := s.sessByID[sid]

	if !ok {
		return nilClientID, ErrSessionDoesNotExist
	}

	return ClientID(sess.Peer), nil
}

func (s *Server) GetConnectedClients() (map[SessID]ClientID, error) {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	retMap := make(map[SessID]ClientID)

	for k, v := range s.sessByID {
		retMap[SessID(k)] = ClientID(v.Peer)
	}

	return retMap, nil
}

func (s *Server) UpsertVisibilityPair(id, id2 ClientID, pair VisibilityPair) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	s.pendingLock.Lock()
	defer s.pendingLock.Unlock()

	if err := s.vGraph.UpsertEdge(id, id2, &pair); err != nil {
		return err
	}

	sess1, ok1 := s.sessByNode[key.NodePublic(id)]
	sess2, ok2 := s.sessByNode[key.NodePublic(id2)]

	if !ok1 || !ok2 {
		// nobody to notify
		return nil
	}

	if sess1.state != Established || sess2.state != Established {
		// either state is not ready yet
		// TODO if its dangling, we need to inform of pending updates and such

		return nil
	}

	s.pendingPairs <- []PairOperation{{
		A:              sess1.ID,
		B:              sess2.ID,
		AN:             sess1.Peer,
		BN:             sess2.Peer,
		VisibilityPair: &pair,
	}}

	return nil
}

func (s *Server) UpsertMultiVisibilityPair(id ClientID, m map[ClientID]VisibilityPair) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	s.pendingLock.Lock()
	defer s.pendingLock.Unlock()

	var ops []PairOperation

	sess1, ok1 := s.sessByNode[key.NodePublic(id)]

	for id2, pair := range m {
		if err := s.vGraph.UpsertEdge(id, id2, &pair); err != nil {
			return err
		}

		sess2, ok2 := s.sessByNode[key.NodePublic(id2)]

		if !ok1 || !ok2 {
			// nobody to notify
			continue
		}

		if sess1.state != Established || sess2.state != Established {
			// either state is not ready yet
			// TODO if its dangling, we need to inform of pending updates and such

			continue
		}

		ops = append(ops, PairOperation{
			A:              sess1.ID,
			B:              sess2.ID,
			AN:             sess1.Peer,
			BN:             sess2.Peer,
			VisibilityPair: &pair,
		})
	}

	s.pendingPairs <- ops

	return nil
}

func (s *Server) RemoveVisibilityPair(from, to ClientID) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	s.pendingLock.Lock()
	defer s.pendingLock.Unlock()

	sess1, ok1 := s.sessByNode[key.NodePublic(from)]
	sess2, ok2 := s.sessByNode[key.NodePublic(to)]

	if err := s.vGraph.RemoveEdge(from, to); err != nil {
		return err
	}

	if !ok1 || !ok2 {
		// nobody to notify
		return nil
	}

	if sess1.state != Established || sess2.state != Established {
		// either state is not ready yet
		// TODO if its dangling, we need to inform of pending updates and such

		return nil
	}

	s.pendingPairs <- []PairOperation{{
		A:  sess1.ID,
		B:  sess2.ID,
		AN: sess1.Peer,
		BN: sess2.Peer,
	}}

	return nil
}

func (s *Server) GetVisibilityPairs(id ClientID) (map[ClientID]VisibilityPair, error) {
	pairs := s.vGraph.GetEdges(id)

	if pairs == nil {
		return nil, errors.New("client id is not known")
	} else if len(pairs) == 0 {
		return nil, errors.New("client does not have any edges")
	}

	return pairs, nil
}

func (s *Server) DisconnectSession(id SessID) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	sess, ok := s.sessByID[string(id)]

	if !ok {
		return ErrSessionDoesNotExist
	}

	sess.Ccc(ErrNeedsDisconnect)

	return nil
}

func (s *Server) DisconnectClient(id ClientID) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	sess, ok := s.sessByNode[key.NodePublic(id)]

	if !ok {
		return ErrClientNotConnected
	}

	sess.Ccc(ErrNeedsDisconnect)

	return nil
}

//nolint:unused
func (s *Server) atomicDoVisibilityPairs(id key.NodePublic, f func(map[ClientID]VisibilityPair) error) error {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	return s.sessLockedDoVisibilityPairs(id, f)
}

func (s *Server) sessLockedDoVisibilityPairs(id key.NodePublic, f func(map[ClientID]VisibilityPair) error) error {
	s.pendingLock.Lock()
	defer s.pendingLock.Unlock()

	return f(s.vGraph.GetEdges(ClientID(id)))
}
