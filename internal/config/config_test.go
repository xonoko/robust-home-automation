package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func noEnv() []string { return nil }

func TestDefaultsAreValid(t *testing.T) {
	cfg, err := load("", nil, noEnv)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" || cfg.Telemetry.LogFormat != "json" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.yaml")
	content := "http:\n  addr: \":9000\"\ntelemetry:\n  log_level: warn\nshutdown:\n  drain_delay: 1s\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	environ := func() []string {
		return []string{
			"HUB_TELEMETRY__LOG_LEVEL=debug",
			"HUB_NATS__URLS=nats://a:4222, nats://b:4222",
			"OTHER_VAR=ignored",
		}
	}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse([]string{"--node-id=node-a"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := load(path, fs, environ)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.HTTP.Addr != ":9000" {
		t.Errorf("file should set http.addr, got %q", cfg.HTTP.Addr)
	}
	if cfg.Telemetry.LogLevel != "debug" {
		t.Errorf("env should override file, got %q", cfg.Telemetry.LogLevel)
	}
	if len(cfg.NATS.URLs) != 2 || cfg.NATS.URLs[1] != "nats://b:4222" {
		t.Errorf("env list not split: %v", cfg.NATS.URLs)
	}
	if cfg.Node.ID != "node-a" {
		t.Errorf("flag should set node.id, got %q", cfg.Node.ID)
	}
	if cfg.Shutdown.DrainDelay != time.Second {
		t.Errorf("duration not parsed: %v", cfg.Shutdown.DrainDelay)
	}
}

func TestUnchangedFlagDoesNotOverride(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	environ := func() []string { return []string{"HUB_HTTP__ADDR=:7000"} }

	cfg, err := load("", fs, environ)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Addr != ":7000" {
		t.Errorf("default flag value overrode env: %q", cfg.HTTP.Addr)
	}
}

func TestValidate(t *testing.T) {
	cfg := Default()
	cfg.Node.ID = ""
	cfg.NATS.URLs = nil
	cfg.Telemetry.LogFormat = "xml"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected errors")
	}
}
