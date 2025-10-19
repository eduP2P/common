package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/edup2p/common/types"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

var multicastIface string

func init() {
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(fmt.Errorf("could not list network interfaces: %w", err))
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback != 0 {
			multicastIface = iface.Name
		}
	}
}

//nolint:unused
var (
	MDNSPort             uint16 = 5353
	ip4MDNSBroadcastBare        = netip.MustParseAddr("224.0.0.251")
	ip6MDNSBroadcastBare        = netip.MustParseAddr("ff02::fb")

	ip4MDNSUnspecifiedAP = netip.AddrPortFrom(netip.IPv4Unspecified(), MDNSPort)
	ip6MDNSUnspecifiedAP = netip.AddrPortFrom(netip.IPv6Unspecified(), MDNSPort)

	ip4MDNSBroadcastAP = netip.AddrPortFrom(ip4MDNSBroadcastBare, MDNSPort)
	ip6MDNSBroadcastAP = netip.AddrPortFrom(ip6MDNSBroadcastBare, MDNSPort)

	ip4MDNSLoopBackAP    = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), MDNSPort)
	ip4MDNSLoopBackAPAlt = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.2"), MDNSPort)
	ip6MDNSLoopBackAP    = netip.AddrPortFrom(netip.IPv6Loopback(), MDNSPort)
	ip6MDNSLoopBackAPAlt = netip.AddrPortFrom(netip.MustParseAddr("::2"), MDNSPort)
)

const bit15 = 2 << 14

func main() {
	if len(os.Args) < 2 {
		println("Usage: mdns_test <.local name>")
		os.Exit(1)
	}

	name := os.Args[1] + ".local."

	dnsName, err := dnsmessage.NewName(name)
	if err != nil {
		panic(fmt.Errorf("failed to make DNS name: %w", err))
	}
	allServicesName := dnsmessage.MustNewName("_services._dns-sd._udp.local.")

	nameQM := dnsmessage.Question{
		Name:  dnsName,
		Type:  dnsmessage.TypeA,
		Class: dnsmessage.ClassINET,
	}

	servicesQM := dnsmessage.Question{
		Name:  allServicesName,
		Type:  dnsmessage.TypePTR,
		Class: dnsmessage.ClassINET,
	}

	ml4, p4, err := makeIPv4MDNSListener()
	if err != nil {
		panic(fmt.Errorf("failed to make ipv4 mdns listener: %w", err))
	}
	defer ml4.Close()

	ml6, p6, err := makeIPv6MDNSListener()
	if err != nil {
		panic(fmt.Errorf("failed to make ipv6 mdns listener: %w", err))
	}
	defer ml6.Close()

	u4, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(ip4MDNSLoopBackAP))
	if err != nil {
		panic(fmt.Errorf("failed to make ipv4 unicast listener: %w", err))
	}
	defer u4.Close()

	u6, err := net.DialUDP("udp6", nil, net.UDPAddrFromAddrPort(ip6MDNSLoopBackAP))
	if err != nil {
		panic(fmt.Errorf("failed to make ipv6 unicast listener: %w", err))
	}
	defer u6.Close()

	var respMu sync.Mutex
	var responses []*dnsmessage.Message

	appendResponse := func(msg *dnsmessage.Message) {
		respMu.Lock()
		defer respMu.Unlock()
		responses = append(responses, msg)
	}

	go func() {
		buf := make([]byte, 1<<16)

		for {
			n, cm, ap, err := p6.ReadFrom(buf)
			if err != nil {
				panic(fmt.Errorf("failed to read from ipv6 mdns listener: %w", err))
			}

			var dst net.IP

			if cm != nil && cm.Dst != nil {
				dst = cm.Dst
			}

			slog.Info("received ipv6 packet", "from", ap.String(), "len", n, "dst", dst)

			msg := new(dnsmessage.Message)

			if err := msg.Unpack(buf[:n]); err != nil {
				slog.Error("could not unpack ipv6 packet into mdns", "error", err)
				continue
			}

			appendResponse(msg)
		}
	}()

	go func() {
		buf := make([]byte, 1<<16)

		for {
			n, cm, ap, err := p4.ReadFrom(buf)
			if err != nil {
				panic(fmt.Errorf("failed to read from ipv4 mdns listener: %w", err))
			}

			var dst net.IP

			if cm != nil && cm.Dst != nil {
				dst = cm.Dst
			}

			slog.Info("received ipv4 packet", "from", ap.String(), "len", n, "dst", dst)

			msg := new(dnsmessage.Message)

			if err := msg.Unpack(buf[:n]); err != nil {
				slog.Error("could not unpack ipv4 packet into mdns", "error", err)
				continue
			}

			appendResponse(msg)
		}
	}()

	listener := func(name string, conn *net.UDPConn) {
		buf := make([]byte, 1<<16)

		for {
			n, ap, err := conn.ReadFromUDPAddrPort(buf)
			if err != nil {
				panic(fmt.Errorf(name+": failed to read: %w", err))
			}

			slog.Info(name+": received packet", "from", ap.String(), "len", n)

			msg := new(dnsmessage.Message)

			if err := msg.Unpack(buf[:n]); err != nil {
				slog.Error(name+": could not unpack packet into mdns", "error", err)
				continue
			}

			appendResponse(msg)
		}
	}

	go listener("uni4", u4)
	go listener("uni6", u6)

	nameQU := nameQM
	// unicast-response
	nameQU.Class |= bit15

	servicesQU := servicesQM
	servicesQU.Class |= bit15

	questions := []dnsmessage.Question{
		// nameQM,
		// nameQU,
		servicesQM,
		servicesQU,
	}

	var queries []*dnsmessage.Message

	for _, q := range questions {
		queries = append(queries, makeQuery(q))
	}

	type writeTo struct {
		conn types.UDPConn
		name string
		to   *netip.AddrPort
	}

	toWrite := []writeTo{
		{conn: ml6, to: &ip6MDNSBroadcastAP, name: "ml6bc"},
		{conn: ml4, to: &ip4MDNSBroadcastAP, name: "ml4bc"},
		{conn: ml6, to: &ip6MDNSLoopBackAP, name: "ml6lo"},
		{conn: ml4, to: &ip4MDNSLoopBackAP, name: "ml4lo"},
		{conn: u4, name: "u4"},
		{conn: u6, name: "u6"},
	}

	qna := make(map[*dnsmessage.Message]map[string][]*dnsmessage.Message)

	for _, q := range queries {
		current := make(map[string][]*dnsmessage.Message)
		qna[q] = current

		for _, w := range toWrite {
			doWrite(w.conn, w.name, w.to, q)

			time.Sleep(1 * time.Second)

			respMu.Lock()
			current[w.name] = responses
			responses = nil
			respMu.Unlock()
		}
	}

	processQNA(qna)
}

func processQNA(m map[*dnsmessage.Message]map[string][]*dnsmessage.Message) {
	println("\n\n\n")
	slog.Info("Printing QNA result")

	for query, result := range m {
		println("\n")
		slog.Info("Printing for query")
		debugMDNS(query)
		println()

		for name, responses := range result {
			slog.Info("Printing responses for name", "name", name)

			for _, msg := range responses {
				debugMDNS(msg)
			}

			println()
		}
	}
}

func doWrite(c types.UDPConn, name string, to *netip.AddrPort, msg *dnsmessage.Message) {
	q, err := msg.Pack()
	if err != nil {
		log.Fatal(fmt.Errorf("%s: could not pack message: %w", name, err))
	}

	if to == nil {
		if _, err := c.Write(q); err != nil {
			log.Println(fmt.Errorf("%s: failed to write query: %w", name, err))
		}
	} else {
		if _, err := c.WriteToUDPAddrPort(q, *to); err != nil {
			log.Println(fmt.Errorf("%s: failed to write query to addrport: %w", name, err))
		}
	}
}

func makeQuery(q dnsmessage.Question) *dnsmessage.Message {
	return &dnsmessage.Message{
		Header:    dnsmessage.Header{},
		Questions: []dnsmessage.Question{q},
	}
}

func makeIPv4MDNSListener() (types.UDPConn, *ipv4.PacketConn, error) {
	ua := net.UDPAddrFromAddrPort(ip4MDNSBroadcastAP)

	conn, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return nil, nil, fmt.Errorf("ListenUDP error: %w", err)
	}

	p4 := ipv4.NewPacketConn(conn)

	ift, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get interfaces: %w", err)
	}
	for _, ifi := range ift {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagPointToPoint == 0 {
			if err := p4.JoinGroup(&ifi, &net.UDPAddr{IP: ip4MDNSBroadcastBare.AsSlice()}); err != nil && !errors.Is(err, syscall.EAFNOSUPPORT) {
				slog.Warn("p4 multicast JoinGroup failed", "err", err, "iface", ifi.Name)
			}
		}
	}

	if loop, err := p4.MulticastLoopback(); err == nil {
		if !loop {
			if err := p4.SetMulticastLoopback(true); err != nil {
				return nil, nil, fmt.Errorf("cannot set multicast loopback: %w", err)
			}
			slog.Info("Multicast Loopback enabled")
		} else {
			slog.Info("Multicast Loopback was enabled")
		}
	} else {
		return nil, nil, fmt.Errorf("cannot get MulticastLoopback: %w", err)
	}

	ifi, err := net.InterfaceByName(multicastIface)
	if err != nil {
		panic(err)
	}

	if err := p4.SetMulticastInterface(ifi); err != nil {
		return nil, nil, fmt.Errorf("cannot set multicast interface: %w", err)
	}

	if err := p4.SetTTL(255); err != nil {
		return nil, nil, fmt.Errorf("cannot set TTL: %w", err)
	}
	if err := p4.SetMulticastTTL(255); err != nil {
		return nil, nil, fmt.Errorf("cannot set Multicast TTL: %w", err)
	}

	if err = p4.SetControlMessage(ipv4.FlagDst, true); err != nil {
		slog.Warn("cannot set control message dstflag", "err", err)
	}

	return conn, p4, nil
}

func makeIPv6MDNSListener() (types.UDPConn, *ipv6.PacketConn, error) {
	ua := net.UDPAddrFromAddrPort(ip6MDNSBroadcastAP)

	conn, err := net.ListenUDP("udp6", ua)
	if err != nil {
		return nil, nil, fmt.Errorf("ListenUDP error: %w", err)
	}

	p6 := ipv6.NewPacketConn(conn)

	ift, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get interfaces: %w", err)
	}
	for _, ifi := range ift {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagPointToPoint == 0 {
			if err := p6.JoinGroup(&ifi, &net.UDPAddr{IP: ip6MDNSBroadcastBare.AsSlice()}); err != nil && !errors.Is(err, syscall.EAFNOSUPPORT) {
				slog.Warn("p6 multicast JoinGroup failed", "err", err, "iface", ifi.Name)
			}
		}
	}

	if loop, err := p6.MulticastLoopback(); err == nil {
		if !loop {
			if err := p6.SetMulticastLoopback(true); err != nil {
				return nil, nil, fmt.Errorf("cannot set multicast loopback: %w", err)
			}
			slog.Info("Multicast Loopback enabled")
		} else {
			slog.Info("Multicast Loopback was enabled")
		}
	} else {
		return nil, nil, fmt.Errorf("cannot get MulticastLoopback: %w", err)
	}

	ifi, err := net.InterfaceByName(multicastIface)
	if err != nil {
		panic(err)
	}

	if err := p6.SetMulticastInterface(ifi); err != nil {
		return nil, nil, fmt.Errorf("cannot set multicast interface: %w", err)
	}

	if err = p6.SetControlMessage(ipv6.FlagDst, true); err != nil {
		slog.Warn("cannot set control message dstflag", "err", err)
	}

	return conn, p6, nil
}

func debugMDNS(msg *dnsmessage.Message) {
	slog.Info("debugMDNS: TXID", "txid", msg.ID)

	for _, q := range msg.Questions {
		slog.Info(
			"debugMDNS: Q",
			"txid", msg.ID,
			"name", q.Name,
			"type", q.Type.GoString(),
			"class", q.Class.GoString(),
		)
	}
	for _, a := range msg.Answers {
		slog.Info(
			"debugMDNS: A",
			"txid", msg.ID,
			"header", a.Header.GoString(),
			"body", a.Body.GoString(),
		)
	}
}
