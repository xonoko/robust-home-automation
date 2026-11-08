// Package config loads the hub's bootstrap configuration.
//
// Only what a node needs to start lives here (identity, listeners, where NATS
// and Postgres are). Everything users configure at runtime lives in the
// database. Sources, lowest priority first: defaults, an optional YAML file,
// HUB_* environment variables, then command-line flags.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

// EnvPrefix is the prefix of environment variables read by the hub.
// Nesting uses a double underscore: HUB_TELEMETRY__LOG_LEVEL=debug sets telemetry.log_level.
const EnvPrefix = "HUB_"

// Config is the bootstrap configuration of one hub node.
type Config struct {
	Node      Node      `koanf:"node"`
	HTTP      HTTP      `koanf:"http"`
	NATS      NATS      `koanf:"nats"`
	Database  Database  `koanf:"database"`
	Telemetry Telemetry `koanf:"telemetry"`
	Shutdown  Shutdown  `koanf:"shutdown"`
}

// Node identifies this hub instance.
type Node struct {
	// ID is unique per node. In Kubernetes it is the node name.
	ID string `koanf:"id"`
}

// HTTP configures the API, health and metrics listener.
type HTTP struct {
	Addr string `koanf:"addr"`
}

// NATS configures the connection to the NATS cluster.
type NATS struct {
	// URLs lists every server we may connect to. Comma-separated in the environment.
	URLs []string `koanf:"urls"`
	// CredsFile is an optional NATS credentials file.
	CredsFile string `koanf:"creds_file"`
}

// Database configures the connection to Postgres.
type Database struct {
	// URL is a Postgres connection URL. Empty disables the store.
	URL string `koanf:"url"`
}

// Telemetry configures logs, metrics and traces.
type Telemetry struct {
	LogLevel  string `koanf:"log_level"`
	LogFormat string `koanf:"log_format"`
	// OTLPEndpoint is an OTLP/gRPC endpoint URL for traces, e.g. http://otel-collector:4317. Empty disables export.
	OTLPEndpoint string `koanf:"otlp_endpoint"`
}

// Shutdown configures graceful termination.
type Shutdown struct {
	// DrainDelay is how long the node reports "not ready" before it stops serving,
	// so Kubernetes and peers stop sending it new work.
	DrainDelay time.Duration `koanf:"drain_delay"`
	// Timeout bounds the whole shutdown.
	Timeout time.Duration `koanf:"timeout"`
}

// Default returns the configuration used when nothing else is set.
func Default() Config {
	host, _ := os.Hostname()
	return Config{
		Node:      Node{ID: host},
		HTTP:      HTTP{Addr: ":8080"},
		NATS:      NATS{URLs: []string{"nats://127.0.0.1:4222"}},
		Telemetry: Telemetry{LogLevel: "info", LogFormat: "json"},
		Shutdown:  Shutdown{DrainDelay: 2 * time.Second, Timeout: 20 * time.Second},
	}
}

// flagKeys maps command-line flag names to configuration keys.
var flagKeys = map[string]string{
	"node-id":   "node.id",
	"http-addr": "http.addr",
	"log-level": "telemetry.log_level",
	"nats-url":  "nats.urls",
}

// RegisterFlags adds the flags that can override configuration keys.
func RegisterFlags(fs *pflag.FlagSet) {
	d := Default()
	fs.String("node-id", d.Node.ID, "unique ID of this node")
	fs.String("http-addr", d.HTTP.Addr, "listen address for the API, health and metrics")
	fs.String("log-level", d.Telemetry.LogLevel, "log level: debug, info, warn, error")
	fs.StringSlice("nats-url", d.NATS.URLs, "NATS server URLs")
}

// Load builds the configuration from defaults, the optional file at path,
// the environment, then flags (fs may be nil).
func Load(path string, fs *pflag.FlagSet) (Config, error) {
	return load(path, fs, os.Environ)
}

func load(path string, fs *pflag.FlagSet, environ func() []string) (Config, error) {
	k := koanf.New(".")

	if err := k.Load(structs.Provider(Default(), "koanf"), nil); err != nil {
		return Config{}, fmt.Errorf("load defaults: %w", err)
	}

	if path != "" {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return Config{}, fmt.Errorf("load config file %s: %w", path, err)
		}
	}

	envProvider := env.Provider(".", env.Opt{
		Prefix:        EnvPrefix,
		EnvironFunc:   environ,
		TransformFunc: transformEnv,
	})
	if err := k.Load(envProvider, nil); err != nil {
		return Config{}, fmt.Errorf("load environment: %w", err)
	}

	if fs != nil {
		flags := posflag.ProviderWithFlag(fs, ".", k, func(f *pflag.Flag) (string, any) {
			key, ok := flagKeys[f.Name]
			if !ok {
				return "", nil
			}
			return key, posflag.FlagVal(fs, f)
		})
		if err := k.Load(flags, nil); err != nil {
			return Config{}, fmt.Errorf("load flags: %w", err)
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// transformEnv turns HUB_TELEMETRY__LOG_LEVEL into telemetry.log_level and
// splits list values on commas.
func transformEnv(key, value string) (string, any) {
	key = strings.ToLower(strings.TrimPrefix(key, EnvPrefix))
	key = strings.ReplaceAll(key, "__", ".")
	if key == "nats.urls" {
		var urls []string
		for _, u := range strings.Split(value, ",") {
			if u = strings.TrimSpace(u); u != "" {
				urls = append(urls, u)
			}
		}
		return key, urls
	}
	return key, value
}

// Validate reports every invalid setting at once.
func (c Config) Validate() error {
	var errs []error
	if strings.TrimSpace(c.Node.ID) == "" {
		errs = append(errs, errors.New("node.id is required"))
	}
	if c.HTTP.Addr == "" {
		errs = append(errs, errors.New("http.addr is required"))
	}
	if len(c.NATS.URLs) == 0 {
		errs = append(errs, errors.New("nats.urls needs at least one URL"))
	}
	if _, err := ParseLogLevel(c.Telemetry.LogLevel); err != nil {
		errs = append(errs, err)
	}
	if f := c.Telemetry.LogFormat; f != "json" && f != "text" {
		errs = append(errs, fmt.Errorf("telemetry.log_format must be json or text, got %q", f))
	}
	if c.Shutdown.DrainDelay < 0 || c.Shutdown.Timeout <= c.Shutdown.DrainDelay {
		errs = append(errs, errors.New("shutdown.timeout must be greater than shutdown.drain_delay"))
	}
	return errors.Join(errs...)
}

// ParseLogLevel converts a level name to a slog level.
func ParseLogLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("telemetry.log_level: %w", err)
	}
	return l, nil
}
