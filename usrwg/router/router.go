package router

import "net/netip"

// TODO: we could probably refactor this package out of usrwg into something more universal
//  currently it'll be coupled with the data that tun_* gives, so that's a consideration

// Router holds the common interfaces for setting a
type Router interface {
	Up() error
	Set(*Config) error
}

type Config struct {
	Prefixes []netip.Prefix
}
