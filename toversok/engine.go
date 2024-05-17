package toversok

import (
	"context"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/toversok/actors"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

type EngineOptions struct {
	Ctx context.Context
	Ccc context.CancelCauseFunc

	PrivKey key.NakedKey

	IP4 netip.Prefix
	IP6 netip.Prefix

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

type Engine struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	stage ifaces.Stage

	// Mapping of peers to local ports
	localMapping map[key.NodePublic]*mapping
	extBind      *types.UDPConnCloseCatcher

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

	e := &Engine{
		ctx: opts.Ctx,
		ccc: opts.Ccc,

		localMapping: make(map[key.NodePublic]*mapping),

		extPort: opts.ExtBindPort,

		wg: opts.WG,
		fw: opts.FW,
	}

	var err error
	e.wgPort, err = e.wg.Init(opts.PrivKey, opts.IP4, opts.IP6)
	if err != nil {
		return nil, err
	}

	e.stage = actors.MakeStage(opts.Ctx, key.NodePrivateFrom(opts.PrivKey), e.getExtConn, e.ensuredConnFor)

	return e, nil
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
		m := e.bindLocal()
		e.localMapping[ev.Key] = m

		//keepAlive := WGKeepAlive

		if err := e.wg.UpdatePeer(ev.Key, PeerCfg{
			Set:               true,
			VIPs:              &ev.VIPs,
			KeepAliveInterval: nil,
			LocalEndpointPort: &m.port,
		}); err != nil {
			return fmt.Errorf("failed to update wireguard: %w", err)
		}

		if err := e.stage.AddPeer(ev.Key, ev.HomeRelayId, ev.Endpoints, ev.SessionKey); err != nil {
			return fmt.Errorf("failed to update stage: %w", err)
		}
	case PeerUpdate:
		if ev.Endpoints.Present {
			if err := e.stage.SetEndpoints(ev.Key, ev.Endpoints.Val); err != nil {
				return fmt.Errorf("failed to update endpoints: %w", err)
			}
		}

		if ev.SessionKey.Present {
			if err := e.stage.UpdateSessionKey(ev.Key, ev.SessionKey.Val); err != nil {
				return fmt.Errorf("failed to update session key: %w", err)
			}
		}

		if ev.HomeRelayId.Present {
			if err := e.stage.UpdateHomeRelay(ev.Key, ev.HomeRelayId.Val); err != nil {
				return fmt.Errorf("failed to update home relay: %w", err)
			}
		}
	case PeerRemoval:
		e.stage.RemovePeer(ev.Key)

		if err := e.wg.RemovePeer(ev.Key); err != nil {
			return fmt.Errorf("failed to remove peer from wireguard: %w", err)
		}
	case RelayUpdate:
		e.stage.UpdateRelays(ev.Set)
	default:
		// TODO warn-log about unknown type instead of panic
		panic("Unknown type!")
	}

	return nil
}
