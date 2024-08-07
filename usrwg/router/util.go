package router

import (
	"log"
	"net/netip"
	"os/exec"
)

func prefixesToAdd(new, curr []netip.Prefix) (add []netip.Prefix) {
	for _, cur := range new {
		found := false
		for _, v := range curr {
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

func prefixesToRemove(new, curr []netip.Prefix) (remove []netip.Prefix) {
	for _, cur := range curr {
		found := false
		for _, v := range new {
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

func inet(p netip.Prefix) string {
	if p.Addr().Is6() {
		return "inet6"
	}
	return "inet"
}

func cmd(args ...string) *exec.Cmd {
	if len(args) == 0 {
		log.Fatalf("exec.Cmd(%#v) invalid; need argv[0]", args)
	}
	return exec.Command(args[0], args[1:]...)
}

func prefixToSingle(prefix netip.Prefix) netip.Prefix {
	return netip.PrefixFrom(prefix.Addr(), prefix.Addr().BitLen())
}
