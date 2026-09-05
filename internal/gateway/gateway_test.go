package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
)

func testConfig(safeState string) config.Config {
	return config.Config{
		Bind: "127.0.0.1",
		Defaults: config.Defaults{
			TimeoutMS:        3000,
			SafeStateOnError: safeState,
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	gateway := New(config.Config{}, "/tmp/config.yaml", "")

	snapshot := gateway.ConfigSnapshot()

	if snapshot.Bind != "0.0.0.0" {
		t.Fatalf(
			"expected default bind 0.0.0.0, got %q",
			snapshot.Bind,
		)
	}

	if snapshot.Defaults.TimeoutMS != 3000 {
		t.Fatalf(
			"expected default timeout 3000, got %d",
			snapshot.Defaults.TimeoutMS,
		)
	}

	if snapshot.Defaults.SafeStateOnError != "off" {
		t.Fatalf(
			"expected default safe state off, got %q",
			snapshot.Defaults.SafeStateOnError,
		)
	}

	if gateway.SafeOutput() {
		t.Fatal("expected default safe output to be false")
	}

	if gateway.shelly == nil {
		t.Fatal("expected Shelly client to be initialized")
	}
}

func TestNewSetsSafeOutputOn(t *testing.T) {
	cfg := testConfig("on")

	gateway := New(cfg, "/tmp/config.yaml", "")

	if !gateway.SafeOutput() {
		t.Fatal("expected safe output to be true")
	}
}

func TestConfigSnapshotReturnsCurrentConfig(t *testing.T) {
	cfg := testConfig("off")

	gateway := New(cfg, "/tmp/config.yaml", "")

	snapshot := gateway.ConfigSnapshot()

	if !reflect.DeepEqual(snapshot, cfg) {
		t.Fatalf(
			"unexpected config snapshot\nexpected: %#v\nactual:   %#v",
			cfg,
			snapshot,
		)
	}
}

func TestUpdateConfigWritesFileAndUpdatesState(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	originalContents := []byte("original configuration\n")

	if err := os.WriteFile(configPath, originalContents, 0640); err != nil {
		t.Fatalf("failed to create original config: %v", err)
	}

	gateway := New(
		testConfig("off"),
		configPath,
		"",
	)

	newCfg := config.Config{
		Defaults: config.Defaults{
			SafeStateOnError: "on",
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.20",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	if err := gateway.UpdateConfig(newCfg); err != nil {
		t.Fatalf("UpdateConfig returned an error: %v", err)
	}

	snapshot := gateway.ConfigSnapshot()

	if snapshot.Bind != "0.0.0.0" {
		t.Fatalf(
			"expected default bind 0.0.0.0, got %q",
			snapshot.Bind,
		)
	}

	if snapshot.Defaults.TimeoutMS != 3000 {
		t.Fatalf(
			"expected default timeout 3000, got %d",
			snapshot.Defaults.TimeoutMS,
		)
	}

	if snapshot.Defaults.SafeStateOnError != "on" {
		t.Fatalf(
			"expected safe state on, got %q",
			snapshot.Defaults.SafeStateOnError,
		)
	}

	if !gateway.SafeOutput() {
		t.Fatal("expected safe output to be true after update")
	}

	savedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if !reflect.DeepEqual(savedCfg, snapshot) {
		t.Fatalf(
			"saved config does not match gateway state\nsaved:    %#v\ngateway:  %#v",
			savedCfg,
			snapshot,
		)
	}

	backupContents, err := os.ReadFile(configPath + ".bak")
	if err != nil {
		t.Fatalf("failed to read config backup: %v", err)
	}

	if !reflect.DeepEqual(backupContents, originalContents) {
		t.Fatalf(
			"unexpected backup contents\nexpected: %q\nactual:   %q",
			originalContents,
			backupContents,
		)
	}
}

func TestUpdateConfigRejectsInvalidConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	originalContents := []byte("original configuration\n")

	if err := os.WriteFile(configPath, originalContents, 0640); err != nil {
		t.Fatalf("failed to create original config: %v", err)
	}

	originalCfg := testConfig("off")
	gateway := New(originalCfg, configPath, "")

	invalidCfg := config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.11",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	err := gateway.UpdateConfig(invalidCfg)
	if err == nil {
		t.Fatal("expected invalid config to be rejected")
	}

	if !strings.Contains(err.Error(), "duplicate port") {
		t.Fatalf(
			"expected duplicate port error, got %q",
			err,
		)
	}

	snapshot := gateway.ConfigSnapshot()

	if !reflect.DeepEqual(snapshot, originalCfg) {
		t.Fatalf(
			"gateway config changed after rejected update\nexpected: %#v\nactual:   %#v",
			originalCfg,
			snapshot,
		)
	}

	currentContents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	if !reflect.DeepEqual(currentContents, originalContents) {
		t.Fatalf(
			"config file changed after rejected update\nexpected: %q\nactual:   %q",
			originalContents,
			currentContents,
		)
	}
}

func TestUpdateConfigReturnsWriteError(t *testing.T) {
	tempDir := t.TempDir()

	configPath := filepath.Join(
		tempDir,
		"directory-that-does-not-exist",
		"config.yaml",
	)

	originalCfg := testConfig("off")
	gateway := New(originalCfg, configPath, "")

	newCfg := testConfig("on")
	newCfg.Tools[0].Port = 8082

	err := gateway.UpdateConfig(newCfg)
	if err == nil {
		t.Fatal("expected config write to fail")
	}

	if !strings.Contains(err.Error(), "failed to write config") {
		t.Fatalf(
			"expected config write error, got %q",
			err,
		)
	}

	snapshot := gateway.ConfigSnapshot()

	if !reflect.DeepEqual(snapshot, originalCfg) {
		t.Fatalf(
			"gateway config changed after write failure\nexpected: %#v\nactual:   %#v",
			originalCfg,
			snapshot,
		)
	}

	if gateway.SafeOutput() {
		t.Fatal("safe output changed after failed config write")
	}
}

func TestRunRejectsInvalidEnabledTool(t *testing.T) {
	cfg := config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "EQU-BROKEN-01",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	gateway := New(cfg, "/tmp/config.yaml", "")

	err := gateway.Run(context.Background())
	if err == nil {
		t.Fatal("expected Run to reject invalid enabled tool")
	}

	if !strings.Contains(err.Error(), "missing ip") {
		t.Fatalf(
			"expected missing IP error, got %q",
			err,
		)
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	gateway := New(config.Config{}, "/tmp/config.yaml", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := gateway.Run(ctx); err != nil {
		t.Fatalf(
			"expected clean shutdown after context cancellation, got %v",
			err,
		)
	}
}

func TestRunSkipsInvalidDisabledTool(t *testing.T) {
	cfg := config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "",
				IP:            "",
				Port:          0,
				SwitchID:      -1,
				Enabled:       false,
			},
		},
	}

	gateway := New(cfg, "/tmp/config.yaml", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := gateway.Run(ctx); err != nil {
		t.Fatalf(
			"expected disabled invalid tool to be skipped, got %v",
			err,
		)
	}
}

func TestConcurrentConfigReadsAndUpdates(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	initialCfg := testConfig("off")

	if err := config.WriteAtomic(configPath, initialCfg); err != nil {
		t.Fatalf("failed to create initial config: %v", err)
	}

	gateway := New(initialCfg, configPath, "")

	const readerCount = 8
	const iterations = 100

	var waitGroup sync.WaitGroup

	for reader := 0; reader < readerCount; reader++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				snapshot := gateway.ConfigSnapshot()

				if snapshot.Bind == "" {
					t.Error("config snapshot unexpectedly had empty bind")
				}

				_ = gateway.SafeOutput()
			}
		}()
	}

	for iteration := 0; iteration < iterations; iteration++ {
		safeState := "off"
		if iteration%2 == 0 {
			safeState = "on"
		}

		newCfg := testConfig(safeState)
		newCfg.Tools[0].Port = 8081 + iteration

		if err := gateway.UpdateConfig(newCfg); err != nil {
			t.Fatalf(
				"config update %d failed: %v",
				iteration,
				err,
			)
		}
	}

	waitGroup.Wait()
}

func TestNewStoresTLSInitializationError(t *testing.T) {
	cfg := testConfig("off")
	cfg.Defaults.ShellyTLS = config.ShellyTLSConfig{
		ServerCAFile: "/does/not/exist/server-ca.crt",
	}

	gateway := New(cfg, "/tmp/config.yaml", "")

	if gateway.initErr == nil {
		t.Fatal("expected TLS initialization error")
	}
	if gateway.shelly != nil {
		t.Fatal("expected Shelly client to be nil after initialization failure")
	}
}

func TestRunReturnsTLSInitializationError(t *testing.T) {
	cfg := testConfig("off")
	cfg.Defaults.ShellyTLS = config.ShellyTLSConfig{
		ServerCAFile: "/does/not/exist/server-ca.crt",
	}

	gateway := New(cfg, "/tmp/config.yaml", "")

	err := gateway.Run(context.Background())
	if err == nil {
		t.Fatal("expected TLS initialization error")
	}
	if !strings.Contains(err.Error(), "initialize Shelly client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewInitializesClientWithValidTLSFiles(t *testing.T) {
	tlsConfig := gatewayTestTLSConfig(t)
	cfg := testConfig("off")
	cfg.Defaults.ShellyTLS = tlsConfig

	gateway := New(cfg, "/tmp/config.yaml", "")

	if gateway.initErr != nil {
		t.Fatalf("unexpected TLS initialization error: %v", gateway.initErr)
	}
	if gateway.shelly == nil {
		t.Fatal("expected Shelly client to be initialized")
	}
}

func TestRunRejectsHTTPSWithoutTLSPaths(t *testing.T) {
	cfg := testConfig("off")
	cfg.Tools[0].Protocol = "https"

	gateway := New(cfg, "/tmp/config.yaml", "")

	err := gateway.Run(context.Background())
	if err == nil {
		t.Fatal("expected missing TLS configuration to be rejected")
	}
	if !strings.Contains(err.Error(), "server_ca_file is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func gatewayTestTLSConfig(t *testing.T) config.ShellyTLSConfig {
	t.Helper()

	dir := t.TempDir()
	now := time.Now()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Gateway Test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(
		rand.Reader,
		caTemplate,
		caTemplate,
		&caKey.PublicKey,
		caKey,
	)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "fbs-interlock-gateway-cluster-test",
		},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(
		rand.Reader,
		clientTemplate,
		caCertificate,
		&clientKey.PublicKey,
		caKey,
	)
	if err != nil {
		t.Fatalf("create client certificate: %v", err)
	}

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}

	serverCAFile := filepath.Join(dir, "server-ca.crt")
	clientCertFile := filepath.Join(dir, "gateway-client.crt")
	clientKeyFile := filepath.Join(dir, "gateway-client.key")

	writeGatewayTestFile(t, serverCAFile, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caDER,
	}))
	writeGatewayTestFile(t, clientCertFile, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: clientDER,
	}))
	writeGatewayTestFile(t, clientKeyFile, pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: clientKeyDER,
	}))

	return config.ShellyTLSConfig{
		ServerCAFile:   serverCAFile,
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	}
}

func writeGatewayTestFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestConfigSnapshotReturnsIndependentCopy(t *testing.T) {
	username := "admin"
	password := "secret"

	cfg := testConfig("off")
	cfg.Tools[0].Username = &username
	cfg.Tools[0].Password = &password

	gateway := New(cfg, "/tmp/config.yaml", "")

	snapshot := gateway.ConfigSnapshot()

	snapshot.Tools[0].InterlockName = "CHANGED"
	snapshot.Tools[0].Enabled = false
	*snapshot.Tools[0].Username = "changed-user"
	*snapshot.Tools[0].Password = "changed-password"

	secondSnapshot := gateway.ConfigSnapshot()

	if secondSnapshot.Tools[0].InterlockName != "EQU-TEST-01" {
		t.Fatalf(
			"stored name changed to %q",
			secondSnapshot.Tools[0].InterlockName,
		)
	}

	if !secondSnapshot.Tools[0].Enabled {
		t.Fatal("stored Enabled changed to false")
	}

	if got := *secondSnapshot.Tools[0].Username; got != "admin" {
		t.Fatalf(
			"stored username = %q, want admin",
			got,
		)
	}

	if got := *secondSnapshot.Tools[0].Password; got != "secret" {
		t.Fatalf(
			"stored password = %q, want secret",
			got,
		)
	}
}
