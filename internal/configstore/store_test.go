package configstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
)

func TestStoreRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gateway.sqlite3")
	store, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	if _, err := store.Load(); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("expected ErrUninitialized, got %v", err)
	}

	cfg := testConfig()
	if err := store.Initialize(cfg); err != nil {
		t.Fatalf("Initialize returned an error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("round trip mismatch\nexpected: %#v\nactual: %#v", cfg, loaded)
	}
	if err := store.QuickCheck(); err != nil {
		t.Fatalf("QuickCheck returned an error: %v", err)
	}
}

func TestReplacePreservesStableToolIDWhenPortChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	if err := store.Initialize(cfg); err != nil {
		t.Fatalf("Initialize returned an error: %v", err)
	}

	originalID := toolID(t, store, "EQU-TEST-01")
	cfg.Tools[0].Port = 8083
	if err := store.Replace(cfg); err != nil {
		t.Fatalf("Replace returned an error: %v", err)
	}
	updatedID := toolID(t, store, "EQU-TEST-01")
	if updatedID != originalID {
		t.Fatalf("tool id changed after port update: before=%d after=%d", originalID, updatedID)
	}
}

func TestReplaceDoesNotCollideNewAndPreservedIDs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	if err := store.Initialize(cfg); err != nil {
		t.Fatalf("Initialize returned an error: %v", err)
	}
	secondID := toolID(t, store, "EQU-TEST-02")

	newTool := cfg.Tools[0]
	newTool.InterlockName = "EQU-NEW-01"
	newTool.Port = 8083
	newTool.IP = "new.example.local"
	cfg.Tools = []config.Tool{newTool, cfg.Tools[1]}
	if err := store.Replace(cfg); err != nil {
		t.Fatalf("Replace returned an error: %v", err)
	}
	if got := toolID(t, store, "EQU-TEST-02"); got != secondID {
		t.Fatalf("preserved tool id changed: before=%d after=%d", secondID, got)
	}
}

func TestReplaceRejectsInvalidConfigWithoutChangingDatabase(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	original := testConfig()
	if err := store.Initialize(original); err != nil {
		t.Fatalf("Initialize returned an error: %v", err)
	}

	invalid := testConfig()
	invalid.Tools[1].Port = invalid.Tools[0].Port
	if err := store.Replace(invalid); err == nil {
		t.Fatal("expected invalid duplicate-port config to be rejected")
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if !reflect.DeepEqual(loaded, original) {
		t.Fatalf("database changed after rejected update\nexpected: %#v\nactual: %#v", original, loaded)
	}
}

func TestCompatibilityMirrorIsGeneratedAfterReplace(t *testing.T) {
	tempDir := t.TempDir()
	mirrorPath := filepath.Join(tempDir, "config.yaml")
	originalYAML := []byte("# original human-authored config\n")
	if err := os.WriteFile(mirrorPath, originalYAML, 0640); err != nil {
		t.Fatalf("failed to write original YAML: %v", err)
	}

	store, err := Open(filepath.Join(tempDir, "gateway.sqlite3"), Options{MirrorPath: mirrorPath})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	if err := store.Initialize(cfg); err != nil {
		t.Fatalf("Initialize returned an error: %v", err)
	}
	contents, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("read mirror after Initialize: %v", err)
	}
	if !reflect.DeepEqual(contents, originalYAML) {
		t.Fatalf("Initialize unexpectedly rewrote legacy YAML: %q", contents)
	}

	cfg.Tools[0].IP = "interlock-new.example.local"
	if err := store.Replace(cfg); err != nil {
		t.Fatalf("Replace returned an error: %v", err)
	}

	contents, err = os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("read generated mirror: %v", err)
	}
	if !strings.HasPrefix(string(contents), "# GENERATED FILE. SQLite gateway.sqlite3 is authoritative.") {
		t.Fatalf("missing generated mirror header: %q", contents)
	}
	if !strings.Contains(string(contents), "interlock-new.example.local") {
		t.Fatalf("generated mirror does not contain updated tool: %q", contents)
	}

	backup, err := os.ReadFile(mirrorPath + ".bak")
	if err != nil {
		t.Fatalf("read compatibility backup: %v", err)
	}
	if !reflect.DeepEqual(backup, originalYAML) {
		t.Fatalf("unexpected backup contents: %q", backup)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	if _, err := store.db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	if _, err := Open(path, Options{}); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected newer-schema rejection, got %v", err)
	}
}

func toolID(t *testing.T, store *Store, name string) int64 {
	t.Helper()
	var id int64
	if err := store.db.QueryRow("SELECT id FROM tools WHERE interlock_name = ?", name).Scan(&id); err != nil {
		t.Fatalf("query tool id: %v", err)
	}
	return id
}

func testConfig() config.Config {
	username := "admin"
	password1 := "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
	password2 := "0123456789ABCDEFGHIJKLMNOPQRSTUV"

	return config.Config{
		Bind: "0.0.0.0",
		Defaults: config.Defaults{
			TimeoutMS:        5000,
			SafeStateOnError: "off",
			ShellyTLS: config.ShellyTLSConfig{
				ServerCAFile:   "/etc/fbs-interlock-gateway-cluster/tls/server-ca.crt",
				ClientCertFile: "/etc/fbs-interlock-gateway-cluster/tls/gateway-client.crt",
				ClientKeyFile:  "/etc/fbs-interlock-gateway-cluster/tls/gateway-client.key",
			},
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "interlock-01.example.local",
				Protocol:      "https",
				Port:          8081,
				SwitchID:      0,
				Username:      &username,
				Password:      &password1,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.12",
				Protocol:      "http",
				Port:          8082,
				SwitchID:      0,
				Username:      &username,
				Password:      &password2,
				Enabled:       false,
			},
		},
	}
}

func TestStoreRejectsInvalidPassword(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	password := "not-32-characters"
	cfg.Tools[0].Password = &password

	if err := store.Initialize(cfg); err == nil {
		t.Fatal("expected invalid password to be rejected")
	}
}

func TestStoreRejectsInterlockNameOver16Characters(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	cfg.Tools[0].InterlockName = "EQU-THIS-NAME-IS-TOO-LONG"

	if err := store.Initialize(cfg); err == nil {
		t.Fatal("expected interlock name over 16 characters to be rejected")
	}
}

func TestStoreRejectsDuplicateInterlockName(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	cfg.Tools[1].InterlockName = cfg.Tools[0].InterlockName

	if err := store.Initialize(cfg); err == nil {
		t.Fatal("expected duplicate interlock name to be rejected")
	}
}

func TestStoreRejectsDuplicateIP(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.sqlite3"), Options{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer store.Close()

	cfg := testConfig()
	cfg.Tools[1].IP = cfg.Tools[0].IP

	if err := store.Initialize(cfg); err == nil {
		t.Fatal("expected duplicate IP to be rejected")
	}
}
