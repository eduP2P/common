package main

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/toversok"
	"github.com/shadowjonathan/edup2p/types/key"
	"go4.org/netipx"
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

// WGCtrl is a toversok.WireGuardConfigurator implementation that takes a preconfigured `client`-tool compatible
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
}

func (w *WGCtrl) Init(privateKey key.NakedKey, addr4, addr6 netip.Prefix) (port uint16, err error) {
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

	err = w.client.ConfigureDevice(w.name, wgtypes.Config{
		PrivateKey:   (*wgtypes.Key)(&privateKey),
		ReplacePeers: true,
		Peers:        []wgtypes.PeerConfig{},
	})
	if err != nil {
		return
	}

	var device *wgtypes.Device
	device, err = w.client.Device(w.name)

	port = (uint16)(device.ListenPort)

	return
}

func (w *WGCtrl) UpdatePeer(publicKey key.NodePublic, cfg toversok.PeerCfg) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	peercfg := wgtypes.PeerConfig{
		PublicKey:                   wgtypes.Key(publicKey),
		Remove:                      false,
		UpdateOnly:                  !cfg.Set,
		PersistentKeepaliveInterval: cfg.KeepAliveInterval,
	}

	if cfg.LocalEndpointPort != nil {
		peercfg.Endpoint = net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), *cfg.LocalEndpointPort),
		)
	}

	if cfg.VIPs != nil {
		peercfg.ReplaceAllowedIPs = true
		peercfg.AllowedIPs = []net.IPNet{
			*netipx.AddrIPNet(cfg.VIPs.IPv4),
			*netipx.AddrIPNet(cfg.VIPs.IPv6),
		}
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
