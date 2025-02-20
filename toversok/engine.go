package toversok

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
)

// Engine is the main and most high-level object for any client implementation.
//
// It holds the WireGuardHost, FirewallHost, and ControlHost, and utilises these for connectivity
// with peers, the control server, and maintain these according to Control Server instruction.
type Engine struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	sess *Session

	extBind *types.UDPConnCloseCatcher
	extPort uint16

	wg WireGuardHost
	fw FirewallHost
	co ControlHost

	nodePriv key.NodePrivate

	state         stateObserver
	doAutoRestart bool
	dirty         bool
}

// Start will fire up the Engine.
//
// It will return an error if for any reason it cannot start. Reasons include;
// - It cannot connect to the control server
// - It cannot start the network interface
// - Reason for any other startup error.
//
// After the engine has successfully started once, it will automatically restart on any failure.
func (e *Engine) Start() error {
	return e.start(true)
}

func (e *Engine) start(allowLogon bool) error {
	if e.ctx.Err() != nil {
		// If the engine has been cancelled, do nothing
		return errors.New("engine context already closed")
	}

	if !e.state.change(NoSession, CreatingSession) {
		return errors.New("cannot start; already running")
	}

	if e.sess != nil && e.sess.ctx.Err() == nil {
		// Session is still running, even though that shouldn't be the case, as we checked for NoSession above
		e.sess.ccc(errors.New("engine state desynced, shutting down"))
	}

	if e.dirty {
		if err := e.wg.Reset(); err != nil {
			e.slog().Error("dirty start: could not reset wireguard", "err", err)
			e.state.set(NoSession)
			return err
		}

		if err := e.fw.Reset(); err != nil {
			e.slog().Error("dirty start: could not reset firewall", "err", err)
			e.state.set(NoSession)
			return err
		}
	}

	e.dirty = true

	if err := e.installSession(allowLogon); err != nil {
		return fmt.Errorf("could not install session: %w", err)
	}

	return nil
}

func (e *Engine) Context() context.Context {
	return e.ctx
}

// StalledEngineRestartInterval represents how many seconds to wait before restarting an engine,
// after it has stalled/failed.
const StalledEngineRestartInterval = time.Second * 2

func (e *Engine) autoRestart() {
	if e.WillRestart() {
		if err := e.start(false); err != nil {
			slog.Info("autoRestart: will retry in 10 seconds")
			time.AfterFunc(StalledEngineRestartInterval, e.autoRestart)
		}
	}
}

// Stop the engine.
func (e *Engine) Stop() {
	if !(e.state.change(Established, StoppingSession) || e.state.change(CreatingSession, StoppingSession) || e.state.change(NeedsLogin, StoppingSession)) {
		// Already stopped or stopping
		return
	}

	e.doAutoRestart = false

	if e.sess.ctx.Err() != nil {
		e.sess.ccc(errors.New("shutting down"))
	}

	var stillDirty bool

	if err := e.wg.Reset(); err != nil {
		e.slog().Warn("stop: error when resetting wireguard", "err", err)
		stillDirty = true
	}
	if err := e.fw.Reset(); err != nil {
		e.slog().Warn("stop: error when resetting firewall", "err", err)
		stillDirty = true
	}

	if !stillDirty {
		e.dirty = false
	}

	e.state.change(StoppingSession, NoSession)
}

// Assumes stateLock is held
func (e *Engine) installSession(allowLogon bool) error {
	// TODO check for logon somewhere and stop engine

	var logon types.LogonCallback

	if allowLogon {
		logon = func(url string, _ chan<- string) error {
			// TODO register/use device key channel

			e.state.alter(func(o *stateObserver) {
				o.currentLoginURL = url
			})

			e.state.change(CreatingSession, NeedsLogin)
			return nil
		}
	}

	var err error
	e.sess, err = SetupSession(e.ctx, e.wg, e.fw, e.co, e.getExtConn, e.getNodePriv, logon)
	if err != nil {
		return fmt.Errorf("failed to setup session: %w", err)
	}

	e.state.alter(func(o *stateObserver) {
		o.expiry = e.sess.cs.Expiry()
	})

	if !(e.state.change(CreatingSession, Established) || e.state.change(NeedsLogin, Established)) {
		e.ccc(errors.New("incorrect state transition"))
		panic("incorrect state transition to established")
	}

	context.AfterFunc(e.sess.ctx, func() {
		e.state.set(NoSession)
		e.autoRestart()
	})

	e.sess.Start()

	return err
}

// WillRestart says whether the engine strives to be in a running state.
func (e *Engine) WillRestart() bool {
	return e.doAutoRestart
}

func (e *Engine) slog() *slog.Logger {
	return slog.With("from", "engine")
}

func newStateObserver() stateObserver {
	return stateObserver{}
}

type stateObserver struct {
	mu        sync.Mutex
	state     EngineState
	callbacks []func(state EngineState)

	currentLoginURL string
	expiry          time.Time
}

func (s *stateObserver) CurrentState() EngineState {
	return s.state
}

func (s *stateObserver) RegisterStateChangeListener(f func(state EngineState)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callbacks = append(s.callbacks, f)
}

var ErrWrongState = errors.New("wrong state")

func (s *stateObserver) GetNeedsLoginState() (url string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != NeedsLogin {
		return "", ErrWrongState
	}

	return s.currentLoginURL, nil
}

func (s *stateObserver) GetEstablishedState() (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != Established {
		return time.Time{}, ErrWrongState
	}

	return s.expiry, nil
}

func (s *stateObserver) change(oldState, newState EngineState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != oldState {
		return false
	}

	slog.Debug("changing state", "oldState", oldState.String(), "newState", newState.String())

	s.state = newState

	s.asyncFireCallbacks(newState)

	return true
}

func (s *stateObserver) set(newState EngineState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	slog.Debug("setting state", "newState", newState.String())

	s.state = newState

	s.asyncFireCallbacks(newState)
}

func (s *stateObserver) alter(f func(observer *stateObserver)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f(s)
}

func (s *stateObserver) asyncFireCallbacks(state EngineState) {
	for _, cb := range s.callbacks {
		go cb(state)
	}
}

// TODO add status update event channels (to display connection status, control status, session status, IP, etc.)

// NewEngine creates a new engine and initiates it.
//
// `parentCtx` can be `nil`, will assume `context.Background()`.
func NewEngine(
	parentCtx context.Context,
	wg WireGuardHost,
	fw FirewallHost,
	co ControlHost,

	extBindPort uint16,

	privateKey key.NodePrivate,
) (*Engine, error) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	switch {
	case wg == nil:
		return nil, errors.New("cannot initialise toversok engine with nil WireGuardHost")
	case fw == nil:
		return nil, errors.New("cannot initialise toversok engine with nil FirewallHost")
	case co == nil:
		return nil, errors.New("cannot initialise toversok engine with nil ControlHost")
	case privateKey.IsZero():
		return nil, errors.New("cannot initialise toversok engine with zero privateKey")
	}

	ctx, ccc := context.WithCancelCause(parentCtx)

	e := &Engine{
		ctx:  ctx,
		ccc:  ccc,
		sess: nil,

		extBind: nil,
		extPort: extBindPort,

		wg: wg,
		fw: fw,
		co: co,

		nodePriv: privateKey,
		state:    newStateObserver(),
	}

	e.Observer().RegisterStateChangeListener(func(state EngineState) {
		if state == NeedsLogin {
			url, err := e.Observer().GetNeedsLoginState()
			if err == nil {
				e.slog().Info("control wants logon", "url", url)
			} else {
				e.slog().Error("could not get login state when prompted for it", "err", err)
			}
		} else if state == Established {
			expiry, err := e.Observer().GetEstablishedState()
			if err != nil {
				panic("should never happen")
			}
			if expiry != (time.Time{}) {
				slog.Info("established session with expiry", "expiry", expiry, "in", time.Until(expiry))
			}
		}
	})

	return e, nil
}

func (e *Engine) getNodePriv() *key.NodePrivate {
	return &e.nodePriv
}

func (e *Engine) getExtConn() types.UDPConn {
	if e.extBind == nil || e.extBind.Closed {
		conn, err := e.bindExt()
		if err != nil {
			panic(fmt.Sprintf("could not bind ext: %s", err))
		}

		slog.Info("created ext sock", "addr", conn.LocalAddr().String(), "extPort", e.extPort)

		e.extBind = &types.UDPConnCloseCatcher{
			UDPConn: conn,
			Closed:  false,
		}
	}

	return e.extBind
}

func (e *Engine) bindExt() (*net.UDPConn, error) {
	ua := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.IPv4Unspecified(), e.extPort)) // 42069

	return net.ListenUDP("udp", ua)
}

func (e *Engine) Observer() Observer {
	return &e.state
}

func (e *Engine) SupplyDeviceKey(string) error {
	// TODO
	panic("not implemented")
}

type FakeControl struct {
	controlKey key.ControlPublic
	ipv4       netip.Prefix
	ipv6       netip.Prefix
}

func (f *FakeControl) ControlKey() key.ControlPublic {
	return f.controlKey
}

func (f *FakeControl) IPv4() netip.Prefix {
	return f.ipv4
}

func (f *FakeControl) IPv6() netip.Prefix {
	return f.ipv6
}

func (f *FakeControl) Expiry() time.Time {
	return time.Time{}
}

func (f *FakeControl) Context() context.Context {
	return context.Background()
}

func (f *FakeControl) InstallCallbacks(ifaces.ControlCallbacks) {
	// NOP
}

func (f *FakeControl) UpdateEndpoints([]netip.AddrPort) error {
	// NOP
	return nil
}

func (f *FakeControl) UpdateHomeRelay(int64) error {
	// NOP
	return nil
}
