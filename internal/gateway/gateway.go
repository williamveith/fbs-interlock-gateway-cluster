package gateway

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/admin"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/config"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/fbs"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/process"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/shelly"
	"github.com/williamveith/fbs-interlock-gateway-cluster/internal/status"
)

type ConfigPersistence interface {
	Replace(config.Config) error
}

type fileConfigPersistence struct {
	path string
}

func (p fileConfigPersistence) Replace(cfg config.Config) error {
	return config.WriteAtomic(p.path, cfg)
}

type Gateway struct {
	mu sync.RWMutex

	cfg config.Config

	persistence ConfigPersistence

	adminAddr string

	safeOutput bool

	shelly *shelly.Client

	statusStore *status.Store

	initErr error
}

func New(
	cfg config.Config,
	configPath string,
	adminAddr string,
) *Gateway {
	return NewWithStore(cfg, fileConfigPersistence{path: configPath}, adminAddr)
}

func NewWithStore(
	cfg config.Config,
	persistence ConfigPersistence,
	adminAddr string,
) *Gateway {
	config.ApplyDefaults(&cfg)

	cfg = config.Clone(cfg)

	shellyClient, err := shelly.NewClientWithTLS(
		time.Duration(cfg.Defaults.TimeoutMS)*time.Millisecond,
		cfg.Defaults.ShellyTLS,
	)

	return &Gateway{
		cfg:         cfg,
		persistence: persistence,
		adminAddr:   adminAddr,
		safeOutput:  config.SafeOutput(cfg),
		shelly:      shellyClient,
		statusStore: status.New(cfg, config.SafeOutput(cfg)),
		initErr:     err,
	}
}

func (g *Gateway) Run(ctx context.Context) error {
	g.mu.RLock()

	cfg := g.cfg

	adminAddr := g.adminAddr

	initErr := g.initErr

	g.mu.RUnlock()

	if initErr != nil {
		return fmt.Errorf("initialize Shelly client: %w", initErr)
	}

	if err := config.ValidateEnabledTools(cfg); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	restartRequested := make(chan struct{}, 1)

	errCh := make(chan error, len(cfg.Tools)+1)

	var wg sync.WaitGroup

	if adminAddr != "" {
		adminServer := admin.New(adminAddr, g, g.shelly, g.statusStore, restartRequested)

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := adminServer.Run(runCtx); err != nil {
				errCh <- err
			}
		}()
	} else {
		log.Printf("admin UI disabled")
	}

	fbsServer := fbs.NewServer(cfg.Bind, g.SafeOutput(), g.shelly, g.statusStore)

	enabledCount := 0

	for _, tool := range cfg.Tools {
		if !tool.Enabled {
			log.Printf("tool=%s port=%d disabled; skipping", tool.InterlockName, tool.Port)
			continue
		}

		enabledCount++
		if err := process.KillPort(tool.Port); err != nil {
			log.Printf("warning: failed to clear port %d for tool=%s: %v", tool.Port, tool.InterlockName, err)
		}

		tool := tool

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := fbsServer.RunToolServer(runCtx, tool); err != nil {
				errCh <- err
			}
		}()
	}

	log.Printf("fbs-interlock-gateway-cluster started with %d enabled tool(s)", enabledCount)

	select {
	case <-ctx.Done():
		log.Println("shutdown requested")
	case <-restartRequested:
		log.Println("restart requested from admin UI")
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}

	cancel()
	wg.Wait()
	log.Println("shutdown complete")
	return nil
}

func (g *Gateway) ConfigSnapshot() config.Config {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return config.Clone(g.cfg)
}

func (g *Gateway) SafeOutput() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.safeOutput
}

func (g *Gateway) UpdateConfig(newCfg config.Config) error {
	config.ApplyDefaults(&newCfg)
	newCfg = config.Clone(newCfg)

	if err := config.Validate(newCfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if g.persistence == nil {
		return fmt.Errorf("failed to write config: no configuration persistence store configured")
	}
	if err := g.persistence.Replace(newCfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	g.mu.Lock()
	g.cfg = newCfg
	g.safeOutput = config.SafeOutput(newCfg)
	g.mu.Unlock()

	return nil
}
