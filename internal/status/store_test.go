package status

import (
	"errors"
	"sync"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
)

func testConfig() config.Config {
	return config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "ENABLED-HTTP",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "DISABLED-HTTPS",
				IP:            "192.0.2.11",
				Protocol:      " HTTPS ",
				Port:          8082,
				SwitchID:      1,
				Enabled:       false,
			},
		},
	}
}

func TestNewCreatesOrderedPlaceholders(t *testing.T) {
	store := New(testConfig(), true)
	rows := store.Snapshot()

	if len(rows) != 2 {
		t.Fatalf("Snapshot length = %d, want 2", len(rows))
	}

	enabled := rows[0]
	if enabled.InterlockName != "ENABLED-HTTP" || enabled.Port != 8081 {
		t.Fatalf("first row = %#v, want enabled tool", enabled)
	}
	if enabled.Protocol != "http" {
		t.Fatalf("enabled protocol = %q, want http", enabled.Protocol)
	}
	if enabled.Connected {
		t.Fatal("enabled placeholder Connected = true, want false")
	}
	if !enabled.Output {
		t.Fatal("enabled placeholder Output = false, want safe output true")
	}
	if enabled.Error != notYetRefreshedError {
		t.Fatalf("enabled placeholder Error = %q, want %q", enabled.Error, notYetRefreshedError)
	}

	disabled := rows[1]
	if disabled.InterlockName != "DISABLED-HTTPS" || disabled.Port != 8082 {
		t.Fatalf("second row = %#v, want disabled tool", disabled)
	}
	if disabled.Protocol != "https" {
		t.Fatalf("disabled protocol = %q, want https", disabled.Protocol)
	}
	if disabled.Connected || disabled.Output || disabled.Error != "" {
		t.Fatalf("disabled placeholder = %#v, want zero runtime state", disabled)
	}
}

func TestRecordSuccessUpdatesOnlyMatchingTool(t *testing.T) {
	cfg := testConfig()
	store := New(cfg, false)
	revision := store.NextRevision()

	store.RecordSuccess(cfg.Tools[0], true, revision)
	rows := store.Snapshot()

	if !rows[0].Connected || !rows[0].Output || rows[0].Error != "" {
		t.Fatalf("successful row = %#v", rows[0])
	}
	if rows[1].Connected || rows[1].Output || rows[1].Error != "" {
		t.Fatalf("unrelated row changed = %#v", rows[1])
	}
}

func TestRecordFailureUsesSafeOutput(t *testing.T) {
	cfg := testConfig()
	store := New(cfg, false)

	store.RecordFailure(
		cfg.Tools[0],
		true,
		errors.New("Shelly unreachable"),
		store.NextRevision(),
	)

	row := store.Snapshot()[0]
	if row.Connected {
		t.Fatal("failure row Connected = true, want false")
	}
	if !row.Output {
		t.Fatal("failure row Output = false, want safe output true")
	}
	if row.Error != "Shelly unreachable" {
		t.Fatalf("failure row Error = %q", row.Error)
	}
}

func TestRecordRejectsOlderRevision(t *testing.T) {
	cfg := testConfig()
	store := New(cfg, false)

	newer := store.NextRevision()
	store.RecordSuccess(cfg.Tools[0], true, newer)

	store.Record(
		ToolStatus{
			InterlockName: cfg.Tools[0].InterlockName,
			IP:            cfg.Tools[0].IP,
			Protocol:      "http",
			Port:          cfg.Tools[0].Port,
			SwitchID:      cfg.Tools[0].SwitchID,
			Enabled:       true,
			Connected:     true,
			Output:        false,
		},
		newer-1,
	)

	row := store.Snapshot()[0]
	if !row.Output {
		t.Fatal("older revision overwrote newer output")
	}
}

func TestRecordAcceptsEqualAndNewerRevisions(t *testing.T) {
	cfg := testConfig()
	store := New(cfg, false)
	revision := store.NextRevision()

	store.RecordSuccess(cfg.Tools[0], true, revision)
	store.RecordSuccess(cfg.Tools[0], false, revision)
	if store.Snapshot()[0].Output {
		t.Fatal("equal revision was not accepted")
	}

	store.RecordSuccess(cfg.Tools[0], true, store.NextRevision())
	if !store.Snapshot()[0].Output {
		t.Fatal("newer revision was not accepted")
	}
}

func TestSnapshotIsIndependent(t *testing.T) {
	store := New(testConfig(), false)
	first := store.Snapshot()
	first[0].InterlockName = "CHANGED"
	first[0].Output = true

	second := store.Snapshot()
	if second[0].InterlockName == "CHANGED" || second[0].Output {
		t.Fatal("mutating snapshot changed store contents")
	}
}

func TestRecordAddsPreviouslyUnknownPortAtEnd(t *testing.T) {
	store := New(testConfig(), false)
	store.Record(
		ToolStatus{
			InterlockName: "NEW",
			IP:            "192.0.2.12",
			Protocol:      "http",
			Port:          8083,
			SwitchID:      0,
			Enabled:       true,
			Connected:     true,
		},
		store.NextRevision(),
	)

	rows := store.Snapshot()
	if len(rows) != 3 || rows[2].Port != 8083 {
		t.Fatalf("rows = %#v, want new port at end", rows)
	}
}

func TestConcurrentUpdatesAndSnapshots(t *testing.T) {
	cfg := testConfig()
	store := New(cfg, false)

	const goroutines = 32
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for worker := 0; worker < goroutines; worker++ {
		worker := worker
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				revision := store.NextRevision()
				store.RecordSuccess(
					cfg.Tools[0],
					(worker+i)%2 == 0,
					revision,
				)
				_ = store.Snapshot()
			}
		}()
	}

	wg.Wait()

	rows := store.Snapshot()
	if len(rows) != 2 {
		t.Fatalf("Snapshot length = %d, want 2", len(rows))
	}
	if !rows[0].Connected {
		t.Fatal("final updated row Connected = false, want true")
	}
}
