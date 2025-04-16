package router

import (
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/tun"
)

func NewRouter(device tun.Device) (Router, error) {
	name, err := device.Name()
	if err != nil {
		return nil, err
	}

	return &linuxRouter{
		iface:        name,
		currPrefixes: make([]netip.Prefix, 0),
	}, nil
}

type linuxRouter struct {
	iface        string
	currPrefixes []netip.Prefix
}

func (r *linuxRouter) Up() error {
	//if out, err := cmd("ip", "link", "set", "dev", r.iface, "up").CombinedOutput(); err != nil {
	//	return fmt.Errorf("failed bringing up device: %w\n%s", err, out)
	//}

	link, err := r.link()
	if err != nil {
		return fmt.Errorf("bringing interface up, %w", err)
	}
	return netlink.LinkSetUp(link)
}

func (r *linuxRouter) Close() error {
	// TODO implement me
	return nil
}

func (r *linuxRouter) Set(c *Config) (retErr error) {
	setErr := func(err error) {
		if retErr == nil {
			retErr = err
		}
	}

	for _, prefix := range prefixesToRemove(c.RoutingPrefixes, r.currPrefixes) {
		if err := r.removeAddr(prefix); err != nil {
			setErr(err)
			slog.Warn("removeAddr failed", "for", prefix.String(), "err", err)
		}
	}

	for _, prefix := range prefixesToAdd(c.RoutingPrefixes, r.currPrefixes) {
		if err := r.addAddr(prefix); err != nil {
			setErr(err)
			slog.Warn("addAddr failed", "for", prefix.String(), "err", err)
		}
	}

	if retErr == nil {
		r.currPrefixes = c.RoutingPrefixes
	}

	return
}

func (r *linuxRouter) removeAddr(prefix netip.Prefix) error {
	link, err := r.link()
	if err != nil {
		return fmt.Errorf("deleting address %v, %w", prefix, err)
	}
	if err := netlink.AddrDel(link, nlAddrOfPrefix(prefix)); err != nil {
		return fmt.Errorf("deleting address %v from tunnel interface: %w", prefix, err)
	}

	//if out, err := cmd("ip", "addr", "del", prefix.String(), "dev", r.iface).CombinedOutput(); err != nil {
	//	return fmt.Errorf("deleting address %q from tunnel interface: %w\n%s", prefix, err, out)
	//}

	return nil
}

func (r *linuxRouter) addAddr(prefix netip.Prefix) error {
	link, err := r.link()
	if err != nil {
		return fmt.Errorf("adding address %v, %w", prefix, err)
	}
	if err := netlink.AddrReplace(link, nlAddrOfPrefix(prefix)); err != nil {
		return fmt.Errorf("adding address %v from tunnel interface: %w", prefix, err)
	}

	//if out, err := cmd("ip", "addr", "add", prefix.String(), "dev", r.iface).CombinedOutput(); err != nil {
	//	return fmt.Errorf("adding address %q to tunnel interface: %w\n%s", prefix, err, out)
	//}

	return nil
}

func (r *linuxRouter) link() (netlink.Link, error) {
	link, err := netlink.LinkByName(r.iface)
	if err != nil {
		return nil, fmt.Errorf("failed to look up link %q: %w", r.iface, err)
	}
	return link, nil
}
