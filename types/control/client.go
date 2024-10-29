package control

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
	"log/slog"
	"net/netip"
	"time"
)

type Client struct {
	ctx context.Context

	cc *Conn

	getPriv func() *key.NodePrivate
	getSess func() *key.SessionPrivate

	ControlKey key.ControlPublic

	SessionID *string

	IPv4 netip.Prefix
	IPv6 netip.Prefix

	// TODO
}

func EstablishClient(parentCtx context.Context, mc types.MetaConn, brw *bufio.ReadWriter, timeout time.Duration, getPriv func() *key.NodePrivate, getSess func() *key.SessionPrivate, controlKey key.ControlPublic, session *string) (*Client, error) {
	c := &Client{
		ctx: parentCtx,

		cc: NewConn(parentCtx, mc, brw),

		getPriv: getPriv,
		getSess: getSess,

		ControlKey: controlKey,
		SessionID:  session,
	}

	if err := c.Handshake(timeout); err != nil {
		return nil, err
	} else {
		return c, nil
	}
}

func (c *Client) Handshake(timeout time.Duration) error {

	// TODO
	//  1. send ClientHello
	//  2. expect ServerHello
	//  3. send Logon
	//  4. (optional) expect LogonAuthenticate
	//    - Allow sending LogonDeviceKey
	//  4. expect LogonAccept|LogonReject

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
	} else {
		if serverHello.ControlNodePub != c.ControlKey {
			return fmt.Errorf("client-stated control key does not match server-given control key")
		}
	}

	clearData, ok := c.getPriv().OpenFromControl(c.ControlKey, serverHello.CheckData)
	if !ok {
		return fmt.Errorf("could not unseal checkdata from control")
	}

	if err := c.cc.Write(&msgcontrol.Logon{
		SessKey:            c.getSess().Public(),
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

	switch m := msg.(type) {
	case *msgcontrol.LogonAuthenticate:
		// TODO
		panic("authenticate logic not implemented")
	case *msgcontrol.LogonReject:
		return fmt.Errorf(
			"logon rejected after-logon: %s; retry strategy: %w",
			m.Reason,
			m.RetryStrategy,
		)
	case *msgcontrol.LogonAccept:
		c.SessionID = &m.SessionID

		//c.IPv4 = netip.PrefixFrom(netip.Addr(m.IPv4Addr), int(m.IPv4Mask))
		//c.IPv6 = netip.PrefixFrom(netip.Addr(m.IPv6Addr), int(m.IPv6Mask))

		c.IPv4 = m.IP4
		c.IPv6 = m.IP6

		return nil
	default:
		return fmt.Errorf("received unknown message type after-logon: %d", m)
	}

	//switch typ {
	//case msgcontrol.LogonAuthenticateType:
	//	// TODO
	//	panic("authenticate logic not implemented")
	//case msgcontrol.LogonAcceptType:
	//	accept := new(msgcontrol.LogonAccept)
	//	if err := ReadMessage(c.reader, msgLen, accept); err != nil {
	//		return fmt.Errorf("error when reading after-logon reject message: %w", err)
	//	}
	//
	//	c.SessionID = &accept.SessionID
	//	c.IPv4 = netip.PrefixFrom(netip.Addr(accept.IPv4Addr), int(accept.IPv4Mask))
	//	c.IPv6 = netip.PrefixFrom(netip.Addr(accept.IPv6Addr), int(accept.IPv6Mask))
	//
	//	return nil
	//
	//case msgcontrol.LogonRejectType:
	//	reject := new(msgcontrol.LogonReject)
	//	if err := ReadMessage(c.reader, msgLen, reject); err != nil {
	//		return fmt.Errorf("error when reading after-logon reject message: %w", err)
	//	}
	//
	//	return fmt.Errorf(
	//		"logon rejected after-logon: %s; retry strategy: %w",
	//		reject.Reason,
	//		types.PtrOr(reject.RetryStrategy, msgcontrol.NoRetryStrategy),
	//	)
	//default:
	//	return fmt.Errorf("received unknown message type after-logon: %d", typ)
	//}
	//
	//typ, msgLen, err = ReadMessageHeader(c.reader)
	//if err != nil {
	//	return fmt.Errorf("error when receiving after-authenticate message: %w", err)
	//}
	//
	//switch typ {
	//case msgcontrol.LogonAcceptType:
	//	// TODO
	//	panic("implement me")
	//case msgcontrol.LogonRejectType:
	//	reject := new(msgcontrol.LogonReject)
	//	if err := ReadMessage(c.reader, msgLen, reject); err != nil {
	//		return fmt.Errorf("error when reading after-authenticate reject message: %w", err)
	//	}
	//
	//	return fmt.Errorf(
	//		"logon rejected after-authenticate: %s; retry strategy: %w",
	//		reject.Reason,
	//		types.PtrOr(reject.RetryStrategy, msgcontrol.NoRetryStrategy),
	//	)
	//default:
	//	return fmt.Errorf("received unknown message type after-authenticate: %d", typ)
	//}
}

var ClosedErr = errors.New("client closed")

func (c *Client) Send(msg msgcontrol.ControlMessage) error {
	if types.IsContextDone(c.ctx) {
		return ClosedErr
	}

	return c.cc.Write(msg)
}

// Recv blocks until it receives a package, it will return (nil, nil) if timeout
func (c *Client) Recv(ttfbTimeout time.Duration) (msgcontrol.ControlMessage, error) {
	if types.IsContextDone(c.ctx) {
		return nil, ClosedErr
	}

	return c.cc.Read(ttfbTimeout)
}

//	if errors.Is(err, os.ErrDeadlineExceeded) {
//		return nil, nil
//	}

func (c *Client) Close() {
	c.cc.mc.Close()
}
