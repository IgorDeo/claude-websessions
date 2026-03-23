package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Sessions.ScanInterval != 30*time.Second {
		t.Errorf("expected scan interval 30s, got %v", cfg.Sessions.ScanInterval)
	}
	if cfg.Sessions.OutputBufferSize != 10*1024*1024 {
		t.Errorf("expected buffer 10MB, got %d", cfg.Sessions.OutputBufferSize)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
server:
  port: 9090
  host: 127.0.0.1
sessions:
  scan_interval: 10s
  output_buffer_size: 5MB
  default_dir: /tmp/test
notifications:
  desktop: false
  events: [completed]
auth:
  enabled: true
  token: "secret123"
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Auth.Token != "secret123" {
		t.Errorf("expected token secret123, got %s", cfg.Auth.Token)
	}
	if cfg.Sessions.OutputBufferSize != 5*1024*1024 {
		t.Errorf("expected buffer 5MB, got %d", cfg.Sessions.OutputBufferSize)
	}
}

func TestLoadFromEnvOverride(t *testing.T) {
	t.Setenv("WEBSESSIONS_AUTH_TOKEN", "envtoken")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Token != "envtoken" {
		t.Errorf("expected token envtoken, got %s", cfg.Auth.Token)
	}
}
