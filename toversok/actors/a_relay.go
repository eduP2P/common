package actors

import (
	"context"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/dial"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgactor"
	"github.com/edup2p/common/types/msgsess"
	"github.com/edup2p/common/types/relay"
	"github.com/edup2p/common/types/relay/relayhttp"
	"log/slog"
	"runtime"
	"time"
)

// RestartableRelayConn is a Relay connection that will automatically reconnect,
// as long as there are pending packets.
type RestartableRelayConn struct {
	*ActorCommon

	man *RelayManager

	config relay.Information

	client *relay.Client

	stay bool

	connected bool

	lastActivity time.Time

	// Buffered packet channel
	bufferCh chan relay.SendPacket

	// an internal poke channel
	pokeCh chan interface{}
}

func (c *RestartableRelayConn) noteActivity() {
	c.lastActivity = time.Now()
}

func (c *RestartableRelayConn) Poke() {
	select {
	case c.pokeCh <- struct{}{}:
	default:
	}
}

func (c *RestartableRelayConn) Run() {
	for {
		if c.shouldIdle() {
			select {
			case <-c.ctx.Done():
				return
			case <-c.pokeCh:
				continue
			case p := <-c.bufferCh:
				c.noteActivity()
				go func() {
					c.bufferCh <- p
				}()
			}
		}

		if !c.establish() {
			// Failed to establish, retry.
			select {
			case <-time.After(RelayConnectionRetryInterval):
				continue
			case <-c.ctx.Done():
				return
			}
		}

		// Established

		c.connected = true

		err := c.loop()

		c.connected = false

		// Possibly the client exited because the relayConn is being closed, check for that first
		select {
		case <-c.ctx.Done():
			return
		default:
			// fallthrough
		}
		if err != nil {
			c.L().Warn("relay client exited", "error", err)
		}
	}
}

func (c *RestartableRelayConn) shouldIdle() bool {
	if c.stay {
		return false
	}

	return time.Now().After(c.lastActivity.Add(RelayConnectionIdleAfter))
}

// establish tests, and/or starts the client, returns when this has timed out, or accomplished.
//
// the boolean success value determines if the establishment has completed.
func (c *RestartableRelayConn) establish() (success bool) {
	var port uint16

	if c.config.IsInsecure {
		port = types.PtrOr(c.config.HTTPPort, 0)
	} else {
		port = types.PtrOr(c.config.HTTPSPort, 0)
	}

	var err error
	c.client, err = relayhttp.Dial(c.ctx, dial.Opts{
		Domain:       c.config.Domain,
		Addrs:        types.SliceOrNil(c.config.IPs),
		Port:         port,
		TLS:          !c.config.IsInsecure,
		ExpectCertCN: types.PtrOr(c.config.CertCN, c.config.Domain),
		// Connect and establishment timeouts are default
		// TODO maybe allow Control to tweak this setting?
	}, c.man.s.getNodePriv, c.config.Key)

	if err != nil {
		c.L().Warn("failed to establish connection to relay", "error", err)
		return false
	} else if c.client == nil {
		c.L().Error("Dial returned no error, but client stayed nil, this is very likely a bug")
		return false
	}

	go c.client.RunSend()
	go c.client.RunReceive()

	c.L().Debug("established")

	return true
}

// loop runs the established connection loop, handling packets.
//
// it returns when the client is dead.
func (c *RestartableRelayConn) loop() error {
	checker := time.NewTicker(time.Second)
	defer checker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return nil
		case <-c.client.Done():
			return fmt.Errorf("relay client exited: %w", c.client.Err())

		case <-checker.C:
			if c.shouldIdle() {
				c.client.Close()
				return nil
			}

		case p := <-c.bufferCh:
			c.noteActivity()
			select {
			case <-c.ctx.Done():
				return nil
			case <-c.client.Done():
				return fmt.Errorf("relay client exited: %w", c.client.Err())
			case c.client.Send() <- p:
			}
		case p := <-c.client.Recv():
			c.noteActivity()
			c.man.inCh <- ifaces.RelayedPeerFrame{
				SrcRelay: c.config.ID,
				SrcPeer:  p.Src,
				Pkt:      p.Data,
			}
		}
	}
}

func (c *RestartableRelayConn) Close() {
	c.ctxCan()
}

// Queue queues the pkt for dst in a non-blocking fashion
func (c *RestartableRelayConn) Queue(pkt []byte, dst key.NodePublic) {
	p := relay.SendPacket{Dst: dst, Data: pkt}

	select {
	case c.bufferCh <- p:
	default:
		// Buffer full, take from the head and drop it
		<-c.bufferCh
		// Try again
		select {
		case c.bufferCh <- p:
		default:
			// Buffer seems full and congested, fail.
		}
	}
}

func (c *RestartableRelayConn) Update(info relay.Information) {
	c.config = info

	// Close the client to trigger a reconnect
	if c.client != nil {
		c.client.Close()
	}
}

func (c *RestartableRelayConn) StayConnected(stay bool) {
	c.stay = stay

	c.Poke()
}

func (c *RestartableRelayConn) IsConnected() bool {
	return c.connected
}

func (c *RestartableRelayConn) L() *slog.Logger {
	return L(c).With("relay", c.config.ID)
}

type RelayConnActor interface {
	ifaces.Actor

	Queue(pkt []byte, peer key.NodePublic)
	Update(info relay.Information)
	StayConnected(bool)
	IsConnected() bool
}

type relayWriteRequest struct {
	toRelay int64
	toPeer  key.NodePublic
	pkt     []byte
}

type RelayManager struct {
	*ActorCommon

	s *Stage

	homeRelay             int64
	latestHomeRelayChange time.Time

	relays map[int64]RelayConnActor

	inCh chan ifaces.RelayedPeerFrame

	writeCh chan relayWriteRequest
}

const HomeRelayChangeInterval = time.Minute * 5

func (s *Stage) makeRM() *RelayManager {
	return &RelayManager{
		ActorCommon: MakeCommon(s.Ctx, RelayManInboxChLen),
		s:           s,
		homeRelay:   0,

		relays:  make(map[int64]RelayConnActor),
		inCh:    make(chan ifaces.RelayedPeerFrame, RelayManFrameChLen),
		writeCh: make(chan relayWriteRequest, RelayManWriteChLen),
	}
}

func (rm *RelayManager) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(rm).Error("panicked", "panic", v)
			rm.Cancel()
			bail(rm.ctx, v)
		}
	}()

	if !rm.running.CheckOrMark() {
		L(rm).Warn("tried to run agent, while already running")
		return
	}

	runtime.LockOSThread()

	for {
		select {
		case <-rm.ctx.Done():
			rm.Close()
			return
		case m := <-rm.inbox:
			switch m := m.(type) {
			case *msgactor.UpdateRelayConfiguration:
				for _, c := range m.Config {
					rm.update(c)
				}
			case *msgactor.RManRelayLatencyResults:
				newRelay := rm.selectRelay(m.RelayLatency)
				oldRelay := rm.homeRelay

				if newRelay != oldRelay {
					if !time.Now().After(rm.latestHomeRelayChange.Add(HomeRelayChangeInterval)) {
						// it is too soon since the latest change, we want to prevent flapping

						if !rm.relays[oldRelay].IsConnected() {
							// special case: old relay is not connected anymore
							slog.Warn("rman: proceeding with home relay change, even though it is too soon since the latest change; old home relay is not connected anymore")
						} else {
							slog.Warn("rman: home relay change was suggested, but its too soon since the latest change", "old-relay", oldRelay, "new-relay", newRelay, "latest-change", rm.latestHomeRelayChange.String())
							continue
						}
					}

					rm.homeRelay = newRelay

					rm.relays[oldRelay].StayConnected(false)
					rm.relays[newRelay].StayConnected(true)

					L(rm).Info("chosen new home relay based on latency", "old-relay", oldRelay, "new-relay", newRelay)

					if err := rm.s.control.UpdateHomeRelay(newRelay); err != nil {
						L(rm).Warn("control: failed to update home relay", "err", err)
					}

					rm.latestHomeRelayChange = time.Now()
				}
			default:
				rm.logUnknownMessage(m)
			}
		case req := <-rm.writeCh:
			conn := rm.getConn(req.toRelay)

			if conn == nil {
				// This only happens if we don't know the relay definition for this relay.
				// Either;
				//  - some home relay got changed to an arbitrary number
				//  - new relay information is still being pushed to us
				//  - there is a bug
				// In any case, log it.

				L(rm).Warn("cannot forward to relay; unknown relay",
					"to_relay", req.toRelay,
					"to_peer", req.toPeer,
				)

				continue
			}

			conn.Queue(req.pkt, req.toPeer)

		case frame := <-rm.inCh:
			rm.s.RRouter.Push(frame)
		}
	}

}

func (rm *RelayManager) Close() {
	rm.ctxCan()
}

func (rm *RelayManager) selectRelay(latencies map[int64]time.Duration) int64 {
	var srid int64 = 0
	var slat = 60 * time.Second

	L(rm).Debug("selectRelay: starting latency check")

	for rid, lat := range latencies {
		L(rm).Log(context.Background(), types.LevelTrace, "selectRelay", "rid", rid, "latency", lat.String())

		if slat > lat {
			srid = rid
			slat = lat
		}
	}

	L(rm).Debug("selectRelay: ending latency check", "selected", srid, "latency", slat.String())

	return srid
}

func (rm *RelayManager) getConn(id int64) RelayConnActor {
	return rm.relays[id]
}

func (rm *RelayManager) update(info relay.Information) {
	if r, ok := rm.relays[info.ID]; ok {
		r.Update(info)
	}

	r := &RestartableRelayConn{
		ActorCommon: MakeCommon(rm.ctx, -1),
		man:         rm,
		config:      info,

		stay:     info.ID == rm.homeRelay,
		bufferCh: make(chan relay.SendPacket, RelayConnSendBufferSize),
		pokeCh:   make(chan interface{}, 1),
	}

	go r.Run()

	rm.relays[info.ID] = r
}

// WriteTo queues a packet relay request to a relay ID, for a certain public key.
//
// Will be called by other actors.
func (rm *RelayManager) WriteTo(pkt []byte, relay int64, dst key.NodePublic) {
	go func() {
		rm.writeCh <- relayWriteRequest{
			toRelay: relay,
			toPeer:  dst,
			pkt:     pkt,
		}
	}()
}

type RelayRouter struct {
	*ActorCommon

	s *Stage

	frameCh chan ifaces.RelayedPeerFrame
}

func (s *Stage) makeRR() *RelayRouter {
	return &RelayRouter{
		ActorCommon: MakeCommon(s.Ctx, -1),
		s:           s,
		frameCh:     make(chan ifaces.RelayedPeerFrame, RelayRouterFrameChLen),
	}
}

func (rr *RelayRouter) Push(frame ifaces.RelayedPeerFrame) {
	go func() {
		rr.frameCh <- frame
	}()
}

func (rr *RelayRouter) Run() {
	defer func() {
		if v := recover(); v != nil {
			L(rr).Warn("panicked", "error", v)
			rr.Cancel()
			bail(rr.ctx, v)
		}
	}()

	if !rr.running.CheckOrMark() {
		L(rr).Warn("tried to run agent, while already running")
		return
	}

	runtime.LockOSThread()

	for {
		select {
		case <-rr.ctx.Done():
			rr.Close()
			return
		case frame := <-rr.frameCh:
			if msgsess.LooksLikeSessionWireMessage(frame.Pkt) {
				SendMessage(rr.s.SMan.Inbox(), &msgactor.SManSessionFrameFromRelay{
					Relay:          frame.SrcRelay,
					Peer:           frame.SrcPeer,
					FrameWithMagic: frame.Pkt,
				})

				continue
			}

			in := rr.s.InConnFor(frame.SrcPeer)

			if in == nil {
				// todo log? metric?
				continue
			}

			in.ForwardPacket(frame.Pkt)
		}
	}
}

func (rr *RelayRouter) Close() {
	// TODO nothing much to close?
}
