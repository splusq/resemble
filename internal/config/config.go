package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type RouteConfig struct {
	Match string        `yaml:"match"`
	TTL   time.Duration `yaml:"ttl"`
}

func (r *RouteConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Match string `yaml:"match"`
		TTL   string `yaml:"ttl"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	r.Match = raw.Match
	if raw.TTL != "" {
		d, err := time.ParseDuration(raw.TTL)
		if err != nil {
			return fmt.Errorf("invalid ttl %q: %w", raw.TTL, err)
		}
		r.TTL = d
	}
	return nil
}

type UpstreamConfig struct {
	Host   string        `yaml:"host"`
	Scheme string        `yaml:"scheme"`
	Routes []RouteConfig `yaml:"routes"`
}

type Defaults struct {
	TTL           time.Duration `yaml:"-"`
	RawTTL        string        `yaml:"ttl"`
	Mode          string        `yaml:"mode"`
	IgnoreHeaders []string      `yaml:"ignore_headers"`
	IgnoreQuery   []string      `yaml:"ignore_query"`
	IgnoreBody    bool          `yaml:"ignore_body"`
	StrictDrift   bool          `yaml:"strict_drift"`
}

type Config struct {
	CacheRepo string           `yaml:"cache_repo"`
	Listen    string           `yaml:"listen"`
	Upstreams []UpstreamConfig `yaml:"upstreams"`
	Defaults  Defaults         `yaml:"defaults"`

	// Resolved at runtime (from flags/env, not YAML)
	Mode     string `yaml:"-"`
	Upstream string `yaml:"-"`
	Port     int    `yaml:"-"`
	CacheDir string `yaml:"-"`
}

var DefaultConfig = Config{
	Listen: ":9999",
	Defaults: Defaults{
		TTL:  24 * time.Hour,
		Mode: "auto",
		IgnoreHeaders: []string{
			"Authorization",
			"X-Request-Id",
			"Date",
			"User-Agent",
		},
		IgnoreQuery: []string{},
		StrictDrift: false,
	},
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig

	// Layer 1: Load from config file (if it exists)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	// Layer 2: Apply RESEMBLE_CONFIG env var (overrides file)
	if envCfg := os.Getenv("RESEMBLE_CONFIG"); envCfg != "" {
		if err := yaml.Unmarshal([]byte(envCfg), &cfg); err != nil {
			return nil, fmt.Errorf("parsing RESEMBLE_CONFIG env var: %w", err)
		}
	}

	// Parse TTL duration from string
	if cfg.Defaults.RawTTL != "" {
		d, err := time.ParseDuration(cfg.Defaults.RawTTL)
		if err != nil {
			return nil, fmt.Errorf("invalid defaults.ttl %q: %w", cfg.Defaults.RawTTL, err)
		}
		cfg.Defaults.TTL = d
	}

	// Layer 3: Apply individual RESEMBLE_* env var overrides
	if v := os.Getenv("RESEMBLE_CACHE_REPO"); v != "" {
		cfg.CacheRepo = v
	}
	if v := os.Getenv("RESEMBLE_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("RESEMBLE_MODE"); v != "" {
		cfg.Defaults.Mode = v
	}

	return &cfg, nil
}

func LoadDefault() (*Config, error) {
	return Load(".testproxy.yml")
}
