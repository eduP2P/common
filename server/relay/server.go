package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/msg"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"time"
)

type Server struct {
	pubKey  key.NodePublic
	privKey key.NodePrivate

	mu      sync.RWMutex
	clients map[key.NodePublic]*ServerClient
}

func NewServer(privKey key.NodePrivate) *Server {
	pub := privKey.Public()

	return &Server{
		pubKey:  pub,
		privKey: privKey,
		mu:      sync.RWMutex{},
		clients: make(map[key.NodePublic]*ServerClient),
	}
}

// PublicKey returns the server's public key.
func (s *Server) PublicKey() key.NodePublic {
	return s.pubKey
}

func (s *Server) L() *slog.Logger {
	return slog.With("relay-server", s.pubKey.Debug())
}

type AbstractConn interface {
	io.WriteCloser
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

func (s *Server) sendServerKey(writer *bufio.Writer) (err error) {
	if err = writeFrameHeader(writer, frameServerKey, key.Len); err != nil {
		return
	}

	pKey := s.PublicKey()

	_, err = writer.Write(pKey[:])

	if err != nil {
		return
	}

	err = writer.Flush()

	return
}

func (s *Server) receiveClientKeyAndInfo(reader *bufio.Reader) (clientKey key.NodePublic, info *ClientInfo, err error) {
	var (
		frType FrameType
		frLen  uint32
	)

	if frType, frLen, err = readFrameHeader(reader); err != nil {
		return
	}

	if frType != frameClientInfo {
		err = fmt.Errorf("frame type was not clientinfo, got %d", frType)
		return
	}

	const minLen = key.Len + msg.NaclBoxNonceLen
	if frLen < minLen {
		err = errors.New("short client info")
		return
	} else if frLen > 256<<10 {
		err = errors.New("long client info")
		return
	}

	if _, err = io.ReadFull(reader, clientKey[:]); err != nil {
		return
	}

	msgLen := int(frLen - key.Len)
	msgbox := make([]byte, msgLen)
	if _, err = io.ReadFull(reader, msgbox); err != nil {
		err = fmt.Errorf("msgbox: %v", err)
		return
	}
	m, ok := s.privKey.OpenFrom(clientKey, msgbox)
	if !ok {
		err = fmt.Errorf("msgbox: cannot open len=%d with client key %s", msgLen, clientKey)
		return
	}
	info = new(ClientInfo)
	if err = json.Unmarshal(m, info); err != nil {
		err = fmt.Errorf("msg Unmarshal: %v", err)
		return
	}
	return clientKey, info, nil
}

func (s *Server) sendServerInfo(client *ServerClient) error {
	m, err := json.Marshal(ServerInfo{})
	if err != nil {
		return err
	}

	msgbox := s.privKey.SealTo(client.nodeKey, m)
	if err := writeFrameHeader(client.buffWriter, frameServerInfo, uint32(len(msgbox))); err != nil {
		return err
	}
	if _, err := client.buffWriter.Write(msgbox); err != nil {
		return err
	}
	return client.buffWriter.Flush()
}

func (s *Server) Accept(ctx context.Context, nc AbstractConn, brw *bufio.ReadWriter, remoteAddrPort netip.AddrPort) error {
	reader := brw.Reader
	// TODO: Tailscale mentions that bufio writer buffers take up a large portion of their memory,
	//  and so they've made an implementation that lazily grabs memory from a pool,
	//  and release it if the write buffer is empty.
	//  We could do the same, but this is simpler for now.
	writer := brw.Writer

	nc.SetDeadline(time.Now().Add(10 * time.Second))
	if err := s.sendServerKey(writer); err != nil {
		return fmt.Errorf("send server key: %v", err)
	}

	nc.SetDeadline(time.Now().Add(10 * time.Second))
	clientKey, clientInfo, err := s.receiveClientKeyAndInfo(reader)
	if err != nil {
		return err
	}

	// TODO add verification here?

	// We now trust the client, clear deadline.
	nc.SetDeadline(time.Time{})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	innerCtx, ccc := context.WithCancelCause(ctx)

	client := &ServerClient{
		ctx: innerCtx,
		ccc: ccc,

		server:  s,
		nodeKey: clientKey,
		netConn: nc,

		buffReader: reader,
		buffWriter: writer,

		remoteAddrPort: remoteAddrPort,

		sendCh:        make(chan ServerPacket, ServerClientSendQueueDepth),
		sendSessionCh: make(chan ServerPacket, ServerClientSendQueueDepth),
		sendPongCh:    make(chan PingData, 1),

		info: clientInfo,
	}

	s.registerClient(client)
	defer s.unregisterClient(client)

	if err = s.sendServerInfo(client); err != nil {
		return fmt.Errorf("send server info: %v", err)
	}

	return client.Run()
}

func (s *Server) getClient(peer key.NodePublic) *ServerClient {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.clients[peer]
}

func (s *Server) registerClient(client *ServerClient) {

	// Check if there's a client active on this key already.
	if sc := s.getClient(client.nodeKey); sc != nil {
		// Just cancel the old connected client.
		//
		// This may have a client start fighting another active process if two of them exist at the same time,
		// but considering that such a scenario is a bug or an attack, this disruption is warranted.
		//
		// Or; the very rare case that two nodes get the same public key, but we should check for this in Control,
		// not in relays.
		s.unregisterClient(sc)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.clients[client.nodeKey] = client
}

func (s *Server) unregisterClient(client *ServerClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sc, ok := s.clients[client.nodeKey]

	if !ok {
		return
	}

	// TODO do we sanity check that client == sc?

	sc.Cancel()
	delete(s.clients, client.nodeKey)
}
