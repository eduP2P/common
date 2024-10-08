package relay

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"io"
	"log/slog"
	"math/rand"
	"net/netip"
	"time"
)

// ServerPacket is a transient packet type handled by the server
type ServerPacket struct {
	bytes []byte

	src key.NodePublic
}

type PingData [8]byte

// ServerClient represents an active client connected to a Server.
type ServerClient struct {
	ctx context.Context
	// context cancel cause
	ccc context.CancelCauseFunc

	server *Server

	nodeKey key.NodePublic

	// sendCh contains wireguard packets and whatnot
	sendCh chan ServerPacket

	// sendSessionCh is a biased sidechannel queue for session messages
	sendSessionCh chan ServerPacket

	// An asynchronous pong return channel, hopping a pong between RunReceiver and RunSender
	sendPongCh chan PingData

	netConn types.MetaConn

	remoteAddrPort netip.AddrPort

	// Not thread-safe; owned by RunReceiver
	buffReader *bufio.Reader
	// Not thread-safe; owned by RunSender
	buffWriter *bufio.Writer

	info *ClientInfo
}

// SendPacket will be called by other goroutines than the ServerClient-owning Run goroutine.
func (sc *ServerClient) SendPacket(pkt ServerPacket) {
	queue := sc.sendCh
	if msgsess.LooksLikeSessionWireMessage(pkt.bytes) {
		queue = sc.sendSessionCh
	}

	// First pass trying to queue directly
	select {
	case <-sc.ctx.Done():
		// return, dst is gone
		sc.L().Warn("could not send packet; sc context done", "src", pkt.src.Debug())
		return
	case queue <- pkt:
		return
	default:
		// fallthrough
	}

	// Second pass, create a goroutine that tries for 5 more seconds
	// TODO this can create a lot of goroutines if the client is slow at emptying their queue,
	//  these only exist for 5 seconds, but its still a bunch.
	go func() {
		select {
		case <-sc.ctx.Done():
			sc.L().Warn("could not send packet after delay; sc context done", "src", pkt.src.Debug())
			// return, dst is gone
			return
		case queue <- pkt:
			return
		case <-time.NewTimer(time.Second * 5).C:
			// Timed out, return
			return
		}
	}()
}

// Run will be called by Server.Accept in a blocking fashion.
func (sc *ServerClient) Run() (err error) {
	go sc.RunReceiver()
	go sc.RunSender()

	sc.L().Info("new client", "peer", sc.nodeKey.Debug())

	<-sc.ctx.Done()

	return sc.ctx.Err()
}

func (sc *ServerClient) RunReceiver() {
	defer func() {
		if v := recover(); v != nil {
			sc.ccc(fmt.Errorf("receiver panicked: %s", v))
		}
	}()

	for {
		frType, frLen, err := readFrameHeader(sc.buffReader)

		if err != nil {
			if errors.Is(err, io.EOF) {
				sc.ccc(fmt.Errorf("reader: read EOF"))
				return
			}
			sc.ccc(fmt.Errorf("reader: read error: client %s: readFrameHeader: %w", sc.nodeKey.HexString(), err))
			return
		}

		// First see if the context has been cancelled
		select {
		case <-sc.ctx.Done():
			return
		default:
		}

		switch frType {
		case frameSendPacket:
			err = sc.handleSend(frLen)
		case framePing:
			err = sc.handlePing(frLen)
		default:
			err = sc.handleUnknownFrame(frType, frLen)
		}

		if err != nil {
			sc.ccc(err)
			return
		}
	}
}

func (sc *ServerClient) handleSend(frLen uint32) error {

	dstKey, contents, err := sc.readSend(frLen)
	if err != nil {
		return err
	}

	dstClient := sc.server.getClient(dstKey)

	if dstClient == nil {
		// We can't do much more than drop the packet
		// TODO tailscale sends back that the peer is gone,
		//   we currently dont take into account if peers are connected when sending over relay,
		//   we assume the home relay always is able to send packets.
		sc.L().Warn("handleSend dropping packet", "to-peer", dstKey.Debug(), "reason", "client-not-connected")
		return nil
	}

	slog.Debug("sending packet", "src", sc.nodeKey.Debug(), "dst", dstClient.nodeKey.Debug())

	dstClient.SendPacket(ServerPacket{
		bytes: contents,
		src:   sc.nodeKey,
	})

	return nil
}

func (sc *ServerClient) readSend(frLen uint32) (dstKey key.NodePublic, contents []byte, err error) {
	if frLen < key.Len {
		err = errors.New("short send packet frame")
		return
	}

	if _, err = io.ReadFull(sc.buffReader, dstKey[:]); err != nil {
		return
	}

	packetLen := frLen - key.Len

	if packetLen > MaxPacketSize {
		err = fmt.Errorf("data packet longer (%d) than max of %v", packetLen, MaxPacketSize)
		return
	}

	contents = make([]byte, packetLen)

	_, err = io.ReadFull(sc.buffReader, contents)

	return
}

func (sc *ServerClient) handlePing(frLen uint32) error {
	var m PingData
	if frLen < uint32(len(m)) {
		return fmt.Errorf("short ping: %v", frLen)
	}
	if frLen > 1000 {
		// unreasonably extra large. We leave some extra
		// space for future extensibility, but not too much.
		return fmt.Errorf("ping body too large: %v", frLen)
	}
	_, err := io.ReadFull(sc.buffReader, m[:])
	if err != nil {
		return err
	}
	if extra := int64(frLen) - int64(len(m)); extra > 0 {
		_, err = io.CopyN(io.Discard, sc.buffReader, extra)
	}
	select {
	case sc.sendPongCh <- m:
	default:
		// They're pinging too fast. Ignore.
		// TODO: maybe add a rate limiter too?
	}

	return err
}

func (sc *ServerClient) handleUnknownFrame(frameType FrameType, frameLength uint32) error {
	sc.L().Warn("got unknown frame type", "frame-type", frameType)

	// Discard the frame, we can't do much with it
	_, err := io.CopyN(io.Discard, sc.buffReader, int64(frameLength))
	return err
}

func (sc *ServerClient) RunSender() {
	jitter := time.Duration(rand.Intn(5000)) * time.Millisecond
	keepAliveTicker := time.NewTicker(ServerClientKeepAlive + jitter)
	defer keepAliveTicker.Stop()
	defer func() {
		if v := recover(); v != nil {
			sc.ccc(fmt.Errorf("sender panicked: %s", v))
		}
	}()

	var werr error // last write error
	for {
		if werr != nil {
			sc.ccc(fmt.Errorf("sender write error: %w", werr))
			return
		}
		// First, a non-blocking select (with a default) that
		// does as many non-flushing writes as possible.
		select {
		case <-sc.ctx.Done():
			return
		case pkt := <-sc.sendCh:
			werr = sc.sendPacket(pkt.src, pkt.bytes)
			continue
		case pkt := <-sc.sendSessionCh:
			werr = sc.sendPacket(pkt.src, pkt.bytes)
			continue
		case data := <-sc.sendPongCh:
			werr = sc.sendPong(data)
			continue
		case <-keepAliveTicker.C:
			werr = sc.sendKeepAlive()
			continue
		default:
			// Flush any writes from the 3 sends above, or from
			// the blocking loop below.
			if werr = sc.buffWriter.Flush(); werr != nil {
				// we will catch the error in the beginning of the loop below; less duplication
				continue
			}
		}

		// Then a blocking select with same:
		select {
		case <-sc.ctx.Done():
			return
		case pkt := <-sc.sendCh:
			werr = sc.sendPacket(pkt.src, pkt.bytes)
		case pkt := <-sc.sendSessionCh:
			werr = sc.sendPacket(pkt.src, pkt.bytes)
		case data := <-sc.sendPongCh:
			werr = sc.sendPong(data)
		case <-keepAliveTicker.C:
			werr = sc.sendKeepAlive()
		}
	}
}

func (sc *ServerClient) setWriteDeadline() {
	sc.netConn.SetWriteDeadline(time.Now().Add(ServerClientWriteTimeout))
}

// sendKeepAlive sends a keep-alive frame, without flushing.
func (sc *ServerClient) sendKeepAlive() error {
	sc.setWriteDeadline()

	return writeFrameHeader(sc.buffWriter, frameKeepAlive, 0)
}

func (sc *ServerClient) sendPacket(src key.NodePublic, data []byte) (err error) {
	sc.setWriteDeadline()

	pktLen := len(data) + key.Len

	if err = writeFrameHeader(sc.buffWriter, frameRecvPacket, uint32(pktLen)); err != nil {
		return err
	}

	if _, err := sc.buffWriter.Write(src[:]); err != nil {
		return err
	}
	_, err = sc.buffWriter.Write(data)

	return err
}

func (sc *ServerClient) sendPong(data [8]byte) error {
	sc.setWriteDeadline()

	if err := writeFrameHeader(sc.buffWriter, framePong, uint32(len(data))); err != nil {
		return err
	}
	_, err := sc.buffWriter.Write(data[:])
	return err
}

func (sc *ServerClient) L() *slog.Logger {
	return sc.server.L().With("server-client", sc.nodeKey.Debug())
}

func (sc *ServerClient) Cancel() {
	sc.ccc(fmt.Errorf("cancelled"))
}
