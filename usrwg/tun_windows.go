package usrwg

import (
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(mtu int) (tun.Device, error) {
	return tun.CreateTUN("toversok", mtu)
}

func createTUNFromFile(file *os.File, mtu int) (tun.Device, error) {
	return nil, errors.New("not implemented on windows")
}

func init() {
	tun.WintunTunnelType = "ToverSok"
	guid, err := windows.GUIDFromString("{37217669-42da-4657-a55b-13375d328250}")
	if err != nil {
		// We can create a GUID from a static string without error
		panic(err)
	}
	tun.WintunStaticRequestedGUID = &guid
}
