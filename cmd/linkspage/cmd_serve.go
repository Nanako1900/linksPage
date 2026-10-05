package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/httpapi"
	"github.com/Nanako1900/linksPage/internal/netx"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store"
	"github.com/Nanako1900/linksPage/internal/webui"
)

func cmdServe(ctx context.Context, args []string, stderr io.Writer) int {
	if !noArgs("serve", args, stderr) {
		return 2
	}
	loaded, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	logger := newLogger(stderr, loaded.Config.Log)
	logWarnings(logger, loaded.Warnings)
	if err := serve(ctx, loaded.Config, logger, listenTCP); err != nil {
		logger.Error("server stopped with error", slog.Any("error", err))
		return 1
	}
	return 0
}

// listenFunc opens the server listener for addr.
type listenFunc func(ctx context.Context, addr string) (net.Listener, error)

func listenTCP(ctx context.Context, addr string) (net.Listener, error) {
	return (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
}

// server is everything serve needs once the components are built.
type server struct {
	cfg     *config.Config
	logger  *slog.Logger
	pool    *pgxpool.Pool
	holder  *site.Holder
	ready   *httpapi.Readiness
	app     *app
	handler http.Handler
}

func serve(ctx context.Context, cfg *config.Config, logger *slog.Logger, listen listenFunc) error {
	logger.Info("starting linkspage", slog.String("version", version), slog.String("commit", commit),
		slog.String("base_url", cfg.BaseURL), slog.String("addr", cfg.Server.Addr))
	cfg, err := prepareDataDir(cfg, logger)
	if err != nil {
		return err
	}
	pool, err := store.Connect(ctx, cfg.DB)
	if err != nil {
		return err
	}
	s := &server{
		cfg: cfg, logger: logger, pool: pool, holder: site.NewHolder(site.DefaultSnapshot()),
		ready: httpapi.NewReadiness(pool.Ping),
	}
	if s.app, err = buildApp(cfg, logger, pool, s.holder); err != nil {
		pool.Close()
		return err
	}
	var ln net.Listener
	if s.handler, err = s.newHandler(); err == nil {
		if ln, err = listen(ctx, cfg.Server.Addr); err != nil {
			err = fmt.Errorf("listen on %s: %w", cfg.Server.Addr, err)
		}
	}
	if err != nil {
		pool.Close()
		return joinClose(err, s.app.mstore)
	}
	return s.run(ctx, ln)
}

// prepareDataDir checks data_dir and makes sure a secret key exists.
func prepareDataDir(cfg *config.Config, logger *slog.Logger) (*config.Config, error) {
	if err := probeDataDir(cfg.DataDir); err != nil {
		return nil, err
	}
	cfg, generated, err := cfg.EnsureSecretKey()
	if err != nil {
		return nil, fmt.Errorf("secret key: %w", err)
	}
	if generated {
		logger.Info("generated a new secret key", slog.String("path", filepath.Join(cfg.DataDir, config.SecretKeyFileName)))
	}
	return cfg, nil
}

// newHandler builds the root HTTP handler.
func (s *server) newHandler() (http.Handler, error) {
	assets, err := webui.LoadAssets(webui.DistFS())
	if err != nil {
		return nil, fmt.Errorf("frontend assets: %w", err)
	}
	web, err := webui.NewRenderer(webui.Options{
		Assets: assets, BaseURL: s.cfg.BaseURL, AppVersion: version, Logger: s.logger, Snapshot: s.holder.Current,
	})
	if err != nil {
		return nil, err
	}
	trusted, err := netx.ParseTrustedProxies(s.cfg.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("trusted_proxies: %w", err)
	}
	deps := httpapi.Deps{
		BaseURL: s.cfg.BaseURL, Version: version, Logger: s.logger, Ready: s.ready, Snapshots: s.holder,
		Web: web, Resolver: netx.NewResolver(trusted, s.cfg.ClientIPHeader), HSTS: s.cfg.HSTS,
		ProxyAuth: s.cfg.Edge.ProxyAuth.Reveal(),
	}
	s.app.handlers.apply(&deps)
	return httpapi.NewHandler(deps)
}

// run serves until ctx ends, then shuts down in the documented order
// (doc 4.11): readiness 503 → HTTP shutdown → startup → scheduler →
// media store → database pool.
func (s *server) run(ctx context.Context, ln net.Listener) error {
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	sched := newBackgroundTask("scheduler", s.logger)
	runner := newStartupRunner(dbStartupSteps(s.cfg, s.pool, s.logger, s.app, s.holder), s.holder, s.ready, s.logger)
	runner.onReady = func() { sched.start(runCtx, s.app.sched.Run) }
	startupDone := make(chan struct{})
	go func() {
		defer close(startupDone)
		runner.run(runCtx, cancel)
	}()

	srv := httpapi.NewServer(s.cfg.Server.Addr, s.handler, s.logger)
	s.logger.Info("listening", slog.String("addr", ln.Addr().String()))
	err := httpapi.Serve(runCtx, srv, ln, s.ready, s.logger,
		// Stop the startup goroutine before the pool it uses is closed; it
		// may still start the scheduler, which is stopped next.
		httpapi.ShutdownStep{Name: "startup", Run: func(sctx context.Context) error {
			cancel(nil)
			select {
			case <-startupDone:
				return nil
			case <-sctx.Done():
				return fmt.Errorf("startup did not stop: %w", sctx.Err())
			}
		}},
		httpapi.ShutdownStep{Name: "scheduler", Run: sched.stop},
		// M3 inserts "flush analytics" here.
		httpapi.ShutdownStep{Name: "media store", Run: func(context.Context) error { return s.app.mstore.Close() }},
		httpapi.ShutdownStep{Name: "database pool", Run: func(context.Context) error { s.pool.Close(); return nil }},
	)
	if cause := context.Cause(runCtx); errors.Is(cause, errFatalStartup) {
		return errors.Join(cause, err)
	}
	return err
}

// probeDataDir verifies data_dir is writable.
func probeDataDir(dir string) error {
	f, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("data_dir %s is not writable (%w); fix with: chown -R 65532:65532 ./data", dir, err)
		}
		return fmt.Errorf("data_dir %s: %w", dir, err)
	}
	name := f.Name()
	return errors.Join(f.Close(), os.Remove(name))
}
