package main

import (
	"fmt"
	"github.com/edup2p/common/toversok"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"go4.org/netipx"
	"golang.org/x/exp/maps"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"log/slog"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"sync"
)

// A wireguard configurator by the help of wgtools shell commands.
//
// Should only be used for development, not in actual application.

// WGCtrl is a toversok.WireGuardController implementation that takes a preconfigured `client`-tool compatible
// interface and interacts with it.
//
// On macos, run:
// - sudo wireguard-go utun
// - sudo chown $USER /var/run/wireguard/utun*
// - wg show
//
// To shut down the socket, run:
// - sudo rm /var/run/wireguard/utun*
type WGCtrl struct {
	// Control client
	client *wgctrl.Client
	// Device name
	name string

	mu sync.Mutex

	wgPort uint16

	localMapping map[key.NodePublic]*mapping
}

func (w *WGCtrl) Reset() error {
	for _, m := range w.localMapping {
		m.conn.Close()
	}

	maps.Clear(w.localMapping)

	zeroKey := wgtypes.Key{}

	if err := w.client.ConfigureDevice(w.name, wgtypes.Config{
		PrivateKey:   &zeroKey,
		ReplacePeers: true,
		Peers:        []wgtypes.PeerConfig{},
	}); err != nil {
		return fmt.Errorf("error resetting wg device: %w", err)
	}

	return nil
}

type mapping struct {
	conn types.UDPConnCloseCatcher

	port uint16
}

func (w *WGCtrl) Controller(privateKey key.NodePrivate, addr4, addr6 netip.Prefix) (toversok.WireGuardController, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	const sep = "; "

	switch runtime.GOOS {
	case "darwin":
		const (
			ifconfig4 = "sudo ifconfig %s inet %s/32 %s"
			ifconfig6 = "sudo ifconfig %s inet6 %s %s prefixlen 128"

			route4 = "sudo route add -inet %s -iface %s"
			route6 = "sudo route add -inet6 %s -iface %s"
		)

		slog.Warn("Please run these lines in a separate terminal:")
		slog.Warn(
			strings.Join([]string{
				fmt.Sprintf(ifconfig4, w.name, addr4.Addr().String(), addr4.Addr().String()),
				fmt.Sprintf(ifconfig6, w.name, addr6.Addr().String(), addr6.Addr().String()),
				fmt.Sprintf(route4, addr4.String(), w.name),
				fmt.Sprintf(route6, addr6.String(), w.name),
			}, sep),
		)
	case "linux":
		const (
			ip = "sudo ip address add %s dev %s"
		)

		slog.Warn("Please run these lines in a separate terminal:")
		slog.Warn(
			strings.Join([]string{
				fmt.Sprintf(ip, addr4.String(), w.name),
				fmt.Sprintf(ip, addr6.String(), w.name),
			}, sep),
		)
	}

	unveiledKey := key.UnveilPrivate(privateKey)

	err := w.client.ConfigureDevice(w.name, wgtypes.Config{
		PrivateKey:   (*wgtypes.Key)(&unveiledKey),
		ReplacePeers: true,
		Peers:        []wgtypes.PeerConfig{},
	})
	if err != nil {
		return nil, err
	}

	var device *wgtypes.Device
	device, err = w.client.Device(w.name)

	if err != nil {
		return nil, err
	}

	w.wgPort = (uint16)(device.ListenPort)

	return w, nil
}

func (w *WGCtrl) ConnFor(node key.NodePublic) types.UDPConn {
	return &w.ensureLocalConn(node).conn
}

func (w *WGCtrl) UpdatePeer(publicKey key.NodePublic, cfg toversok.PeerCfg) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	m := w.ensureLocalConn(publicKey)

	peercfg := wgtypes.PeerConfig{
		PublicKey:                   wgtypes.Key(publicKey),
		Remove:                      false,
		PersistentKeepaliveInterval: cfg.KeepAliveInterval,
		Endpoint: net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), m.port),
		),
	}

	peercfg.ReplaceAllowedIPs = true
	peercfg.AllowedIPs = []net.IPNet{
		*netipx.AddrIPNet(cfg.VIPs.IPv4),
		*netipx.AddrIPNet(cfg.VIPs.IPv6),
	}

	return w.client.ConfigureDevice(w.name, wgtypes.Config{
		ReplacePeers: false,
		Peers: []wgtypes.PeerConfig{
			peercfg,
		},
	})
}

func (w *WGCtrl) RemovePeer(publicKey key.NodePublic) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.client.ConfigureDevice(w.name, wgtypes.Config{
		ReplacePeers: false,
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: wgtypes.Key(publicKey),
				Remove:    true,
			},
		},
	})
}

func (w *WGCtrl) GetStats(publicKey key.NodePublic) (*toversok.WGStats, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	device, err := w.client.Device(w.name)
	if err != nil {
		return nil, err
	}

	var foundPeer *wgtypes.Peer

	for _, peer := range device.Peers {
		if peer.PublicKey == (wgtypes.Key)(publicKey) {
			foundPeer = &peer
			break
		}
	}

	if foundPeer == nil {
		return nil, nil
	}

	return &toversok.WGStats{
		LastHandshake: foundPeer.LastHandshakeTime,
		TxBytes:       foundPeer.TransmitBytes,
		RxBytes:       foundPeer.ReceiveBytes,
	}, nil
}

func (w *WGCtrl) ensureLocalConn(peer key.NodePublic) *mapping {
	m, ok := w.localMapping[peer]

	if !ok {
		m = w.bindLocal()
		w.localMapping[peer] = m
	}

	if m.conn.Closed {
		if err := w.rebindMapping(m); err != nil {
			slog.Warn("wgctrl: received error when trying to rebind conn", "error", err)
		}
	}

	return m
}

func (w *WGCtrl) rebindMapping(m *mapping) error {
	conn, err := w.getWGConn(&m.port)

	if err == nil {
		m.conn = types.UDPConnCloseCatcher{UDPConn: conn}
	}

	return err
}

func (w *WGCtrl) bindLocal() *mapping {
	conn, err := w.getWGConn(nil)

	if err != nil {
		panic(fmt.Sprintf("error when first binding to wgport: %s", err))
	}

	return mappingFromUDPConn(conn)
}

func (w *WGCtrl) getWGConn(fromPort *uint16) (*net.UDPConn, error) {
	var laddr *net.UDPAddr = nil

	if fromPort != nil {
		laddr = net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), *fromPort),
		)
	}

	return net.DialUDP("udp", laddr,
		net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), w.wgPort),
		),
	)
}

func mappingFromUDPConn(udp *net.UDPConn) *mapping {
	ap := netip.MustParseAddrPort(udp.LocalAddr().String())

	return &mapping{
		conn: types.UDPConnCloseCatcher{UDPConn: udp},
		port: ap.Port(),
	}
}
