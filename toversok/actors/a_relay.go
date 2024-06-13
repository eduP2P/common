package actors

import (
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/dial"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msgactor"
	"github.com/shadowjonathan/edup2p/types/msgsess"
	"github.com/shadowjonathan/edup2p/types/relay"
	"github.com/shadowjonathan/edup2p/types/relay/relayhttp"
	"log/slog"
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

	lastActivity time.Time

	// Buffered packet channel
	bufferCh chan relay.SendPacket

	// an internal poke channel
	pokeCh chan interface{}
}

func (c *RestartableRelayConn) noteActivity() {
	c.lastActivity = time.Now()
}

func (c *RestartableRelayConn) Run() {
	for {
		if c.shouldIdle() {
			select {
			case <-c.ctx.Done():
				return
			case <-c.pokeCh:
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

		err := c.loop()

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
		ExpectCertCN: types.PtrOr(c.config.CertCN, ""),
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
}

func (c *RestartableRelayConn) L() *slog.Logger {
	return L(c).With("relay", c.config.ID)
}

type RelayConnActor interface {
	ifaces.Actor

	Queue(pkt []byte, peer key.NodePublic)
	Update(info relay.Information)
	StayConnected(bool)
}

type relayWriteRequest struct {
	toRelay int64
	toPeer  key.NodePublic
	pkt     []byte
}

type RelayManager struct {
	*ActorCommon

	s *Stage

	homeRelay int64

	relays map[int64]RelayConnActor

	inCh chan ifaces.RelayedPeerFrame

	writeCh chan relayWriteRequest
}

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

	for {
		select {
		case <-rm.ctx.Done():
			rm.Close()
			return
		case m := <-rm.inbox:
			if update, ok := m.(*msgactor.UpdateRelayConfiguration); ok {
				for _, c := range update.Config {
					rm.update(c)
				}
			} else {
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
