package router

import (
	"fmt"
	"golang.zx2c4.com/wireguard/tun"
	"log/slog"
	"net/netip"
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
	if out, err := cmd("ip", "link", "set", "dev", r.iface, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("failed bringing up device: %w\n%s", err, out)
	}

	return nil
}

func (r *linuxRouter) Set(c *Config) (retErr error) {
	setErr := func(err error) {
		if retErr == nil {
			retErr = err
		}
	}

	for _, prefix := range prefixesToRemove(c.Prefixes, r.currPrefixes) {
		if err := r.removeAddr(prefix); err != nil {
			setErr(err)
			slog.Warn("removeAddr failed", "for", prefix.String(), "err", err)
		}
	}

	for _, prefix := range prefixesToAdd(c.Prefixes, r.currPrefixes) {
		if err := r.addAddr(prefix); err != nil {
			setErr(err)
			slog.Warn("addAddr failed", "for", prefix.String(), "err", err)
		}
	}

	if retErr == nil {
		r.currPrefixes = c.Prefixes
	}

	return
}

func (r *linuxRouter) removeAddr(prefix netip.Prefix) error {
	if out, err := cmd("ip", "addr", "del", prefix.String(), "dev", r.iface).CombinedOutput(); err != nil {
		return fmt.Errorf("deleting address %q from tunnel interface: %w\n%s", prefix, err, out)
	}

	return nil
}

func (r *linuxRouter) addAddr(prefix netip.Prefix) error {
	if out, err := cmd("ip", "addr", "add", prefix.String(), "dev", r.iface).CombinedOutput(); err != nil {
		return fmt.Errorf("adding address %q to tunnel interface: %w\n%s", prefix, err, out)
	}

	return nil
}
