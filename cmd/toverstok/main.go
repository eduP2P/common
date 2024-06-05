package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"github.com/LukaGiorgadze/gonull"
	"github.com/abiosoft/ishell/v2"
	"github.com/shadowjonathan/edup2p/toversok"
	"github.com/shadowjonathan/edup2p/toversok/actors"
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"golang.zx2c4.com/wireguard/wgctrl"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	wg           *WGCtrl
	privKey      *key.NodePrivate
	programLevel = new(slog.LevelVar) // Info by default
	ip4          *netip.Prefix
	ip6          *netip.Prefix
	extPort      uint16
	controlOpts  *dial.Opts
	controlKey   *key.ControlPublic
	engine       *toversok.Engine
	eccc         context.CancelCauseFunc
)

func main() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel, AddSource: true})
	slog.SetDefault(slog.New(h))
	programLevel.Set(slog.LevelDebug)

	actors.DebugSManTakeNodeAsSession = true

	shell := ishell.New()

	shell.SetHomeHistoryPath(".tssh_history")

	shell.Println("ToverStok Interactive Shell")

	traceCmd := &ishell.Cmd{
		Name: "trace",
		Help: "set log level to e",
		Func: func(c *ishell.Context) {
			programLevel.Set(-8)
		},
	}

	debugCmd := &ishell.Cmd{
		Name: "debug",
		Help: "set log level to debug",
		Func: func(c *ishell.Context) {
			programLevel.Set(slog.LevelDebug)
		},
	}

	infoCmd := &ishell.Cmd{
		Name: "info",
		Help: "set log level to debug",
		Func: func(c *ishell.Context) {
			programLevel.Set(slog.LevelInfo)
		},
	}

	shell.AddCmd(traceCmd)
	shell.AddCmd(debugCmd)
	shell.AddCmd(infoCmd)

	shell.AddCmd(wgCmd())
	shell.AddCmd(tsCmd())
	shell.AddCmd(keyCmd())
	shell.AddCmd(ctrlCmd())

	shell.Run()
}

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

			if p, err := key.UnmarshalPrivate(line); err != nil {
				c.Err(err)
				return
			} else {
				privKey = p
			}
		},
	})

	c.AddCmd(&ishell.Cmd{Name: "pub", Help: "show the pubkey", Func: func(c *ishell.Context) {
		if privKey != nil {
			c.Println("pub:", privKey.Public().Marshal())
		} else {
			c.Err(errors.New("private key not set"))
		}
	}})

	return c
}

func ctrlCmd() *ishell.Cmd {
	c := &ishell.Cmd{
		Name:    "control",
		Help:    "manipulate control variables",
		Aliases: []string{"ctrl"},
		Func: func(c *ishell.Context) {
			if controlOpts == nil {
				c.Println("control dial opts: nil")
			} else {
				c.Println("control dial opts:", *controlOpts)
			}

			if controlKey == nil {
				c.Println("control key: nil")
			} else {
				c.Println("control key:", controlKey.Debug())
			}
		},
	}

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

			if p, err := key.UnmarshalControlPublic(line); err != nil {
				c.Err(err)
				return
			} else {
				controlKey = p
			}
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

			if controlOpts == nil {
				controlOpts = new(dial.Opts)
			}

			controlOpts.Domain = line

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

			if controlOpts == nil {
				controlOpts = new(dial.Opts)
			}

			var ip netip.Addr
			var err error

			if ip, err = netip.ParseAddr(line); err != nil {
				c.Err(err)
				return
			}

			controlOpts.Addrs = []netip.Addr{ip}

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

			if controlOpts == nil {
				controlOpts = new(dial.Opts)
			}

			if i, err := strconv.ParseInt(c.Args[0], 10, 16); err != nil {
				c.Err(err)
			} else {
				controlOpts.Port = uint16(i)
				c.Println("set port:", controlOpts.Port)
			}
		},
	})

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
				c.Println("wg: using", wg.name)
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

			wg = &WGCtrl{
				client: client,
				name:   device,
				mu:     sync.Mutex{},
			}

			c.Println("now using wg device", device)
		},
	})

	c.AddCmd(&ishell.Cmd{
		Name: "init",
		Help: "Perform Init() on the wg configurator. wg init <privkey addr4/cidr addr6/cidr>",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Err(errors.New("usage: privkey addr4 addr6"))
				return
			} else if wg == nil {
				c.Err(errors.New("wg not setup"))
			} else {
				privkeyStr := c.Args[0]
				addr4Str := c.Args[1]
				addr6Str := c.Args[2]

				privkeySlice, err := hex.DecodeString(privkeyStr)
				if err != nil {
					c.Err(err)
					return
				} else if len(privkeySlice) != key.Len {
					c.Err(errors.New(fmt.Sprintf("unexpected key length, expected 32, got %d", len(privkeySlice))))
					return
				}

				privkey := *(*key.NakedKey)(privkeySlice)

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

				port, err := wg.Init(privkey, addr4, addr6)
				if err != nil {
					c.Err(err)
					return
				}

				c.Println("wg listen port:", port)
			}
		},
	})

	return c
}

func tsCmd() *ishell.Cmd {

	c := &ishell.Cmd{
		Name: "ts",
		Help: "toverstok engine and subcommands",
		Func: func(c *ishell.Context) {
			// TODO show status
		},
	}

	c.AddCmd(&ishell.Cmd{
		Name: "create",
		Help: "create a new engine, stopping the previous one if it already existed",
		Func: func(c *ishell.Context) {
			var err error

			if controlOpts == nil && controlKey == nil && ip4 == nil && ip6 == nil {
				err = errors.New("ip or control options not set")
			} else if (controlOpts != nil || controlKey != nil) && (controlOpts == nil || controlKey == nil) {
				err = errors.New("control partially set")
			} else if (ip4 != nil || ip6 != nil) && (ip4 == nil || ip6 == nil) {
				err = errors.New("ip partially set")
			} else if wg == nil {
				err = errors.New("wg is not set")
			} else if privKey == nil {
				err = errors.New("key is not set")
			}
			if err != nil {
				c.Err(err)
				return
			}

			if engine != nil {
				slog.Info("previous engine exists, stopping...")
				eccc(errors.New("stop"))

				engine = nil
				eccc = nil
			}

			ctx, ccc := context.WithCancelCause(context.Background())
			opts := toversok.EngineOptions{
				Ctx:         ctx,
				Ccc:         ccc,
				PrivKey:     key.UnveilPrivate(*privKey),
				ExtBindPort: extPort,
				WG:          wg,
				FW:          nil,
			}

			if controlKey != nil {
				opts.Control = *controlOpts
				opts.ControlKey = *controlKey
			} else {
				opts.OverrideControl = true
				opts.OverrideIPv4 = *ip4
				opts.OverrideIPv6 = *ip6
			}

			e, err := toversok.NewEngine(opts)
			if err != nil {
				c.Err(err)
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

	c.AddCmd(&ishell.Cmd{Name: "ip4", Help: "set and get the ip4 prefix", Func: func(c *ishell.Context) {
		if len(c.Args) == 0 {
			c.Println("ip4:", ip4)
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
			ip4 = &p
			c.Println("set ip4:", ip4)
		}
	}})
	c.AddCmd(&ishell.Cmd{Name: "ip6", Help: "set and get the ip6 prefix", Func: func(c *ishell.Context) {
		if len(c.Args) == 0 {
			c.Println("ip6:", ip6)
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
			ip6 = &p
			c.Println("set ip6:", ip6)
		}
	}})
	c.AddCmd(&ishell.Cmd{
		Name: "port",
		Help: "set the external port",
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Println("port:", extPort)
			} else {
				if i, err := strconv.ParseInt(c.Args[0], 10, 16); err != nil {
					c.Err(err)
				} else {
					extPort = uint16(i)
					c.Println("set port:", extPort)
				}
			}
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

			if ip4, err = netip.ParseAddr(c.Args[2]); err != nil {
				c.Err(err)
				return
			} else {
				if !ip4.Is4() {
					c.Err(errors.New("ip4 isnt ipv4"))
					return
				}
			}

			if ip6, err = netip.ParseAddr(c.Args[3]); err != nil {
				c.Err(err)
				return
			} else {
				if !ip6.Is6() {
					c.Err(errors.New("ip6 isnt ipv6"))
					return
				}
			}

			for _, e := range c.Args[4:] {
				ap, err := netip.ParseAddrPort(e)
				if err != nil {
					c.Err(err)
					return
				}

				endpoints = append(endpoints, ap)
			}

			if err = engine.Handle(toversok.PeerAddition{
				Key:         *peerKey,
				HomeRelayId: relay,
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
		Name:    "update",
		Aliases: []string{"u"},
		Help:    "update a peer: <pubkey:hex> -r [relay] -e [endpoint,...]",
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("did not define peer key"))
				return
			}

			peerKey, err := key.UnmarshalPublic(c.Args[0])

			if err != nil {
				c.Err(fmt.Errorf("error parsing peer key: %w", err))
				return
			}

			fs := flag.NewFlagSet("peer-update", flag.ContinueOnError)

			r := fs.Int64("r", math.MaxInt64, "relay (int64)")
			endpoints := fs.String("e", "", "endpoints (comma-seperated IPs)")

			if err := fs.Parse(c.Args[1:]); err != nil {
				c.Err(fmt.Errorf("could not parse flags: %w", err))
				return
			}

			pu := toversok.PeerUpdate{
				Key: *peerKey,
			}

			if *r != math.MaxInt64 {
				pu.HomeRelayId = gonull.NewNullable(*r)
			}

			if *endpoints != "" {
				as := *endpoints

				aps := make([]netip.AddrPort, 0)

				for _, addr := range strings.Split(as, ",") {
					a, err := netip.ParseAddrPort(addr)
					if err != nil {
						c.Err(err)
						return
					}

					aps = append(aps, a)
				}

				pu.Endpoints = gonull.NewNullable(aps)
			}

			if err = engine.Handle(pu); err != nil {
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
				if err = engine.Handle(toversok.PeerRemoval{Key: *peerKey}); err != nil {
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
				//IPs:             gonull.Nullable[[]netip.Addr]{},
				//STUNPort:        gonull.Nullable[uint16]{},
				//HTTPSPort:       gonull.Nullable[uint16]{},
				//HTTPPort:        gonull.Nullable[uint16]{},
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

			if err = engine.Handle(toversok.RelayUpdate{Set: []relay.Information{ri}}); err != nil {
				c.Err(err)
			}
		},
	})

	return c
}
