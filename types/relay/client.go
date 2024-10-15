package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgsess"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"
)

const (
	PacketChanLen = 16

	PingInterval = 30 * time.Second
)

var (
	errInvalidFrameType    = errors.New("invalid frame type")
	errPacketTooLarge      = errors.New("packet too large")
	errKeepAliveNonZeroLen = errors.New("keepalive frame has non-zero length")
)

// Client is a Relay client that lives as long as its conn does
type Client struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	mc types.MetaConn

	recvMutex sync.Mutex
	reader    *bufio.Reader

	sendMutex sync.Mutex
	writer    *bufio.Writer

	getPriv func() *key.NodePrivate

	relayServerKey key.NodePublic

	sendCh chan SendPacket
	recvCh chan RecvPacket

	closed bool
}

func (c *Client) Send() chan<- SendPacket {
	return c.sendCh
}

func (c *Client) Recv() <-chan RecvPacket {
	return c.recvCh
}

func (c *Client) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *Client) Err() error {
	return c.ctx.Err()
}

// TODO these two types are almost certainly also defined somewhere else, dedup

type SendPacket struct {
	Dst key.NodePublic

	Data []byte
}

type RecvPacket struct {
	Src key.NodePublic

	Data []byte
}

// EstablishClient creates a new relay.Client on a given MetaConn with associated bufio.ReadWriter.
//
// It logs in and authenticates the server before returning a Client object.
// If any error occurs, or no client can be established before timeout, it returns.
func EstablishClient(parentCtx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, timeout time.Duration, getPriv func() *key.NodePrivate) (*Client, error) {
	ctx, ccc := context.WithCancelCause(parentCtx)

	c := &Client{
		ctx: ctx,
		ccc: ccc,

		mc: mc,

		recvMutex: sync.Mutex{},
		reader:    brw.Reader,

		sendMutex: sync.Mutex{},
		writer:    brw.Writer,

		getPriv: getPriv,

		sendCh: make(chan SendPacket, PacketChanLen),
		recvCh: make(chan RecvPacket, PacketChanLen),
	}

	// Make sure any reads that don't complete before the deadline return with an error.
	if err := mc.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("could not set deadline: %w", err)
	}

	ver, err := c.recvVersion()
	if err != nil {
		return nil, fmt.Errorf("error receiving server version: %w", err)
	}

	if ver != relayProtocolV0 {
		return nil, fmt.Errorf("unsupported relay version, expected v0, got %d", ver)
	}

	if err = c.recvServerKey(); err != nil {
		return nil, fmt.Errorf("error receiving server key: %w", err)
	}

	if err = c.sendClientInfo(); err != nil {
		return nil, fmt.Errorf("error sending client info: %w", err)
	}

	// We discard the server info for now.
	// TODO use server info
	if _, err = c.recvServerInfo(); err != nil {
		return nil, fmt.Errorf("error receiving server info: %w", err)
	}

	// Reset the deadline mechanism
	if err = mc.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("could not reset deadline: %w", err)
	}

	go func() {
		<-c.ctx.Done()
		c.Close()
	}()

	return c, nil
}

func (c *Client) privateKey() *key.NodePrivate {
	return c.getPriv()
}

func (c *Client) publicKey() key.NodePublic {
	return c.privateKey().Public()
}

// RelayKey returns the key of the relay we're connected to.
func (c *Client) RelayKey() key.NodePublic {
	return c.relayServerKey
}

// recvVersion assumes the caller has ownership, or lock
func (c *Client) recvVersion() (ProtocolVersion, error) {
	b, err := c.reader.ReadByte()

	return ProtocolVersion(b), err
}

// recvServerKey assumes the caller has ownership, or lock
func (c *Client) recvServerKey() error {
	frTyp, frLen, err := readFrameHeader(c.reader)
	if err != nil {
		return err
	}

	if frTyp != frameServerKey {
		return errInvalidFrameType
	}

	if frLen < key.Len {
		return errors.New("server key frame length too small")
	} else if frLen > key.Len {
		return errors.New("server key frame length too big")
	}

	var buf [32]byte

	_, err = io.ReadFull(c.reader, buf[:])

	if err != nil {
		return err
	}

	c.relayServerKey = buf

	return nil
}

// sendClientInfo assumes the caller has ownership, or lock
func (c *Client) sendClientInfo() error {
	m, err := json.Marshal(ClientInfo{SendKeepalive: true})
	if err != nil {
		return err
	}
	msgbox := c.privateKey().SealTo(c.relayServerKey, m)

	pub := c.publicKey()

	buf := slices.Concat(pub[:], msgbox)

	err = writeFrameHeader(c.writer, frameClientInfo, uint32(len(buf)))
	if err != nil {
		return err
	}

	if _, err = c.writer.Write(buf); err != nil {
		return err
	}

	return c.writer.Flush()
}

// recvServerInfo assumes the caller has ownership, or lock
func (c *Client) recvServerInfo() (*ServerInfo, error) {
	frTyp, frLen, err := readFrameHeader(c.reader)
	if err != nil {
		return nil, err
	}

	if frTyp != frameServerInfo {
		return nil, errInvalidFrameType
	}

	if frLen < msgsess.NaclBoxNonceLen {
		return nil, errors.New("frame too small for naclbox nonce")
	} else if frLen > MaxPacketSize {
		return nil, errPacketTooLarge
	}

	var msgbox = make([]byte, frLen)

	if _, err = io.ReadFull(c.reader, msgbox); err != nil {
		return nil, err
	}

	text, ok := c.privateKey().OpenFrom(c.relayServerKey, msgbox)

	if !ok {
		return nil, errors.New("could not open server info msgbox")
	}

	info := new(ServerInfo)

	if err = json.Unmarshal(text, info); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return info, nil
}

func (c *Client) Cancel(err error) {
	c.ccc(err)
	c.mc.SetDeadline(time.Now().Add(10 * time.Millisecond))
}

func (c *Client) Close() {
	if c.closed || context.Cause(c.ctx) != nil {
		return
	}

	c.mc.Close()
	close(c.sendCh)
	close(c.recvCh)

	c.closed = true
}

func (c *Client) Closed() bool {
	return c.closed
}

func (c *Client) RunReceive() {
	if !c.recvMutex.TryLock() {
		slog.Error("could not lock recvMutex, is RunReceive already running?")
		return
	}
	defer c.recvMutex.Unlock()

	defer func() {
		if v := recover(); v != nil {
			c.ccc(fmt.Errorf("reader panicked: %s", v))
		}
	}()

	var (
		frTyp FrameType
		frLen uint32
		err   error
	)

	for {
		frTyp, frLen, err = readFrameHeader(c.reader)

		select {
		case <-c.ctx.Done():
			return
		default:
			// fallthrough
		}

		if err != nil {
			c.Cancel(fmt.Errorf("error receiving frame header: %w", err))
			return
		}

		switch frTyp {
		case frameRecvPacket:
			if frLen < key.Len {
				err = errors.New("recvpacket len too small for key")
				break
			}

			pkt := RecvPacket{
				Data: make([]byte, frLen-key.Len),
			}

			if _, err = io.ReadFull(c.reader, pkt.Src[:]); err != nil {
				break
			}

			if _, err = io.ReadFull(c.reader, pkt.Data); err != nil {
				break
			}

			// TODO this could block, should we do this in a goroutine?
			c.recvCh <- pkt
		case framePong:
			// Ignore for now
			// FIXME do checking that we sent the ping?
			_, err = c.reader.Discard(int(frLen))
		case frameKeepAlive:
			if frLen != 0 {
				err = errKeepAliveNonZeroLen
			}
			// We've acked it by receiving it, fallthrough
		default:
			err = fmt.Errorf("received unknown frame type: %d", frTyp)
		}

		if err != nil {
			c.Cancel(fmt.Errorf("error processing frame of type %d: %w", frTyp, err))
			return
		}
	}
}

func (c *Client) RunSend() {
	if !c.sendMutex.TryLock() {
		slog.Error("could not lock sendMutex, is RunSend already running?")
		return
	}
	defer c.sendMutex.Unlock()

	pingTicker := time.NewTicker(ServerClientKeepAlive)
	defer pingTicker.Stop()

	defer func() {
		if v := recover(); v != nil {
			c.Cancel(fmt.Errorf("sender panicked: %s", v))
		}
	}()

	var err error

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-pingTicker.C:
			if err = writeFrameHeader(c.writer, framePing, 8); err != nil {
				break
			}

			// TODO proper ping/pong handling
			_, err = c.writer.Write([]byte("toversok"))

		case pkt := <-c.sendCh:
			if err = writeFrameHeader(c.writer, frameSendPacket, uint32(len(pkt.Data)+key.Len)); err != nil {
				break
			}

			// TODO proper ping/pong handling
			if _, err = c.writer.Write(pkt.Dst[:]); err != nil {
				break
			}

			if _, err = c.writer.Write(pkt.Data); err != nil {
				break
			}

			err = c.writer.Flush()
		}

		if err != nil {
			c.Cancel(fmt.Errorf("error writing: %w", err))
			return
		}
	}
}
