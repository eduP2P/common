package toversok

import (
	"context"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/toversok/actors"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

type EngineOptions struct {
	Ctx context.Context
	Ccc context.CancelCauseFunc

	PrivKey key.NakedKey

	Control    dial.Opts
	ControlKey key.ControlPublic

	// Do not contact control
	OverrideControl bool
	OverrideIPv4    netip.Prefix
	OverrideIPv6    netip.Prefix

	ExtBindPort uint16

	WG WireGuardConfigurator
	// TODO use fw
	FW FirewallConfigurator
}

type mapping struct {
	conn types.UDPConnCloseCatcher

	port uint16
}

func mappingFromUDPConn(udp *net.UDPConn) *mapping {
	ap := netip.MustParseAddrPort(udp.LocalAddr().String())

	return &mapping{
		conn: types.UDPConnCloseCatcher{UDPConn: udp},
		port: ap.Port(),
	}
}

// TODO add stall support;
//   make sure control logon rejects (with retry) are handled by restarting the entire engine

type Engine struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	stage ifaces.Stage

	// Mapping of peers to local ports
	localMapping map[key.NodePublic]*mapping
	extBind      *types.UDPConnCloseCatcher

	nodePriv key.NodePrivate
	sessPriv key.SessionPrivate

	extPort uint16

	wg     WireGuardConfigurator
	wgPort uint16

	fw FirewallConfigurator

	started bool
}

func (e *Engine) Start() error {
	if e.started {
		return errors.New("already started")
	}

	e.stage.Start()

	// TODO?

	e.started = true

	return nil
}

func (e *Engine) GetSession() key.SessionPublic {
	// TODO
	panic("todo")
}

func (e *Engine) GetEndpoints() []netip.AddrPort {
	// TODO
	panic("todo")
}

func (e *Engine) EndpointUpdates() <-chan []netip.AddrPort {
	// TODO
	panic("todo")
}

func (e *Engine) GetHomeRelay() int64 {
	// TODO
	panic("todo")
}

func (e *Engine) HomeRelayUpdates() <-chan int64 {
	// TODO
	panic("todo")
}

// NewEngine creates a new engine and initiates it
func NewEngine(opts EngineOptions) (*Engine, error) {
	if opts.WG == nil {
		return nil, errors.New("cannot initialise toversok with nil WireGuardConfigurator")
	}

	if opts.FW == nil {
		slog.Warn("initialising toversok with nil FirewallConfigurator")
	}

	var nodePriv = key.NodePrivateFrom(opts.PrivKey)

	var sessPriv key.SessionPrivate

	const DEBUG = true

	if DEBUG {
		sessPriv = key.DevNewSessionFromPrivate(nodePriv)
	} else {
		sessPriv = key.NewSession()
	}

	e := &Engine{
		ctx: opts.Ctx,
		ccc: opts.Ccc,

		localMapping: make(map[key.NodePublic]*mapping),

		extPort: opts.ExtBindPort,

		nodePriv: nodePriv,
		sessPriv: sessPriv,

		wg: opts.WG,
		fw: opts.FW,
	}

	var c ifaces.FullControlInterface
	var err error

	if opts.OverrideControl {
		c = &FakeControl{
			controlKey: opts.ControlKey,
			ipv4:       opts.OverrideIPv4,
			ipv6:       opts.OverrideIPv6,
		}
	} else {
		c, err = e.login(opts.Control, opts.ControlKey)
		if err != nil {
			return nil, fmt.Errorf("could not create control client: %w", err)
		}
	}

	e.wgPort, err = e.wg.Init(opts.PrivKey, c.IPv4(), c.IPv6())
	if err != nil {
		return nil, err
	}

	e.stage = actors.MakeStage(opts.Ctx, nodePriv, sessPriv, e.getExtConn, e.ensuredConnFor, c)

	if err := c.InstallCallbacks(e); err != nil {
		log.Fatalf("could not install callbacks: %s", err)
	}

	return e, nil
}

// Login to control
func (e *Engine) login(control dial.Opts, controlKey key.ControlPublic) (ifaces.FullControlInterface, error) {
	slog.Info("engine: control login")

	return CreateControlSession(e.ctx, control, controlKey,
		func() *key.NodePrivate {
			return &e.nodePriv
		},
		func() *key.SessionPrivate {
			return &e.sessPriv
		},
	)
}

func (e *Engine) ensuredConnFor(key key.NodePublic) types.UDPConn {
	m, ok := e.localMapping[key]

	if !ok {
		// no mapping info, cant do anything
		return nil
	}

	if !m.conn.Closed {
		return &m.conn
	}

	conn, err := e.getWGConn(&m.port)

	if err == nil {
		m.conn = types.UDPConnCloseCatcher{UDPConn: conn}

		return &m.conn
	}

	// err != nil

	slog.Warn("engine: received error when trying to rebind conn", "error", err)

	conn, err = e.getWGConn(nil)

	if err != nil {
		panic(fmt.Sprintf("could not rebind: %s", err))
	}

	nm := mappingFromUDPConn(conn)

	e.localMapping[key] = nm

	err = e.wg.UpdatePeer(key, PeerCfg{LocalEndpointPort: &nm.port})

	if err != nil {
		panic(fmt.Sprintf("could not update peer with new local conn after rebind: %s", err))
	}

	return &nm.conn
}

func (e *Engine) getWGConn(fromPort *uint16) (*net.UDPConn, error) {
	var laddr *net.UDPAddr = nil

	if fromPort != nil {
		laddr = net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), *fromPort),
		)
	}

	return net.DialUDP("udp", laddr,
		net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), e.wgPort),
		),
	)
}

func (e *Engine) bindLocal() *mapping {
	conn, err := e.getWGConn(nil)

	if err != nil {
		panic(fmt.Sprintf("error when first binding to wgport: %s", err))
	}

	return mappingFromUDPConn(conn)
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

const WGKeepAlive = time.Second * 20

func (e *Engine) Handle(ev Event) error {
	switch ev := ev.(type) {
	case PeerAddition:
		return e.AddPeer(ev.Key, ev.HomeRelayId, ev.Endpoints, ev.SessionKey, ev.VIPs.IPv4, ev.VIPs.IPv6)
	case PeerUpdate:
		// FIXME the reason for the panic below is because this function is essentially deprecated, and it still uses
		//  gonull, which is a pain
		panic("cannot handle PeerUpdate via handle")

		//if ev.Endpoints.Present {
		//	if err := e.stage.SetEndpoints(ev.Key, ev.Endpoints.Val); err != nil {
		//		return fmt.Errorf("failed to update endpoints: %w", err)
		//	}
		//}
		//
		//if ev.SessionKey.Present {
		//	if err := e.stage.UpdateSessionKey(ev.Key, ev.SessionKey.Val); err != nil {
		//		return fmt.Errorf("failed to update session key: %w", err)
		//	}
		//}
		//
		//if ev.HomeRelayId.Present {
		//	if err := e.stage.UpdateHomeRelay(ev.Key, ev.HomeRelayId.Val); err != nil {
		//		return fmt.Errorf("failed to update home relay: %w", err)
		//	}
		//}
	case PeerRemoval:
		return e.RemovePeer(ev.Key)
	case RelayUpdate:
		return e.UpdateRelays(ev.Set)
	default:
		// TODO warn-log about unknown type instead of panic
		panic("Unknown type!")
	}

	return nil
}

func (e *Engine) ClearPeers() {
	e.stage.ClearPeers()
}

func (e *Engine) AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, ip4 netip.Addr, ip6 netip.Addr) error {
	m := e.bindLocal()
	e.localMapping[peer] = m

	if err := e.wg.UpdatePeer(peer, PeerCfg{
		Set: true,
		VIPs: &VirtualIPs{
			IPv4: ip4,
			IPv6: ip6,
		},
		KeepAliveInterval: nil,
		LocalEndpointPort: &m.port,
	}); err != nil {
		return fmt.Errorf("failed to update wireguard: %w", err)
	}

	if err := e.stage.AddPeer(peer, homeRelay, endpoints, session, ip4, ip6); err != nil {
		return fmt.Errorf("failed to update stage: %w", err)
	}
	return nil
}

func (e *Engine) UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic) error {
	return e.stage.UpdatePeer(peer, homeRelay, endpoints, session)
}

func (e *Engine) RemovePeer(peer key.NodePublic) error {
	if err := e.stage.RemovePeer(peer); err != nil {
		return err
	}

	if err := e.wg.RemovePeer(peer); err != nil {
		return fmt.Errorf("failed to remove peer from wireguard: %w", err)
	}

	return nil
}

func (e *Engine) UpdateRelays(relay []relay.Information) error {
	return e.stage.UpdateRelays(relay)
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

func (f *FakeControl) InstallCallbacks(callbacks ifaces.ControlCallbacks) error {
	// NOP
	return nil
}

func (f *FakeControl) UpdateEndpoints(ports []netip.AddrPort) error {
	// NOP
	return nil
}
