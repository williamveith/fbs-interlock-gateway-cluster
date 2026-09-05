package configstore

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var ErrUninitialized = errors.New("configuration database is uninitialized")

type Options struct {
	MirrorPath string
}

type Store struct {
	db         *sql.DB
	path       string
	mirrorPath string
}

func Open(path string, opts Options) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", absolutePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{
		db:         db,
		path:       absolutePath,
		mirrorPath: opts.MirrorPath,
	}

	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.QuickCheck(); err != nil {
		_ = db.Close()
		return nil, err
	}

	_ = os.Chmod(absolutePath, 0600)

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) configure() error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = DELETE",
		"PRAGMA synchronous = FULL",
	}
	for _, statement := range pragmas {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("configure sqlite (%s): %w", statement, err)
		}
	}
	return nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("configuration database schema %d is newer than supported schema %d", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if version == 0 {
		if _, err := tx.Exec(schemaV1); err != nil {
			return fmt.Errorf("create sqlite schema v1: %w", err)
		}
		if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
			return fmt.Errorf("set sqlite schema version: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}

const schemaV1 = `
CREATE TABLE gateway_settings (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    bind TEXT NOT NULL,
    timeout_ms INTEGER NOT NULL CHECK (timeout_ms > 0),
    safe_state_on_error TEXT NOT NULL CHECK (lower(safe_state_on_error) IN ('on', 'off')),
    server_ca_file TEXT NOT NULL,
    client_cert_file TEXT NOT NULL,
    client_key_file TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE tools (
    id INTEGER PRIMARY KEY,
    sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
    interlock_name TEXT NOT NULL
        CHECK (
            interlock_name = trim(interlock_name)
            AND length(interlock_name) BETWEEN 1 AND 16
        ),
    ip TEXT NOT NULL CHECK (length(trim(ip)) > 0),
    protocol TEXT NOT NULL CHECK (protocol IN ('http', 'https')),
    port INTEGER NOT NULL CHECK (port BETWEEN 8081 AND 8981),
    switch_id INTEGER NOT NULL CHECK (switch_id >= 0),
    username TEXT,
    password TEXT CHECK (
        password IS NULL OR (
            length(password) = 32
            AND password NOT GLOB '*[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz]*'
        )
    ),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(interlock_name),
    UNIQUE(ip),
    UNIQUE(port),
    UNIQUE(sort_order)
) STRICT;

CREATE INDEX tools_enabled_idx ON tools(enabled, sort_order);
`

func (s *Store) QuickCheck() error {
	var result string
	if err := s.db.QueryRow("PRAGMA quick_check(1)").Scan(&result); err != nil {
		return fmt.Errorf("run sqlite quick_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("sqlite quick_check failed: %s", result)
	}
	return nil
}

func (s *Store) Load() (config.Config, error) {
	var cfg config.Config
	var updatedAt string

	err := s.db.QueryRow(`
SELECT bind, timeout_ms, safe_state_on_error,
       server_ca_file, client_cert_file, client_key_file, updated_at
FROM gateway_settings
WHERE singleton = 1
`).Scan(
		&cfg.Bind,
		&cfg.Defaults.TimeoutMS,
		&cfg.Defaults.SafeStateOnError,
		&cfg.Defaults.ShellyTLS.ServerCAFile,
		&cfg.Defaults.ShellyTLS.ClientCertFile,
		&cfg.Defaults.ShellyTLS.ClientKeyFile,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return config.Config{}, ErrUninitialized
	}
	if err != nil {
		return config.Config{}, fmt.Errorf("load gateway settings: %w", err)
	}

	rows, err := s.db.Query(`
SELECT interlock_name, ip, protocol, port, switch_id,
       username, password, enabled
FROM tools
ORDER BY sort_order
`)
	if err != nil {
		return config.Config{}, fmt.Errorf("query tools: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tool config.Tool
		var username, password sql.NullString
		var enabled int
		if err := rows.Scan(
			&tool.InterlockName,
			&tool.IP,
			&tool.Protocol,
			&tool.Port,
			&tool.SwitchID,
			&username,
			&password,
			&enabled,
		); err != nil {
			return config.Config{}, fmt.Errorf("scan tool: %w", err)
		}
		tool.Username = nullStringPtr(username)
		tool.Password = nullStringPtr(password)
		tool.Enabled = enabled != 0
		cfg.Tools = append(cfg.Tools, tool)
	}
	if err := rows.Err(); err != nil {
		return config.Config{}, fmt.Errorf("iterate tools: %w", err)
	}

	config.ApplyDefaults(&cfg)
	if err := config.Validate(cfg); err != nil {
		return config.Config{}, fmt.Errorf("database contains invalid configuration: %w", err)
	}
	return config.Clone(cfg), nil
}

type existingTool struct {
	id        int64
	port      int
	name      string
	createdAt string
}

func (s *Store) Initialize(cfg config.Config) error {
	return s.replace(cfg, false)
}

func (s *Store) Replace(cfg config.Config) error {
	return s.replace(cfg, true)
}

func (s *Store) replace(cfg config.Config, writeMirror bool) error {
	config.ApplyDefaults(&cfg)
	cfg = config.Clone(cfg)
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin config transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := loadExistingTools(tx)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
INSERT INTO gateway_settings (
    singleton, bind, timeout_ms, safe_state_on_error,
    server_ca_file, client_cert_file, client_key_file, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(singleton) DO UPDATE SET
    bind = excluded.bind,
    timeout_ms = excluded.timeout_ms,
    safe_state_on_error = excluded.safe_state_on_error,
    server_ca_file = excluded.server_ca_file,
    client_cert_file = excluded.client_cert_file,
    client_key_file = excluded.client_key_file,
    updated_at = excluded.updated_at
`,
		cfg.Bind,
		cfg.Defaults.TimeoutMS,
		strings.ToLower(strings.TrimSpace(cfg.Defaults.SafeStateOnError)),
		cfg.Defaults.ShellyTLS.ServerCAFile,
		cfg.Defaults.ShellyTLS.ClientCertFile,
		cfg.Defaults.ShellyTLS.ClientKeyFile,
		now,
	); err != nil {
		return fmt.Errorf("write gateway settings: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM tools"); err != nil {
		return fmt.Errorf("replace tools: %w", err)
	}

	byPort := make(map[int]existingTool, len(existing))
	nameCount := make(map[string]int, len(existing))
	byName := make(map[string]existingTool, len(existing))
	var nextID int64 = 1
	for _, item := range existing {
		byPort[item.port] = item
		key := strings.ToLower(strings.TrimSpace(item.name))
		nameCount[key]++
		byName[key] = item
		if item.id >= nextID {
			nextID = item.id + 1
		}
	}

	for index, tool := range cfg.Tools {
		createdAt := now
		var preservedID *int64
		key := strings.ToLower(strings.TrimSpace(tool.InterlockName))
		if nameCount[key] == 1 {
			if item, ok := byName[key]; ok {
				id := item.id
				preservedID = &id
				createdAt = item.createdAt
			}
		}
		if preservedID == nil {
			if item, ok := byPort[tool.Port]; ok {
				id := item.id
				preservedID = &id
				createdAt = item.createdAt
			}
		}

		if preservedID == nil {
			id := nextID
			nextID++
			preservedID = &id
		}

		protocol := config.ToolProtocol(tool)
		if err := insertTool(tx, *preservedID, index, tool, protocol, createdAt, now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit config transaction: %w", err)
	}

	if writeMirror && s.mirrorPath != "" {
		if err := writeGeneratedMirror(s.mirrorPath, cfg); err != nil {
			// The authoritative transaction is already committed. A failed rollback
			// mirror must never make the live configuration appear uncommitted.
			log.Printf("warning: SQLite config committed but compatibility YAML mirror could not be written: %v", err)
		}
	}

	return nil
}

func loadExistingTools(tx *sql.Tx) ([]existingTool, error) {
	rows, err := tx.Query(`SELECT id, port, interlock_name, created_at FROM tools`)
	if err != nil {
		return nil, fmt.Errorf("load existing tool identities: %w", err)
	}
	defer rows.Close()

	var result []existingTool
	for rows.Next() {
		var item existingTool
		if err := rows.Scan(&item.id, &item.port, &item.name, &item.createdAt); err != nil {
			return nil, fmt.Errorf("scan existing tool identity: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing tool identities: %w", err)
	}
	return result, nil
}

func insertTool(
	tx *sql.Tx,
	id int64,
	sortOrder int,
	tool config.Tool,
	protocol string,
	createdAt string,
	updatedAt string,
) error {
	_, err := tx.Exec(`
INSERT INTO tools (
    id, sort_order, interlock_name, ip, protocol, port, switch_id,
    username, password, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		id,
		sortOrder,
		strings.TrimSpace(tool.InterlockName),
		strings.TrimSpace(tool.IP),
		protocol,
		tool.Port,
		tool.SwitchID,
		stringPtrValue(tool.Username),
		stringPtrValue(tool.Password),
		boolInt(tool.Enabled),
		createdAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert tool %q: %w", tool.InterlockName, err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func stringPtrValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func writeGeneratedMirror(path string, cfg config.Config) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve mirror path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
		return fmt.Errorf("create mirror directory: %w", err)
	}

	body, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal compatibility YAML: %w", err)
	}
	header := []byte("# GENERATED FILE. SQLite gateway.sqlite3 is authoritative.\n# Manual changes to this file are ignored after database initialization.\n")
	data := append(header, body...)

	if oldData, err := os.ReadFile(absolutePath); err == nil {
		if err := os.WriteFile(absolutePath+".bak", oldData, 0640); err != nil {
			return fmt.Errorf("write compatibility YAML backup: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing compatibility YAML: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(absolutePath), ".config.yaml.*.tmp")
	if err != nil {
		return fmt.Errorf("create compatibility YAML temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if err := tempFile.Chmod(0640); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod compatibility YAML temp file: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write compatibility YAML temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync compatibility YAML temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close compatibility YAML temp file: %w", err)
	}
	if err := os.Rename(tempPath, absolutePath); err != nil {
		return fmt.Errorf("replace compatibility YAML: %w", err)
	}
	return nil
}
