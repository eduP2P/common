package usrwg

import (
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(mtu int) (tun.Device, error) {
	return tun.CreateTUN("toversok", mtu)
}

func init() {
	tun.WintunTunnelType = "ToverSok"
	guid, err := windows.GUIDFromString("{37217669-42da-4657-a55b-13375d328250}")
	if err != nil {
		panic(err)
	}
	tun.WintunStaticRequestedGUID = &guid
}
