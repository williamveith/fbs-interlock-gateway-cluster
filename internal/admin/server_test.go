package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/shelly"
	statusstore "github.com/williamveith/fbs-interlock-gateway-cluster/internal/status"
)

type fakeConfigStore struct {
	cfg         config.Config
	safeOutput  bool
	updateErr   error
	updated     config.Config
	updateCalls int
}

func (f *fakeConfigStore) ConfigSnapshot() config.Config {
	return f.cfg
}

func (f *fakeConfigStore) UpdateConfig(newCfg config.Config) error {
	f.updateCalls++
	f.updated = newCfg

	if f.updateErr != nil {
		return f.updateErr
	}

	f.cfg = newCfg
	return nil
}

func (f *fakeConfigStore) SafeOutput() bool {
	return f.safeOutput
}

type fakeStatusClient struct {
	getStatus func(
		ctx context.Context,
		tool config.Tool,
	) (shelly.SwitchStatus, error)

	getStatusAdmin func(
		ctx context.Context,
		tool config.Tool,
	) (shelly.SwitchStatus, error)
}

func (f fakeStatusClient) GetStatus(
	ctx context.Context,
	tool config.Tool,
) (shelly.SwitchStatus, error) {
	if f.getStatus == nil {
		return shelly.SwitchStatus{},
			errors.New("unexpected GetStatus call")
	}

	return f.getStatus(ctx, tool)
}

func (f fakeStatusClient) GetStatusAdmin(
	ctx context.Context,
	tool config.Tool,
) (shelly.SwitchStatus, error) {
	if f.getStatusAdmin != nil {
		return f.getStatusAdmin(ctx, tool)
	}

	// Preserve all existing tests that initialize only getStatus.
	return f.GetStatus(ctx, tool)
}

func newTestAdminServer(
	addr string,
	store *fakeConfigStore,
	statusClient StatusClient,
	restartRequested chan<- struct{},
) *Server {
	sharedStatus := statusstore.New(
		store.ConfigSnapshot(),
		store.SafeOutput(),
	)

	return New(
		addr,
		store,
		statusClient,
		sharedStatus,
		restartRequested,
	)
}

func TestHandleConfigGet(t *testing.T) {
	stored := config.Config{
		Bind: "127.0.0.1",
		Defaults: config.Defaults{
			TimeoutMS:        3000,
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
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.11",
				Protocol:      "https",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	expected := stored
	expected.Tools = append([]config.Tool(nil), stored.Tools...)
	expected.Tools[0].Protocol = "http"

	store := &fakeConfigStore{cfg: stored}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/config",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf(
			"expected application/json content type, got %q",
			contentType,
		)
	}

	var actual config.Config

	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"unexpected config\nexpected: %#v\nactual:   %#v",
			expected,
			actual,
		)
	}
}

func TestHandleConfigPut(t *testing.T) {
	requestConfig := config.Config{
		Bind: "0.0.0.0",
		Defaults: config.Defaults{
			TimeoutMS:        1200,
			SafeStateOnError: "off",
			ShellyTLS: config.ShellyTLSConfig{
				ServerCAFile:   "/etc/fbs-interlock-gateway-cluster/tls/server-ca.crt",
				ClientCertFile: "/etc/fbs-interlock-gateway-cluster/tls/gateway-client.crt",
				ClientKeyFile:  "/etc/fbs-interlock-gateway-cluster/tls/gateway-client.key",
			},
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.20",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-03",
				IP:            "192.0.2.21",
				Protocol:      " HTTPS ",
				Port:          8083,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	expected := requestConfig
	expected.Tools = append([]config.Tool(nil), requestConfig.Tools...)
	expected.Tools[0].Protocol = "http"
	expected.Tools[1].Protocol = "https"

	body, err := json.Marshal(requestConfig)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	store := &fakeConfigStore{}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/config",
		bytes.NewReader(body),
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	if store.updateCalls != 1 {
		t.Fatalf(
			"expected UpdateConfig to be called once, got %d",
			store.updateCalls,
		)
	}

	if !reflect.DeepEqual(store.updated, expected) {
		t.Fatalf(
			"unexpected updated config\nexpected: %#v\nactual:   %#v",
			expected,
			store.updated,
		)
	}

	var result map[string]bool

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !result["saved"] {
		t.Fatal("expected saved=true")
	}

	if !result["restart_required"] {
		t.Fatal("expected restart_required=true")
	}
}

func TestHandleConfigPutRejectsInvalidJSON(t *testing.T) {
	store := &fakeConfigStore{}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/config",
		strings.NewReader(`{"bind":`),
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			response.Code,
		)
	}

	if store.updateCalls != 0 {
		t.Fatalf(
			"expected UpdateConfig not to be called, got %d calls",
			store.updateCalls,
		)
	}

	if !strings.Contains(response.Body.String(), "invalid JSON") {
		t.Fatalf(
			"expected invalid JSON error, got %q",
			response.Body.String(),
		)
	}
}

func TestHandleConfigPutReturnsStoreError(t *testing.T) {
	store := &fakeConfigStore{
		updateErr: errors.New("configuration rejected"),
	}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/config",
		strings.NewReader(`{"bind":"0.0.0.0","tools":[]}`),
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			response.Code,
		)
	}

	if store.updateCalls != 1 {
		t.Fatalf(
			"expected UpdateConfig to be called once, got %d",
			store.updateCalls,
		)
	}

	if !strings.Contains(
		response.Body.String(),
		"configuration rejected",
	) {
		t.Fatalf(
			"expected store error in response, got %q",
			response.Body.String(),
		)
	}
}

func TestHandleConfigRejectsUnsupportedMethod(t *testing.T) {
	store := &fakeConfigStore{}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/config",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			response.Code,
		)
	}

	if store.updateCalls != 0 {
		t.Fatalf(
			"expected no config update, got %d calls",
			store.updateCalls,
		)
	}
}

func TestCollectStatuses(t *testing.T) {
	store := &fakeConfigStore{
		safeOutput: true,
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "DISABLED",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       false,
				},
				{
					InterlockName: "CONNECTED",
					IP:            "192.0.2.11",
					Protocol:      "https",
					Port:          8082,
					SwitchID:      0,
					Enabled:       true,
				},
				{
					InterlockName: "FAILED",
					IP:            "192.0.2.12",
					Port:          8083,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	statusClient := fakeStatusClient{
		getStatus: func(
			_ context.Context,
			tool config.Tool,
		) (shelly.SwitchStatus, error) {
			switch tool.InterlockName {
			case "CONNECTED":
				return shelly.SwitchStatus{
					ID:     tool.SwitchID,
					Output: true,
				}, nil

			case "FAILED":
				return shelly.SwitchStatus{},
					errors.New("Shelly unreachable")

			default:
				return shelly.SwitchStatus{},
					errors.New("unexpected status request")
			}
		},
	}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		statusClient,
		nil,
	)

	revision := server.statusStore.NextRevision()

	if err := server.collectStatuses(
		context.Background(),
		revision,
	); err != nil {
		t.Fatalf("collectStatuses() error = %v", err)
	}

	results := server.statusStore.Snapshot()

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	disabled := results[0]

	if disabled.InterlockName != "DISABLED" {
		t.Fatalf(
			"expected first result to be DISABLED, got %q",
			disabled.InterlockName,
		)
	}

	if disabled.Enabled {
		t.Fatal("expected disabled tool to have enabled=false")
	}

	if disabled.Connected {
		t.Fatal("expected disabled tool to have connected=false")
	}

	if disabled.Output {
		t.Fatal("expected disabled tool output to remain false")
	}

	if disabled.Error != "" {
		t.Fatalf(
			"expected disabled tool error to be empty, got %q",
			disabled.Error,
		)
	}

	if disabled.Protocol != "http" {
		t.Fatalf("disabled protocol = %q, want http", disabled.Protocol)
	}

	connected := results[1]

	if !connected.Enabled {
		t.Fatal("expected connected tool to have enabled=true")
	}

	if !connected.Connected {
		t.Fatal("expected connected tool to have connected=true")
	}

	if !connected.Output {
		t.Fatal("expected connected tool output=true")
	}

	if connected.Error != "" {
		t.Fatalf(
			"expected connected tool error to be empty, got %q",
			connected.Error,
		)
	}

	if connected.Protocol != "https" {
		t.Fatalf("connected protocol = %q, want https", connected.Protocol)
	}

	failed := results[2]

	if !failed.Enabled {
		t.Fatal("expected failed tool to have enabled=true")
	}

	if failed.Connected {
		t.Fatal("expected failed tool to have connected=false")
	}

	if !failed.Output {
		t.Fatal(
			"expected failed tool output to use safe output=true",
		)
	}

	if failed.Error != "Shelly unreachable" {
		t.Fatalf(
			"expected Shelly error, got %q",
			failed.Error,
		)
	}

	if failed.Protocol != "http" {
		t.Fatalf("failed protocol = %q, want http", failed.Protocol)
	}
}

func TestHandleRestart(t *testing.T) {
	restartRequested := make(chan struct{}, 1)

	server := newTestAdminServer(
		"127.0.0.1:0",
		&fakeConfigStore{},
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		restartRequested,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/restart",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleRestart(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	var result map[string]bool

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode restart response: %v", err)
	}

	if !result["restart_requested"] {
		t.Fatal("expected restart_requested=true")
	}

	select {
	case <-restartRequested:
		// Expected.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restart request")
	}
}

func TestHandleRestartRejectsUnsupportedMethod(t *testing.T) {
	restartRequested := make(chan struct{}, 1)

	server := newTestAdminServer(
		"127.0.0.1:0",
		&fakeConfigStore{},
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		restartRequested,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/restart",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleRestart(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			response.Code,
		)
	}

	select {
	case <-restartRequested:
		t.Fatal(
			"restart signal should not be sent for unsupported method",
		)
	default:
		// Expected.
	}
}

func waitForStatusRefresh(
	t *testing.T,
	server *Server,
) {
	t.Helper()

	deadline := time.Now().Add(time.Second)

	for {
		server.statusMu.Lock()
		inFlight := server.statusRefreshInFlight
		server.statusMu.Unlock()

		if !inFlight {
			return
		}

		if time.Now().After(deadline) {
			t.Fatal(
				"status refresh did not complete within one second",
			)
		}

		time.Sleep(time.Millisecond)
	}
}

func TestStatusReadDoesNotRefreshDevices(t *testing.T) {
	store := &fakeConfigStore{
		safeOutput: true,
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "TEST",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	var calls atomic.Int32
	called := make(chan struct{}, 1)

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				calls.Add(1)

				select {
				case called <- struct{}{}:
				default:
				}

				return shelly.SwitchStatus{
					Output: true,
				}, nil
			},
		},
		nil,
	)

	response := httptest.NewRecorder()

	server.handleStatus(
		response,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status",
			nil,
		),
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"response code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	if got := response.Header().Get(
		statusRefreshInProgressHeader,
	); got != "false" {
		t.Fatalf(
			"%s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"false",
		)
	}

	// Give an incorrectly started background refresh enough time to call
	// GetStatus.
	select {
	case <-called:
		t.Fatal(
			"ordinary status read unexpectedly queried a Shelly",
		)

	case <-time.After(25 * time.Millisecond):
		// Expected.
	}

	if got := calls.Load(); got != 0 {
		t.Fatalf(
			"GetStatus calls = %d, want 0",
			got,
		)
	}

	var statuses []ToolStatus

	if err := json.NewDecoder(response.Body).Decode(
		&statuses,
	); err != nil {
		t.Fatalf(
			"failed to decode status response: %v",
			err,
		)
	}

	if len(statuses) != 1 {
		t.Fatalf(
			"status count = %d, want 1",
			len(statuses),
		)
	}

	status := statuses[0]

	if status.Connected {
		t.Fatal(
			"placeholder status should not be connected",
		)
	}

	if !status.Output {
		t.Fatal(
			"placeholder status should use safe output=true",
		)
	}

	if status.Error != "status not yet refreshed" {
		t.Fatalf(
			"placeholder error = %q, want %q",
			status.Error,
			"status not yet refreshed",
		)
	}
}

func TestExplicitStatusRefreshPopulatesCache(t *testing.T) {
	store := &fakeConfigStore{
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "TEST",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	var calls atomic.Int32

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				calls.Add(1)

				return shelly.SwitchStatus{
					Output: true,
				}, nil
			},
		},
		nil,
	)

	refreshResponse := httptest.NewRecorder()

	server.handleStatus(
		refreshResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status?refresh=1",
			nil,
		),
	)

	if refreshResponse.Code != http.StatusOK {
		t.Fatalf(
			"refresh response code = %d, want %d",
			refreshResponse.Code,
			http.StatusOK,
		)
	}

	if got := refreshResponse.Header().Get(
		statusRefreshInProgressHeader,
	); got != "true" {
		t.Fatalf(
			"%s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"true",
		)
	}

	waitForStatusRefresh(t, server)

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"GetStatus calls after explicit refresh = %d, want 1",
			got,
		)
	}

	cachedResponse := httptest.NewRecorder()

	server.handleStatus(
		cachedResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status",
			nil,
		),
	)

	if cachedResponse.Code != http.StatusOK {
		t.Fatalf(
			"cached response code = %d, want %d",
			cachedResponse.Code,
			http.StatusOK,
		)
	}

	if got := cachedResponse.Header().Get(
		statusRefreshInProgressHeader,
	); got != "false" {
		t.Fatalf(
			"%s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"false",
		)
	}

	var statuses []ToolStatus

	if err := json.NewDecoder(cachedResponse.Body).Decode(
		&statuses,
	); err != nil {
		t.Fatalf(
			"failed to decode cached status response: %v",
			err,
		)
	}

	if len(statuses) != 1 {
		t.Fatalf(
			"status count = %d, want 1",
			len(statuses),
		)
	}

	status := statuses[0]

	if !status.Connected {
		t.Fatal(
			"cached status should report connected=true",
		)
	}

	if !status.Output {
		t.Fatal(
			"cached status should report output=true",
		)
	}

	if status.Error != "" {
		t.Fatalf(
			"cached status error = %q, want empty",
			status.Error,
		)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"ordinary cached read caused GetStatus calls = %d, want 1 total",
			got,
		)
	}

	// A second explicitly requested refresh should perform one additional
	// Shelly query.
	secondRefreshResponse := httptest.NewRecorder()

	server.handleStatus(
		secondRefreshResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status?refresh=1",
			nil,
		),
	)

	if got := secondRefreshResponse.Header().Get(
		statusRefreshInProgressHeader,
	); got != "true" {
		t.Fatalf(
			"second refresh %s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"true",
		)
	}

	waitForStatusRefresh(t, server)

	if got := calls.Load(); got != 2 {
		t.Fatalf(
			"GetStatus calls after second explicit refresh = %d, want 2",
			got,
		)
	}
}

func TestConcurrentExplicitStatusRefreshesShareOneRefresh(
	t *testing.T,
) {
	store := &fakeConfigStore{
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "TEST",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	var calls atomic.Int32

	started := make(chan struct{}, 1)
	release := make(chan struct{})

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				ctx context.Context,
				tool config.Tool,
			) (shelly.SwitchStatus, error) {
				calls.Add(1)

				select {
				case started <- struct{}{}:
				default:
				}

				select {
				case <-release:
					return shelly.SwitchStatus{
						Output: true,
					}, nil

				case <-ctx.Done():
					return shelly.SwitchStatus{},
						ctx.Err()
				}
			},
		},
		nil,
	)

	first := httptest.NewRecorder()

	server.handleStatus(
		first,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status?refresh=1",
			nil,
		),
	)

	if first.Code != http.StatusOK {
		t.Fatalf(
			"first response code = %d, want %d",
			first.Code,
			http.StatusOK,
		)
	}

	if got := first.Header().Get(
		statusRefreshInProgressHeader,
	); got != "true" {
		t.Fatalf(
			"first %s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"true",
		)
	}

	select {
	case <-started:
		// Expected.

	case <-time.After(time.Second):
		t.Fatal(
			"first explicit status refresh did not start",
		)
	}

	second := httptest.NewRecorder()

	server.handleStatus(
		second,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status?refresh=1",
			nil,
		),
	)

	if second.Code != http.StatusOK {
		t.Fatalf(
			"second response code = %d, want %d",
			second.Code,
			http.StatusOK,
		)
	}

	if got := second.Header().Get(
		statusRefreshInProgressHeader,
	); got != "true" {
		t.Fatalf(
			"second %s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"true",
		)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"GetStatus calls while refresh in flight = %d, want 1",
			got,
		)
	}

	close(release)
	waitForStatusRefresh(t, server)

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"GetStatus calls after shared refresh = %d, want 1",
			got,
		)
	}

	// Verify that the completed result was stored and that reading it does
	// not start another refresh.
	cached := httptest.NewRecorder()

	server.handleStatus(
		cached,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status",
			nil,
		),
	)

	if got := cached.Header().Get(
		statusRefreshInProgressHeader,
	); got != "false" {
		t.Fatalf(
			"cached %s = %q, want %q",
			statusRefreshInProgressHeader,
			got,
			"false",
		)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"cached read caused GetStatus calls = %d, want 1 total",
			got,
		)
	}
}

func TestStatusReadReturnsSharedStoreUpdate(t *testing.T) {
	store := &fakeConfigStore{
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "TEST",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	sharedStatus := statusstore.New(
		store.ConfigSnapshot(),
		store.SafeOutput(),
	)
	tool := store.cfg.Tools[0]
	sharedStatus.RecordSuccess(
		tool,
		true,
		sharedStatus.NextRevision(),
	)

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				t.Fatal("cache-only status read queried a Shelly")
				return shelly.SwitchStatus{}, nil
			},
		},
		sharedStatus,
		nil,
	)

	response := httptest.NewRecorder()
	server.handleStatus(
		response,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status",
			nil,
		),
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"response code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	var rows []ToolStatus
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		t.Fatalf("decode status response: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("status count = %d, want 1", len(rows))
	}
	if !rows[0].Connected || !rows[0].Output || rows[0].Error != "" {
		t.Fatalf("shared status row = %#v", rows[0])
	}
}

func TestCollectStatusesPublishesAndPreservesPartialResults(
	t *testing.T,
) {
	store := &fakeConfigStore{
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "FAST",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       true,
				},
				{
					InterlockName: "BLOCKED",
					IP:            "192.0.2.11",
					Port:          8082,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	blockedStarted := make(chan struct{})

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				ctx context.Context,
				tool config.Tool,
			) (shelly.SwitchStatus, error) {
				switch tool.InterlockName {
				case "FAST":
					return shelly.SwitchStatus{
						Output: true,
					}, nil

				case "BLOCKED":
					close(blockedStarted)

					<-ctx.Done()

					return shelly.SwitchStatus{},
						ctx.Err()

				default:
					return shelly.SwitchStatus{},
						errors.New("unexpected tool")
				}
			},
		},
		nil,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	defer cancel()

	revision := server.statusStore.NextRevision()
	collectionDone := make(chan error, 1)

	go func() {
		collectionDone <- server.collectStatuses(
			ctx,
			revision,
		)
	}()

	select {
	case <-blockedStarted:
		// The blocked request is now keeping collection in progress.
	case <-time.After(time.Second):
		t.Fatal("blocked status request did not start")
	}

	// Wait until the fast result has been published even though the
	// complete fleet collection is still running.
	deadline := time.Now().Add(time.Second)

	for {
		rows := server.statusStore.Snapshot()

		if rows[0].Connected && rows[0].Output {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf(
				"fast status was not published incrementally: %#v",
				rows[0],
			)
		}

		time.Sleep(time.Millisecond)
	}

	select {
	case err := <-collectionDone:
		t.Fatalf(
			"collection completed before blocked request was canceled: %v",
			err,
		)
	default:
		// Collection remains active, but the fast result is visible.
	}

	cancel()

	select {
	case err := <-collectionDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf(
				"collectStatuses() error = %v, want context canceled",
				err,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("collectStatuses did not stop after cancellation")
	}

	// The completed result must remain stored after the overall
	// collection fails or times out.
	rows := server.statusStore.Snapshot()

	if !rows[0].Connected || !rows[0].Output {
		t.Fatalf(
			"completed result was discarded after cancellation: %#v",
			rows[0],
		)
	}
}

func TestCollectStatusesPreservesRowWhenAdminStatusDeferred(
	t *testing.T,
) {
	tool := config.Tool{
		InterlockName: "TEST",
		IP:            "192.0.2.10",
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}

	store := &fakeConfigStore{
		cfg: config.Config{
			Tools: []config.Tool{tool},
		},
	}

	server := newTestAdminServer(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatusAdmin: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{},
					shelly.ErrAdminStatusDeferred
			},
		},
		nil,
	)

	// Seed an existing valid result that the deferred Admin probe
	// must not replace with an error.
	fbsRevision := server.statusStore.NextRevision()
	server.statusStore.RecordSuccess(
		tool,
		true,
		fbsRevision,
	)

	adminRevision := server.statusStore.NextRevision()

	if err := server.collectStatuses(
		context.Background(),
		adminRevision,
	); err != nil {
		t.Fatalf("collectStatuses() error = %v", err)
	}

	rows := server.statusStore.Snapshot()

	if len(rows) != 1 {
		t.Fatalf("status count = %d, want 1", len(rows))
	}

	if !rows[0].Connected {
		t.Fatal("deferred Admin status replaced connected state")
	}

	if !rows[0].Output {
		t.Fatal("deferred Admin status replaced output=true")
	}

	if rows[0].Error != "" {
		t.Fatalf(
			"deferred Admin status recorded error %q",
			rows[0].Error,
		)
	}
}
