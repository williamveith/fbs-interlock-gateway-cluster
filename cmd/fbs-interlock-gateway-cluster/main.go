package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/configstore"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/gateway"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/updateauth"
	"gopkg.in/yaml.v3"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "verify-update-checksum" {
		checksum, err := updateauth.VerifyUpdateChecksumCommand(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(checksum)
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "config" {
		if err := runConfigCommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runGateway(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func runGateway(args []string) error {
	defaults, err := defaultPaths()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("fbs-interlock-gateway-cluster", flag.ContinueOnError)
	dbPath := flags.String("db", "", "path to authoritative SQLite configuration database")
	configPath := flags.String("config", defaults.yaml, "legacy config.yaml import path and generated rollback mirror")
	showVersion := flags.Bool("version", false, "print version and exit")
	adminAddr := flags.String("admin", "127.0.0.1:18090", "admin UI listen address; empty disables admin UI")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("fbs-interlock-gateway-cluster version=%s commit=%s date=%s\n", version, commit, date)
		return nil
	}

	resolvedDBPath := resolveRuntimeDBPath(*dbPath, *configPath)
	store, err := configstore.Open(resolvedDBPath, configstore.Options{MirrorPath: *configPath})
	if err != nil {
		return fmt.Errorf("open configuration database %q: %w", resolvedDBPath, err)
	}
	defer store.Close()

	cfg, migrated, err := loadOrMigrate(store, *configPath)
	if err != nil {
		return err
	}
	if migrated {
		log.Printf("imported legacy YAML configuration %s into SQLite database %s; SQLite is now authoritative", *configPath, store.Path())
	}

	log.Printf(
		"fbs-interlock-gateway-cluster version=%s commit=%s date=%s db=%s legacy_config=%s",
		version,
		commit,
		date,
		store.Path(),
		*configPath,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := gateway.NewWithStore(cfg, store, *adminAddr)
	return app.Run(ctx)
}

func loadOrMigrate(store *configstore.Store, legacyConfigPath string) (config.Config, bool, error) {
	cfg, err := store.Load()
	if err == nil {
		return cfg, false, nil
	}
	if !errors.Is(err, configstore.ErrUninitialized) {
		return config.Config{}, false, fmt.Errorf("load configuration database %q: %w", store.Path(), err)
	}

	legacyCfg, err := config.Load(legacyConfigPath)
	if err != nil {
		return config.Config{}, false, fmt.Errorf(
			"configuration database %q is uninitialized and legacy config %q could not be loaded: %w",
			store.Path(),
			legacyConfigPath,
			err,
		)
	}
	config.ApplyDefaults(&legacyCfg)
	if err := config.Validate(legacyCfg); err != nil {
		return config.Config{}, false, fmt.Errorf("legacy config %q is invalid: %w", legacyConfigPath, err)
	}

	// Do not rewrite the human-authored legacy YAML during first migration. It
	// remains an immediate rollback source. Subsequent Admin saves generate a
	// compatibility mirror and move the prior YAML to config.yaml.bak.
	if err := store.Initialize(legacyCfg); err != nil {
		return config.Config{}, false, fmt.Errorf("import legacy config into SQLite: %w", err)
	}

	cfg, err = store.Load()
	if err != nil {
		return config.Config{}, false, fmt.Errorf("reload imported SQLite configuration: %w", err)
	}
	return cfg, true, nil
}

type paths struct {
	db   string
	yaml string
}

const dbPathEnv = "FBS_GATEWAY_DB_PATH"

func resolveRuntimeDBPath(flagValue, configPath string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(dbPathEnv)); value != "" {
		return value
	}

	return filepath.Join(filepath.Dir(configPath), "gateway.sqlite3")
}

func defaultPaths() (paths, error) {
	exePath, err := os.Executable()
	if err != nil {
		return paths{}, fmt.Errorf("get executable path: %w", err)
	}
	dir := filepath.Dir(exePath)
	dbPath := strings.TrimSpace(os.Getenv(dbPathEnv))
	if dbPath == "" {
		dbPath = filepath.Join(dir, "gateway.sqlite3")
	}
	return paths{
		db:   dbPath,
		yaml: filepath.Join(dir, "config.yaml"),
	}, nil
}

func runConfigCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fbs-interlock-gateway-cluster config <export|import> [flags]")
	}

	switch args[0] {
	case "export":
		return runConfigExport(args[1:])
	case "import":
		return runConfigImport(args[1:])
	default:
		return fmt.Errorf("unknown config command %q; expected export or import", args[0])
	}
}

func runConfigExport(args []string) error {
	defaults, err := defaultPaths()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("config export", flag.ContinueOnError)
	dbPath := flags.String("db", defaults.db, "path to SQLite configuration database")
	output := flags.String("output", "-", "output YAML path, or - for stdout")
	redactSecrets := flags.Bool("redact-secrets", false, "replace stored passwords in the export")
	if err := flags.Parse(args); err != nil {
		return err
	}

	store, err := configstore.Open(*dbPath, configstore.Options{})
	if err != nil {
		return fmt.Errorf("open configuration database: %w", err)
	}
	defer store.Close()

	cfg, err := store.Load()
	if err != nil {
		return fmt.Errorf("load configuration database: %w", err)
	}
	if *redactSecrets {
		redactConfigSecrets(&cfg)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal YAML export: %w", err)
	}
	data = append([]byte("# Exported from the authoritative SQLite configuration database.\n"), data...)

	if *output == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return writePrivateAtomic(*output, data)
}

func runConfigImport(args []string) error {
	defaults, err := defaultPaths()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("config import", flag.ContinueOnError)
	dbPath := flags.String("db", defaults.db, "path to SQLite configuration database")
	input := flags.String("input", "", "YAML configuration to import")
	mirrorPath := flags.String("mirror-config", "", "optional generated YAML compatibility mirror path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" {
		return errors.New("config import requires -input <config.yaml>")
	}

	cfg, err := config.Load(*input)
	if err != nil {
		return fmt.Errorf("load import file %q: %w", *input, err)
	}
	config.ApplyDefaults(&cfg)
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid import file %q: %w", *input, err)
	}

	store, err := configstore.Open(*dbPath, configstore.Options{MirrorPath: *mirrorPath})
	if err != nil {
		return fmt.Errorf("open configuration database: %w", err)
	}
	defer store.Close()

	if err := store.Replace(cfg); err != nil {
		return fmt.Errorf("import configuration: %w", err)
	}
	fmt.Printf("Imported %d tool(s) into %s\n", len(cfg.Tools), store.Path())
	return nil
}

func redactConfigSecrets(cfg *config.Config) {
	for index := range cfg.Tools {
		if cfg.Tools[index].Password != nil {
			redacted := "REDACTED"
			cfg.Tools[index].Password = &redacted
		}
	}
}

func writePrivateAtomic(path string, data []byte) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(absolutePath), ".config-export.*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary export: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temporary export: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temporary export: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temporary export: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary export: %w", err)
	}
	if err := os.Rename(tempPath, absolutePath); err != nil {
		return fmt.Errorf("replace export file: %w", err)
	}
	return nil
}
