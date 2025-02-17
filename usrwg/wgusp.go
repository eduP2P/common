package usrwg

import (
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/edup2p/common/toversok"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/usrwg/router"
	"golang.zx2c4.com/wireguard/device"
)

func init() {
}

func NewUsrWGHost() *UserSpaceWireGuardHost {
	return &UserSpaceWireGuardHost{}
}

type UserSpaceWireGuardHost struct {
	running *UserSpaceWireGuardController
}

func (u *UserSpaceWireGuardHost) Reset() error {
	if u.running != nil {
		u.running.Close()
		u.running = nil
	}

	return nil
}

const WGGOIPCDevSetup = `private_key=%s
`

func (u *UserSpaceWireGuardHost) Controller(privateKey key.NodePrivate, addr4, addr6 netip.Prefix) (toversok.WireGuardController, error) {
	if u.running != nil {
		if err := u.Reset(); err != nil {
			return nil, fmt.Errorf("usrwg: failed to reset running usrwg controller: %v", err)
		}
	}

	// TODO set this to 1392 per https://docs.eduvpn.org/server/v3/wireguard.html
	//  and make adjustable by environment variable
	tunDev, err := createTUN(1280)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %w", err)
	}

	r, err := router.NewRouter(tunDev)
	if err != nil {
		return nil, fmt.Errorf("failed to create router: %w", err)
	}

	var interfaceName string

	if interfaceName, err = tunDev.Name(); err == nil {
		slog.Info("using TUN device", "name", interfaceName)
	} else {
		slog.Warn("got error trying to get TUN device name", "err", err)
	}

	bind := createBind()

	wgDev := device.NewDevice(tunDev, bind, &device.Logger{
		Verbosef: func(format string, args ...any) {
			slog.Debug(fmt.Sprintf(format, args...), "from", "wireguard-go")
		},
		Errorf: func(format string, args ...any) {
			slog.Error(fmt.Sprintf(format, args...), "from", "wireguard-go")
		},
	})

	nKey := key.UnveilPrivate(privateKey)

	if err := wgDev.IpcSet(fmt.Sprintf(WGGOIPCDevSetup, nKey.HexString())); err != nil {
		return nil, fmt.Errorf("failed to set private key on wireguard device: %w", err)
	}

	if err := wgDev.Up(); err != nil {
		return nil, fmt.Errorf("failed to bring up wireguard device: %w", err)
	}

	if err = r.Set(&router.Config{
		LocalAddrs:      []netip.Addr{addr4.Addr(), addr6.Addr()},
		RoutingPrefixes: []netip.Prefix{addr4, addr6},
	}); err != nil {
		return nil, fmt.Errorf("failed to set routing config: %w", err)
	}

	if err = r.Up(); err != nil {
		return nil, fmt.Errorf("failed to bring up device through router: %w", err)
	}

	usrwgc := &UserSpaceWireGuardController{
		wgDev:  wgDev,
		bind:   bind,
		router: r,
	}

	u.running = usrwgc

	return usrwgc, nil
}

type UserSpaceWireGuardController struct {
	wgDev  *device.Device
	bind   *ToverSokBind
	router router.Router
}

const WGGOIPCAddPeer = `public_key=%s
replace_allowed_ips=true
allowed_ip=%s/32
allowed_ip=%s/128
endpoint=%s
`

func (u *UserSpaceWireGuardController) UpdatePeer(publicKey key.NodePublic, cfg toversok.PeerCfg) error {
	err := u.wgDev.IpcSet(
		fmt.Sprintf(
			WGGOIPCAddPeer,
			publicKey.HexString(), cfg.VIPs.IPv4.String(), cfg.VIPs.IPv6.String(), publicKey.Marshal(),
		),
	)
	if err != nil {
		err = fmt.Errorf("failed to do IPC set: %w", err)
	}

	return err
}

func (u *UserSpaceWireGuardController) RemovePeer(publicKey key.NodePublic) error {
	u.wgDev.RemovePeer(device.NoisePublicKey(publicKey))

	u.bind.CloseConn(publicKey)

	return nil
}

func (u *UserSpaceWireGuardController) GetStats(_ key.NodePublic) (*toversok.WGStats, error) {
	// TODO implement me

	return nil, nil
}

func (u *UserSpaceWireGuardController) ConnFor(node key.NodePublic) types.UDPConn {
	return u.bind.GetConn(node)
}

func (u *UserSpaceWireGuardController) Close() {
	u.wgDev.Close()
	if err := u.bind.Close(); err != nil {
		slog.Error("Failed to close wireguard bind", "err", err)
	}
	if err := u.router.Close(); err != nil {
		slog.Error("Failed to close router", "err", err)
	}
}
