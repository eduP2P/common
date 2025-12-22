package usrwg

import (
	"os"

	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(mtu int) (tun.Device, error) {
	return tun.CreateTUN("ts0", mtu)
}

func createTUNFromFile(file *os.File, mtu int) (tun.Device, error) {
	return tun.CreateTUNFromFile(file, mtu)
}

func createTUNFromFD(fd uintptr, _ int) (tun.Device, error) {
	dev, _, err := tun.CreateUnmonitoredTUNFromFD(int(fd))
	return dev, err
}
