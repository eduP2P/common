package actors

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/msgactor"
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

// SendMessage is a convenience function to allow for "go SendMessage()"
func SendMessage(ch chan<- msgactor.ActorMessage, msg msgactor.ActorMessage) {
	ch <- msg
}

func L(a ifaces.Actor) *slog.Logger {
	return slog.With("actor", fmt.Sprintf("%T", a))
}
