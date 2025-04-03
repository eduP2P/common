package usrwg

import (
	"os"

	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(mtu int) (tun.Device, error) {
	return tun.CreateTUN("utun", mtu)
}

func createTUNFromFile(file *os.File, mtu int) (tun.Device, error) {
	return tun.CreateTUNFromFile(file, mtu)
}
