package actors

import (
	"fmt"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
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

	session func() *key.SessionPrivate
}

var DebugSManTakeNodeAsSession = false

func (s *Stage) makeSM(priv func() *key.SessionPrivate) *SessionManager {
	sm := &SessionManager{
		ActorCommon: MakeCommon(s.Ctx, SessManInboxChLen),
		s:           s,
		session:     priv,
	}

	L(sm).Debug("sman with session key", "sess", priv().Public().Debug())

	return sm
}

func (sm *SessionManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(sm).Error("panicked", "panic", v)
			sm.Cancel()
			bail(sm.ctx, v)
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

func (sm *SessionManager) Handle(msg msgactor.ActorMessage) {
	switch m := msg.(type) {
	case *msgactor.SManSessionFrameFromRelay:
		cm, err := sm.Unpack(m.FrameWithMagic)
		if err != nil {
			L(sm).Error("error when unpacking session frame from relay",
				"err", err,
				"peer", m.Peer,
				"relay", m.Relay,
				"frame", m.FrameWithMagic,
			)
			return
		}
		sm.s.TMan.Inbox() <- &msgactor.TManSessionMessageFromRelay{
			Relay: m.Relay,
			Peer:  m.Peer,
			Msg:   cm,
		}
	case *msgactor.SManSessionFrameFromAddrPort:
		cm, err := sm.Unpack(m.FrameWithMagic)
		if err != nil {
			L(sm).Error("error when unpacking session frame from direct",
				"err", err,
				"addrport", m.AddrPort,
				"frame", m.FrameWithMagic,
			)
			return
		}
		sm.s.TMan.Inbox() <- &msgactor.TManSessionMessageFromDirect{
			AddrPort: m.AddrPort,
			Msg:      cm,
		}
	case *msgactor.SManSendSessionMessageToRelay:
		frame := sm.Pack(m.Msg, m.ToSession)
		sm.s.RMan.WriteTo(frame, m.Relay, m.Peer)
	case *msgactor.SManSendSessionMessageToDirect:
		frame := sm.Pack(m.Msg, m.ToSession)
		sm.s.DMan.WriteTo(frame, m.AddrPort)
	default:
		sm.logUnknownMessage(m)
	}
}

func (sm *SessionManager) Unpack(frameWithMagic []byte) (*msgsess.ClearMessage, error) {
	if string(frameWithMagic[:len(msgsess.Magic)]) != msgsess.Magic {
		panic("Somehow received non-session message in unpack")
	}

	b := frameWithMagic[len(msgsess.Magic):]

	sessionKey := key.MakeSessionPublic([key.Len]byte(b[:key.Len]))

	b = b[key.Len:]

	clearBytes, ok := sm.session().Shared(sessionKey).Open(b)

	if !ok {
		return nil, fmt.Errorf("could not decrypt session message")
	}

	sMsg, err := msgsess.ParseSessionMessage(clearBytes)

	if err != nil {
		return nil, fmt.Errorf("could not parse session message: %s", err)
	}

	return &msgsess.ClearMessage{
		Session: sessionKey,
		Message: sMsg,
	}, nil
}

func (sm *SessionManager) Pack(sMsg msgsess.SessionMessage, toSession key.SessionPublic) []byte {
	clearBytes := sMsg.MarshalSessionMessage()

	cipherBytes := sm.session().Shared(toSession).Seal(clearBytes)

	return slices.Concat(msgsess.MagicBytes, sm.session().Public().ToByteSlice(), cipherBytes)
}

func (sm *SessionManager) Session() key.SessionPublic {
	return sm.session().Public()
}

func (sm *SessionManager) Close() {
	// noop
}
