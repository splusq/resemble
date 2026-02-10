package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir, _ := os.MkdirTemp("", "resemble-config-*")
	defer os.RemoveAll(dir)

	configContent := `
cache_repo: git@github.com:myorg/cache.git
listen: ":8888"

upstreams:
  - host: api.example.com
    scheme: https
    routes:
      - match: "/v1/*"
        ttl: 12h

defaults:
  ttl: 48h
  mode: record
  ignore_headers:
    - Authorization
  ignore_query:
    - nonce
  strict_drift: true
`
	configPath := filepath.Join(dir, ".testproxy.yml")
	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.CacheRepo != "git@github.com:myorg/cache.git" {
		t.Errorf("cache_repo = %q", cfg.CacheRepo)
	}
	if cfg.Listen != ":8888" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if len(cfg.Upstreams) != 1 {
		t.Fatalf("upstreams count = %d, want 1", len(cfg.Upstreams))
	}
	if cfg.Upstreams[0].Host != "api.example.com" {
		t.Errorf("upstream host = %q", cfg.Upstreams[0].Host)
	}
	if cfg.Upstreams[0].Routes[0].TTL != 12*time.Hour {
		t.Errorf("route ttl = %v", cfg.Upstreams[0].Routes[0].TTL)
	}
	if cfg.Defaults.TTL != 48*time.Hour {
		t.Errorf("default ttl = %v", cfg.Defaults.TTL)
	}
	if cfg.Defaults.Mode != "record" {
		t.Errorf("default mode = %q", cfg.Defaults.Mode)
	}
	if len(cfg.Defaults.IgnoreHeaders) != 1 || cfg.Defaults.IgnoreHeaders[0] != "Authorization" {
		t.Errorf("ignore_headers = %v", cfg.Defaults.IgnoreHeaders)
	}
	if len(cfg.Defaults.IgnoreQuery) != 1 || cfg.Defaults.IgnoreQuery[0] != "nonce" {
		t.Errorf("ignore_query = %v", cfg.Defaults.IgnoreQuery)
	}
	if !cfg.Defaults.StrictDrift {
		t.Error("strict_drift should be true")
	}
}

func TestLoad_MissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/.testproxy.yml")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Listen != ":9999" {
		t.Errorf("default listen = %q, want :9999", cfg.Listen)
	}
	if cfg.Defaults.TTL != 24*time.Hour {
		t.Errorf("default ttl = %v, want 24h", cfg.Defaults.TTL)
	}
	if cfg.Defaults.Mode != "auto" {
		t.Errorf("default mode = %q, want auto", cfg.Defaults.Mode)
	}
	if len(cfg.Defaults.IgnoreHeaders) != 4 {
		t.Errorf("default ignore_headers = %d, want 4", len(cfg.Defaults.IgnoreHeaders))
	}
}

func TestLoad_DefaultsAppliedForMissingFields(t *testing.T) {
	dir, _ := os.MkdirTemp("", "resemble-config-*")
	defer os.RemoveAll(dir)

	configPath := filepath.Join(dir, ".testproxy.yml")
	os.WriteFile(configPath, []byte("cache_repo: test\n"), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.CacheRepo != "test" {
		t.Errorf("cache_repo = %q", cfg.CacheRepo)
	}
	// Defaults should still apply
	if cfg.Defaults.Mode != "auto" {
		t.Errorf("mode should default to auto, got %q", cfg.Defaults.Mode)
	}
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	dir, _ := os.MkdirTemp("", "resemble-config-*")
	defer os.RemoveAll(dir)

	configPath := filepath.Join(dir, ".testproxy.yml")
	os.WriteFile(configPath, []byte("cache_repo: original\nlisten: \":9999\"\n"), 0644)

	t.Setenv("RESEMBLE_CACHE_REPO", "from-env")
	t.Setenv("RESEMBLE_LISTEN", ":7777")
	t.Setenv("RESEMBLE_MODE", "replay")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.CacheRepo != "from-env" {
		t.Errorf("cache_repo = %q, want from-env", cfg.CacheRepo)
	}
	if cfg.Listen != ":7777" {
		t.Errorf("listen = %q, want :7777", cfg.Listen)
	}
	if cfg.Defaults.Mode != "replay" {
		t.Errorf("mode = %q, want replay", cfg.Defaults.Mode)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir, _ := os.MkdirTemp("", "resemble-config-*")
	defer os.RemoveAll(dir)

	configPath := filepath.Join(dir, ".testproxy.yml")
	os.WriteFile(configPath, []byte("listen:\n  - invalid\n  nested: bad"), 0644)

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_InvalidTTL(t *testing.T) {
	dir, _ := os.MkdirTemp("", "resemble-config-*")
	defer os.RemoveAll(dir)

	configPath := filepath.Join(dir, ".testproxy.yml")
	os.WriteFile(configPath, []byte("defaults:\n  ttl: not-a-duration\n"), 0644)

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid TTL")
	}
}
