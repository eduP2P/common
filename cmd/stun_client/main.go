package main

import (
	"github.com/shadowjonathan/edup2p/types/stun"
	"log"
	"net"
	"net/netip"
	"os"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) != 2 {
		log.Fatalf("usage: %s <hostname>", os.Args[0])
	}
	host := os.Args[1]

	a, err := netip.ParseAddr(host)
	if err != nil {
		log.Fatal(err)
	}

	uaddr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(a, stun.DefaultPort))
	c, err := net.ListenUDP("udp", nil)
	if err != nil {
		log.Fatal(err)
	}

	txID := stun.NewTxID()
	req := stun.Request(txID)

	_, err = c.WriteToUDP(req, uaddr)
	if err != nil {
		log.Fatal(err)
	}

	var buf [1024]byte
	n, raddr, err := c.ReadFromUDPAddrPort(buf[:])
	if err != nil {
		log.Fatal(err)
	}

	tid, saddr, err := stun.ParseResponse(buf[:n])
	if err != nil {
		log.Fatal(err)
	}
	if tid != txID {
		log.Fatalf("txid mismatch: got %v, want %v", tid, txID)
	}

	if raddr.Addr().Is4In6() {
		raddr = netip.AddrPortFrom(netip.AddrFrom4(raddr.Addr().As4()), raddr.Port())
	}

	log.Printf("local   : %v", c.LocalAddr())
	log.Printf("sent  ->  %v", uaddr)
	log.Printf("recv  <-  %v", raddr)
	log.Printf("stun    : %v", saddr)
}
