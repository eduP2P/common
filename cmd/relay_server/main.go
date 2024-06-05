package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/shadowjonathan/edup2p/types/key"
	"github.com/shadowjonathan/edup2p/types/relay"
	"github.com/shadowjonathan/edup2p/types/relay/relayhttp"
	stunserver "github.com/shadowjonathan/edup2p/types/stun"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	dev        = flag.Bool("dev", false, "run in localhost development mode (overrides -a)")
	addr       = flag.String("a", ":443", "server HTTP/HTTPS listen address, in form \":port\", \"ip:port\", or for IPv6 \"[ip]:port\". If the IP is omitted, it defaults to all interfaces. Serves HTTPS if the port is 443 and/or -certmode is manual, otherwise HTTP.")
	configPath = flag.String("c", "", "config file path")
	stunPort   = flag.Int("stun-port", stunserver.DefaultPort, "The UDP port on which to serve STUN. The listener is bound to the same IP (if any) as specified in the -a flag.")
)

const ToverSokRelayDefaultHTML = `
<html>
	<body>
		<h1>ToverSok Relay</h1>
		<p>
		  This is a toversok-serving relay server.
		</p>
    </body>
</html>
`

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *dev {
		*addr = "127.0.0.1:3340"
		log.Printf("Running in dev mode.")
	}

	// TODO remove. fuck it, we ball
	{
		programLevel := new(slog.LevelVar) // Info by default
		h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
		slog.SetDefault(slog.New(h))
		programLevel.Set(slog.LevelDebug)
	}

	listenHost, _, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("invalid server address: %v", err)
	}

	ap, err := netip.ParseAddrPort(net.JoinHostPort(listenHost, fmt.Sprint(*stunPort)))
	if err != nil {
		log.Fatalf("could not parse stun-combined addrport: %v", err)
	}

	stunServer := stunserver.NewServer(ctx)
	go stunServer.ListenAndServe(ap)

	// TODO add STUN here

	// TODO continue here

	cfg := loadConfig()

	log.Printf("relay: using public key %s", cfg.PrivateKey.Public().Debug())

	server := relay.NewServer(cfg.PrivateKey)

	mux := http.NewServeMux()

	mux.Handle("/relay", relayhttp.ServerHandler(server))

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		browserHeaders(w)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		w.WriteHeader(200)

		io.WriteString(w, ToverSokRelayDefaultHTML)
	}))

	mux.Handle("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		browserHeaders(w)
		io.WriteString(w, "User-agent: *\nDisallow: /\n")
	}))
	mux.Handle("/generate_204", http.HandlerFunc(serverCaptivePortalBuster))

	httpsrv := &http.Server{
		Addr:    *addr,
		Handler: mux,
		// TODO
		//ErrorLog: slog.NewLogLogger(),

		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		httpsrv.Shutdown(ctx)
	}()

	// TODO setup TLS with autocert

	slog.Info("relay: serving", "addr", *addr)
	err = httpsrv.ListenAndServe()

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("relay: error %s", err)
	}
}

func browserHeaders(w http.ResponseWriter) {
	w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; block-all-mixed-content; object-src 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

const (
	noContentChallengeHeader = "X-Captive-Challenge"
	noContentResponseHeader  = "X-Captive-Response"
)

// For captive portal detection
func serverCaptivePortalBuster(w http.ResponseWriter, r *http.Request) {
	if challenge := r.Header.Get(noContentChallengeHeader); challenge != "" {
		badChar := strings.IndexFunc(challenge, func(r rune) bool {
			return !isChallengeChar(r)
		}) != -1
		if len(challenge) <= 64 && !badChar {
			w.Header().Set(noContentResponseHeader, "response "+challenge)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func isChallengeChar(c rune) bool {
	// Semi-randomly chosen as a limited set of valid characters
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') ||
		('0' <= c && c <= '9') ||
		c == '.' || c == '-' || c == '_'
}

type Config struct {
	PrivateKey key.NodePrivate
}

func loadConfig() Config {
	if *dev {
		return newConfig()
	}
	if *configPath == "" {
		if os.Getuid() == 0 {
			*configPath = "/var/lib/toversok/relay.key"
		} else {
			log.Fatalf("relay: -c <config path> not specified")
		}
		log.Printf("no config path specified; using %s", *configPath)
	}
	b, err := os.ReadFile(*configPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return writeNewConfig()
	case err != nil:
		log.Fatal(err)
		panic("unreachable")
	default:
		var cfg Config
		if err := json.Unmarshal(b, &cfg); err != nil {
			log.Fatalf("relay: config: %v", err)
		}
		return cfg
	}
}

func writeNewConfig() Config {
	if err := os.MkdirAll(filepath.Dir(*configPath), 0777); err != nil {
		log.Fatal(err)
	}
	cfg := newConfig()
	b, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*configPath, b, 0600); err != nil {
		log.Fatal(err)
	}
	return cfg
}

func newConfig() Config {
	return Config{PrivateKey: key.NewNode()}
}
