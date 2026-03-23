package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Sessions      SessionsConfig      `yaml:"sessions"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Auth          AuthConfig          `yaml:"auth"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type SessionsConfig struct {
	ScanInterval     time.Duration `yaml:"-"`
	ScanIntervalRaw  string        `yaml:"scan_interval"`
	OutputBufferSize int64         `yaml:"-"`
	OutputBufferRaw  string        `yaml:"output_buffer_size"`
	DefaultDir       string        `yaml:"default_dir"`
}

type NotificationsConfig struct {
	Desktop bool     `yaml:"desktop"`
	Events  []string `yaml:"events"`
}

type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Sessions: SessionsConfig{
			ScanInterval: 30 * time.Second, ScanIntervalRaw: "30s",
			OutputBufferSize: 10 * 1024 * 1024, OutputBufferRaw: "10MB",
			DefaultDir: "~/projects",
		},
		Notifications: NotificationsConfig{Desktop: true, Events: []string{"completed", "errored", "waiting"}},
		Auth:          AuthConfig{Enabled: false, Token: ""},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
		if err := cfg.parseRawFields(); err != nil {
			return nil, err
		}
	}
	cfg.applyEnvOverrides()
	return cfg, nil
}

func (c *Config) parseRawFields() error {
	if c.Sessions.ScanIntervalRaw != "" {
		d, err := time.ParseDuration(c.Sessions.ScanIntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing scan_interval: %w", err)
		}
		c.Sessions.ScanInterval = d
	}
	if c.Sessions.OutputBufferRaw != "" {
		size, err := parseByteSize(c.Sessions.OutputBufferRaw)
		if err != nil {
			return fmt.Errorf("parsing output_buffer_size: %w", err)
		}
		c.Sessions.OutputBufferSize = size
	}
	return nil
}

func (c *Config) applyEnvOverrides() {
	if token := os.Getenv("WEBSESSIONS_AUTH_TOKEN"); token != "" {
		c.Auth.Token = token
	}
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return n * multiplier, nil
}
