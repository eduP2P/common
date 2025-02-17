package toversok

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"

	"github.com/edup2p/common/toversok/actors"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/ifaces"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
	"github.com/edup2p/common/types/relay"
)

// Session represents one single session; a session key is generated here, and used inside a Stage
type Session struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	wg WireGuardController
	fw FirewallController

	quarantineMu     sync.Mutex
	quarantinedPeers map[key.NodePublic]bool
	peerAddrs        map[key.NodePublic][]netip.Addr

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
	logon types.LogonCallback,
) (*Session, error) {
	ctx, ccc := context.WithCancelCause(engineCtx)

	sCtx := context.WithValue(ctx, types.CCC, ccc)

	sess := &Session{
		ctx:              sCtx,
		ccc:              ccc,
		quarantineMu:     sync.Mutex{},
		quarantinedPeers: make(map[key.NodePublic]bool),
		peerAddrs:        make(map[key.NodePublic][]netip.Addr),
		sessionKey:       key.NewSession(),

		stage: nil,
	}

	cc, err := co.CreateClient(sess.ctx, getNodePriv, sess.getPriv, logon)
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

	sess.stage = actors.MakeStage(sess.ctx, getNodePriv, sess.getPriv, getExtSock, sess.wg.ConnFor, cc, nil)

	cc.InstallCallbacks(sess)

	return sess, nil
}

func (s *Session) Start() {
	s.stage.Start()
}

func (s *Session) getPriv() *key.SessionPrivate {
	return &s.sessionKey
}

func (s *Session) registerPeerAddrs(peer key.NodePublic, ip4, ip6 netip.Addr) {
	s.quarantineMu.Lock()
	defer s.quarantineMu.Unlock()

	s.peerAddrs[peer] = []netip.Addr{ip4, ip6}
}

func (s *Session) upsertQuarantine(peer key.NodePublic) {
	s.quarantineMu.Lock()
	defer s.quarantineMu.Unlock()

	if !s.quarantinedPeers[peer] {
		s.quarantinedPeers[peer] = true
		s.triggerQuarantineUpdate()
	}
}

func (s *Session) delQuarantine(peer key.NodePublic) {
	s.quarantineMu.Lock()
	defer s.quarantineMu.Unlock()

	if s.quarantinedPeers[peer] {
		s.quarantinedPeers[peer] = false
		s.triggerQuarantineUpdate()
	}
}

// (assumes locked quarantineMu)
func (s *Session) triggerQuarantineUpdate() {
	var addrs []netip.Addr

	for peer, isQuarantined := range s.quarantinedPeers {
		if isQuarantined {
			addrs = append(addrs, s.peerAddrs[peer]...)
		}
	}

	if err := s.fw.QuarantineNodes(addrs); err != nil {
		slog.Error("could not update firewall with quarantined peers", "err", err, "addrs", addrs)
	}
}

// CONTROL CALLBACKS

func (s *Session) AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, ip4, ip6 netip.Addr, prop msgcontrol.Properties) error {
	s.registerPeerAddrs(peer, ip4, ip6)

	if prop.Quarantine {
		s.upsertQuarantine(peer)
	} else {
		s.delQuarantine(peer)
	}

	if err := s.wg.UpdatePeer(peer, PeerCfg{
		VIPs: VirtualIPs{
			IPv4: ip4,
			IPv6: ip6,
		},
		KeepAliveInterval: nil,
	}); err != nil {
		return fmt.Errorf("failed to update wireguard: %w", err)
	}

	if err := s.stage.AddPeer(peer, homeRelay, endpoints, session, ip4, ip6, prop); err != nil {
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

func (s *Session) UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic, prop *msgcontrol.Properties) error {
	if prop != nil {
		if prop.Quarantine {
			s.upsertQuarantine(peer)
		}
	}

	return s.stage.UpdatePeer(peer, homeRelay, endpoints, session, prop)
}

// PASSTHROUGH

func (s *Session) UpdateRelays(relay []relay.Information) error {
	return s.stage.UpdateRelays(relay)
}
