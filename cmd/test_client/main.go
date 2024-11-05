package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/edup2p/common/toversok"
	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/dial"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/usrwg"
	"golang.zx2c4.com/wireguard/wgctrl"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// Flags
var (
	extWgDevice string
	configFile  string
	logLevel    string
	extPort     int

	controlHost   string
	controlPort   int
	controlKeyStr string
)

func init() {
	flag.StringVar(&extWgDevice, "ext-wg-device", "", "external wireguard device to use")
	flag.StringVar(&configFile, "config", "./test_client.json", "path to config file")
	flag.StringVar(&logLevel, "log-level", "", "log level")
	flag.IntVar(&extPort, "ext-port", 0, "external port to use")

	flag.StringVar(&controlHost, "control-host", "", "control host to use")
	flag.IntVar(&controlPort, "control-port", 0, "control port to use")
	flag.StringVar(&controlKeyStr, "control-key", "", "control key to use")
}

var (
	programLevel = new(slog.LevelVar) // Info by default

	engineExtPort uint16
	controlPort16 uint16

	parsedControlKey *key.ControlPublic

	normalisedControlFile string

	config *Config
)

type Config struct {
	PrivateKey key.NodePrivate

	ControlHost string
	ControlPort uint16
	ControlKey  key.ControlPublic
}

func main() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel, AddSource: true})
	slog.SetDefault(slog.New(h))
	programLevel.Set(slog.LevelInfo)

	flag.Parse()

	var level = slog.LevelInfo

	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "":
		break
	default:
		slog.Warn("could not recognise flag --log-level, will use log level info", "unrecognised-argument", logLevel)
	}

	programLevel.Set(level)

	if extPort < 0 || extPort > 65535 {
		slog.Error("external port out of range 0-65535, aborting", "ext-port", extPort)
		os.Exit(1)
	} else {
		engineExtPort = uint16(extPort)
	}

	if controlPort < 0 || controlPort > 65535 {
		slog.Error("control port out of range 0-65535, aborting", "control-port", controlPort)
		os.Exit(1)
	} else {
		controlPort16 = uint16(controlPort)
	}

	var err error

	if parsedControlKey, err = parseControlKey(controlKeyStr); err != nil {
		slog.Error("could not parse control key string", "err", err)
		os.Exit(1)
	}

	if normalisedControlFile, err = normalisePath(configFile); err != nil {
		slog.Error("could not normalise config file", "err", err, "file", configFile)
		os.Exit(1)
	}

	if config, err = getOrGenerateConfig(normalisedControlFile); err != nil {
		slog.Error("could not get or generate config file", "err", err)
		os.Exit(1)
	}

	if controlHost != "" || controlPort != 0 || controlKeyStr != "" {
		var mustWrite = false

		if config.ControlPort != controlPort16 {
			slog.Warn("config control port and given control port disagree, overwriting config", "config", config.ControlPort, "cli-given", controlPort16)
			config.ControlPort = controlPort16
			mustWrite = true
		}

		if config.ControlHost != controlHost {
			slog.Warn("config control host and given control host disagree, overwriting config", "config", config.ControlHost, "cli-given", controlHost)
			config.ControlHost = controlHost
			mustWrite = true
		}

		if config.ControlKey != *parsedControlKey {
			slog.Warn("config control key and given control key disagree, overwriting config", "config", config.ControlKey.Debug(), "cli-given", parsedControlKey.Debug())
			config.ControlKey = *parsedControlKey
			mustWrite = true
		}

		if mustWrite {
			slog.Info("writing config to file again, since flag values have overwritten some parts of it")
			if err = writeConfig(config, normalisedControlFile); err != nil {
				slog.Error("could not write config file", "err", err)
				os.Exit(1)
			}
		}
	}

	var wg toversok.WireGuardHost

	if wg, err = getWireguardHost(); err != nil {
		slog.Error("could not get/create wireguard host", "err", err)
		os.Exit(1)
	}

	fw := &StokFirewall{}

	ctrl := &toversok.DefaultControlHost{
		Key: config.ControlKey,

		Opts: dial.Opts{
			Domain: config.ControlHost,
			Port:   config.ControlPort,
			TLS:    false,
		},
	}

	ctx, ccc := context.WithCancelCause(context.Background())

	engine, err := toversok.NewEngine(ctx, wg, fw, ctrl, engineExtPort, config.PrivateKey)
	if err != nil {
		slog.Error("could not create engine", "err", err)
		os.Exit(1)
	}

	if err = engine.Start(); err != nil {
		slog.Error("could not start engine", "err", err)
		os.Exit(1)
	}

	cancelChan := make(chan os.Signal, 1)
	// catch SIGETRM or SIGINTERRUPT
	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)

	var interrupted bool

	go func() {
		<-cancelChan

		interrupted = true

		slog.Info("SIGTERM/INT, exiting")

		ccc(errors.New("interrupted"))
	}()

	<-engine.Context().Done()

	if !interrupted {
		slog.Warn("engine exited with error", "err", engine.Context().Err())
		os.Exit(1)
	}
}

func parseControlKey(str string) (*key.ControlPublic, error) {
	if controlKeyStr == "" {
		return nil, nil
	}

	if p, err := key.UnmarshalControlPublic(str); err != nil {

		return nil, fmt.Errorf("could not parse control key: %w", err)
	} else {
		return p, nil
	}
}

func normalisePath(file string) (string, error) {
	var err error

	file = strings.TrimSpace(file)

	// I hate that golang is like this
	if strings.HasPrefix(file, "~/") {
		dirname, err := os.UserHomeDir()
		if err != nil {
			// at this point, just give up
			panic(err)
		}

		file = filepath.Join(dirname, file[2:])
	}

	if file, err = filepath.Abs(file); err != nil {
		return "", fmt.Errorf("failed to normalise path: %w", err)
	}

	return file, nil
}

func getOrGenerateConfig(file string) (*Config, error) {
	var err error
	var c *Config

	data, err := os.ReadFile(file)

	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("config file does not exist, generating new config...", "file", file)

			if controlHost == "" || parsedControlKey == nil || controlPort == 0 {
				return nil, errors.New("cannot generate new config; missing control server parameters")
			}

			c = &Config{
				PrivateKey: key.NewNode(),

				ControlHost: controlHost,
				ControlPort: controlPort16,
				ControlKey:  *parsedControlKey,
			}

			slog.Info("config generated, writing to file...")

			if err = writeConfig(c, file); err != nil {
				return nil, fmt.Errorf("failed to write config to file: %w", err)
			}

			return c, nil
		}

		return nil, fmt.Errorf("cannot read config file %s: %w", file, err)
	}

	if err = json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config key: %w", err)
	}

	slog.Info("loaded config from file", "file", file)

	return c, nil
}

func writeConfig(c *Config, file string) error {
	jsonData, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(file, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write config to file: %w", err)
	}

	return nil
}

func getWireguardHost() (toversok.WireGuardHost, error) {
	if extWgDevice != "" {
		if wg, err := getWgControl(extWgDevice); err != nil {
			return nil, fmt.Errorf("could not initialise external wireguard device: %w", err)
		} else {
			return wg, nil
		}
	} else {
		return usrwg.NewUsrWGHost(), nil
	}
}

func getWgControl(device string) (*types.WGCtrl, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("could not initialise wgctrl: %w", err)
	}

	if _, err := client.Device(device); err != nil {
		return nil, fmt.Errorf("could not find/initialise wgctrl device: %w", err)
	}

	return types.NewWGCtrl(client, device), nil
}

// A dummy firewall
type StokFirewall struct{}

func (s *StokFirewall) Reset() error {
	slog.Info("StokFirewall Reset called")

	return nil
}

func (s *StokFirewall) Controller() (toversok.FirewallController, error) {
	slog.Info("StokFirewall Controller called")

	return s, nil
}

func (s *StokFirewall) QuarantineNodes(ips []netip.Addr) error {
	slog.Info("StokFirewall QuarantineNodes called", "ips", ips)

	return nil
}

func (s *StokFirewall) LocalAddresses() ([]netip.Addr, error) {
	slog.Info("StokFirewall LocalAddresses called")

	return []netip.Addr{}, nil
}
