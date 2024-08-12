package toversok

import (
	"context"
	"fmt"
	"github.com/shadowjonathan/edup2p/toversok/actors"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"net/netip"
)

// Session represents one single session; a session key is generated here, and used inside a Stage
type Session struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	wg WireGuardController
	fw FirewallController

	//control ifaces.ControlSession

	stage ifaces.Stage

	sessionKey key.SessionPrivate
}

func SetupSession(
	engineCtx context.Context,
	wg WireGuardHost,
	fw FirewallHost,
	co ControlHost,
	getExtSock func() types.UDPConn,
	getNodePriv func() *key.NodePrivate,
) (*Session, error) {
	ctx, ccc := context.WithCancelCause(engineCtx)

	sCtx := context.WithValue(ctx, "ccc", ccc)

	sess := &Session{
		ctx:        sCtx,
		ccc:        ccc,
		sessionKey: key.NewSession(),

		stage: nil,
	}

	cc, err := co.CreateClient(sess.ctx, getNodePriv, sess.getPriv)
	if err != nil {
		sess.ccc(err)
		return nil, fmt.Errorf("could not create control client: %w", err)
	}

	if sess.wg, err = wg.Controller(*getNodePriv(), cc.IPv4(), cc.IPv6()); err != nil {
		err = fmt.Errorf("could not init wireguard: %w", err)
		sess.ccc(err)
		return nil, err
	}

	if sess.fw, err = fw.Controller(); err != nil {
		err = fmt.Errorf("could not init firewall: %w", err)
		sess.ccc(err)
		return nil, err
	}

	sess.stage = actors.MakeStage(sess.ctx, getNodePriv, sess.getPriv, getExtSock, sess.wg.ConnFor, cc)

	cc.InstallCallbacks(sess)

	return sess, nil
}

func (s *Session) Start() {
	s.stage.Start()
}

func (s *Session) getPriv() *key.SessionPrivate {
	return &s.sessionKey
}

// CONTROL CALLBACKS

func (s *Session) AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, ip4 netip.Addr, ip6 netip.Addr) error {
	if err := s.wg.UpdatePeer(peer, PeerCfg{
		VIPs: VirtualIPs{
			IPv4: ip4,
			IPv6: ip6,
		},
		KeepAliveInterval: nil,
	}); err != nil {
		return fmt.Errorf("failed to update wireguard: %w", err)
	}

	if err := s.stage.AddPeer(peer, homeRelay, endpoints, session, ip4, ip6); err != nil {
		return fmt.Errorf("failed to update stage: %w", err)
	}

	return nil
}

func (s *Session) RemovePeer(peer key.NodePublic) error {
	if err := s.stage.RemovePeer(peer); err != nil {
		return err
	}

	if err := s.wg.RemovePeer(peer); err != nil {
		return fmt.Errorf("failed to remove peer from wireguard: %w", err)
	}

	return nil
}

// PASSTHROUGH

func (s *Session) UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic) error {
	return s.stage.UpdatePeer(peer, homeRelay, endpoints, session)
}

func (s *Session) UpdateRelays(relay []relay.Information) error {
	return s.stage.UpdateRelays(relay)
}
