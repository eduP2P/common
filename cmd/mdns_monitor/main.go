package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/sethvargo/go-limiter/memorystore"
	"golang.org/x/net/dns/dnsmessage"
)

func walkInterfaces() {
	ift, err := net.Interfaces()
	if err != nil {
		log.Fatal(err)
	}
	for _, ifi := range ift {
		isLoopBack := ifi.Flags&net.FlagLoopback != 0
		isPtP := ifi.Flags&net.FlagPointToPoint != 0

		fmt.Printf("iface %s: lo(%t) ptp(%t)\n", ifi.Name, isLoopBack, isPtP)
	}
}

func main() {
	// this code is specific to macos, for now

	walkInterfaces()

	IP := "224.0.0.251:5353"
	// IP := "[ff02::fb]:5353"

	ua := net.UDPAddrFromAddrPort(netip.MustParseAddrPort(IP))

	iface, err := net.InterfaceByName("lo0")
	if err != nil {
		log.Fatal(err)
	}

	bind, err := net.ListenMulticastUDP("udp4", iface, ua)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("got multicast udp")

	store, err := memorystore.New(&memorystore.Config{
		// Number of tokens allowed per interval.
		Tokens: 1,

		// Interval until tokens reset.
		Interval: 20 * time.Second,

		SweepInterval: 1 * time.Minute,
		SweepMinTTL:   1 * time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 1<<16)

	QUBit := uint16(1 << 15)

	for {
		n, ap, err := bind.ReadFromUDPAddrPort(buf)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("read %d bytes from %s\n", n, ap.String())

		data := buf[:n]

		msg := dnsmessage.Message{}
		if err = msg.Unpack(data); err != nil {
			log.Printf("Error unpacking DNS message: %s\n", err)
			continue
		}

		_, _, _, ok, err := store.Take(context.Background(), msg.GoString())
		if err != nil {
			log.Fatal(err)
		}

		if !ok {
			log.Println("message rate limited")
			continue
		}

		questions := msg.Questions

		msg.Questions = []dnsmessage.Question{}

		fmt.Printf("got mdns: %s\n", msg.GoString())

		for _, q := range questions {
			isQU := uint16(q.Class)&QUBit != 0

			if isQU {
				fmt.Printf("found QU: %s\n", q.GoString())
			} else {
				fmt.Printf("found QM: %s\n", q.GoString())
			}
		}
	}
}
