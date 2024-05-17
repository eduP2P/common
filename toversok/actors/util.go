package actors

import (
	"context"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/actor_msg"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"log/slog"
	"sync/atomic"
)

// RunCheck ensures that only one instance of the actor is running at all times.
type RunCheck struct {
	*atomic.Bool
}

func MakeRunCheck() RunCheck {
	return RunCheck{
		&atomic.Bool{},
	}
}

// CheckOrMark atomically checks if its already running, else marks as running, returns a false value if the instance is already running.
func (rc *RunCheck) CheckOrMark() bool {
	return rc.CompareAndSwap(false, true)
}

// IsContextDone does a quick check on a context to see if its dead.
func IsContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// SendMessage is a convenience function to allow for "go SendMessage()"
func SendMessage(ch chan<- actor_msg.ActorMessage, msg actor_msg.ActorMessage) {
	ch <- msg
}

func L(a ifaces.Actor) *slog.Logger {
	return slog.With("actor", fmt.Sprintf("%T", a))
}
