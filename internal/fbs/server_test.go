package fbs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/shelly"
	statusstore "github.com/williamveith/fbs-interlock-gateway-cluster/internal/status"
)

type fakeShellyClient struct {
	status    shelly.SwitchStatus
	statusErr error
	setErr    error
	setCalls  []bool
}

func (f *fakeShellyClient) GetStatus(
	context.Context,
	config.Tool,
) (shelly.SwitchStatus, error) {
	return f.status, f.statusErr
}

func (f *fakeShellyClient) Set(
	_ context.Context,
	_ config.Tool,
	on bool,
) error {
	f.setCalls = append(f.setCalls, on)
	return f.setErr
}

func testTool() config.Tool {
	return config.Tool{
		InterlockName: "EQU-TEST-01",
		IP:            "127.0.0.1",
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}
}

func newTestServer(
	safeOutput bool,
	client ShellyClient,
) (*Server, *statusstore.Store) {
	tool := testTool()
	sharedStatus := statusstore.New(
		config.Config{Tools: []config.Tool{tool}},
		safeOutput,
	)

	return NewServer(
		"127.0.0.1",
		safeOutput,
		client,
		sharedStatus,
	), sharedStatus
}

func TestStatusReturnsShellyOutputAndUpdatesSharedStatus(t *testing.T) {
	client := &fakeShellyClient{
		status: shelly.SwitchStatus{
			ID:     0,
			Output: true,
		},
	}
	server, sharedStatus := newTestServer(false, client)

	req := httptest.NewRequest(
		http.MethodGet,
		"http://gateway/status",
		nil,
	)
	res := httptest.NewRecorder()
	server.handleFBSRequest(res, req, testTool())

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Body.String(); got != `{"Success":1,"State":1}` {
		t.Fatalf("body = %q", got)
	}

	row := sharedStatus.Snapshot()[0]
	if !row.Connected || !row.Output || row.Error != "" {
		t.Fatalf("shared status = %#v", row)
	}
}

func TestOnAndOffCommandsUpdateSharedStatus(t *testing.T) {
	tests := []struct {
		path string
		want bool
		body string
	}{
		{
			path: "/on",
			want: true,
			body: `{"Success":1,"State":1}`,
		},
		{
			path: "/off",
			want: false,
			body: `{"Success":1,"State":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			client := &fakeShellyClient{}
			server, sharedStatus := newTestServer(false, client)

			req := httptest.NewRequest(
				http.MethodGet,
				"http://gateway"+tt.path,
				nil,
			)
			res := httptest.NewRecorder()
			server.handleFBSRequest(res, req, testTool())

			if res.Code != http.StatusOK {
				t.Fatalf(
					"status = %d, want %d",
					res.Code,
					http.StatusOK,
				)
			}

			if len(client.setCalls) != 1 || client.setCalls[0] != tt.want {
				t.Fatalf(
					"set calls = %v, want [%t]",
					client.setCalls,
					tt.want,
				)
			}

			if got := res.Body.String(); got != tt.body {
				t.Fatalf(
					"body = %q, want %q",
					got,
					tt.body,
				)
			}

			row := sharedStatus.Snapshot()[0]
			if !row.Connected || row.Output != tt.want || row.Error != "" {
				t.Fatalf("shared status = %#v", row)
			}
		})
	}
}

func TestRequestsWithQueryStringsAreRejected(t *testing.T) {
	tests := []string{
		"/?state=1",
		"/?state=0",
		"/on?state=1",
		"/off?state=0",
		"/status?refresh=1",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			client := &fakeShellyClient{}
			server, sharedStatus := newTestServer(false, client)
			before := sharedStatus.Snapshot()

			req := httptest.NewRequest(
				http.MethodGet,
				"http://gateway"+path,
				nil,
			)
			res := httptest.NewRecorder()
			server.handleFBSRequest(res, req, testTool())

			if res.Code != http.StatusBadRequest {
				t.Fatalf(
					"status = %d, want %d",
					res.Code,
					http.StatusBadRequest,
				)
			}

			if len(client.setCalls) != 0 {
				t.Fatalf(
					"rejected request caused Set calls: %v",
					client.setCalls,
				)
			}

			after := sharedStatus.Snapshot()
			if len(after) != len(before) || after[0] != before[0] {
				t.Fatalf(
					"rejected request changed shared status: before=%#v after=%#v",
					before,
					after,
				)
			}
		})
	}
}

func TestNonGETRequestsAreRejected(t *testing.T) {
	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			client := &fakeShellyClient{}
			server, sharedStatus := newTestServer(false, client)
			before := sharedStatus.Snapshot()

			req := httptest.NewRequest(
				method,
				"http://gateway/on",
				nil,
			)
			res := httptest.NewRecorder()
			server.handleFBSRequest(res, req, testTool())

			if res.Code != http.StatusMethodNotAllowed {
				t.Fatalf(
					"status = %d, want %d",
					res.Code,
					http.StatusMethodNotAllowed,
				)
			}

			if len(client.setCalls) != 0 {
				t.Fatalf(
					"rejected method caused Set calls: %v",
					client.setCalls,
				)
			}

			after := sharedStatus.Snapshot()
			if len(after) != len(before) || after[0] != before[0] {
				t.Fatalf(
					"rejected method changed shared status: before=%#v after=%#v",
					before,
					after,
				)
			}
		})
	}
}

func TestUnknownPathsAreRejectedWithoutChangingRelayOrStatus(t *testing.T) {
	paths := []string{
		"/not-a-command",
		"/ON",
		"/OFF",
		"/STATUS",
		"/on/",
		"/off/",
		"/status/",
		"/something/on",
		"/onwhatever",
		"/statusblah",
		"/",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			client := &fakeShellyClient{}
			server, sharedStatus := newTestServer(false, client)
			before := sharedStatus.Snapshot()

			req := httptest.NewRequest(
				http.MethodGet,
				"http://gateway"+path,
				nil,
			)
			res := httptest.NewRecorder()
			server.handleFBSRequest(res, req, testTool())

			if res.Code != http.StatusNotFound {
				t.Fatalf(
					"status = %d, want %d",
					res.Code,
					http.StatusNotFound,
				)
			}

			if len(client.setCalls) != 0 {
				t.Fatalf(
					"unknown path caused Set calls: %v",
					client.setCalls,
				)
			}

			after := sharedStatus.Snapshot()
			if len(after) != len(before) || after[0] != before[0] {
				t.Fatalf(
					"unknown path changed shared status: before=%#v after=%#v",
					before,
					after,
				)
			}
		})
	}
}

func TestShellyStatusFailureReturnsAndRecordsConfiguredSafeState(
	t *testing.T,
) {
	client := &fakeShellyClient{
		statusErr: errors.New("offline"),
	}
	server, sharedStatus := newTestServer(true, client)

	req := httptest.NewRequest(
		http.MethodGet,
		"http://gateway/status",
		nil,
	)
	res := httptest.NewRecorder()
	server.handleFBSRequest(res, req, testTool())

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}

	if got := res.Body.String(); got != `{"Success":1,"State":1}` {
		t.Fatalf("body = %q, want safe-on response", got)
	}

	row := sharedStatus.Snapshot()[0]
	if row.Connected || !row.Output || row.Error != "offline" {
		t.Fatalf("shared failure status = %#v", row)
	}
}

func TestShellySetFailureReturnsAndRecordsConfiguredSafeState(
	t *testing.T,
) {
	client := &fakeShellyClient{
		setErr: errors.New("set failed"),
	}
	server, sharedStatus := newTestServer(false, client)

	req := httptest.NewRequest(
		http.MethodGet,
		"http://gateway/on",
		nil,
	)
	res := httptest.NewRecorder()
	server.handleFBSRequest(res, req, testTool())

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}

	if got := res.Body.String(); got != `{"Success":1,"State":0}` {
		t.Fatalf("body = %q, want safe-off response", got)
	}

	row := sharedStatus.Snapshot()[0]
	if row.Connected || row.Output || row.Error != "set failed" {
		t.Fatalf("shared failure status = %#v", row)
	}
}
