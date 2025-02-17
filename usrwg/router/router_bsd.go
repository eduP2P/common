//go:build darwin || freebsd

package router

import (
	"fmt"
	"log/slog"
	"net/netip"
	"runtime"

	"go4.org/netipx"
	"golang.zx2c4.com/wireguard/tun"
)

func NewRouter(device tun.Device) (Router, error) {
	name, err := device.Name()
	if err != nil {
		return nil, err
	}

	return &bsdRouter{
		tunName:      name,
		currPrefixes: make([]netip.Prefix, 0),
	}, nil
}

type bsdRouter struct {
	tunName      string
	currPrefixes []netip.Prefix
}

func (r *bsdRouter) Up() error {
	if out, err := cmd("ifconfig", r.tunName, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("running ifconfig failed: %w\n%s", err, out)
	}
	return nil
}

func (r *bsdRouter) Close() error {
	// TODO implement me
	return nil
}

func (r *bsdRouter) Set(c *Config) (retErr error) {
	setErr := func(err error) {
		if retErr == nil {
			retErr = err
		}
	}

	for _, prefix := range prefixesToRemove(c.RoutingPrefixes, r.currPrefixes) {
		if err := r.removeRoute(prefix); err != nil {
			setErr(err)
			slog.Warn("removeRoute failed", "for", prefix.String(), "err", err)
		}

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

		if err := r.addRoute(prefix); err != nil {
			setErr(err)
			slog.Warn("addRoute failed", "for", prefix.String(), "err", err)
		}
	}

	if retErr == nil {
		r.currPrefixes = c.RoutingPrefixes
	}

	return
}

func (r *bsdRouter) addAddr(prefix netip.Prefix) error {
	addr := prefixToSingle(prefix)

	args := []string{"ifconfig", r.tunName, inet(addr), addr.String(), addr.Addr().String()}

	if out, err := cmd(args...).CombinedOutput(); err != nil {
		return fmt.Errorf("addr add failed: %v => %w\n%s", args, err, out)
	}

	return nil
}

func (r *bsdRouter) removeAddr(prefix netip.Prefix) error {
	addr := prefixToSingle(prefix)

	arg := []string{"ifconfig", r.tunName, inet(addr), addr.String(), "-alias"}

	if out, err := cmd(arg...).CombinedOutput(); err != nil {
		return fmt.Errorf("addr del failed: %v => %w\n%s", arg, err, out)
	}

	return nil
}

func (r *bsdRouter) addRoute(prefix netip.Prefix) error {
	net := netipx.PrefixIPNet(prefix)
	// TODO replace with .Masked()?
	nip := net.IP.Mask(net.Mask)
	nstr := fmt.Sprintf("%v/%d", nip, prefix.Bits())

	args := []string{
		"route", "-q", "-n",
		"add", "-" + inet(prefix), nstr,
		"-iface", r.tunName,
	}

	if out, err := cmd(args...).CombinedOutput(); err != nil {
		return fmt.Errorf("route add failed: %v => %w\n%s", args, err, out)
	}

	return nil
}

func (r *bsdRouter) removeRoute(prefix netip.Prefix) error {
	net := netipx.PrefixIPNet(prefix)
	// TODO replace with .Masked()?
	nip := net.IP.Mask(net.Mask)
	nstr := fmt.Sprintf("%v/%d", nip, prefix.Bits())
	del := "del"
	if runtime.GOOS == "darwin" {
		del = "delete"
	}
	routedel := []string{
		"route", "-q", "-n",
		del, "-" + inet(prefix), nstr,
		"-iface", r.tunName,
	}

	if out, err := cmd(routedel...).CombinedOutput(); err != nil {
		return fmt.Errorf("route del failed: %v: %w\n%s", routedel, err, out)
	}

	return nil
}
