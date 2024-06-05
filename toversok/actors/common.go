package actors

import (
	"context"
	"github.com/shadowjonathan/edup2p/types/msgactor"
	"log/slog"
)

type ActorCommon struct {
	inbox   chan msgactor.ActorMessage
	ctx     context.Context
	ctxCan  context.CancelFunc
	running RunCheck
}

func MakeCommon(pCtx context.Context, chLen int) *ActorCommon {
	ctx, ctxCan := context.WithCancel(pCtx)

	var inbox chan msgactor.ActorMessage = nil

	if chLen >= 0 {
		inbox = make(chan msgactor.ActorMessage, chLen)
	}

	return &ActorCommon{
		inbox:   inbox,
		ctx:     ctx,
		ctxCan:  ctxCan,
		running: MakeRunCheck(),
	}
}

func (ac *ActorCommon) Inbox() chan<- msgactor.ActorMessage {
	return ac.inbox
}

func (ac *ActorCommon) Cancel() {
	ac.ctxCan()
}

func (ac *ActorCommon) logUnknownMessage(am msgactor.ActorMessage) {
	// TODO make better; somehow get actor name in there
	slog.Error("got unknown message", "ac", ac, "am", am)
}
