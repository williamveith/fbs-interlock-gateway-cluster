package status

import (
	"sync"
	"sync/atomic"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
)

const notYetRefreshedError = "status not yet refreshed"

// ToolStatus is the shared in-memory representation returned by the admin API.
// revision is intentionally private so it is never serialized to the browser.
type ToolStatus struct {
	InterlockName string `json:"interlock_name"`
	IP            string `json:"ip"`
	Protocol      string `json:"protocol"`
	Port          int    `json:"port"`
	SwitchID      int    `json:"switch_id"`
	Enabled       bool   `json:"enabled"`
	Connected     bool   `json:"connected"`
	Output        bool   `json:"output"`
	Error         string `json:"error,omitempty"`

	revision uint64
}

// Store holds the latest known state for every configured tool. Tool listener
// ports are used as keys because configuration validation makes them unique and
// the admin frontend already uses them as stable row identifiers.
type Store struct {
	mu       sync.RWMutex
	statuses map[int]ToolStatus
	order    []int
	next     atomic.Uint64
}

// New creates placeholder rows in configuration order. Enabled tools start in
// the configured safe state until normal FBS traffic or a manual refresh records
// a result. Disabled tools remain visible but do not report an error.
func New(cfg config.Config, safeOutput bool) *Store {
	store := &Store{
		statuses: make(map[int]ToolStatus, len(cfg.Tools)),
		order:    make([]int, 0, len(cfg.Tools)),
	}

	for _, tool := range cfg.Tools {
		row := toolStatus(tool)
		if tool.Enabled {
			row.Output = safeOutput
			row.Error = notYetRefreshedError
		}

		if _, exists := store.statuses[tool.Port]; !exists {
			store.order = append(store.order, tool.Port)
		}
		store.statuses[tool.Port] = row
	}

	return store
}

// NextRevision returns a monotonically increasing revision used to order
// concurrent updates. A result with an older revision cannot overwrite a newer
// result for the same tool.
func (s *Store) NextRevision() uint64 {
	return s.next.Add(1)
}

// RecordSuccess records a successful Shelly status or set operation.
func (s *Store) RecordSuccess(
	tool config.Tool,
	output bool,
	revision uint64,
) {
	row := toolStatus(tool)
	row.Connected = true
	row.Output = output
	s.Record(row, revision)
}

// RecordFailure records a failed Shelly operation using the configured safe
// output. A nil error is accepted defensively and leaves Error empty.
func (s *Store) RecordFailure(
	tool config.Tool,
	safeOutput bool,
	err error,
	revision uint64,
) {
	row := toolStatus(tool)
	row.Output = safeOutput
	if err != nil {
		row.Error = err.Error()
	}
	s.Record(row, revision)
}

// Record merges a complete status result into the store. It is used by the
// admin fleet refresh, while FBS handlers normally use RecordSuccess or
// RecordFailure.
func (s *Store) Record(row ToolStatus, revision uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.statuses[row.Port]
	if exists && revision < current.revision {
		return
	}

	if !exists {
		s.order = append(s.order, row.Port)
	}

	row.revision = revision
	s.statuses[row.Port] = row
}

// Snapshot returns an independent slice in configuration order.
func (s *Store) Snapshot() []ToolStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ToolStatus, 0, len(s.order))
	for _, port := range s.order {
		if row, exists := s.statuses[port]; exists {
			result = append(result, row)
		}
	}
	return result
}

func toolStatus(tool config.Tool) ToolStatus {
	return ToolStatus{
		InterlockName: tool.InterlockName,
		IP:            tool.IP,
		Protocol:      config.ToolProtocol(tool),
		Port:          tool.Port,
		SwitchID:      tool.SwitchID,
		Enabled:       tool.Enabled,
	}
}
