package control

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
)

type Client struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	cc *Conn

	getPriv func() *key.NodePrivate
	getSess func() *key.SessionPrivate

	ControlKey key.ControlPublic

	SessionID *string

	IPv4 netip.Prefix
	IPv6 netip.Prefix

	Expiry time.Time
}

func EstablishClient(parentCtx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, timeout time.Duration, getPriv func() *key.NodePrivate, getSess func() *key.SessionPrivate, controlKey key.ControlPublic, session *string, logon types.LogonCallback) (*Client, error) {
	ctx, ccc := context.WithCancelCause(parentCtx)

	c := &Client{
		ctx: ctx,
		ccc: ccc,

		cc: NewConn(ctx, mc, brw),

		getPriv: getPriv,
		getSess: getSess,

		ControlKey: controlKey,
		SessionID:  session,
	}

	if err := c.Handshake(timeout, logon); err != nil {
		return nil, err
	}

	context.AfterFunc(c.ctx, c.Close)

	return c, nil
}

func (c *Client) Handshake(timeout time.Duration, logon types.LogonCallback) error {
	if timeout != 0 {
		if err := c.cc.mc.SetDeadline(time.Now().Add(timeout)); err != nil {
			return fmt.Errorf("can't set deadline: %w", err)
		}
		defer func() {
			if err := c.cc.mc.SetDeadline(time.Time{}); err != nil {
				slog.Error("failed to reset deadline in defer", "err", err)
			}
		}()
	}

	if err := c.cc.Write(&msgcontrol.ClientHello{
		ClientNodePub: c.getPriv().Public(),
	}); err != nil {
		return fmt.Errorf("error when sending clienthello: %w", err)
	}

	serverHello := new(msgcontrol.ServerHello)
	if err := c.cc.Expect(serverHello, 0); err != nil {
		return fmt.Errorf("error when receiving serverhello: %w", err)
	}
	if c.ControlKey.IsZero() {
		c.ControlKey = serverHello.ControlNodePub
		// TODO log TOFU?
	} else if serverHello.ControlNodePub != c.ControlKey {
		return fmt.Errorf("client-stated control key does not match server-given control key")
	}

	clearData, ok := c.getPriv().OpenFromControl(c.ControlKey, serverHello.CheckData)
	if !ok {
		return fmt.Errorf("could not unseal checkdata from control")
	}

	nodePubKey := c.getPriv().Public()
	sessPubKey := c.getSess().Public()

	if err := c.cc.Write(&msgcontrol.Logon{
		SessKey:            sessPubKey,
		NodeKeyAttestation: c.getPriv().SealToControl(c.ControlKey, clearData),
		SessKeyAttestation: c.getSess().SealToControl(c.ControlKey, clearData),
		ResumeSessionID:    c.SessionID,
	}); err != nil {
		return fmt.Errorf("error when sending logon: %w", err)
	}

	// Disable timeout for this
	if err := c.cc.mc.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf("failed to reset deadline: %w", err)
	}
	msg, err := c.cc.Read(0)
	if err != nil {
		return fmt.Errorf("error when receiving after-logon message: %w", err)
	}

	if a, ok := msg.(*msgcontrol.LogonAuthenticate); ok {
		if msg, err = c.handleLogon(a.AuthenticateURL, logon); err != nil {
			return fmt.Errorf("error when handling logon: %w", err)
		}
	}

	switch m := msg.(type) {
	case *msgcontrol.LogonReject:
		return fmt.Errorf(
			"logon rejected after-logon: %s; retry strategy: %w",
			m.Reason,
			m.RetryStrategy,
		)
	case *msgcontrol.LogonAccept:
		c.SessionID = &m.SessionID

		c.IPv4 = m.IP4
		c.IPv6 = m.IP6

		c.Expiry = m.AuthExpiry

		slog.Debug("logon accepted", "as-peer", nodePubKey.Debug(), "as-sess", sessPubKey.Debug(), "with-sess-id", types.PtrOr(c.SessionID, "<nil>"), "with-ipv4", c.IPv4.String(), "with-ipv6", c.IPv6.String())

		return nil
	default:
		return fmt.Errorf("received unknown message type after-logon: %d", m)
	}
}

var ErrNeedsLogon = errors.New("needs logon callback")

func (c *Client) handleLogon(url string, logon types.LogonCallback) (msgcontrol.ControlMessage, error) {
	if logon == nil {
		// No way we can start or create a logon session, abort
		return nil, fmt.Errorf("logonauthenticate requested when no interactive logon callback exists, aborting; %w", ErrNeedsLogon)
	}

	deviceKeyChan := make(chan string)
	defer close(deviceKeyChan)

	if err := logon(url, deviceKeyChan); err != nil {
		return nil, fmt.Errorf("error when calling back logon: %w", err)
	}

	errChan, msgChan := make(chan error, 1), make(chan msgcontrol.ControlMessage, 1)
	defer close(errChan)
	defer close(msgChan)

	go func() {
		msg, err := c.cc.Read(0)
		if err != nil {
			errChan <- err
		} else {
			msgChan <- msg
		}
	}()

	select {
	case deviceKey := <-deviceKeyChan:
		if err := c.cc.Write(&msgcontrol.LogonDeviceKey{
			DeviceKey: deviceKey,
		}); err != nil {
			return nil, fmt.Errorf("error when sending device key: %w", err)
		}

		select {
		case msg := <-msgChan:
			return msg, nil
		case err := <-errChan:
			return nil, fmt.Errorf("error when receiving post-logon message: %w", err)
		case <-c.ctx.Done():
			return nil, fmt.Errorf("context closed: %w", c.ctx.Err())
		}
	case err := <-errChan:
		return nil, fmt.Errorf("error when receiving post-logon message: %w", err)
	case msg := <-msgChan:
		return msg, nil

	case <-c.ctx.Done():
		return nil, fmt.Errorf("context closed: %w", c.ctx.Err())
	}
}

var ErrClosed = errors.New("client closed")

func (c *Client) Send(msg msgcontrol.ControlMessage) error {
	if types.IsContextDone(c.ctx) {
		return ErrClosed
	}

	return c.cc.Write(msg)
}

// Recv blocks until it receives a package, it will return (nil, nil) if timeout
func (c *Client) Recv(ttfbTimeout time.Duration) (msgcontrol.ControlMessage, error) {
	if types.IsContextDone(c.ctx) {
		return nil, ErrClosed
	}

	return c.cc.Read(ttfbTimeout)
}

//	if errors.Is(err, os.ErrDeadlineExceeded) {
//		return nil, nil
//	}

func (c *Client) Close() {
	if err := c.cc.mc.Close(); err != nil {
		slog.Error("error when closing control client", "err", err)
	}
}

func (c *Client) Cancel(err error) {
	c.ccc(err)
}
