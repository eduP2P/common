package usrwg

import "golang.zx2c4.com/wireguard/tun"

func createTUN(mtu int) (tun.Device, error) {
	return tun.CreateTUN("utun", mtu)
}
