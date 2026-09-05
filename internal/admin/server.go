package admin

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/shelly"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/status"
)

const (
	maxConfigRequestBytes = 1 << 20 // 1 MiB
	maxHeaderBytes        = 32 << 10

	maxConcurrentStatusRequests   = 32
	statusRefreshTimeout          = 2 * time.Minute
	statusRefreshInProgressHeader = "X-Status-Refresh-In-Progress"

	readHeaderTimeout = 2 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 30 * time.Second
	shutdownTimeout   = 3 * time.Second
	restartDelay      = 500 * time.Millisecond
)

type ConfigStore interface {
	ConfigSnapshot() config.Config
	UpdateConfig(newCfg config.Config) error
	SafeOutput() bool
}

type StatusClient interface {
	GetStatusAdmin(
		ctx context.Context,
		tool config.Tool,
	) (shelly.SwitchStatus, error)
}

type Server struct {
	addr             string
	store            ConfigStore
	statusClient     StatusClient
	statusStore      *status.Store
	restartRequested chan<- struct{}

	statusMu              sync.Mutex
	statusRefreshInFlight bool
}

type ToolStatus = status.ToolStatus

type adminConfigRequest struct {
	Bind     string             `json:"bind"`
	Defaults config.Defaults    `json:"defaults"`
	Tools    []adminToolRequest `json:"tools"`
}

type adminToolRequest struct {
	InterlockName string  `json:"interlock_name"`
	IP            string  `json:"ip"`
	Protocol      string  `json:"protocol"`
	Port          int     `json:"port"`
	SwitchID      int     `json:"switch_id"`
	Username      *string `json:"username"`
	Password      *string `json:"password"`

	// PasswordSet is accepted because the frontend receives it from GET
	// and may send it back with the configuration. It is informational only.
	PasswordSet bool `json:"password_set,omitempty"`

	// ClearPassword must be true to intentionally remove a stored password.
	ClearPassword bool `json:"clear_password,omitempty"`

	Enabled bool `json:"enabled"`
}

type adminConfigResponse struct {
	Bind     string                `json:"bind"`
	Defaults adminDefaultsResponse `json:"defaults"`
	Tools    []adminToolResponse   `json:"tools"`
}

type adminDefaultsResponse struct {
	TimeoutMS        int                    `json:"timeout_ms"`
	SafeStateOnError string                 `json:"safe_state_on_error"`
	ShellyTLS        config.ShellyTLSConfig `json:"shelly_tls"`
}

type adminToolResponse struct {
	InterlockName string  `json:"interlock_name"`
	IP            string  `json:"ip"`
	Protocol      string  `json:"protocol"`
	Port          int     `json:"port"`
	SwitchID      int     `json:"switch_id"`
	Username      *string `json:"username"`
	PasswordSet   bool    `json:"password_set"`
	Enabled       bool    `json:"enabled"`
}

//go:embed web
var embeddedWeb embed.FS

func New(
	addr string,
	store ConfigStore,
	statusClient StatusClient,
	statusStore *status.Store,
	restartRequested chan<- struct{},
) *Server {
	return &Server{
		addr:             addr,
		store:            store,
		statusClient:     statusClient,
		statusStore:      statusStore,
		restartRequested: restartRequested,
	}
}

func (s *Server) Run(ctx context.Context) error {

	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return fmt.Errorf(
			"failed to load embedded web files: %w",
			err,
		)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/restart", s.handleRestart)
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	var handler http.Handler = mux

	handler = s.securityHeadersMiddleware(handler)
	handler = s.crossSiteProtectionMiddleware(handler)

	server := &http.Server{
		Addr:              s.addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	serverFinished := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(
				context.Background(),
				shutdownTimeout,
			)
			defer cancel()

			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf(
					"admin server graceful shutdown failed: %v",
					err,
				)

				if closeErr := server.Close(); closeErr != nil {
					log.Printf(
						"admin server forced close failed: %v",
						closeErr,
					)
				}
			}

		case <-serverFinished:
		}
	}()

	if !isLoopbackAdminAddress(s.addr) {
		log.Printf("warning: admin UI is listening on a non-loopback address: %s", s.addr)
	} else {
		log.Printf("admin UI listening on %s", s.addr)
	}

	err = server.ListenAndServe()
	close(serverFinished)

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("admin server error: %w", err)
	}

	return nil
}

func (s *Server) handleConfig(
	w http.ResponseWriter,
	r *http.Request,
) {
	setAPINoStoreHeaders(w)

	switch r.Method {
	case http.MethodGet:
		s.handleGetConfig(w)

	case http.MethodPut:
		s.handlePutConfig(w, r)

	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
	}
}

func (s *Server) handleGetConfig(w http.ResponseWriter) {
	cfg := s.store.ConfigSnapshot()

	response := adminConfigResponse{
		Bind: cfg.Bind,
		Defaults: adminDefaultsResponse{
			TimeoutMS:        cfg.Defaults.TimeoutMS,
			SafeStateOnError: cfg.Defaults.SafeStateOnError,
			ShellyTLS:        cfg.Defaults.ShellyTLS,
		},
		Tools: make([]adminToolResponse, len(cfg.Tools)),
	}

	for i, tool := range cfg.Tools {
		response.Tools[i] = adminToolResponse{
			InterlockName: tool.InterlockName,
			IP:            tool.IP,
			Protocol:      config.ToolProtocol(tool),
			Port:          tool.Port,
			SwitchID:      tool.SwitchID,
			Username:      cloneStringPointer(tool.Username),
			PasswordSet:   hasNonEmptyString(tool.Password),
			Enabled:       tool.Enabled,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handlePutConfig(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request adminConfigRequest

	if err := decodeSingleJSON(
		w,
		r,
		maxConfigRequestBytes,
		&request,
	); err != nil {
		http.Error(
			w,
			"invalid JSON: "+err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	current := s.store.ConfigSnapshot()
	updated := buildUpdatedConfig(current, request)

	if err := s.store.UpdateConfig(updated); err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{
		"saved":            true,
		"restart_required": true,
	})
}

func buildUpdatedConfig(
	current config.Config,
	request adminConfigRequest,
) config.Config {
	passwordsByPort := make(
		map[int]*string,
		len(current.Tools),
	)

	passwordsByName := make(
		map[string]*string,
		len(current.Tools),
	)

	for _, tool := range current.Tools {
		password := cloneStringPointer(tool.Password)

		passwordsByPort[tool.Port] = password

		normalizedName := normalizeToolName(tool.InterlockName)
		if normalizedName != "" {
			passwordsByName[normalizedName] =
				cloneStringPointer(tool.Password)
		}
	}

	tools := make([]config.Tool, len(request.Tools))

	for i, incoming := range request.Tools {
		password := requestedPassword(
			incoming,
			passwordsByPort,
			passwordsByName,
		)

		tools[i] = config.Tool{
			InterlockName: strings.TrimSpace(
				incoming.InterlockName,
			),
			IP: strings.TrimSpace(incoming.IP),
			Protocol: config.ToolProtocol(config.Tool{
				Protocol: incoming.Protocol,
			}),
			Port:     incoming.Port,
			SwitchID: incoming.SwitchID,
			Username: normalizeOptionalString(
				incoming.Username,
			),
			Password: password,
			Enabled:  incoming.Enabled,
		}
	}

	return config.Config{
		Bind:     strings.TrimSpace(request.Bind),
		Defaults: request.Defaults,
		Tools:    tools,
	}
}

func requestedPassword(
	incoming adminToolRequest,
	passwordsByPort map[int]*string,
	passwordsByName map[string]*string,
) *string {
	if incoming.ClearPassword {
		return nil
	}

	if hasNonEmptyString(incoming.Password) {
		return cloneTrimmedStringPointer(incoming.Password)
	}

	if password, ok := passwordsByPort[incoming.Port]; ok {
		return cloneStringPointer(password)
	}

	normalizedName := normalizeToolName(
		incoming.InterlockName,
	)

	if normalizedName != "" {
		if password, ok := passwordsByName[normalizedName]; ok {
			return cloneStringPointer(password)
		}
	}

	return nil
}

func normalizeToolName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func cloneTrimmedStringPointer(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func hasNonEmptyString(value *string) bool {
	return value != nil &&
		strings.TrimSpace(*value) != ""
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}

func (s *Server) handleStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	setAPINoStoreHeaders(w)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	refreshRequested := r.URL.Query().Get("refresh") == "1"

	results, refreshInProgress := s.statusSnapshot(
		refreshRequested,
	)

	if refreshInProgress {
		w.Header().Set(
			statusRefreshInProgressHeader,
			"true",
		)
	} else {
		w.Header().Set(
			statusRefreshInProgressHeader,
			"false",
		)
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) statusSnapshot(
	refreshRequested bool,
) ([]ToolStatus, bool) {
	s.statusMu.Lock()

	startRefresh := refreshRequested &&
		!s.statusRefreshInFlight

	var refreshRevision uint64
	if startRefresh {
		s.statusRefreshInFlight = true
		refreshRevision = s.statusStore.NextRevision()
	}

	refreshInProgress := s.statusRefreshInFlight
	s.statusMu.Unlock()

	if startRefresh {
		go s.refreshStatuses(refreshRevision)
	}

	return s.statusStore.Snapshot(), refreshInProgress
}

func (s *Server) refreshStatuses(revision uint64) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		statusRefreshTimeout,
	)
	defer cancel()

	// Always release the refresh lock, regardless of whether collection
	// completes successfully or the refresh context expires.
	defer func() {
		s.statusMu.Lock()
		s.statusRefreshInFlight = false
		s.statusMu.Unlock()
	}()

	if err := s.collectStatuses(ctx, revision); err != nil {
		log.Printf(
			"admin status refresh failed: %v",
			err,
		)
	}
}

func (s *Server) collectStatuses(
	ctx context.Context,
	revision uint64,
) error {
	cfg := s.store.ConfigSnapshot()
	safeOutput := s.store.SafeOutput()
	enabledCount := 0

	// Disabled tools do not require a Shelly request, so publish their
	// configuration rows immediately.
	for _, tool := range cfg.Tools {
		if tool.Enabled {
			enabledCount++
			continue
		}

		s.statusStore.Record(
			adminToolStatus(tool),
			revision,
		)
	}

	if enabledCount == 0 {
		return nil
	}

	workerCount := min(
		enabledCount,
		maxConcurrentStatusRequests,
	)

	jobs := make(chan config.Tool)

	var workers sync.WaitGroup
	workers.Add(workerCount)

	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()

			for tool := range jobs {
				if ctx.Err() != nil {
					return
				}

				switchStatus, err :=
					s.statusClient.GetStatusAdmin(ctx, tool)

				if errors.Is(err, shelly.ErrAdminStatusDeferred) {
					// Preserve the existing row. The FBS operation that displaced
					// this probe will publish the authoritative result.
					continue
				}

				if err != nil {
					s.statusStore.RecordFailure(
						tool,
						safeOutput,
						err,
						revision,
					)
					continue
				}

				s.statusStore.RecordSuccess(
					tool,
					switchStatus.Output,
					revision,
				)
			}
		}()
	}

sendJobs:
	for _, tool := range cfg.Tools {
		if !tool.Enabled {
			continue
		}

		select {
		case jobs <- tool:
		case <-ctx.Done():
			break sendJobs
		}
	}

	close(jobs)
	workers.Wait()

	return ctx.Err()
}

func adminToolStatus(tool config.Tool) ToolStatus {
	return ToolStatus{
		InterlockName: tool.InterlockName,
		IP:            tool.IP,
		Protocol:      config.ToolProtocol(tool),
		Port:          tool.Port,
		SwitchID:      tool.SwitchID,
		Enabled:       tool.Enabled,
	}
}

func (s *Server) handleRestart(
	w http.ResponseWriter,
	r *http.Request,
) {
	setAPINoStoreHeaders(w)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{
		"restart_requested": true,
	})

	go func() {
		timer := time.NewTimer(restartDelay)
		defer timer.Stop()

		<-timer.C

		select {
		case s.restartRequested <- struct{}{}:
		default:
		}
	}()
}

func decodeSingleJSON(
	w http.ResponseWriter,
	r *http.Request,
	maxBytes int64,
	destination any,
) error {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxBytes,
	)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var trailing any

	err := decoder.Decode(&trailing)

	switch {
	case errors.Is(err, io.EOF):
		return nil

	case err == nil:
		return errors.New(
			"request body must contain exactly one JSON value",
		)

	default:
		return fmt.Errorf(
			"invalid trailing data: %w",
			err,
		)
	}
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	value any,
) {
	var buffer bytes.Buffer

	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(true)

	if err := encoder.Encode(value); err != nil {
		log.Printf(
			"failed to encode JSON response: %v",
			err,
		)

		http.Error(
			w,
			"failed to encode response",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json; charset=utf-8",
	)
	w.WriteHeader(status)

	if _, err := w.Write(buffer.Bytes()); err != nil {
		log.Printf(
			"failed to write JSON response: %v",
			err,
		)
	}
}

func setAPINoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set(
		"Cache-Control",
		"no-store, max-age=0",
	)
	w.Header().Set("Pragma", "no-cache")
}

func (s *Server) crossSiteProtectionMiddleware(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if strings.EqualFold(
				r.Header.Get("Sec-Fetch-Site"),
				"cross-site",
			) {
				http.Error(
					w,
					"cross-site request rejected",
					http.StatusForbidden,
				)
				return
			}

			origin := strings.TrimSpace(
				r.Header.Get("Origin"),
			)

			if origin != "" &&
				!originMatchesRequest(origin, r) {
				http.Error(
					w,
					"request origin rejected",
					http.StatusForbidden,
				)
				return
			}

			next.ServeHTTP(w, r)
		},
	)
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet,
		http.MethodHead,
		http.MethodOptions:
		return true

	default:
		return false
	}
}

func originMatchesRequest(
	origin string,
	r *http.Request,
) bool {
	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if parsedOrigin.Scheme != "http" &&
		parsedOrigin.Scheme != "https" {
		return false
	}

	return strings.EqualFold(
		parsedOrigin.Host,
		r.Host,
	)
}

func (s *Server) securityHeadersMiddleware(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(
				"X-Content-Type-Options",
				"nosniff",
			)
			w.Header().Set(
				"X-Frame-Options",
				"DENY",
			)
			w.Header().Set(
				"Referrer-Policy",
				"no-referrer",
			)
			w.Header().Set(
				"Permissions-Policy",
				"camera=(), microphone=(), geolocation=()",
			)
			w.Header().Set(
				"Content-Security-Policy",
				"default-src 'self'; "+
					"base-uri 'none'; "+
					"object-src 'none'; "+
					"frame-ancestors 'none'; "+
					"form-action 'self'; "+
					"img-src 'self' data:; "+
					"style-src 'self' 'unsafe-inline'; "+
					"script-src 'self' 'unsafe-inline'",
			)

			next.ServeHTTP(w, r)
		},
	)
}

func isLoopbackAdminAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
