package stun

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

type Server struct {
	ctx  context.Context // ctx signals service shutdown
	bind *net.UDPConn    // bind is the UDP listener
}

func NewServer(ctx context.Context) *Server {
	return &Server{ctx: ctx}
}

func (s *Server) Listen(addrPort netip.AddrPort) error {
	ua := net.UDPAddrFromAddrPort(addrPort)

	var err error
	s.bind, err = net.ListenUDP("udp", ua)
	if err != nil {
		return err
	}
	log.Printf("STUN server listening on %v", s.LocalAddr())
	// close the listener on shutdown in order to break out of the read loop
	go func() {
		<-s.ctx.Done()
		if err := s.bind.Close(); err != nil {
			slog.Error("failed to close bind", "err", err)
		}
	}()
	return nil
}

// LocalAddr returns the local address of the STUN server. It must not be called before ListenAndServe.
func (s *Server) LocalAddr() net.Addr {
	return s.bind.LocalAddr()
}

func (s *Server) Serve() error {
	var buf [64 << 10]byte
	var (
		n   int
		ua  *net.UDPAddr
		err error
	)
	for {
		n, ua, err = s.bind.ReadFromUDP(buf[:])
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("STUN ReadFrom: %v", err)
			time.Sleep(time.Second)
			continue
		}

		pkt := buf[:n]
		if !Is(pkt) {
			// Just drop it (like its hot)
			continue
		}

		txid, err := ParseBindingRequest(pkt)
		if err != nil {
			slog.Debug("ParseBindingRequest failed", "error", err)
			continue
		}

		addr, _ := netip.AddrFromSlice(ua.IP)
		res := Response(txid, netip.AddrPortFrom(addr, uint16(ua.Port)))

		if _, err = s.bind.WriteTo(res, ua); err != nil {
			slog.Info("writing back STUN response failed", "error", err)
		}
	}
}

// ListenAndServe starts the STUN server on listenAddr.
func (s *Server) ListenAndServe(listenAddr netip.AddrPort) error {
	if err := s.Listen(listenAddr); err != nil {
		return err
	}
	return s.Serve()
}
