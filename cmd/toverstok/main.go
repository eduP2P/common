package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/abiosoft/ishell/v2"
	"github.com/shadowjonathan/edup2p/types/key"
	"golang.zx2c4.com/wireguard/wgctrl"
	"net/netip"
	"sync"
)

func main() {
	shell := ishell.New()

	shell.SetHomeHistoryPath(".tssh_history")

	var wg *WGCtrl

	shell.Println("ToverStok Interactive Shell")

	wgCmd := &ishell.Cmd{
		Name: "wg",
		Help: "wireguard configurator peer_state and subcommands",
		Func: func(c *ishell.Context) {
			if wg == nil {
				c.Println("wg: nil")
			} else {
				c.Println("wg: using", wg.name)
			}
		},
	}

	wgCmd.AddCmd(&ishell.Cmd{
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
		},
	})

	wgCmd.AddCmd(&ishell.Cmd{
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

	tsCmd := &ishell.Cmd{
		Name: "ts",
		Help: "toverstok peer_state and subcommands",
		Func: func(c *ishell.Context) {
			// TODO implement
			panic("to implement")
		},
	}

	shell.AddCmd(wgCmd)
	shell.AddCmd(tsCmd)

	shell.Run()
}
