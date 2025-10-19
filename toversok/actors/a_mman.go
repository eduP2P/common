package actors

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"runtime/debug"
	"slices"
	"syscall"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/msgactor"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/memorystore"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type MDNSManager struct {
	*ActorCommon
	s *Stage

	rlStore limiter.Store

	b4Sock *SockRecv
	b6Sock *SockRecv

	u4Sock *SockRecv
	u6Sock *SockRecv

	working bool
}

func (s *Stage) makeMM() *MDNSManager {
	c := MakeCommon(s.Ctx, MdnsManInboxChLen)

	store, err := memorystore.New(&memorystore.Config{
		// Number of tokens allowed per interval.
		Tokens: 1,

		// Interval until tokens reset.
		Interval: 20 * time.Second,

		SweepInterval: 1 * time.Minute,
		SweepMinTTL:   1 * time.Minute,
	})
	if err != nil {
		// memorystore does not return an error, so this is unexpected
		panic(err)
	}

	m := assureClose(&MDNSManager{
		ActorCommon: c,
		s:           s,
		rlStore:     store,
	})

	b4bind, err := m.makeMDNSv4Listener()
	if err != nil {
		L(m).Warn("MDNS ipv4 listener creation failed", "err", err)
	} else {
		m.b4Sock = MakeSockRecv(c.ctx, b4bind)
	}

	b6bind, err := m.makeMDNSv6Listener()
	if err != nil {
		L(m).Warn("MDNS ipv6 listener creation failed", "err", err)
	} else {
		m.b6Sock = MakeSockRecv(c.ctx, b6bind)
	}

	if m.b4Sock == nil && m.b6Sock == nil {
		L(m).Error("could not start MDNS Manager; creating both MDNS broadcast sockets failed")

		return m
	}

	u4bind, err := m.makeIPv4UnicastListener()
	if err != nil {
		L(m).Warn("MDNS ipv4 sender creation failed", "err", err)
	} else {
		m.u4Sock = MakeSockRecv(c.ctx, u4bind)
	}

	u6bind, err := m.makeIPv6UnicastListener()
	if err != nil {
		L(m).Warn("MDNS ipv4 sender creation failed", "err", err)
	} else {
		m.u6Sock = MakeSockRecv(c.ctx, u6bind)
	}

	if m.u4Sock == nil && m.u6Sock == nil {
		L(m).Error("could not start MDNS Manager; creating both MDNS unicast sockets failed")

		return m
	}

	m.working = true

	return m
}

var (
	MDNSPort             uint16 = 5353
	ip4MDNSBroadcastBare        = netip.MustParseAddr("224.0.0.251")
	ip6MDNSBroadcastBare        = netip.MustParseAddr("ff02::fb")

	ip4MDNSBroadcastAP = netip.AddrPortFrom(ip4MDNSBroadcastBare, MDNSPort)
	ip6MDNSBroadcastAP = netip.AddrPortFrom(ip6MDNSBroadcastBare, MDNSPort)

	ip4MDNSLoopBackAP = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), MDNSPort)
	ip6MDNSLoopBackAP = netip.AddrPortFrom(netip.IPv6Loopback(), MDNSPort)
)

func getLoopBackInterface() (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("could not list network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback != 0 {
			return &iface, nil
		}
	}

	return nil, fmt.Errorf("no loopback interface found")
}

func (mm *MDNSManager) makeMDNSv4Listener() (types.UDPConn, error) {
	ua := net.UDPAddrFromAddrPort(ip4MDNSBroadcastAP)

	conn, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return nil, fmt.Errorf("ListenUDP error: %w", err)
	}

	pc4 := ipv4.NewPacketConn(conn)

	ift, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("cannot get interfaces: %w", err)
	}
	for _, ifi := range ift {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagPointToPoint == 0 {
			if err := pc4.JoinGroup(&ifi, &net.UDPAddr{IP: ip4MDNSBroadcastBare.AsSlice()}); err != nil {
				L(mm).Warn("pc4 Multicast JoinGroup failed", "err", err, "iface", ifi.Name)
			}
		}
	}

	if loop, err := pc4.MulticastLoopback(); err == nil {
		if !loop {
			if err := pc4.SetMulticastLoopback(true); err != nil {
				return nil, fmt.Errorf("cannot set multicast loopback: %w", err)
			}
		}
	}

	lo, err := getLoopBackInterface()
	if err != nil {
		return nil, fmt.Errorf("cannot get loopback interface: %w", err)
	}

	if err := pc4.SetMulticastInterface(lo); err != nil {
		return nil, fmt.Errorf("cannot set multicast interface: %w", err)
	}

	if err := pc4.SetTTL(255); err != nil {
		return nil, fmt.Errorf("cannot set TTL: %w", err)
	}
	if err := pc4.SetMulticastTTL(255); err != nil {
		return nil, fmt.Errorf("cannot set Multicast TTL: %w", err)
	}

	return conn, nil
}

func (mm *MDNSManager) makeMDNSv6Listener() (types.UDPConn, error) {
	ua := net.UDPAddrFromAddrPort(ip6MDNSBroadcastAP)

	conn, err := net.ListenUDP("udp6", ua)
	if err != nil {
		return nil, fmt.Errorf("ListenUDP error: %w", err)
	}

	pc6 := ipv6.NewPacketConn(conn)

	ift, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("cannot get interfaces: %w", err)
	}
	for _, ifi := range ift {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagPointToPoint == 0 {
			if err := pc6.JoinGroup(&ifi, &net.UDPAddr{IP: ip6MDNSBroadcastBare.AsSlice()}); err != nil && !errors.Is(err, syscall.EAFNOSUPPORT) {
				L(mm).Warn("pc6 Multicast JoinGroup failed", "err", err, "iface", ifi.Name)
			}
		}
	}

	if loop, err := pc6.MulticastLoopback(); err == nil {
		if !loop {
			if err := pc6.SetMulticastLoopback(true); err != nil {
				return nil, fmt.Errorf("cannot set multicast loopback: %w", err)
			}
		}
	}

	lo, err := getLoopBackInterface()
	if err != nil {
		return nil, fmt.Errorf("cannot get loopback interface: %w", err)
	}

	if err := pc6.SetMulticastInterface(lo); err != nil {
		return nil, fmt.Errorf("cannot set multicast interface: %w", err)
	}

	return conn, nil
}

func (mm *MDNSManager) makeIPv4UnicastListener() (types.UDPConn, error) {
	var laddr *net.UDPAddr
	addr := ip4MDNSLoopBackAP

	if runtime.GOOS == "windows" {
		laddr = net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(mm.s.control.IPv4().Addr(), 0),
		)
		addr = ip4MDNSBroadcastAP
	}

	return net.DialUDP("udp4", laddr, net.UDPAddrFromAddrPort(addr))
}

func (mm *MDNSManager) makeIPv6UnicastListener() (types.UDPConn, error) {
	var laddr *net.UDPAddr
	addr := ip6MDNSLoopBackAP

	if runtime.GOOS == "windows" {
		laddr = net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(mm.s.control.IPv6().Addr(), 0),
		)
		addr = ip6MDNSBroadcastAP
	}

	return net.DialUDP("udp6", laddr, net.UDPAddrFromAddrPort(addr))
}

func dataToB64Hash(b []byte) string {
	h := sha256.Sum256(b)

	return base64.StdEncoding.EncodeToString(h[:])
}

func (mm *MDNSManager) Run() {
	if !mm.running.CheckOrMark() {
		L(mm).Warn("tried to run agent, while already running")
		return
	}

	defer mm.Cancel()
	defer func() {
		if v := recover(); v != nil {
			L(mm).Error("panicked", "panic", v, "stack", string(debug.Stack()))
			bail(mm.ctx, v)
		}
	}()

	if !mm.working {
		mm.deadRun()
		return
	}

	go mm.b4Sock.Run()
	go mm.u4Sock.Run()

	for {
		select {
		case msg := <-mm.inbox:
			// got MDNS message from external; inject
			switch msg := msg.(type) {
			case *msgactor.MManReceivedPacket:
				mm.handleReceivedPacket(msg)
			default:
				mm.logUnknownMessage(msg)
			}
		case frame := <-mm.b4Sock.outCh:
			mm.handleSystemFrame(frame)
		case frame := <-mm.b6Sock.outCh:
			mm.handleSystemFrame(frame)
		case frame := <-mm.u4Sock.outCh:
			mm.handleSystemFrame(frame)
		case frame := <-mm.u6Sock.outCh:
			mm.handleSystemFrame(frame)
		case <-mm.ctx.Done():
			return
		}
	}
}

func (mm *MDNSManager) handleReceivedPacket(msg *msgactor.MManReceivedPacket) {
	pi := mm.s.GetPeerInfo(msg.From)
	if pi == nil {
		L(mm).Warn("ignoring MDNS packet due to nonexistent peerinfo", "from", msg.From.Debug())
		return
	}

	extra := "ip4"
	if msg.IP6 {
		extra = "ip6"
	}

	if _, _, _, ok, _ := mm.rlStore.Take(context.Background(), dataToB64Hash(msg.Data)+extra); !ok {
		// some rudimentary filtering to prevent true loop storms
		return
	}

	L(mm).Debug("processing external MDNS packet", "len", len(msg.Data), "from", msg.From.Debug())

	pkt := mm.processMDNS(msg.Data, false)

	var err error

	// TODO process external mDNS packet

	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// On macOS, we can't use the broadsock's WriteTo, since it just doesn't generate a packet.
		// However, we can use our specialised query sock to poke responses in unicast, even if they're QM.
		if msg.IP6 {
			_, err = mm.u6Sock.Conn.Write(pkt)
		} else {
			_, err = mm.u4Sock.Conn.Write(pkt)
		}
	} else {
		if msg.IP6 {
			_, err = mm.b6Sock.Conn.WriteToUDPAddrPort(pkt, ip6MDNSBroadcastAP)
		} else {
			_, err = mm.b4Sock.Conn.WriteToUDPAddrPort(pkt, ip4MDNSBroadcastAP)
		}
	}
	if err != nil {
		L(mm).Warn("failed to process external MDNS packet", "err", err)
	}
}

func (mm *MDNSManager) handleSystemFrame(frame RecvFrame) {
	// got MDNS message from system; forward

	nap := types.NormaliseAddr(frame.src.Addr())

	extra := "ip4"
	if nap.Is6() {
		extra = "ip6"
	}

	if _, _, _, ok, _ := mm.rlStore.Take(context.Background(), dataToB64Hash(frame.pkt)+extra); !ok {
		// some rudimentary filtering to prevent true loop storms
		return
	}

	if !mm.isSelf(nap) {
		L(mm).Log(context.Background(), types.LevelTrace, "dropping mDNS packet due to non-local origin", "from", frame.src)
		return
	}

	// TODO proper in-depth filtering

	L(mm).Debug("spreading local MDNS packet to peers", "len", len(frame.pkt), "from", frame.src.String())

	pkt := mm.processMDNS(frame.pkt, true)

	SendMessage(mm.s.TMan.Inbox(), &msgactor.TManSpreadMDNSPacket{Pkt: pkt, IP6: nap.Is6()})
}

func (mm *MDNSManager) debugMDNS(msg *dnsmessage.Message) {
	L(mm).Debug("debugMDNS: TXID", "txid", msg.ID)

	for _, q := range msg.Questions {
		L(mm).Debug(
			"debugMDNS: Q",
			"txid", msg.ID,
			"name", q.Name,
			"type", q.Type.GoString(),
			"class", q.Class.GoString(),
		)
	}
	for _, a := range msg.Answers {
		L(mm).Debug(
			"debugMDNS: A",
			"txid", msg.ID,
			"header", a.Header.GoString(),
			"body", a.Body.GoString(),
		)
	}
}

func (mm *MDNSManager) fixResource(res *dnsmessage.Resource) (dirty bool) {
	switch res.Header.Type {
	case dnsmessage.TypeA:
		ar := res.Body.(*dnsmessage.AResource)
		if mm.isLocal(netip.AddrFrom4(ar.A)) {
			ar.A = mm.s.control.IPv4().Addr().As4()
			res.Header.Class |= 32768
			dirty = true
		}
	case dnsmessage.TypeAAAA:
		a4r := res.Body.(*dnsmessage.AAAAResource)

		if mm.isLocal(netip.AddrFrom16(a4r.AAAA)) {
			a4r.AAAA = mm.s.control.IPv6().Addr().As16()
			res.Header.Class |= 32768
			dirty = true
		}
	}

	return
}

func (mm *MDNSManager) isLocal(addr netip.Addr) bool {
	return addr.IsLoopback() || slices.IndexFunc(mm.s.getLocalEndpoints(), func(cAddr netip.Addr) bool {
		return cAddr == addr
	}) != -1
}

func (mm *MDNSManager) isSelf(addr netip.Addr) bool {
	return mm.isLocal(addr) || addr == mm.s.control.IPv4().Addr() || addr == mm.s.control.IPv6().Addr()
}

func (mm *MDNSManager) processMDNS(pkt []byte, local bool) []byte {
	msg := dnsmessage.Message{}
	if err := msg.Unpack(pkt); err != nil {
		L(mm).Warn("failed to unpack MDNS packet", "err", err)
		return pkt
	}

	mm.debugMDNS(&msg)

	var dirty bool

	if local {
		for _, ans := range msg.Answers {
			if mm.fixResource(&ans) {
				dirty = true
			}
		}

		for _, add := range msg.Additionals {
			if mm.fixResource(&add) {
				dirty = true
			}
		}
	} else if msg.Response {
		// RFC 6762:
		//   Multicast DNS responses MUST NOT contain any questions in the
		//   Question Section.  Any questions in the Question Section of a
		//   received Multicast DNS response MUST be silently ignored.  Multicast
		//   DNS queriers receiving Multicast DNS responses do not care what
		//   question elicited the response; they care only that the information
		//   in the response is true and accurate.
		//
		// f.e. avahi doesn't properly work if the questions section is filled out, so we need to process that.
		//
		// The likes of Apple's mDNSResponder haven't gotten this above message, so we need to check for this.
		//
		// TODO this may be because we're regarding it via unicast DNS, and then this is the fallback described in
		//  section 6.7?
		//   If the source UDP port in a received Multicast DNS query is not port
		//   5353, this indicates that the querier originating the query is a
		//   simple resolver such as described in Section 5.1, "One-Shot Multicast
		//   DNS Queries", which does not fully implement all of Multicast DNS.
		//   In this case, the Multicast DNS responder MUST send a UDP response
		//   directly back to the querier, via unicast, to the query packet's
		//   source IP address and port.  This unicast response MUST be a
		//   conventional unicast response as would be generated by a conventional
		//   Unicast DNS server; for example, it MUST repeat the query ID and the
		//   question given in the query message.  In addition, the cache-flush
		//   bit described in Section 10.2, "Announcements to Flush Outdated Cache
		//   Entries", MUST NOT be set in legacy unicast responses.
		if len(msg.Questions) != 0 {
			msg.Questions = []dnsmessage.Question{}
			dirty = true
		}
	}

	if dirty {
		L(mm).Debug("processMDNS: rewritten request")

		mm.debugMDNS(&msg)

		ret, err := msg.Pack()
		if err != nil {
			L(mm).Warn("failed to pack MDNS packet", "err", err)
			return pkt
		}

		return ret
	}

	return pkt
}

func (mm *MDNSManager) deadRun() {
	for {
		select {
		case <-mm.inbox:
		case <-mm.ctx.Done():
			return
		}
	}
}

func (mm *MDNSManager) Close() {
	mm.rlStore.Close(context.Background())
}
