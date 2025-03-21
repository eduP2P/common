package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/edup2p/common/types/control"
	"github.com/edup2p/common/types/control/controlhttp"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/relay"
)

var (
	addr       = flag.String("a", ":443", "server HTTP/HTTPS listen address, in form \":port\", \"ip:port\", or for IPv6 \"[ip]:port\". If the IP is omitted, it defaults to all interfaces. Serves HTTPS if the port is 443 and/or -certmode is manual, otherwise HTTP.")
	configPath = flag.String("c", "", "config file path")

	programLevel = new(slog.LevelVar) // Info by default
)

func main() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: programLevel,
	})
	slog.SetDefault(slog.New(h))
	programLevel.Set(-8)

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cserver := LoadServer(ctx)

	log.Printf("control: using public key %s", cserver.cfg.ControlKey.Public().Debug())

	mux := http.NewServeMux()

	mux.Handle("/control", controlhttp.ServerHandler(cserver.server))

	mux.Handle("/", handleStaticHTML(ToverSokControlDefaultHTML))

	mux.Handle("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		browserHeaders(w)
		if _, err := io.WriteString(w, "User-agent: *\nDisallow: /\n"); err != nil {
			slog.Error("could not write robots.txt", "err", err)
		}
	}))
	mux.Handle("/generate_204", http.HandlerFunc(serverCaptivePortalBuster))

	httpsrv := &http.Server{
		Addr:    *addr,
		Handler: mux,

		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		if err := httpsrv.Shutdown(ctx); err != nil {
			slog.Error("control: failed to shutdown control server", "error", err)
		}
	}()

	slog.Info("control: serving", "addr", *addr)
	err := httpsrv.ListenAndServe()

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("control: error %s", err) //nolint:gocritic
	}
}

type ControlServer struct {
	ctx context.Context

	cfgMu sync.Mutex
	cfg   Config

	server *control.Server
}

func (cs *ControlServer) OnSessionCreate(id control.SessID, cid control.ClientID) {
	slog.Info("OnSessionCreate", "id", id, "cid", cid)

	go func() {
		if err := cs.server.AcceptAuthentication(id); err != nil {
			slog.Error("error accepting authentication", "id", id, "err", err)
		}
	}()
}

func (cs *ControlServer) OnSessionResume(sess control.SessID, cid control.ClientID) {
	slog.Info("OnSessionResume", "sess", sess, "cid", cid)
}

func (cs *ControlServer) OnDeviceKey(sess control.SessID, deviceKey string) {
	slog.Info("OnDeviceKey", "sess", sess, "deviceKey", deviceKey)
}

func (cs *ControlServer) OnSessionFinalize(sess control.SessID, cid control.ClientID) (netip.Prefix, netip.Prefix, time.Time) {
	slog.Info("OnSessionFinalize", "sess", sess, "cid", cid)

	ip4, ip6 := cs.getIPs(key.NodePublic(cid))

	return ip4, ip6, time.Time{}
}

func (cs *ControlServer) OnSessionDestroy(sess control.SessID, cid control.ClientID) {
	slog.Info("OnSessionDestroy", "sess", sess, "cid", cid)
}

func LoadServer(ctx context.Context) *ControlServer {
	cfg := loadConfig()

	s := &ControlServer{
		ctx: ctx,
		cfg: cfg,
	}

	println("creating new server")
	s.server = control.NewServer(cfg.ControlKey, cfg.Relays)
	println("created new server")

	s.server.RegisterCallbacks(s)
	println("loaded callbacks")

	s.loadExistingNodes()
	println("loaded nodes")

	return s
}

func (cs *ControlServer) loadExistingNodes() {
	var clients []control.ClientID

	for node := range cs.cfg.IPMapping {
		clients = append(clients, control.ClientID(node))
	}

	// scuffed full graph of all known nodes
	for _, client := range clients {
		for _, client2 := range clients {
			if client == client2 {
				continue
			}

			if err := cs.server.UpsertVisibilityPair(client, client2, control.VisibilityPair{}); err != nil {
				panic(err)
			}
		}
	}
}

func (cs *ControlServer) addNewNode(node key.NodePublic) {
	for node2 := range cs.cfg.IPMapping {
		if node == node2 {
			continue
		}

		if err := cs.server.UpsertVisibilityPair(control.ClientID(node), control.ClientID(node2), control.VisibilityPair{}); err != nil {
			panic(err)
		}
	}
}

func (cs *ControlServer) isKnown(node key.NodePublic) bool { //nolint:unused
	cs.cfgMu.Lock()
	defer cs.cfgMu.Unlock()

	_, ok := cs.cfg.IPMapping[node]

	return ok
}

func (cs *ControlServer) getIPs(node key.NodePublic) (netip.Prefix, netip.Prefix) {
	cs.cfgMu.Lock()
	defer cs.cfgMu.Unlock()

	mapping, ok := cs.cfg.IPMapping[node]

	if !ok {
		ip4 := cs.findNewIP4()
		ip6 := cs.findNewIP6()

		mapping = IPMapping{
			IP4: ip4,
			IP6: ip6,
		}

		cs.cfg.IPMapping[node] = mapping

		writeConfig(cs.cfg, *configPath)

		cs.addNewNode(node)
	}

	return netip.PrefixFrom(mapping.IP4, cs.cfg.IP4.Bits()), netip.PrefixFrom(mapping.IP6, cs.cfg.IP6.Bits())
}

func findNewIP(ipp netip.Prefix, used func(netip.Addr) bool) (netip.Prefix, netip.Addr) {
	backwards := false

	for {
		var addr netip.Addr

		if backwards {
			addr = ipp.Addr().Prev()
		} else {
			addr = ipp.Addr().Next()
		}

		if !ipp.Contains(addr) {
			if !backwards {
				// we exceeded the boundary, try a back-sweep
				backwards = true
			} else {
				panic("address space exhausted")
			}
		}

		ipp = netip.PrefixFrom(addr, ipp.Bits())

		if used(addr) {
			continue
		}

		return ipp, addr
	}
}

// Find an unused ip4 address, and advance the counter
func (cs *ControlServer) findNewIP4() (addr netip.Addr) {
	cs.cfg.IP4, addr = findNewIP(cs.cfg.IP4, cs.ip4Used)
	return
}

// Find an unused ip6 address, and advance the counter
func (cs *ControlServer) findNewIP6() (addr netip.Addr) {
	cs.cfg.IP6, addr = findNewIP(cs.cfg.IP6, cs.ip6Used)
	return
}

func (cs *ControlServer) ip4Used(a netip.Addr) bool {
	for _, mapping := range cs.cfg.IPMapping {
		if mapping.IP4 == a {
			return true
		}
	}

	return false
}

func (cs *ControlServer) ip6Used(a netip.Addr) bool {
	for _, mapping := range cs.cfg.IPMapping {
		if mapping.IP6 == a {
			return true
		}
	}

	return false
}

func handleStaticHTML(doc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sendStaticHTML(doc, w, r)
	}
}

func sendStaticHTML(doc string, w http.ResponseWriter, _ *http.Request) {
	browserHeaders(w)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(w, doc); err != nil {
		slog.Error("failed to write static HTML page", "error", err)
	}
}

const ToverSokControlDefaultHTML = `
<html>
	<body>
		<h1>ToverSok Control</h1>
		<p>
		  This is a toversok-serving control server.
		</p>
    </body>
</html>
`

func browserHeaders(w http.ResponseWriter) {
	w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; block-all-mixed-content; object-src 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

type Config struct {
	ControlKey key.ControlPrivate

	// current IP counter with mask
	IP4 netip.Prefix
	IP6 netip.Prefix

	IPMapping map[key.NodePublic]IPMapping

	Relays []relay.Information
}

type IPMapping struct {
	IP4 netip.Addr
	IP6 netip.Addr
}

func loadConfig() Config {
	if *configPath == "" {
		if os.Getuid() == 0 {
			*configPath = "/var/lib/toversok/control.json"
		} else {
			log.Fatalf("control: -c <config path> not specified")
		}
		log.Printf("no config path specified; using %s", *configPath)
	}
	b, err := os.ReadFile(*configPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return writeNewConfig()
	case err != nil:
		log.Fatal(err)
		//goland:noinspection GoUnreachableCode
		panic("unreachable")
	default:
		var cfg Config
		if err := json.Unmarshal(b, &cfg); err != nil {
			log.Fatalf("control: config: %v", err)
		}
		return cfg
	}
}

func writeNewConfig() Config {
	cfg := newConfig()

	writeConfig(cfg, *configPath)

	return cfg
}

func writeConfig(cfg Config, path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		log.Fatal(err)
	}
	b, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		log.Fatal(err)
	}
}

func newConfig() Config {
	return Config{
		ControlKey: key.NewControlPrivate(),

		IP4: netip.MustParsePrefix("10.42.0.0/16"),
		IP6: netip.MustParsePrefix("fd42:dead:beef::/64"),

		IPMapping: make(map[key.NodePublic]IPMapping),

		Relays: make([]relay.Information, 0),
	}
}

func isChallengeChar(c rune) bool {
	// Semi-randomly chosen as a limited set of valid characters
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') ||
		('0' <= c && c <= '9') ||
		c == '.' || c == '-' || c == '_'
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
