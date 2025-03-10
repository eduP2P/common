package actors

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/msgactor"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/memorystore"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

type MDNSManager struct {
	*ActorCommon
	s *Stage

	rlStore limiter.Store

	broadSock *SockRecv
	querySock *SockRecv

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

	bind, err := m.makeMDNSListener()
	if err != nil {
		L(m).Error("could not start MDNS Manager; MDNS listener creation failed", "err", err)

		return m
	}
	m.broadSock = MakeSockRecv(c.ctx, bind)

	queryBind, err := m.makeQueryListener()
	if err != nil {
		L(m).Error("could not start MDNS Manager; MDNS sender creation failed", "err", err)

		return m
	}

	m.querySock = MakeSockRecv(c.ctx, queryBind)

	m.working = true

	return m
}

var (
	MDNSPort                uint16 = 5353
	ip4uaMDNSBare                  = net.UDPAddr{IP: net.IPv4(224, 0, 0, 251)}
	ip4MDNSLoopBackAP              = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), MDNSPort)
	ip4MDNSBroadcastAddress        = netip.AddrPortFrom(netip.MustParseAddr("224.0.0.251"), MDNSPort)
)

func (mm *MDNSManager) makeMDNSListener() (types.UDPConn, error) {
	// TODO this only catches ipv4 traffic, which may be a bit "eh",
	//  it may be worth considering firing up one for each stack.
	ua := net.UDPAddrFromAddrPort(ip4MDNSBroadcastAddress)

	conn, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return nil, fmt.Errorf("ListenUDP error: %w", err)
	}

	pc := ipv4.NewPacketConn(conn)

	ift, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("cannot get interfaces: %w", err)
	}
	for _, ifi := range ift {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagPointToPoint == 0 {
			if err := pc.JoinGroup(&ifi, &ip4uaMDNSBare); err != nil {
				L(mm).Warn("Multicast JoinGroup failed", "err", err, "iface", ifi.Name)
			}
		}
	}

	if loop, err := pc.MulticastLoopback(); err == nil {
		if !loop {
			if err := pc.SetMulticastLoopback(true); err != nil {
				return nil, fmt.Errorf("cannot set multicast loopback: %w", err)
			}
		}
	}

	return conn, nil
}

func (mm *MDNSManager) makeQueryListener() (types.UDPConn, error) {
	var laddr *net.UDPAddr
	addr := ip4MDNSLoopBackAP

	if runtime.GOOS == "windows" {
		laddr = net.UDPAddrFromAddrPort(
			netip.AddrPortFrom(mm.s.control.IPv4().Addr(), 0),
		)
		addr = ip4MDNSBroadcastAddress
	}

	return net.DialUDP("udp4", laddr, net.UDPAddrFromAddrPort(addr))
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

	go mm.broadSock.Run()
	go mm.querySock.Run()

	for {
		select {
		case msg := <-mm.inbox:
			// got MDNS message from external; inject
			switch msg := msg.(type) {
			case *msgactor.MManReceivedPacket:
				pi := mm.s.GetPeerInfo(msg.From)
				if pi == nil {
					L(mm).Warn("ignoring MDNS packet due to nonexistent peerinfo", "from", msg.From.Debug())
					continue
				}

				if _, _, _, ok, _ := mm.rlStore.Take(context.Background(), dataToB64Hash(msg.Data)); !ok {
					// some rudimentary filtering to prevent true loop storms
					continue
				}

				L(mm).Debug("processing external MDNS packet", "len", len(msg.Data), "from", msg.From.Debug())

				pkt := mm.processMDNS(msg.Data, false)

				var err error

				// TODO process external mDNS packet

				if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
					// On macOS, we can't use the broadsock's WriteTo, since it just doesn't generate a packet.
					// However, we can use our specialised query sock to poke responses in unicast, even if they're QM.
					_, err = mm.querySock.Conn.Write(pkt)
				} else {
					_, err = mm.broadSock.Conn.WriteToUDPAddrPort(pkt, ip4MDNSBroadcastAddress)
				}
				if err != nil {
					L(mm).Warn("failed to process external MDNS packet", "err", err)
				}
			default:
				mm.logUnknownMessage(msg)
			}
		case frame := <-mm.broadSock.outCh:
			mm.handleSystemFrame(frame)
		case frame := <-mm.querySock.outCh:
			mm.handleSystemFrame(frame)
		case <-mm.ctx.Done():
			return
		}
	}
}

func (mm *MDNSManager) handleSystemFrame(frame RecvFrame) {
	// got MDNS message from system; forward

	if _, _, _, ok, _ := mm.rlStore.Take(context.Background(), dataToB64Hash(frame.pkt)); !ok {
		// some rudimentary filtering to prevent true loop storms
		return
	}

	if !mm.isSelf(frame.src.Addr()) {
		L(mm).Log(context.Background(), types.LevelTrace, "dropping mDNS packet due to non-local origin", "from", frame.src)
		return
	}

	// TODO proper in-depth filtering

	L(mm).Debug("spreading local MDNS packet to peers", "len", len(frame.pkt), "from", frame.src.String())

	pkt := mm.processMDNS(frame.pkt, true)

	SendMessage(mm.s.TMan.Inbox(), &msgactor.TManSpreadMDNSPacket{Pkt: pkt})
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
