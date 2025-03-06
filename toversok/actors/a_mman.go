package actors

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/msgactor"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/memorystore"
	"net"
	"net/netip"
	"time"
)

type MDNSManager struct {
	*ActorCommon
	s *Stage

	rlStore limiter.Store

	sock *SockRecv
	inj  ifaces.Injectable

	working bool
}

func (s *Stage) makeMM(inj ifaces.Injectable) *MDNSManager {
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
		panic(err)
	}

	m := &MDNSManager{
		ActorCommon: c,
		s:           s,
		rlStore:     store,
	}

	if inj == nil {
		L(m).Error("could not start MDNS Manager; injector is non-present")
		return m
	}
	m.inj = inj

	bind, err := makeMDNSListener()
	if err != nil {
		L(m).Error("could not start MDNS Manager; MDNS listener creation failed", "err", err)

		return m
	}
	m.sock = MakeSockRecv(c.ctx, bind)

	m.working = true

	return m
}

var (
	MDNSPort                uint16 = 5353
	ip4MDNSBroadcastAddress        = netip.AddrPortFrom(netip.MustParseAddr("224.0.0.251"), MDNSPort)
)

// loopbackInterface returns an available logical network interface
// for loopback tests. It returns nil if no suitable interface is
// found.
func loopbackInterface() *net.Interface {
	ift, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifi := range ift {
		if ifi.Flags&net.FlagLoopback != 0 && ifi.Flags&net.FlagUp != 0 {
			return &ifi
		}
	}
	return nil
}

func makeMDNSListener() (types.UDPConn, error) {
	// TODO this only catches ipv4 traffic, which may be a bit "eh",
	//  it may be worth considering firing up one for each stack.
	ua := net.UDPAddrFromAddrPort(ip4MDNSBroadcastAddress)

	return net.ListenMulticastUDP("udp4", loopbackInterface(), ua)
}

func dataToB64Hash(b []byte) string {
	h := sha256.Sum256(b)

	return base64.StdEncoding.EncodeToString(h[:])
}

func (mm *MDNSManager) Run() {
	if !mm.working {
		mm.deadRun()
		return
	}

	go mm.sock.Run()

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

				if !mm.inj.Available() {
					L(mm).Debug("dropping MDNS packet due to unavailable injector", "from", msg.From.Debug())
					continue
				}

				if _, _, _, ok, _ := mm.rlStore.Take(context.Background(), dataToB64Hash(msg.Data)); !ok {
					// some rudimentary filtering to prevent true loop storms
					continue
				}

				L(mm).Log(context.Background(), types.LevelTrace, "injecting external MDNS packet", "len", len(msg.Data), "from", msg.From.Debug())

				err := mm.inj.InjectPacket(netip.AddrPortFrom(pi.IPv4, MDNSPort), ip4MDNSBroadcastAddress, msg.Data)
				if err != nil {
					L(mm).Error("failed to inject MDNS packet", "from", msg.From.Debug(), "err", err)
				}
			default:
				mm.logUnknownMessage(msg)
			}
		case frame := <-mm.sock.outCh:
			// got MDNS message from system; forward

			if _, _, _, ok, _ := mm.rlStore.Take(context.Background(), dataToB64Hash(frame.pkt)); !ok {
				// some rudimentary filtering to prevent true loop storms
				continue
			}

			// TODO proper filtering

			//if !frame.src.Addr().IsLoopback() {
			//	// drop non-loopback, is from LAN
			//	continue
			//}

			L(mm).Log(context.Background(), types.LevelTrace, "spreading local MDNS packet to peers", "len", len(frame.pkt), "from", frame.src.String())

			SendMessage(mm.s.TMan.Inbox(), &msgactor.TManSpreadMDNSPacket{Pkt: frame.pkt})
		case <-mm.s.Ctx.Done():
			mm.Close()
			return
		}
	}
}

func (mm *MDNSManager) deadRun() {
	for {
		select {
		case <-mm.inbox:
		case <-mm.s.Ctx.Done():
			mm.Close()
			return
		}
	}
}

func (mm *MDNSManager) Close() {
	mm.rlStore.Close(context.Background())
}
