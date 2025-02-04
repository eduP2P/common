package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/edup2p/common/types/control"
	"github.com/edup2p/common/types/control/controlhttp"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/relay"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	//dev        = flag.Bool("dev", false, "run in localhost development mode (overrides -a)")
	addr       = flag.String("a", ":443", "server HTTP/HTTPS listen address, in form \":port\", \"ip:port\", or for IPv6 \"[ip]:port\". If the IP is omitted, it defaults to all interfaces. Serves HTTPS if the port is 443 and/or -certmode is manual, otherwise HTTP.")
	configPath = flag.String("c", "", "config file path")
	//stunPort   = flag.Int("stun-port", stunserver.DefaultPort, "The UDP port on which to serve STUN. The listener is bound to the same IP (if any) as specified in the -a flag.")

	publicFacingBaseString = flag.String("u", "", "public facing base URL (required)")
	publicFacingBase       *url.URL
	password               = flag.String("p", "", "password")

	programLevel = new(slog.LevelVar) // Info by default
)

func main() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: programLevel,
		//AddSource: true,
	})
	slog.SetDefault(slog.New(h))
	programLevel.Set(-8)

	flag.Parse()

	if *publicFacingBaseString == "" {
		slog.Error("publicly facing base URL is required (-u)")
		os.Exit(1)
	} else if *password == "" {
		slog.Error("password is required (-p)")
		os.Exit(1)
	}

	var err error

	if publicFacingBase, err = url.Parse(*publicFacingBaseString); err != nil {
		slog.Error("could not parse public facing base URL", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cserver := LoadServer(ctx)

	log.Printf("control: using public key %s", cserver.cfg.ControlKey.Public().Debug())

	mux := http.NewServeMux()

	mux.Handle("/control", controlhttp.ServerHandler(cserver.server))

	// TODO below is dup from relayserver main.go; dedup in a common library?

	mux.Handle("/", handleStaticHTML(ToverSokControlDefaultHTML))

	mux.Handle("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		browserHeaders(w)
		io.WriteString(w, "User-agent: *\nDisallow: /\n")
	}))
	mux.Handle("/generate_204", http.HandlerFunc(serverCaptivePortalBuster))

	mux.Handle("/auth/land", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := r.URL.Query()["session"]
		if !ok || len(s) != 1 {
			http.Error(w, "session query param is required", http.StatusBadRequest)
			return
		}

		session := s[0]

		sendStaticHTML(fmt.Sprintf(AuthLandingHTML, session), w, r)
	}))
	mux.Handle("/auth/do", http.HandlerFunc(cserver.HandleAuthRequest))
	mux.Handle("/auth/fail", handleStaticHTML(AuthIncorrectHTML))
	mux.Handle("/auth/success", handleStaticHTML(AuthSuccessHTML))

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

	// TODO setup TLS with autocert?

	slog.Info("control: serving", "addr", *addr)
	err = httpsrv.ListenAndServe()

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("control: error %s", err)
	}
}

type ControlServer struct {
	ctx context.Context

	cfgMu sync.Mutex
	cfg   Config

	server *control.Server
}

func (cs *ControlServer) HandleAuthRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/auth/land", 302)
		return
	}

	p := r.FormValue("password")
	s := r.FormValue("session")
	slog.Info("auth request", "url", r.URL.String(), "pass", p, "session", s)

	if p == *password {
		// Success

		if err := cs.server.AcceptAuthentication(control.SessID(s)); err != nil {
			http.Error(w, "Authentication error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/auth/success", 302)
	} else {
		// Fail
		http.Redirect(w, r, "/auth/fail", 302)
	}

}

func (cs *ControlServer) OnSessionCreate(id control.SessID, cid control.ClientID) {
	println("OnSessionCreate")

	if cs.isKnown(key.NodePublic(cid)) {
		if err := cs.server.AcceptAuthentication(id); err != nil {
			slog.Error("error accepting authentication", "id", id, "err", err)
		}

		return
	}

	url, _ := url.Parse(string("/auth/land?session=" + id))
	if err := cs.server.SendAuthURL(id, publicFacingBase.ResolveReference(url).String()); err != nil {
		slog.Error("error sending auth URL", "id", id, "err", err)
	}
}

func (cs *ControlServer) OnSessionResume(id control.SessID, id2 control.ClientID) {
	println("OnSessionResume")
	return // noop
}

func (cs *ControlServer) OnDeviceKey(id control.SessID, key string) {
	println("OnDeviceKey")
	return // noop
}

func (cs *ControlServer) OnSessionFinalize(id control.SessID, id2 control.ClientID) (netip.Prefix, netip.Prefix) {
	println("OnSessionFinalize")

	return cs.getIPs(key.NodePublic(id2))
}

func (cs *ControlServer) OnSessionDestroy(id control.SessID, id2 control.ClientID) {
	println("OnSessionDestroy")
	return // noop
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

func (cs *ControlServer) isKnown(node key.NodePublic) bool {
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
				// TODO find better way to deal with this
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

func sendStaticHTML(doc string, w http.ResponseWriter, r *http.Request) {
	browserHeaders(w)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.WriteHeader(200)

	io.WriteString(w, doc)
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

const AuthLandingHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Authenticate P2P Device</title>
</head>
<body>
<h1>Authenticate P2P Device</h1>

<form action="/auth/do" method="post">
    <label for="pass">Password:</label>
    <input type="password" id="pass" name="password" required />
    <input type="hidden" id="session" name="session" value="%s"/>
    
    <input type="submit" value="Authenticate" />
</form>
</body>

<script>
    (new URL(window.location.href)).searchParams.forEach((x, y) =>
        document.getElementById(y).value = x)
</script>
</html>`

const AuthSuccessHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Successfully authenticated!</title>
</head>
<body>
<h1>
    Successfully authenticated!
</h1>

<h2>
    You can now close this browser tab, and return to your client.
</h2>
</body>
</html>`

const AuthIncorrectHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Incorrect Password</title>
</head>
<body>
<h1>Incorrect Password</h1>

<p>Return to the previous page and try again.</p>
</body>
</html>`

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
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		log.Fatal(err)
	}
	b, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		log.Fatal(err)
	}
}

func newConfig() Config {
	return Config{
		ControlKey: key.NewControlPrivate(),

		//// TODO REPLACE WITH CONFIGURABLE VALUES
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
