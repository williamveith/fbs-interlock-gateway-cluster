package shelly

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
)

func toolForServer(server *httptest.Server) config.Tool {
	return config.Tool{
		InterlockName: "EQU-TEST-01",
		IP:            strings.TrimPrefix(server.URL, "http://"),
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}
}

func TestClientGetStatusAndSetWithoutAuthentication(t *testing.T) {
	var setValue string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Switch.GetStatus":
			fmt.Fprint(w, `{"id":0,"output":true}`)
		case "/rpc/Switch.Set":
			setValue = r.URL.Query().Get("on")
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := toolForServer(server)

	status, err := client.GetStatus(context.Background(), tool)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Output {
		t.Fatal("status output = false, want true")
	}

	if err := client.Set(context.Background(), tool, false); err != nil {
		t.Fatal(err)
	}
	if setValue != "false" {
		t.Fatalf("set value = %q, want false", setValue)
	}
}

func TestClientRetriesWithDigestAuthorization(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="shelly", nonce="nonce", algorithm=SHA-256, qop="auth"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	username := "admin"
	password := "password"
	tool := toolForServer(server)
	tool.Username = &username
	tool.Password = &password

	client := NewClient(time.Second)
	if _, err := client.GetStatus(context.Background(), tool); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestClientRejectsInvalidStatusJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{not-json}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	if _, err := client.GetStatus(context.Background(), toolForServer(server)); err == nil {
		t.Fatal("expected JSON decoding error")
	}
}

func TestGetStatusReturnsTypedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad switch id", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := testTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if err == nil {
		t.Fatal("expected an error")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}

	if httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", httpErr.StatusCode)
	}

	if !IsHTTPStatus(err, http.StatusBadRequest) {
		t.Fatal("expected IsHTTPStatus to match 400")
	}
}

func TestRateLimitSchedulesSingleReboot(t *testing.T) {
	var rebootRequests atomic.Int32
	rebootSeen := make(chan struct{}, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Switch.GetStatus":
			http.Error(w, "Too many Requests", http.StatusLocked)

		case "/rpc/Shelly.Reboot":
			rebootRequests.Add(1)
			rebootSeen <- struct{}{}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.rebootCooldown = time.Hour
	tool := testTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusLocked) {
		t.Fatalf("expected HTTP 423, got %v", err)
	}

	select {
	case <-rebootSeen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reboot request")
	}

	_, err = client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusLocked) {
		t.Fatalf("expected second HTTP 423, got %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if got := rebootRequests.Load(); got != 1 {
		t.Fatalf("expected exactly one reboot request, got %d", got)
	}
}

func TestBadRequestDoesNotScheduleReboot(t *testing.T) {
	var rebootRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc/Shelly.Reboot" {
			rebootRequests.Add(1)
			_, _ = w.Write([]byte("null"))
			return
		}

		http.Error(w, "invalid argument", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := testTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusBadRequest) {
		t.Fatalf("expected HTTP 400, got %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if got := rebootRequests.Load(); got != 0 {
		t.Fatalf("expected no reboot request, got %d", got)
	}
}

func testTool(server *httptest.Server) config.Tool {
	return config.Tool{
		InterlockName: "test-tool",
		IP:            server.Listener.Addr().String(),
		SwitchID:      0,
		Enabled:       true,
	}
}

func authenticatedTestTool(server *httptest.Server) config.Tool {
	username := "admin"
	password := "password"
	tool := testTool(server)
	tool.Username = &username
	tool.Password = &password
	return tool
}

func TestDigestNonceIsReusedAndNonceCountIncrements(t *testing.T) {
	var challengeRequests atomic.Int32
	var mu sync.Mutex
	var nonceCounts []int

	ncPattern := regexp.MustCompile(`\bnc=([0-9a-fA-F]{8})\b`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if authorization == "" {
			challengeRequests.Add(1)
			w.Header().Set(
				"WWW-Authenticate",
				`Digest realm="shelly", nonce="nonce-one", algorithm=SHA-256, qop="auth"`,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		match := ncPattern.FindStringSubmatch(authorization)
		if len(match) != 2 {
			t.Errorf("Authorization header missing nonce count: %q", authorization)
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}

		parsed, err := strconv.ParseInt(match[1], 16, 32)
		if err != nil {
			t.Errorf("parse nonce count: %v", err)
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		nonceCounts = append(nonceCounts, int(parsed))
		mu.Unlock()

		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := authenticatedTestTool(server)

	for i := 0; i < 2; i++ {
		if _, err := client.GetStatus(context.Background(), tool); err != nil {
			t.Fatal(err)
		}
	}

	if got := challengeRequests.Load(); got != 1 {
		t.Fatalf("challenge requests = %d, want 1", got)
	}

	mu.Lock()
	defer mu.Unlock()

	if fmt.Sprint(nonceCounts) != "[1 2]" {
		t.Fatalf("nonce counts = %v, want [1 2]", nonceCounts)
	}
}

func TestConcurrentAuthenticatedRequestsUseNonceCountsInOrder(t *testing.T) {
	var mu sync.Mutex
	var nonceCounts []string
	ncPattern := regexp.MustCompile(`\bnc=([0-9a-fA-F]{8})\b`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if authorization == "" {
			w.Header().Set(
				"WWW-Authenticate",
				`Digest realm="shelly", nonce="nonce-one", algorithm=SHA-256, qop="auth"`,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		match := ncPattern.FindStringSubmatch(authorization)
		if len(match) != 2 {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		nonceCounts = append(nonceCounts, match[1])
		mu.Unlock()

		time.Sleep(10 * time.Millisecond)
		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := authenticatedTestTool(server)

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.GetStatus(context.Background(), tool)
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if strings.Join(nonceCounts, ",") != "00000001,00000002" {
		t.Fatalf("nonce counts = %v, want ordered counts", nonceCounts)
	}
}

func TestTooManyRequestsWaitsRetriesThenSucceeds(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)

		switch requestNumber {
		case 1:
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		case 2:
			w.Header().Set(
				"WWW-Authenticate",
				`Digest realm="shelly", nonce="nonce-one", algorithm=SHA-256, qop="auth"`,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			fmt.Fprint(w, `{"id":0,"output":true}`)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.authenticationThrottleDelay = 5 * time.Millisecond

	status, err := client.GetStatus(
		context.Background(),
		authenticatedTestTool(server),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Output {
		t.Fatal("status output = false, want true")
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
}

func TestPersistentTooManyRequestsSchedulesAuthenticatedReboot(t *testing.T) {
	var statusRequests atomic.Int32
	rebootSeen := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Switch.GetStatus":
			statusRequests.Add(1)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)

		case "/rpc/Shelly.Reboot":
			if r.Header.Get("Authorization") == "" {
				w.Header().Set(
					"WWW-Authenticate",
					`Digest realm="shelly", nonce="reboot-nonce", algorithm=SHA-256, qop="auth"`,
				)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			rebootSeen <- struct{}{}
			fmt.Fprint(w, `null`)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.authenticationThrottleDelay = 5 * time.Millisecond
	client.rebootCooldown = time.Hour
	tool := authenticatedTestTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("expected HTTP 429, got %v", err)
	}

	if got := statusRequests.Load(); got != 2 {
		t.Fatalf("status requests = %d, want 2", got)
	}

	select {
	case <-rebootSeen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for authenticated reboot request")
	}
}

type mutualTLSTestPKI struct {
	serverCAFile      string
	clientCertFile    string
	clientKeyFile     string
	serverCertificate tls.Certificate
	clientCAPool      *x509.CertPool
}

func TestNewClientWithTLSEmptyConfigPreservesHTTPCompatibility(t *testing.T) {
	client, err := NewClientWithTLS(time.Second, config.ShellyTLSConfig{})
	if err != nil {
		t.Fatalf("NewClientWithTLS() error = %v", err)
	}

	transport, ok := client.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.http.Transport)
	}
	if transport.TLSClientConfig != nil {
		t.Fatal("TLSClientConfig should be nil for an empty TLS configuration")
	}
}

func TestNewClientWithTLSRejectsIncompleteConfig(t *testing.T) {
	tests := []struct {
		name      string
		tlsConfig config.ShellyTLSConfig
		wantError string
	}{
		{
			name: "server CA missing",
			tlsConfig: config.ShellyTLSConfig{
				ClientCertFile: "client.crt",
				ClientKeyFile:  "client.key",
			},
			wantError: "server CA file is required",
		},
		{
			name: "client certificate missing",
			tlsConfig: config.ShellyTLSConfig{
				ServerCAFile:  "server-ca.crt",
				ClientKeyFile: "client.key",
			},
			wantError: "client certificate file is required",
		},
		{
			name: "client key missing",
			tlsConfig: config.ShellyTLSConfig{
				ServerCAFile:   "server-ca.crt",
				ClientCertFile: "client.crt",
			},
			wantError: "client key file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClientWithTLS(time.Second, tt.tlsConfig)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf(
					"NewClientWithTLS() error = %v, want error containing %q",
					err,
					tt.wantError,
				)
			}
		})
	}
}

func TestNewClientWithTLSLoadsValidFiles(t *testing.T) {
	pki := createMutualTLSTestPKI(t)

	client, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   pki.serverCAFile,
			ClientCertFile: pki.clientCertFile,
			ClientKeyFile:  pki.clientKeyFile,
		},
	)
	if err != nil {
		t.Fatalf("NewClientWithTLS() error = %v", err)
	}

	transport, ok := client.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.http.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf(
			"MinVersion = %d, want TLS 1.2 (%d)",
			transport.TLSClientConfig.MinVersion,
			tls.VersionTLS12,
		)
	}
	if len(transport.TLSClientConfig.Certificates) != 1 {
		t.Fatalf(
			"client certificates = %d, want 1",
			len(transport.TLSClientConfig.Certificates),
		)
	}
}

func TestNewClientWithTLSRejectsInvalidCA(t *testing.T) {
	pki := createMutualTLSTestPKI(t)
	invalidCA := filepath.Join(t.TempDir(), "invalid-ca.crt")

	if err := os.WriteFile(invalidCA, []byte("not a certificate"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   invalidCA,
			ClientCertFile: pki.clientCertFile,
			ClientKeyFile:  pki.clientKeyFile,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no valid certificates") {
		t.Fatalf("NewClientWithTLS() error = %v, want invalid-CA error", err)
	}
}

func TestNewClientWithTLSRejectsMismatchedClientKey(t *testing.T) {
	first := createMutualTLSTestPKI(t)
	second := createMutualTLSTestPKI(t)

	_, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   first.serverCAFile,
			ClientCertFile: first.clientCertFile,
			ClientKeyFile:  second.clientKeyFile,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "private key does not match") {
		t.Fatalf("NewClientWithTLS() error = %v, want key mismatch error", err)
	}
}

func TestRPCURLUsesConfiguredProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		want     string
	}{
		{
			name:     "omitted protocol",
			protocol: "",
			want:     "http://shelly.dynamic.utexas.edu/rpc/Switch.GetStatus?id=0",
		},
		{
			name:     "HTTPS protocol",
			protocol: " HTTPS ",
			want:     "https://shelly.dynamic.utexas.edu/rpc/Switch.GetStatus?id=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rpcURL(
				config.Tool{
					IP:       "shelly.dynamic.utexas.edu",
					Protocol: tt.protocol,
				},
				"Switch.GetStatus",
				url.Values{"id": []string{"0"}},
			)
			if err != nil {
				t.Fatalf("rpcURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("rpcURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRPCURLRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		tool   config.Tool
		method string
	}{
		{
			name:   "empty host",
			tool:   config.Tool{},
			method: "Switch.GetStatus",
		},
		{
			name: "host includes scheme",
			tool: config.Tool{
				IP: "https://shelly.dynamic.utexas.edu",
			},
			method: "Switch.GetStatus",
		},
		{
			name: "empty method",
			tool: config.Tool{
				IP: "shelly.dynamic.utexas.edu",
			},
			method: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rpcURL(tt.tool, tt.method, nil); err == nil {
				t.Fatal("rpcURL() returned nil error")
			}
		})
	}
}

func TestClientMutualTLSGetStatus(t *testing.T) {
	pki := createMutualTLSTestPKI(t)
	server := newMutualTLSTestServer(t, pki, pki.clientCAPool)
	defer server.Close()

	client, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   pki.serverCAFile,
			ClientCertFile: pki.clientCertFile,
			ClientKeyFile:  pki.clientKeyFile,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	tool := config.Tool{
		InterlockName: "EQU-TEST-HTTPS",
		IP:            strings.TrimPrefix(server.URL, "https://"),
		Protocol:      "https",
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}

	status, err := client.GetStatus(context.Background(), tool)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if !status.Output {
		t.Fatal("status output = false, want true")
	}
}

func TestClientRejectsUntrustedShellyCertificate(t *testing.T) {
	serverPKI := createMutualTLSTestPKI(t)
	otherPKI := createMutualTLSTestPKI(t)
	server := newMutualTLSTestServer(t, serverPKI, serverPKI.clientCAPool)
	defer server.Close()

	client, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   otherPKI.serverCAFile,
			ClientCertFile: serverPKI.clientCertFile,
			ClientKeyFile:  serverPKI.clientKeyFile,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	tool := config.Tool{
		InterlockName: "EQU-UNTRUSTED-SERVER",
		IP:            strings.TrimPrefix(server.URL, "https://"),
		Protocol:      "https",
		SwitchID:      0,
		Enabled:       true,
	}

	if _, err := client.GetStatus(context.Background(), tool); err == nil {
		t.Fatal("expected untrusted Shelly certificate to be rejected")
	}
}

func TestShellyRejectsClientSignedByUntrustedCA(t *testing.T) {
	serverPKI := createMutualTLSTestPKI(t)
	otherPKI := createMutualTLSTestPKI(t)
	server := newMutualTLSTestServer(t, serverPKI, otherPKI.clientCAPool)
	defer server.Close()

	client, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   serverPKI.serverCAFile,
			ClientCertFile: serverPKI.clientCertFile,
			ClientKeyFile:  serverPKI.clientKeyFile,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	tool := config.Tool{
		InterlockName: "EQU-UNTRUSTED-CLIENT",
		IP:            strings.TrimPrefix(server.URL, "https://"),
		Protocol:      "https",
		SwitchID:      0,
		Enabled:       true,
	}

	if _, err := client.GetStatus(context.Background(), tool); err == nil {
		t.Fatal("expected Shelly to reject the untrusted client certificate")
	}
}

func newMutualTLSTestServer(
	t *testing.T,
	pki mutualTLSTestPKI,
	clientCAPool *x509.CertPool,
) *httptest.Server {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rpc/Switch.GetStatus" {
				http.NotFound(w, r)
				return
			}

			fmt.Fprint(w, `{"id":0,"output":true}`)
		},
	))

	server.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{pki.serverCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	}
	server.StartTLS()

	return server
}

func createMutualTLSTestPKI(t *testing.T) mutualTLSTestPKI {
	t.Helper()

	dir := t.TempDir()
	now := time.Now()

	serverCA, serverCAKey, serverCAPEM := createTestCA(
		t,
		"Test Shelly Server CA",
		now,
	)
	clientCA, clientCAKey, clientCAPEM := createTestCA(
		t,
		"Test Gateway Client CA",
		now,
	)

	serverCertPEM, serverKeyPEM := createTestLeafCertificate(
		t,
		serverCA,
		serverCAKey,
		&x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject: pkix.Name{
				CommonName: "127.0.0.1",
			},
			NotBefore:   now.Add(-time.Minute),
			NotAfter:    now.Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		},
	)

	clientCertPEM, clientKeyPEM := createTestLeafCertificate(
		t,
		clientCA,
		clientCAKey,
		&x509.Certificate{
			SerialNumber: big.NewInt(3),
			Subject: pkix.Name{
				CommonName: "fbs-interlock-gateway-cluster-test",
			},
			NotBefore:   now.Add(-time.Minute),
			NotAfter:    now.Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		},
	)

	serverCAFile := filepath.Join(dir, "server-ca.crt")
	clientCertFile := filepath.Join(dir, "gateway-client.crt")
	clientKeyFile := filepath.Join(dir, "gateway-client.key")

	writeTestFile(t, serverCAFile, serverCAPEM, 0644)
	writeTestFile(t, clientCertFile, clientCertPEM, 0644)
	writeTestFile(t, clientKeyFile, clientKeyPEM, 0600)

	serverCertificate, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("load test server certificate: %v", err)
	}

	clientCAPool := x509.NewCertPool()
	if !clientCAPool.AppendCertsFromPEM(clientCAPEM) {
		t.Fatal("append test client CA")
	}

	return mutualTLSTestPKI{
		serverCAFile:      serverCAFile,
		clientCertFile:    clientCertFile,
		clientKeyFile:     clientKeyFile,
		serverCertificate: serverCertificate,
		clientCAPool:      clientCAPool,
	}
}

func createTestCA(
	t *testing.T,
	commonName string,
	now time.Time,
) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&key.PublicKey,
		key,
	)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	return certificate, key, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})
}

func createTestLeafCertificate(
	t *testing.T,
	ca *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	template *x509.Certificate,
) ([]byte, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}

	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		ca,
		&key.PublicKey,
		caKey,
	)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		}), pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: keyDER,
		})
}

func writeTestFile(
	t *testing.T,
	path string,
	data []byte,
	mode os.FileMode,
) {
	t.Helper()

	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type timeoutTestError struct{}

func (timeoutTestError) Error() string   { return "temporary timeout" }
func (timeoutTestError) Timeout() bool   { return true }
func (timeoutTestError) Temporary() bool { return true }

func TestGetStatusRetriesTransientTransportFailure(t *testing.T) {
	client := NewClient(time.Second)
	client.statusRetryDelay = 0

	var attempts atomic.Int32
	client.http.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return nil, timeoutTestError{}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":0,"output":true}`)),
			Request:    req,
		}, nil
	})

	status, err := client.GetStatus(context.Background(), config.Tool{
		InterlockName: "retry-test",
		IP:            "shelly.example.test",
		SwitchID:      0,
	})
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if !status.Output {
		t.Fatal("status output = false, want true")
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestUnauthenticatedRequestsAreSerializedPerDevice(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)

		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}

		time.Sleep(25 * time.Millisecond)
		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := toolForServer(server)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.GetStatus(context.Background(), tool)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent device requests = %d, want 1", got)
	}
}

func TestTLSClientSessionCacheIsEnabled(t *testing.T) {
	pki := createMutualTLSTestPKI(t)
	client, err := NewClientWithTLS(
		time.Second,
		config.ShellyTLSConfig{
			ServerCAFile:   pki.serverCAFile,
			ClientCertFile: pki.clientCertFile,
			ClientKeyFile:  pki.clientKeyFile,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	transport := client.http.Transport.(*http.Transport)
	if transport.TLSClientConfig.ClientSessionCache == nil {
		t.Fatal("ClientSessionCache is nil")
	}
	if transport.ResponseHeaderTimeout <= 0 {
		t.Fatal("ResponseHeaderTimeout must be configured")
	}
	if client.http.Timeout != 0 {
		t.Fatalf("http.Client.Timeout = %s, want 0; operation context owns the deadline", client.http.Timeout)
	}
}

func TestFBSPreemptsActiveAdminStatus(t *testing.T) {
	adminStarted := make(chan struct{})
	releaseFBS := make(chan struct{})

	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requestNumber := requests.Add(1)

			switch requestNumber {
			case 1:
				close(adminStarted)

				// Block until the Admin request context is canceled.
				<-r.Context().Done()

			case 2:
				<-releaseFBS
				fmt.Fprint(
					w,
					`{"id":0,"output":true}`,
				)

			default:
				http.Error(
					w,
					"unexpected request",
					http.StatusInternalServerError,
				)
			}
		},
	))
	defer server.Close()

	client := NewClient(time.Second)
	tool := toolForServer(server)

	adminDone := make(chan error, 1)

	go func() {
		_, err := client.GetStatusAdmin(
			context.Background(),
			tool,
		)
		adminDone <- err
	}()

	select {
	case <-adminStarted:
	case <-time.After(time.Second):
		t.Fatal("Admin status request did not start")
	}

	fbsDone := make(chan error, 1)

	go func() {
		_, err := client.GetStatus(
			context.Background(),
			tool,
		)
		fbsDone <- err
	}()

	select {
	case err := <-adminDone:
		if !errors.Is(
			err,
			ErrAdminStatusDeferred,
		) {
			t.Fatalf(
				"Admin error = %v, want ErrAdminStatusDeferred",
				err,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("FBS did not preempt Admin request")
	}

	close(releaseFBS)

	select {
	case err := <-fbsDone:
		if err != nil {
			t.Fatalf("FBS GetStatus() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("FBS request did not complete")
	}

	if got := requests.Load(); got != 2 {
		t.Fatalf("HTTP requests = %d, want 2", got)
	}
}

func TestAdminStatusDoesNotWaitBehindFBS(t *testing.T) {
	fbsStarted := make(chan struct{})
	releaseFBS := make(chan struct{})

	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)

			close(fbsStarted)
			<-releaseFBS

			fmt.Fprint(
				w,
				`{"id":0,"output":true}`,
			)
		},
	))
	defer server.Close()

	client := NewClient(time.Second)
	tool := toolForServer(server)

	fbsDone := make(chan error, 1)

	go func() {
		_, err := client.GetStatus(
			context.Background(),
			tool,
		)
		fbsDone <- err
	}()

	select {
	case <-fbsStarted:
	case <-time.After(time.Second):
		t.Fatal("FBS request did not start")
	}

	start := time.Now()

	_, err := client.GetStatusAdmin(
		context.Background(),
		tool,
	)

	elapsed := time.Since(start)

	if !errors.Is(err, ErrAdminStatusDeferred) {
		t.Fatalf(
			"GetStatusAdmin() error = %v, want ErrAdminStatusDeferred",
			err,
		)
	}

	if elapsed > 100*time.Millisecond {
		t.Fatalf(
			"Admin request waited %s behind FBS",
			elapsed,
		)
	}

	if got := requests.Load(); got != 1 {
		t.Fatalf(
			"HTTP requests = %d, want 1; Admin should not reach device",
			got,
		)
	}

	close(releaseFBS)

	select {
	case err := <-fbsDone:
		if err != nil {
			t.Fatalf("FBS GetStatus() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("FBS request did not complete")
	}
}
