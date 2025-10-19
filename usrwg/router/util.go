package router

import (
	"fmt"
	"net/netip"
	"os/exec"
)

func prefixesToAdd(newP, currP []netip.Prefix) (add []netip.Prefix) {
	for _, cur := range newP {
		found := false
		for _, v := range currP {
			found = v == cur
			if found {
				break
			}
		}
		if !found {
			add = append(add, cur)
		}
	}
	return
}

func prefixesToRemove(newP, currP []netip.Prefix) (remove []netip.Prefix) {
	for _, cur := range currP {
		found := false
		for _, v := range newP {
			found = v == cur
			if found {
				break
			}
		}
		if !found {
			remove = append(remove, cur)
		}
	}
	return
}

// nolint:unused
// used in router_bsd, golangci-lint on linux trips over it
func inet(p netip.Prefix) string {
	if p.Addr().Is6() {
		return "inet6"
	}
	return "inet"
}

func cmd(args ...string) *exec.Cmd {
	if len(args) == 0 {
		// We control this input, and without argv[0] we can't do anything anyways.
		panic(fmt.Errorf("exec.Cmd(%#v) invalid; need at least 1 argument", args))
	}
	return exec.Command(args[0], args[1:]...)
}

// nolint:unused
// used in router_bsd, golangci-lint on linux trips over it
func prefixToSingle(prefix netip.Prefix) netip.Prefix {
	return netip.PrefixFrom(prefix.Addr(), prefix.Addr().BitLen())
}
