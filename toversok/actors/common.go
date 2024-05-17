package actors

import (
	"context"
	"github.com/shadowjonathan/edup2p/types/actor_msg"
)

type ActorCommon struct {
	inbox   chan actor_msg.ActorMessage
	ctx     context.Context
	ctxCan  context.CancelFunc
	running RunCheck
}

func MakeCommon(pCtx context.Context, chLen int) *ActorCommon {
	ctx, ctxCan := context.WithCancel(pCtx)

	var inbox chan actor_msg.ActorMessage = nil

	if chLen >= 0 {
		inbox = make(chan actor_msg.ActorMessage, chLen)
	}

	return &ActorCommon{
		inbox:   inbox,
		ctx:     ctx,
		ctxCan:  ctxCan,
		running: MakeRunCheck(),
	}
}

func (ac *ActorCommon) Inbox() chan<- actor_msg.ActorMessage {
	return ac.inbox
}

func (ac *ActorCommon) Cancel() {
	ac.ctxCan()
}

func (ac *ActorCommon) logUnknownMessage(am actor_msg.ActorMessage) {
	// TODO
}
