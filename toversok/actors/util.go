package actors

import (
	"context"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/msgactor"
	"log/slog"
	"net/netip"
	"sort"
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

func bail(c context.Context, v any) {
	maybeCcc := c.Value(types.CCC)
	if maybeCcc == nil {
		panic(fmt.Errorf("could not bail, cannot find ccc: %s", v))
	}

	probablyCcc, ok := maybeCcc.(context.CancelCauseFunc)

	if !ok {
		panic(fmt.Errorf("could not bail, ccc is not CancelCauseFunc: %s", v))
	}

	probablyCcc(fmt.Errorf("bailing: %s", v))
}

func sortEndpointSlice(endpoints []netip.AddrPort) {
	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].Addr().Less(endpoints[j].Addr()) && endpoints[i].Port() < endpoints[j].Port()
	})
}
