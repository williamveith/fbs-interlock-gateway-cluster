package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validTool(name string, port int, enabled bool) Tool {
	return Tool{
		InterlockName: name,
		IP:            "127.0.0.1",
		Port:          port,
		SwitchID:      0,
		Enabled:       enabled,
	}
}

func completeTLSConfig() ShellyTLSConfig {
	return ShellyTLSConfig{
		ServerCAFile:   "/etc/fbs-interlock-gateway-cluster/tls/server-ca.crt",
		ClientCertFile: "/etc/fbs-interlock-gateway-cluster/tls/gateway-client.crt",
		ClientKeyFile:  "/etc/fbs-interlock-gateway-cluster/tls/gateway-client.key",
	}
}

func TestApplyDefaults(t *testing.T) {
	var cfg Config
	ApplyDefaults(&cfg)

	if cfg.Bind != "0.0.0.0" {
		t.Fatalf("Bind = %q, want 0.0.0.0", cfg.Bind)
	}
	if cfg.Defaults.TimeoutMS != 3000 {
		t.Fatalf("TimeoutMS = %d, want 3000", cfg.Defaults.TimeoutMS)
	}
	if cfg.Defaults.SafeStateOnError != "off" {
		t.Fatalf("SafeStateOnError = %q, want off", cfg.Defaults.SafeStateOnError)
	}
}

func TestLoadResolvesTLSPathsRelativeToConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	data := []byte(`{
  "defaults": {
    "shelly_tls": {
      "server_ca_file": "./tls/server-ca.crt",
      "client_cert_file": "./tls/gateway-client.crt",
      "client_key_file": "./tls/gateway-client.key"
    }
  },
  "tools": []
}`)

	if err := os.WriteFile(configPath, data, 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	expected := ShellyTLSConfig{
		ServerCAFile: filepath.Join(
			tempDir,
			"tls",
			"server-ca.crt",
		),
		ClientCertFile: filepath.Join(
			tempDir,
			"tls",
			"gateway-client.crt",
		),
		ClientKeyFile: filepath.Join(
			tempDir,
			"tls",
			"gateway-client.key",
		),
	}

	if cfg.Defaults.ShellyTLS != expected {
		t.Fatalf(
			"ShellyTLS = %#v, want %#v",
			cfg.Defaults.ShellyTLS,
			expected,
		)
	}
}

func TestLoadPreservesAbsoluteAndBlankTLSPaths(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	absoluteCA := filepath.Join(tempDir, "absolute-server-ca.crt")

	data := []byte(`{
  "defaults": {
    "shelly_tls": {
      "server_ca_file": "` + absoluteCA + `",
      "client_cert_file": "",
      "client_key_file": "   "
    }
  },
  "tools": []
}`)

	if err := os.WriteFile(configPath, data, 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Defaults.ShellyTLS.ServerCAFile != absoluteCA {
		t.Fatalf(
			"ServerCAFile = %q, want %q",
			cfg.Defaults.ShellyTLS.ServerCAFile,
			absoluteCA,
		)
	}
	if cfg.Defaults.ShellyTLS.ClientCertFile != "" {
		t.Fatalf(
			"ClientCertFile = %q, want blank",
			cfg.Defaults.ShellyTLS.ClientCertFile,
		)
	}
	if cfg.Defaults.ShellyTLS.ClientKeyFile != "" {
		t.Fatalf(
			"ClientKeyFile = %q, want blank",
			cfg.Defaults.ShellyTLS.ClientKeyFile,
		)
	}
}

func TestToolProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		want     string
	}{
		{name: "omitted", protocol: "", want: "http"},
		{name: "blank", protocol: "   ", want: "http"},
		{name: "http normalized", protocol: " HTTP ", want: "http"},
		{name: "https normalized", protocol: " HTTPS ", want: "https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := validTool("tool", 8081, true)
			tool.Protocol = tt.protocol

			if got := ToolProtocol(tool); got != tt.want {
				t.Fatalf("ToolProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRejectsDuplicatePorts(t *testing.T) {
	cfg := Config{Tools: []Tool{
		validTool("tool-a", 8081, true),
		validTool("tool-b", 8081, true),
	}}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate port 8081") {
		t.Fatalf("Validate() error = %v, want duplicate-port error", err)
	}
}

func TestValidateEnabledToolsIgnoresDisabledTool(t *testing.T) {
	cfg := Config{Tools: []Tool{
		validTool("tool-a", 8081, true),
		validTool("disabled-copy", 8081, false),
	}}

	if err := ValidateEnabledTools(cfg); err != nil {
		t.Fatalf("ValidateEnabledTools() error = %v", err)
	}
}

func TestValidateToolAcceptsHTTPAndHTTPS(t *testing.T) {
	for _, protocol := range []string{"", "http", "HTTP", "https", " HTTPS "} {
		t.Run(protocol, func(t *testing.T) {
			tool := validTool("tool", 8081, true)
			tool.Protocol = protocol

			if err := ValidateTool(tool); err != nil {
				t.Fatalf("ValidateTool() error = %v", err)
			}
		})
	}
}

func TestValidateToolRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
	}{
		{name: "missing name", tool: Tool{IP: "host", Port: 8081}},
		{name: "missing ip", tool: Tool{InterlockName: "tool", Port: 8081}},
		{name: "scheme in host", tool: Tool{InterlockName: "tool", IP: "https://host", Port: 8081}},
		{name: "invalid protocol", tool: Tool{InterlockName: "tool", IP: "host", Protocol: "ftp", Port: 8081}},
		{name: "zero port", tool: Tool{InterlockName: "tool", IP: "host", Port: 0}},
		{name: "high port", tool: Tool{InterlockName: "tool", IP: "host", Port: 65536}},
		{name: "negative switch", tool: Tool{InterlockName: "tool", IP: "host", Port: 8081, SwitchID: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTool(tt.tool); err == nil {
				t.Fatal("ValidateTool() returned nil error")
			}
		})
	}
}

func TestValidateRequiresCompleteTLSConfigForHTTPS(t *testing.T) {
	tests := []struct {
		name      string
		tlsConfig ShellyTLSConfig
		wantError string
	}{
		{
			name:      "all missing",
			wantError: "server_ca_file is required",
		},
		{
			name: "client certificate missing",
			tlsConfig: ShellyTLSConfig{
				ServerCAFile: "/tmp/server-ca.crt",
			},
			wantError: "client_cert_file is required",
		},
		{
			name: "client key missing",
			tlsConfig: ShellyTLSConfig{
				ServerCAFile:   "/tmp/server-ca.crt",
				ClientCertFile: "/tmp/gateway-client.crt",
			},
			wantError: "client_key_file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := validTool("https-tool", 8081, true)
			tool.Protocol = "https"

			cfg := Config{
				Defaults: Defaults{ShellyTLS: tt.tlsConfig},
				Tools:    []Tool{tool},
			}

			err := Validate(cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf(
					"Validate() error = %v, want error containing %q",
					err,
					tt.wantError,
				)
			}
		})
	}
}

func TestValidateAcceptsHTTPSWithCompleteTLSConfig(t *testing.T) {
	tool := validTool("https-tool", 8081, true)
	tool.Protocol = "https"

	cfg := Config{
		Defaults: Defaults{ShellyTLS: completeTLSConfig()},
		Tools:    []Tool{tool},
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateEnabledToolsIgnoresDisabledHTTPSWithoutTLSConfig(t *testing.T) {
	tool := validTool("disabled-https-tool", 8081, false)
	tool.Protocol = "https"

	cfg := Config{Tools: []Tool{tool}}

	if err := ValidateEnabledTools(cfg); err != nil {
		t.Fatalf("ValidateEnabledTools() error = %v", err)
	}
}

func TestValidateIncludesDisabledHTTPSWhenValidatingWholeConfig(t *testing.T) {
	tool := validTool("disabled-https-tool", 8081, false)
	tool.Protocol = "https"

	cfg := Config{Tools: []Tool{tool}}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "server_ca_file is required") {
		t.Fatalf("Validate() error = %v, want TLS configuration error", err)
	}
}

func TestCloneReturnsIndependentCopy(t *testing.T) {
	username := "admin"
	password := "secret"

	original := Config{
		Bind: "127.0.0.1",
		Tools: []Tool{
			{
				InterlockName: "TEST",
				IP:            "192.0.2.10",
				Port:          8081,
				Username:      &username,
				Password:      &password,
				Enabled:       true,
			},
		},
	}

	cloned := Clone(original)

	cloned.Tools[0].InterlockName = "CHANGED"
	cloned.Tools[0].Enabled = false
	*cloned.Tools[0].Username = "different-user"
	*cloned.Tools[0].Password = "different-password"

	if original.Tools[0].InterlockName != "TEST" {
		t.Fatalf(
			"original name changed to %q",
			original.Tools[0].InterlockName,
		)
	}

	if !original.Tools[0].Enabled {
		t.Fatal("original Enabled changed to false")
	}

	if got := *original.Tools[0].Username; got != "admin" {
		t.Fatalf(
			"original username = %q, want admin",
			got,
		)
	}

	if got := *original.Tools[0].Password; got != "secret" {
		t.Fatalf(
			"original password = %q, want secret",
			got,
		)
	}
}
