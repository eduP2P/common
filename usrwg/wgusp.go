package usrwg

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"slices"
	"syscall"

	"github.com/edup2p/common/toversok"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/usrwg/router"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

func init() {
}

func NewUsrWGHost() *UserSpaceWireGuardHost {
	return &UserSpaceWireGuardHost{}
}

type UserSpaceWireGuardHost struct {
	running *UserSpaceWireGuardController
	tunFile *os.File
}

func (u *UserSpaceWireGuardHost) SetTUNFile(f *os.File) {
	u.tunFile = f
}

func (u *UserSpaceWireGuardHost) SetTUNFD(fd uintptr) {
	u.tunFile = os.NewFile(fd, "tun")
}

func (u *UserSpaceWireGuardHost) Reset() error {
	if u.running != nil {
		u.running.Close()
		u.running = nil
	}

	return nil
}

const WGGOIPCDevSetup = "private_key=%s\n"

func (u *UserSpaceWireGuardHost) Controller(privateKey key.NodePrivate, addr4, addr6 netip.Prefix) (toversok.WireGuardController, error) {
	if u.running != nil {
		if err := u.Reset(); err != nil {
			return nil, fmt.Errorf("usrwg: failed to reset running usrwg controller: %v", err)
		}
	}

	tunDev, err := u.createTUN()
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
		tunDev: tunDev,
		router: r,
	}

	u.running = usrwgc

	return usrwgc, nil
}

// TODO set this to 1392 per https://docs.eduvpn.org/server/v3/wireguard.html
//  and make adjustable by environment variable

const tunMtu = 1280

func (u *UserSpaceWireGuardHost) createTUN() (tun.Device, error) {

	if u.tunFile != nil {
		return createTUNFromFile(u.tunFile, tunMtu)
	} else {
		return createTUN(tunMtu)
	}
}

type UserSpaceWireGuardController struct {
	wgDev  *device.Device
	bind   *ToverSokBind
	tunDev tun.Device
	router router.Router
}

func (u *UserSpaceWireGuardController) Available() bool {
	return true
}

func (u *UserSpaceWireGuardController) InjectPacket(from, to netip.AddrPort, pkt []byte) error {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	ipv4 := &layers.IPv4{
		Version:  0x4,
		TTL:      255,
		Protocol: syscall.IPPROTO_UDP,
		DstIP:    to.Addr().AsSlice(),
		SrcIP:    from.Addr().AsSlice(),
	}
	udp := &layers.UDP{
		DstPort: layers.UDPPort(to.Port()),
		SrcPort: layers.UDPPort(from.Port()),
	}
	if err := udp.SetNetworkLayerForChecksum(ipv4); err != nil {
		return fmt.Errorf("failed to set udp checksum: %w", err)
	}

	err := gopacket.SerializeLayers(buf, opts,
		ipv4,
		udp,
		gopacket.Payload(pkt),
	)
	if err != nil {
		return fmt.Errorf("failed to serialize packet: %w", err)
	}

	packetData := slices.Concat(make([]byte, 16), buf.Bytes())

	if _, err = u.tunDev.Write([][]byte{packetData}, 16); err != nil {
		return fmt.Errorf("failed to inject packet: %w", err)
	}

	return nil
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

func (u *UserSpaceWireGuardController) GetInterface() *net.Interface {
	name, err := u.tunDev.Name()
	if err != nil {
		slog.Warn("failed to get tun device name", "err", err)
		return nil
	}
	i, err := net.InterfaceByName(name)
	if err != nil {
		slog.Warn("failed to get interface", "name", name, "err", err)
		return nil
	}
	return i
}

func (u *UserSpaceWireGuardController) Close() {
	if err := u.bind.Cancel(); err != nil {
		slog.Error("Failed to close wireguard bind", "err", err)
	}
	if err := u.router.Close(); err != nil {
		slog.Error("Failed to close router", "err", err)
	}
	u.wgDev.Close()
}
