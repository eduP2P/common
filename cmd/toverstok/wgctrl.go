package main

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/toversok"
	"github.com/shadowjonathan/edup2p/types/key"
	"go4.org/netipx"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"net"
	"net/netip"
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

func (w *WGCtrl) Init(privateKey key.NakedKey, addr4, addr6 netip.Prefix) (port int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	port = -1

	fmt.Println("Please run these two lines in a seperate terminal:")
	fmt.Println()
	fmt.Printf("sudo ifconfig %s inet %s/32 %s\n", w.name, addr4.Addr().String(), addr4.Addr().String())
	fmt.Printf("sudo ifconfig %s inet6 %s %s prefixlen 128\n", w.name, addr6.Addr().String(), addr6.Addr().String())
	fmt.Printf("sudo route add -inet %s -iface %s\n", addr4.String(), w.name)
	fmt.Printf("sudo route add -inet6 %s -iface %s\n", addr6.String(), w.name)
	fmt.Println()

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

	port = device.ListenPort

	return
}

func (w *WGCtrl) UpdatePeer(publicKey key.NodePublic, cfg toversok.PeerCfg) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.client.ConfigureDevice(w.name, wgtypes.Config{
		ReplacePeers: false,
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey:                   wgtypes.Key(publicKey),
				Remove:                      false,
				UpdateOnly:                  false,
				Endpoint:                    cfg.Endpoint,
				PersistentKeepaliveInterval: cfg.KeepAliveInterval,
				ReplaceAllowedIPs:           true,
				AllowedIPs: []net.IPNet{
					*netipx.AddrIPNet(cfg.IPv4),
					*netipx.AddrIPNet(cfg.IPv6),
				},
				// PresharedKey:                (*wgtypes.Key)(cfg.PreSharedKey),
			},
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
