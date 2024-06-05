package toversok

import (
	"context"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/control"
	"github.com/shadowjonathan/edup2p/types/control/controlhttp"
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgcontrol"
	"golang.org/x/exp/maps"
	"log/slog"
	"net/netip"
	"sync"
	"time"
)

type DefaultControlHost struct {
	// TODO maybe move this struct somewhere else?

	Opts dial.Opts
	Key  key.ControlPublic
}

func (d *DefaultControlHost) CreateClient(
	pCtx context.Context, getNode func() *key.NodePrivate, getSess func() *key.SessionPrivate,
) (ifaces.FullControlInterface, error) {
	return CreateControlSession(pCtx, d.Opts, d.Key,
		getNode,
		getSess,
	)
}

const MaxAbsence = 10 * time.Minute

// ResumableControlSession represents a connection to control that'll be automatically reconnected and resumed,
// when it breaks.
//
// It'll only permanently fail when a connection cannot be established for an "absence" duration, Control rejects
// a logon with NoRetryStrategy or RegenerateSessionKey, or when authentication is required.
type ResumableControlSession struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	// Airlifted out of Client, expected to stay the same as long as the session does
	ipv4       netip.Prefix
	ipv6       netip.Prefix
	controlKey key.ControlPublic

	session string
	client  *control.Client

	clientOpts dial.Opts
	getPriv    func() *key.NodePrivate
	getSess    func() *key.SessionPrivate

	knownPeers map[key.NodePublic]bool

	queueMutex sync.Mutex
	// Out to control
	msgOutQueue []msgcontrol.ControlMessage
	// In to local
	msgInQueue []msgcontrol.ControlMessage

	callbacks ifaces.ControlCallbacks
}

func CreateControlSession(ctx context.Context, opts dial.Opts, controlKey key.ControlPublic, getPriv func() *key.NodePrivate, getSess func() *key.SessionPrivate) (*ResumableControlSession, error) {
	// TODO authCallback func(url string)

	rcsCtx, rcsCcc := context.WithCancelCause(ctx)

	clientCtx := context.WithoutCancel(rcsCtx)
	c, err := controlhttp.Dial(clientCtx, opts, getPriv, getSess, controlKey, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create control session: %w", err)
	}

	slog.Debug("created initial control connection")

	rcs := &ResumableControlSession{
		ctx: rcsCtx,
		ccc: rcsCcc,

		ipv4:       c.IPv4,
		ipv6:       c.IPv6,
		controlKey: c.ControlKey,

		session: *c.SessionID,
		client:  c,

		knownPeers: make(map[key.NodePublic]bool),

		clientOpts: opts,
		getPriv:    getPriv,
		getSess:    getSess,
	}

	go rcs.Run()

	return rcs, nil
}

func (rcs *ResumableControlSession) Run() {

	go func() {
		<-rcs.ctx.Done()

		slog.Debug("ResumableControlSession exited", "err", context.Cause(rcs.ctx))
	}()

	slog.Debug("ResumableControlSession, starting Run")

	for {
		// main control loop

		for {
			// recv loop

			c := rcs.client

			if c == nil {
				// ??? bail
				break
			}

			err := rcs.FlushOut()

			if err != nil {
				slog.Warn("control connection errored while flushing out", "err", err)

				break
			}

			msg, err := c.Recv(time.Second * 5)

			if types.IsContextDone(rcs.ctx) {
				rcs.client.Close()
				return
			}

			if err != nil {
				slog.Warn("control connection errored", "err", err)

				break
			}

			callbacksReady := rcs.CallbacksReady()

			if callbacksReady {
				if err := rcs.FlushIn(); err != nil {
					slog.Error("error while flushing in", "msg", msg, "err", err)
				}
			}

			if msg != nil {
				if callbacksReady {
					if err := rcs.Handle(msg); err != nil {
						slog.Error("error while handling message", "msg", msg, "err", err)
						// TODO bail here?
					}
				} else {
					rcs.QueueIn(msg)
				}
			}
		}

		rcs.client = nil

		absenceStart := time.Now()

		var session = &rcs.session
		var err error
		var client *control.Client

		for {
			if time.Since(absenceStart) > MaxAbsence {
				rcs.ccc(fmt.Errorf("max absence reached, bailing"))
			}

			clientCtx := context.WithoutCancel(rcs.ctx)

			client, err = controlhttp.Dial(
				clientCtx, rcs.clientOpts, rcs.getPriv, rcs.getSess, rcs.controlKey, session,
			)

			var r = msgcontrol.NoRetryStrategy
			var retry = &r

			if err != nil {
				if errors.As(err, retry) {
					//goland:noinspection GoDirectComparisonOfErrors
					if *retry == msgcontrol.RecreateSession {
						session = nil

						slog.Debug("retrying connection without session")

						continue
					}

					rcs.ccc(fmt.Errorf("got logonReject with incompatible reject strategy: %w", err))

					return
				}

				// retry/resume
				continue
			}

			slog.Debug("resumed control connection")

			break
		}

		rcs.ClearPeers()

		rcs.client = client

		// wrap around
	}
}

func (rcs *ResumableControlSession) Handle(msg msgcontrol.ControlMessage) error {
	slog.Debug("Handle", "msg", msg)

	switch m := msg.(type) {
	case *msgcontrol.PeerAddition:
		rcs.knownPeers[m.PubKey] = true
		return rcs.callbacks.AddPeer(
			m.PubKey,
			m.HomeRelay,
			m.Endpoints,
			m.SessKey,
			m.IPv4,
			m.IPv6,
		)
	case *msgcontrol.PeerUpdate:
		var endpoints []netip.AddrPort
		if m.Endpoints != nil {
			endpoints = m.Endpoints
		}

		return rcs.callbacks.UpdatePeer(
			m.PubKey,
			m.HomeRelay,
			endpoints,
			m.SessKey,
		)
	case *msgcontrol.PeerRemove:
		delete(rcs.knownPeers, m.PubKey)
		return rcs.callbacks.RemovePeer(m.PubKey)
	case *msgcontrol.RelayUpdate:
		return rcs.callbacks.UpdateRelays(m.Relays)
	default:
		return fmt.Errorf("got unexpected message from control: %v", msg)
	}

}

func (rcs *ResumableControlSession) CallbacksReady() bool {
	return rcs.callbacks != nil
}

func (rcs *ResumableControlSession) FlushIn() error {
	rcs.queueMutex.Lock()
	defer rcs.queueMutex.Unlock()

	if rcs.msgInQueue != nil {
		for _, msg := range rcs.msgInQueue {
			err := rcs.Handle(msg)
			if err != nil {
				return err
			}
		}

		rcs.msgInQueue = nil
	}

	return nil
}

func (rcs *ResumableControlSession) FlushOut() error {
	rcs.queueMutex.Lock()
	defer rcs.queueMutex.Unlock()

	if rcs.msgOutQueue != nil {
		for _, msg := range rcs.msgOutQueue {
			err := rcs.client.Send(msg)
			if err != nil {
				return err
			}
		}

		rcs.msgOutQueue = nil
	}

	return nil
}

func (rcs *ResumableControlSession) ClearPeers() {
	for pub := range rcs.knownPeers {
		if err := rcs.callbacks.RemovePeer(pub); err != nil {
			slog.Warn("error when removing peer", "err", err)
		}
	}

	maps.Clear(rcs.knownPeers)
}

func (rcs *ResumableControlSession) ControlKey() key.ControlPublic {
	return rcs.controlKey
}

func (rcs *ResumableControlSession) IPv4() netip.Prefix {
	return rcs.ipv4
}

func (rcs *ResumableControlSession) IPv6() netip.Prefix {
	return rcs.ipv6
}

func (rcs *ResumableControlSession) InstallCallbacks(callbacks ifaces.ControlCallbacks) {
	rcs.callbacks = callbacks
}

func (rcs *ResumableControlSession) send(msg msgcontrol.ControlMessage) error {
	client := rcs.client
	if client != nil {
		err := client.Send(msg)

		if err == nil {
			return nil
		}

		if !errors.Is(err, control.ClosedErr) {
			return err
		}
	}

	rcs.QueueOut(msg)

	return nil
}

func (rcs *ResumableControlSession) UpdateEndpoints(endpoints []netip.AddrPort) error {
	return rcs.send(&msgcontrol.EndpointUpdate{Endpoints: endpoints})
}

func (rcs *ResumableControlSession) QueueIn(msg msgcontrol.ControlMessage) {
	rcs.queueMutex.Lock()
	defer rcs.queueMutex.Unlock()

	rcs.msgInQueue = append(rcs.msgInQueue, msg)
}

func (rcs *ResumableControlSession) QueueOut(msg msgcontrol.ControlMessage) {
	rcs.queueMutex.Lock()
	defer rcs.queueMutex.Unlock()

	rcs.msgOutQueue = append(rcs.msgOutQueue, msg)
}
