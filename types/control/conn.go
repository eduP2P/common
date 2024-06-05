package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/bin"
	"github.com/shadowjonathan/edup2p/types/conn"
	"github.com/shadowjonathan/edup2p/types/msgcontrol"
	"io"
	"os"
	"sync"
	"time"
)

type Conn struct {
	ctx context.Context

	mc conn.MetaConn

	readMutex sync.Mutex
	reader    *bufio.Reader

	writeMutex sync.Mutex
	writer     *bufio.Writer
}

func NewConn(ctx context.Context, mc conn.MetaConn, brw *bufio.ReadWriter) *Conn {
	return &Conn{
		ctx:    ctx,
		mc:     mc,
		reader: brw.Reader,
		writer: brw.Writer,
	}
}

func (c *Conn) UnmarshalInto(data []byte, to msgcontrol.ControlMessage) error {

	if err := json.Unmarshal(data, to); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

func (c *Conn) Expect(to msgcontrol.ControlMessage, ttfbTimeout time.Duration) error {
	c.readMutex.Lock()
	defer c.readMutex.Unlock()

	msgTyp, data, err := c.readRawMessageLocked(ttfbTimeout)

	if err != nil {
		return fmt.Errorf("failed reading message: %w", err)
	}

	if msgTyp != to.CMsgType() {
		return fmt.Errorf("did not get expected message type, expected %v, got %v", to.CMsgType(), msgTyp)
	}

	return c.UnmarshalInto(data, to)
}

func (c *Conn) ReadRaw(ttfbTimeout time.Duration) (msgcontrol.ControlMessageType, []byte, error) {
	c.readMutex.Lock()
	defer c.readMutex.Unlock()

	return c.readRawMessageLocked(ttfbTimeout)
}

// Read returns nil, nil if the timeout is reached
func (c *Conn) Read(ttfbTimeout time.Duration) (msgcontrol.ControlMessage, error) {
	typ, data, err := c.ReadRaw(ttfbTimeout)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, nil
		}
		return nil, err
	}

	var to msgcontrol.ControlMessage
	switch typ {
	case msgcontrol.ClientHelloType:
		to = new(msgcontrol.ClientHello)
	case msgcontrol.ServerHelloType:
		to = new(msgcontrol.ServerHello)
	case msgcontrol.LogonType:
		to = new(msgcontrol.Logon)
	case msgcontrol.LogonAuthenticateType:
		to = new(msgcontrol.LogonAuthenticate)
	case msgcontrol.LogonDeviceKeyType:
		to = new(msgcontrol.LogonDeviceKey)
	case msgcontrol.LogonAcceptType:
		to = new(msgcontrol.LogonAccept)
	case msgcontrol.LogonRejectType:
		to = new(msgcontrol.LogonReject)
	case msgcontrol.LogoutType:
		to = new(msgcontrol.Logout)
	case msgcontrol.PingType:
		to = new(msgcontrol.Ping)
	case msgcontrol.PongType:
		to = new(msgcontrol.Pong)

	case msgcontrol.EndpointUpdateType:
		to = new(msgcontrol.EndpointUpdate)
	case msgcontrol.PeerAdditionType:
		to = new(msgcontrol.PeerAddition)
	case msgcontrol.HomeRelayUpdateType:
		to = new(msgcontrol.HomeRelayUpdate)
	case msgcontrol.PeerUpdateType:
		to = new(msgcontrol.PeerUpdate)
	case msgcontrol.PeerRemoveType:
		to = new(msgcontrol.PeerRemove)
	case msgcontrol.RelayUpdateType:
		to = new(msgcontrol.RelayUpdate)
	default:
		panic(fmt.Sprintf("Unknown type %v", typ))
	}

	if err = c.UnmarshalInto(data, to); err != nil {
		return nil, err
	}

	return to, nil
}

func (c *Conn) readRawMessageLocked(ttfbTimeout time.Duration) (msgcontrol.ControlMessageType, []byte, error) {
	readTyp, msgLength, err := c.readMessageHeaderLocked(ttfbTimeout)
	if err != nil {
		return 0, nil, err
	}

	// TODO check if msgLength isnt too large
	data := make([]byte, msgLength)

	if _, err := io.ReadFull(c.reader, data); err != nil {
		return 0, nil, fmt.Errorf("failed to read data buffer: %w", err)
	}

	return readTyp, data, nil
}

func (c *Conn) readMessageHeaderLocked(ttfbTimeout time.Duration) (typ msgcontrol.ControlMessageType, length uint32, err error) {
	if ttfbTimeout != 0 {
		c.mc.SetReadDeadline(time.Now().Add(ttfbTimeout))
		defer c.mc.SetReadDeadline(time.Time{})
	}
	readType, err := c.reader.ReadByte()
	if ttfbTimeout != 0 {
		c.mc.SetReadDeadline(time.Time{})
	}
	if err != nil {
		err = fmt.Errorf("failed to read type: %w", err)
		return
	}

	typ = msgcontrol.ControlMessageType(readType)

	length, err = bin.ReadUint32(c.reader)
	if err != nil {
		err = fmt.Errorf("failed to read message length: %w", err)
	}

	return
}

func (c *Conn) Write(obj msgcontrol.ControlMessage) error {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	//// FIXME: bson is extremely fucky and will write empty values if it cannot decode something, so be careful with that
	////  or replace this with a registry thingie.
	//data, err := bson.Marshal(obj)

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("could not marshal data: %w", err)
	}

	if err := c.writer.WriteByte(byte(obj.CMsgType())); err != nil {
		return fmt.Errorf("could not write header; type: %w", err)
	}

	if err := bin.WriteUint32(c.writer, uint32(len(data))); err != nil {
		return fmt.Errorf("could not write header; data length: %w", err)
	}

	if _, err := c.writer.Write(data); err != nil {
		return fmt.Errorf("could not write data: %w", err)
	}

	return c.writer.Flush()
}
