package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"

	"github.com/abiosoft/ishell/v2"
	"github.com/edup2p/common/extwg"
	"github.com/edup2p/common/toversok"
	"github.com/edup2p/common/toversok/actors"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
	"github.com/edup2p/common/types/relay"
	"github.com/edup2p/common/usrwg"
	"golang.org/x/exp/maps"
	"golang.zx2c4.com/wireguard/wgctrl"
)

var (
	programLevel = new(slog.LevelVar) // Info by default

	wgCtrl *extwg.WGCtrl
	usrWg  *usrwg.UserSpaceWireGuardHost

	wg toversok.WireGuardHost

	wgC toversok.WireGuardController

	privKey *key.NodePrivate

	fakeControl   StokControl
	properControl toversok.DefaultControlHost
	usedControl   toversok.ControlHost

	engineExtPort uint16

	eccc   context.CancelCauseFunc
	engine *toversok.Engine
)

func init() {
	fakeControl.peers = make(map[key.NodePublic]PeerDef)
	fakeControl.relays = make(map[int64]relay.Information)
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel, AddSource: true})
	slog.SetDefault(slog.New(h))
	programLevel.Set(slog.LevelDebug)

	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

	actors.DebugSManTakeNodeAsSession = true

	shell := ishell.New()

	shell.SetHomeHistoryPath(".tssh_history")

	shell.Println("Toversok Development Interactive Shell")

	logCmd := &ishell.Cmd{
		Name: "log",
		Help: "get log level",
		Func: func(c *ishell.Context) {
			c.Println(programLevel.Level().String())
		},
	}

	logCmd.AddCmd(&ishell.Cmd{
		Name: "info",
		Help: "set log level to info",
		Func: func(_ *ishell.Context) {
			programLevel.Set(slog.LevelInfo)
		},
	})

	logCmd.AddCmd(&ishell.Cmd{
		Name: "debug",
		Help: "set log level to debug",
		Func: func(_ *ishell.Context) {
			programLevel.Set(slog.LevelDebug)
		},
	})

	logCmd.AddCmd(&ishell.Cmd{
		Name: "trace",
		Help: "set log level to trace",
		Func: func(_ *ishell.Context) {
			programLevel.Set(-8)
		},
	})

	shell.AddCmd(logCmd)

	shell.AddCmd(keyCmd())
	shell.AddCmd(wgCmd())
	shell.AddCmd(enCmd())
	shell.AddCmd(pcCmd())
	shell.AddCmd(fcCmd())

	shell.Run()

	if engine != nil {
		engine.Stop()
	}
}

// Key commands
func keyCmd() *ishell.Cmd {
	c := &ishell.Cmd{
		Name: "key",
		Help: "private key setting, generating, and reading",
		Func: func(c *ishell.Context) {
			if privKey == nil {
				c.Println("key: nil")
			} else {
				c.Println("key:", privKey.Marshal())
			}
		},
	}

	c.AddCmd(&ishell.Cmd{
		Name: "gen",
		Help: "generate a new key",
		Func: func(c *ishell.Context) {
			k := key.NewNode()
			privKey = &k

			c.Println("key generated:", privKey.Marshal())
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "file",
		Help: "source or generate a key from a filename",
		Func: func(c *ishell.Context) {
			var file string

			if len(c.Args) == 0 {
				c.Println("no file specified, using ./client.key")
				file = "client.key"
			} else {
				file = c.Args[0]
			}

			k, err := getOrGenerateKey(file, c)
			if err != nil {
				c.Err(err)
				return
			}

			privKey = &k
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "set",
		Help: "set a key",
		Func: func(c *ishell.Context) {
			var line string
			if len(c.Args) == 0 {
				c.Println("enter the key, with 'privkey:' prefix")
				line = c.ReadLine()
			} else {
				line = c.Args[0]
			}

			p, err := key.UnmarshalPrivate(line)
			if err != nil {
				c.Err(err)
				return
			}

			privKey = p
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "pub",
		Help: "show the pubkey",
		Func: func(c *ishell.Context) {
			if privKey != nil {
				c.Println("pub:", privKey.Public().Marshal())
			} else {
				c.Err(errors.New("private key not set"))
			}
		},
	})

	return c
}

func getOrGenerateKey(file string, c *ishell.Context) (key.NodePrivate, error) {
	var err error
	var k key.NodePrivate

	file = strings.TrimSpace(file)

	// I hate that golang is like this
	if strings.HasPrefix(file, "~/") {
		dirname, err := os.UserHomeDir()
		if err != nil {
			// at this point, just give up
			panic(err)
		}

		file = filepath.Join(dirname, file[2:])
	}

	if file, err = filepath.Abs(file); err != nil {
		return k, fmt.Errorf("failed to normalise path: %w", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			c.Println(fmt.Sprintf("%s does not exist, generating new key...", file))
			k = key.NewNode()

			c.Println("key generated, writing to file...")
			jsonData, err := json.Marshal(k)
			if err != nil {
				return k, fmt.Errorf("failed to marshal private key: %w", err)
			}

			if err := os.WriteFile(file, jsonData, 0o644); err != nil {
				return k, fmt.Errorf("failed to write private key to file: %w", err)
			}

			return k, nil
		}

		return k, fmt.Errorf("cannot read key file %s: %w", file, err)
	}

	if err = json.Unmarshal(data, &k); err != nil {
		return k, fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	c.Println("loaded from", file)

	return k, nil
}

// Proper Control commands
func pcCmd() *ishell.Cmd {
	c := &ishell.Cmd{
		Name: "pc",
		Help: "proper control variables",
		Func: func(c *ishell.Context) {
			c.Println("proper control dial opts:", properControl.Opts)

			c.Println("proper control key:", properControl.Key.Debug())
		},
	}

	c.AddCmd(&ishell.Cmd{
		Name: "use",
		Help: "start using the proper control",
		Func: func(_ *ishell.Context) {
			usedControl = &properControl
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "key",
		Help: "set a key",
		Func: func(c *ishell.Context) {
			var line string
			if len(c.Args) == 0 {
				c.Println("enter the key, with 'control:' prefix")
				line = c.ReadLine()
			} else {
				line = c.Args[0]
			}

			p, err := key.UnmarshalControlPublic(line)
			if err != nil {
				c.Err(err)
				return
			}

			properControl.Key = *p
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "domain",
		Help: "set domain of control opts",
		Func: func(c *ishell.Context) {
			var line string
			if len(c.Args) == 0 {
				c.Println("enter domain")
				line = c.ReadLine()
			} else {
				line = c.Args[0]
			}

			properControl.Opts.Domain = line

			c.Println("set domain")
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "ip",
		Help: "set ip of control opts",
		Func: func(c *ishell.Context) {
			var line string
			if len(c.Args) == 0 {
				c.Println("enter ip")
				line = c.ReadLine()
			} else {
				line = c.Args[0]
			}

			var ip netip.Addr
			var err error

			if ip, err = netip.ParseAddr(line); err != nil {
				c.Err(err)
				return
			}

			properControl.Opts.Addrs = []netip.Addr{ip}

			c.Println("set ip")
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "port",
		Help: "set port of control opts",
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("set port in args"))
			}

			if i, err := strconv.ParseInt(c.Args[0], 10, 16); err != nil {
				c.Err(err)
			} else {
				properControl.Opts.Port = uint16(i)
				c.Println("set port")
			}
		},
	})

	return c
}

// Fake Control commands
func fcCmd() *ishell.Cmd {
	c := &ishell.Cmd{
		Name: "fc",
		Help: "fake controlhost variables and handling",
	}

	c.AddCmd(&ishell.Cmd{
		Name: "use",
		Help: "start using the proper control",
		Func: func(_ *ishell.Context) {
			usedControl = &fakeControl
		},
	})

	peerCmd := &ishell.Cmd{Name: "peer", Help: "peer subcommands"}

	peerCmd.AddCmd(&ishell.Cmd{
		Name:    "add",
		Aliases: []string{"a"},
		Help:    "add a peer: <pubkey:hex> <relay> <ip4> <ip6> <endpoints...>",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 4 {
				c.Err(errors.New("not enough arguments, expected at least 5"))
				return
			}

			var (
				err     error
				peerKey *key.NodePublic
				relay   int64
				session key.SessionPublic
				ip4     netip.Addr
				ip6     netip.Addr

				endpoints = make([]netip.AddrPort, 0)
			)

			if peerKey, err = key.UnmarshalPublic(c.Args[0]); err != nil {
				c.Err(err)
				return
			}
			if relay, err = strconv.ParseInt(c.Args[1], 10, 64); err != nil {
				c.Err(err)
				return
			}

			// We assume the public key of the session is the same as the node for this development environment.
			//
			// We (semi-intentionally) break compatibility with any main network because of this.
			session = [32]byte(*peerKey)

			ip4, err = netip.ParseAddr(c.Args[2])

			if err != nil {
				c.Err(err)
				return
			} else if !ip4.Is4() {
				c.Err(errors.New("ip4 isnt ipv4"))
				return
			}

			if ip6, err = netip.ParseAddr(c.Args[3]); err != nil {
				c.Err(err)
				return
			} else if !ip6.Is6() {
				c.Err(errors.New("ip6 isnt ipv6"))
				return
			}

			for _, e := range c.Args[4:] {
				ap, err := netip.ParseAddrPort(e)
				if err != nil {
					c.Err(err)
					return
				}

				endpoints = append(endpoints, ap)
			}

			if err = fakeControl.addPeer(PeerDef{
				Key:         *peerKey,
				HomeRelayID: relay,
				SessionKey:  session,
				Endpoints:   endpoints,
				VIPs: toversok.VirtualIPs{
					IPv4: ip4,
					IPv6: ip6,
				},
			}); err != nil {
				c.Err(err)
			}
		},
	})

	peerCmd.AddCmd(&ishell.Cmd{
		Name:    "delete",
		Aliases: []string{"del", "d"},
		Help:    "remove a peer: <pubkey:hex>",
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("did not define peer key"))
				return
			}

			if peerKey, err := key.UnmarshalPublic(c.Args[0]); err != nil {
				c.Err(err)
			} else {
				if err = fakeControl.delPeer(*peerKey); err != nil {
					c.Err(err)
				}
			}
		},
	})

	c.AddCmd(peerCmd)

	c.AddCmd(&ishell.Cmd{
		Name: "relay",
		Help: "define or update relays: <id> <pubkey:hex> -d [domain] -a [ip,...] -s [stunPort] -t [httpsPort] -h [httpPort] -i (insecure flag)",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Err(errors.New("not enough arguments, expected at least 2"))
				return
			}

			var (
				err      error
				id       int64
				relayKey *key.NodePublic
			)

			if id, err = strconv.ParseInt(c.Args[0], 10, 64); err != nil {
				c.Err(err)
				return
			}

			if relayKey, err = key.UnmarshalPublic(c.Args[1]); err != nil {
				c.Err(err)
				return
			}

			fs := flag.NewFlagSet("relay", flag.ContinueOnError)

			domain := fs.String("d", "", "domain")
			addrs := fs.String("a", "", "addrs (comma-seperated)")
			stunPort := fs.Int("s", math.MaxInt, "stunPort")
			httpsPort := fs.Int("t", math.MaxInt, "httpsPort")
			httpPort := fs.Int("h", math.MaxInt, "httpPort")
			insecure := fs.Bool("i", false, "insecure")

			if err := fs.Parse(c.Args[2:]); err != nil {
				c.Err(fmt.Errorf("could not parse flags: %w", err))
				return
			}

			ri := relay.Information{
				ID:     id,
				Key:    *relayKey,
				Domain: *domain,
				// IPs:             gonull.Nullable[[]netip.Addr]{},
				// STUNPort:        gonull.Nullable[uint16]{},
				// HTTPSPort:       gonull.Nullable[uint16]{},
				// HTTPPort:        gonull.Nullable[uint16]{},
				IsInsecure: *insecure,
			}

			if *addrs != "" {
				as := *addrs

				addrs := make([]netip.Addr, 0)

				for _, addr := range strings.Split(as, ",") {
					a, err := netip.ParseAddr(addr)
					if err != nil {
						c.Err(err)
						return
					}

					addrs = append(addrs, a)
				}

				ri.IPs = addrs
			}

			if *stunPort != math.MaxInt {
				stunPort := uint16(*stunPort)
				ri.STUNPort = &stunPort
			}

			if *httpPort != math.MaxInt {
				httpPort := uint16(*httpPort)
				ri.HTTPPort = &httpPort
			}

			if *httpsPort != math.MaxInt {
				httpsPort := uint16(*httpsPort)
				ri.HTTPSPort = &httpsPort
			}

			if err = fakeControl.updateRelay(ri); err != nil {
				c.Err(err)
			}
		},
	})

	c.AddCmd(&ishell.Cmd{Name: "ip4", Help: "set and get the ip4 prefix", Func: func(c *ishell.Context) {
		if len(c.Args) == 0 {
			c.Println("ip4:", fakeControl.ip4)
		} else {
			p, err := netip.ParsePrefix(c.Args[0])
			if err != nil {
				c.Err(err)
				return
			}
			if !p.Addr().Is4() {
				c.Err(errors.New("address is not ip4"))
				return
			}
			fakeControl.ip4 = &p
			c.Println("set ip4:", fakeControl.ip4)
		}
	}})
	c.AddCmd(&ishell.Cmd{Name: "ip6", Help: "set and get the ip6 prefix", Func: func(c *ishell.Context) {
		if len(c.Args) == 0 {
			c.Println("ip6:", fakeControl.ip6)
		} else {
			p, err := netip.ParsePrefix(c.Args[0])
			if err != nil {
				c.Err(err)
				return
			}
			if !p.Addr().Is6() {
				c.Err(errors.New("address is not ip6"))
				return
			}
			fakeControl.ip6 = &p
			c.Println("set ip6:", fakeControl.ip6)
		}
	}})

	return c
}

func wgCmd() *ishell.Cmd {
	c := &ishell.Cmd{
		Name: "wg",
		Help: "wireguard configurator state and subcommands",
		Func: func(c *ishell.Context) {
			if wg == nil {
				c.Println("wg: nil")
			} else {
				c.Println("wg: using", wg)
			}
		},
	}

	c.AddCmd(&ishell.Cmd{
		Name: "use",
		Help: "Setup wg configurator and use a specific interface",
		Func: func(c *ishell.Context) {
			var device string

			client, err := wgctrl.New()
			if err != nil {
				c.Err(err)
				return
			}

			if len(c.Args) != 0 {
				device = c.Args[0]

				if _, err := client.Device(device); err != nil {
					c.Err(err)
					return
				}
			} else {
				devices, err := client.Devices()
				if err != nil {
					c.Err(err)
					return
				}

				var names []string
				for _, device := range devices {
					names = append(names, device.Name)
				}

				if len(names) == 0 {
					c.Err(errors.New("no devices detected"))

					return
				}

				choice := c.MultiChoice(names, "select device")

				if choice == -1 {
					c.Err(errors.New("no device selected"))

					return
				}

				device = names[choice]
			}

			wgCtrl = extwg.NewWGCtrl(client, device)

			wg = wgCtrl

			c.Println("now using wg device", device)
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "usr",
		Help: "Use User Wireguard",
		Func: func(c *ishell.Context) {
			usrWg = usrwg.NewUsrWGHost()

			wg = usrWg

			c.Println("now using userspace wireguard")
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "init",
		Help: "Perform Init() on the wg configurator. wg init <privkey addr4/cidr addr6/cidr>",
		Func: func(c *ishell.Context) {
			switch {
			case len(c.Args) < 2:
				c.Err(errors.New("usage: privkey addr4 addr6"))
				return
			case wg == nil:
				c.Err(errors.New("wg not setup"))
			default:
				privkeyStr := c.Args[0]
				addr4Str := c.Args[1]
				addr6Str := c.Args[2]
				privkeySlice, err := hex.DecodeString(privkeyStr)
				if err != nil {
					c.Err(err)
					return
				} else if len(privkeySlice) != key.Len {
					c.Err(fmt.Errorf("unexpected key length, expected 32, got %d", len(privkeySlice)))
					return
				}
				privkey := key.NodePrivateFrom((key.NakedKey)(privkeySlice))
				addr4, err := netip.ParsePrefix(addr4Str)
				if err != nil {
					c.Err(err)
					return
				} else if !addr4.Addr().Is4() {
					c.Err(errors.New("first argument is not ipv4 address/cidr"))
					return
				}
				addr6, err := netip.ParsePrefix(addr6Str)
				if err != nil {
					c.Err(err)
					return
				} else if !addr6.Addr().Is6() {
					c.Err(errors.New("second argument is not ipv6 address/cidr"))
					return
				}
				wgC, err = wg.Controller(privkey, addr4, addr6)
				if err != nil {
					c.Err(err)
					return
				}
				c.Println("wg controller:", wgC)
			}
		},
	})

	return c
}

func enCmd() *ishell.Cmd {
	c := &ishell.Cmd{
		Name: "en",
		Help: "toversok engine and subcommands",
		Func: func(c *ishell.Context) {
			if engine == nil {
				c.Println("engine: nil")
			} else {
				started := "not started"

				if engine.WillRestart() {
					started = "started"
				}

				c.Println("engine: created, " + started)
			}
		},
	}

	c.AddCmd(&ishell.Cmd{
		Name: "create",
		Help: "create a new engine, stopping the previous one if it already existed",
		Func: func(c *ishell.Context) {
			var err error

			switch {
			case usedControl == nil:
				err = errors.New("no control host set")
			case wg == nil:
				err = errors.New("wg is not set")
			case privKey == nil:
				err = errors.New("key is not set")
			}

			if err != nil {
				c.Err(err)
				return
			}

			if engine != nil {
				slog.Info("previous engine exists, stopping...")
				eccc(errors.New("stopping previous engine"))

				engine = nil
				eccc = nil
			}

			ctx, ccc := context.WithCancelCause(context.Background())

			fw := &StokFirewall{}

			e, err := toversok.NewEngine(ctx, wg, fw, usedControl, engineExtPort, *privKey)
			if err != nil {
				c.Err(err)
				ccc(err)
				return
			}

			engine = e
			eccc = ccc
		},
	})

	c.AddCmd(&ishell.Cmd{Name: "start", Help: "start the engine", Func: func(c *ishell.Context) {
		if engine != nil {
			err := engine.Start()
			if err != nil {
				c.Err(err)
			}
		} else {
			c.Err(errors.New("engine does not exist"))
		}
	}})

	c.AddCmd(&ishell.Cmd{
		Name: "port",
		Help: "set the external port",
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Println("port:", engineExtPort)
			} else {
				if i, err := strconv.ParseInt(c.Args[0], 10, 16); err != nil {
					c.Err(err)
				} else {
					engineExtPort = uint16(i)
					c.Println("set port:", engineExtPort)
				}
			}
		},
	})

	return c
}

type PeerDef struct {
	Key key.NodePublic

	HomeRelayID int64
	SessionKey  key.SessionPublic
	Endpoints   []netip.AddrPort

	VIPs toversok.VirtualIPs

	Properties msgcontrol.Properties
}

type StokControl struct {
	mu sync.Mutex

	relays map[int64]relay.Information

	peers map[key.NodePublic]PeerDef

	ip4 *netip.Prefix
	ip6 *netip.Prefix

	callback ifaces.ControlCallbacks
}

func (s *StokControl) ControlKey() key.ControlPublic {
	return key.ControlPublic{}
}

func (s *StokControl) IPv4() netip.Prefix {
	return *s.ip4
}

func (s *StokControl) IPv6() netip.Prefix {
	return *s.ip6
}

func (s *StokControl) UpdateEndpoints(endpoints []netip.AddrPort) error {
	slog.Info("called UpdateEndpoints", "endpoints", endpoints)

	return nil
}

func (s *StokControl) UpdateHomeRelay(i int64) error {
	slog.Info("called UpdateHomeRelay", "relay", i)

	return nil
}

func (s *StokControl) InstallCallbacks(callbacks ifaces.ControlCallbacks) {
	s.callback = callbacks

	slog.Info("InstallCallbacks called, flushing definitions")

	if err := callbacks.UpdateRelays(maps.Values(s.relays)); err != nil {
		slog.Error("UpdateRelays errored", "err", err)
		return
	}

	for _, peer := range s.peers {
		if err := callbacks.AddPeer(
			peer.Key, peer.HomeRelayID, peer.Endpoints, peer.SessionKey, peer.VIPs.IPv4, peer.VIPs.IPv6, peer.Properties,
		); err != nil {
			slog.Error("AddPeer errored", "err", err, "peer", peer.Key.Debug())
			return
		}
	}
}

func (s *StokControl) CreateClient(context.Context, func() *key.NodePrivate, func() *key.SessionPrivate, types.LogonCallback) (ifaces.ControlSession, error) {
	return s, nil
}

func (s *StokControl) addPeer(
	peer PeerDef,
) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.peers[peer.Key]; ok {
		return errors.New("peer already defined")
	}

	s.peers[peer.Key] = peer

	if s.callback != nil {
		err = s.callback.AddPeer(
			peer.Key, peer.HomeRelayID, peer.Endpoints, peer.SessionKey, peer.VIPs.IPv4, peer.VIPs.IPv6, peer.Properties,
		)
	}

	return
}

func (s *StokControl) delPeer(peer key.NodePublic) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.peers, peer)

	if s.callback != nil {
		err = s.callback.RemovePeer(peer)
	}

	return
}

func (s *StokControl) updateRelay(ri relay.Information) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.relays[ri.ID] = ri

	if s.callback != nil {
		err = s.callback.UpdateRelays([]relay.Information{ri})
	}

	return
}

// A dummy firewall
type StokFirewall struct{}

func (s *StokFirewall) Reset() error {
	slog.Info("StokFirewall Reset called")

	return nil
}

func (s *StokFirewall) Controller() (toversok.FirewallController, error) {
	slog.Info("StokFirewall Controller called")

	return s, nil
}

func (s *StokFirewall) QuarantineNodes(ips []netip.Addr) error {
	slog.Info("StokFirewall QuarantineNodes called", "ips", ips)

	return nil
}

func (s *StokFirewall) LocalAddresses() ([]netip.Addr, error) {
	slog.Info("StokFirewall LocalAddresses called")

	return []netip.Addr{}, nil
}
