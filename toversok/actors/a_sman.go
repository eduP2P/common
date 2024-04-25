package actors

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/types/key"
	msg2 "github.com/shadowjonathan/edup2p/types/msg"
	"slices"
)

// SessionManager receives frames from routers and decrypts them,
// and forwards the resulting messages to the traffic manager.
//
// It also receives messages from the traffic manager, encrypts them,
// and forwards them to the direct/relay managers.
type SessionManager struct {
	*ActorCommon
	s *Stage

	publicKey  key.SessionPublic
	privateKey key.SessionPrivate
}

func (sm *SessionManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(sm).Error("panicked", "panic", v)
			sm.Cancel()
		}
	}()

	if !sm.running.CheckOrMark() {
		L(sm).Warn("tried to run agent, while already running")
		return
	}

	for {
		select {
		case <-sm.ctx.Done():
			sm.Close()
			return
		case inMsg := <-sm.inbox:
			sm.Handle(inMsg)
		}
	}
}

func (sm *SessionManager) Handle(msg ActorMessage) {
	switch m := msg.(type) {
	case *SManSessionFrameFromRelay:
		cm, err := sm.Unpack(m.frameWithMagic)
		if err != nil {
			L(sm).Error("error when unpacking session frame from relay",
				"err", err,
				"peer", m.peer,
				"relay", m.relay,
				"frame", m.frameWithMagic,
			)
			return
		}
		sm.s.TMan.Inbox() <- &TManSessionMessageFromRelay{
			relay: m.relay,
			peer:  m.peer,
			msg:   cm,
		}
	case *SManSessionFrameFromAddrPort:
		cm, err := sm.Unpack(m.frameWithMagic)
		if err != nil {
			L(sm).Error("error when unpacking session frame from direct",
				"err", err,
				"addrport", m.addrPort,
				"frame", m.frameWithMagic,
			)
			return
		}
		sm.s.TMan.Inbox() <- &TManSessionMessageFromDirect{
			addrPort: m.addrPort,
			msg:      cm,
		}
	case *SManSendSessionMessageToRelay:
		frame := sm.Pack(m.msg, m.toSession)
		sm.s.RMan.WriteTo(frame, m.relay, m.peer)
	case *SManSendSessionMessageToDirect:
		frame := sm.Pack(m.msg, m.toSession)
		sm.s.DMan.WriteTo(frame, m.addrPort)
	default:
		sm.logUnknownMessage(m)
	}
}

func (sm *SessionManager) Unpack(frameWithMagic []byte) (*msg2.ClearMessage, error) {
	if string(frameWithMagic[:len(msg2.MagicBytes)]) != msg2.Magic {
		panic("Somehow received non-session message in unpack")
	}

	b := frameWithMagic[len(msg2.MagicBytes):]

	sessionKey := key.MakeSessionPublic([key.Len]byte(b[:32]))

	b = b[key.Len:]

	clearBytes, ok := sm.privateKey.Shared(sessionKey).Open(b)

	if !ok {
		return nil, fmt.Errorf("could not decrypt session message")
	}

	sMsg, err := msg2.ParseSessionMessage(clearBytes)

	if err != nil {
		return nil, fmt.Errorf("could not parse session message: %s", err)
	}

	return &msg2.ClearMessage{
		Session: sessionKey,
		Message: sMsg,
	}, nil
}

func (sm *SessionManager) Pack(sMsg msg2.SessionMessage, toSession key.SessionPublic) []byte {
	clearBytes := sMsg.MarshalSessionMessage()

	cipherBytes := sm.privateKey.Shared(toSession).Seal(clearBytes)

	return slices.Concat(msg2.MagicBytes, sm.publicKey.ToByteSlice(), cipherBytes)
}

func (sm *SessionManager) Close() {
	// noop
}
