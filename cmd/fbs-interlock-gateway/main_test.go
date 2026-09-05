package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/configstore"
)

func TestLoadOrMigrateImportsLegacyYAMLOnce(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gateway.sqlite3")
	configPath := filepath.Join(dir, "config.yaml")

	legacy := `bind: 127.0.0.1

defaults:
  timeout_ms: 5000
  safe_state_on_error: "off"
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"

tools:
  - interlock_name: "EQU-TEST-01"
    ip: "192.0.2.10"
    protocol: "http"
    port: 8081
    switch_id: 0
    username: "admin"
    password: "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
    enabled: true
`
	if err := os.WriteFile(configPath, []byte(legacy), 0640); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	store, err := configstore.Open(dbPath, configstore.Options{MirrorPath: configPath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	cfg, migrated, err := loadOrMigrate(store, configPath)
	if err != nil {
		t.Fatalf("loadOrMigrate: %v", err)
	}
	if !migrated {
		t.Fatal("expected first startup to migrate legacy YAML")
	}
	if cfg.Bind != "127.0.0.1" {
		t.Fatalf("unexpected bind after migration: %q", cfg.Bind)
	}
	if len(cfg.Tools) != 1 || cfg.Tools[0].InterlockName != "EQU-TEST-01" {
		t.Fatalf("unexpected tools after migration: %#v", cfg.Tools)
	}
	if !filepath.IsAbs(cfg.Defaults.ShellyTLS.ServerCAFile) {
		t.Fatalf("expected relative TLS path to be normalized before import: %q", cfg.Defaults.ShellyTLS.ServerCAFile)
	}

	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read legacy config: %v", err)
	}
	if string(contents) != legacy {
		t.Fatal("first migration rewrote the human-authored legacy YAML")
	}

	// Once SQLite is initialized, YAML changes are intentionally ignored.
	mutated := strings.Replace(legacy, "127.0.0.1", "0.0.0.0", 1)
	if err := os.WriteFile(configPath, []byte(mutated), 0640); err != nil {
		t.Fatalf("mutate legacy config: %v", err)
	}

	cfg, migrated, err = loadOrMigrate(store, configPath)
	if err != nil {
		t.Fatalf("second loadOrMigrate: %v", err)
	}
	if migrated {
		t.Fatal("unexpected second migration")
	}
	if cfg.Bind != "127.0.0.1" {
		t.Fatalf("SQLite was not authoritative after migration; bind=%q", cfg.Bind)
	}
}
