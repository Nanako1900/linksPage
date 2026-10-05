package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/golink"
	"github.com/Nanako1900/linksPage/internal/httpapi"
	"github.com/Nanako1900/linksPage/internal/imgproxy"
	"github.com/Nanako1900/linksPage/internal/jobs"
	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/provider/discord"
	"github.com/Nanako1900/linksPage/internal/provider/kook"
	"github.com/Nanako1900/linksPage/internal/seed"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Page rebuild cadence (contract 2.1 step 11).
const (
	pageBoundaryInterval = 5 * time.Second
	pageRebuildInterval  = time.Minute
)

// app holds the M1 components built from the configuration (contract 2.1).
type app struct {
	builder  *site.Builder
	importer *seed.Importer
	sched    *jobs.Scheduler
	mstore   *media.LocalStore
	handlers publicHandlers
}

// publicHandlers are the non-HTML public routes mounted by httpapi.
type publicHandlers struct {
	favicon, robots, manifest, goLink, uploads, imgProxy, qr http.Handler
}

// providerParts are the outbound provider client and its consumers.
type providerParts struct {
	client    *http.Client
	registry  *provider.Registry
	live      *provider.LiveStore
	catalog   *provider.Catalog
	registrar *imgproxy.Registrar
}

// buildApp wires providers, media, the page builder, seed importer, jobs
// and handlers. holder receives every rebuilt snapshot.
func buildApp(cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, holder *site.Holder) (*app, error) {
	q := dbq.New(pool)
	pp, err := buildProviders(cfg, logger, q)
	if err != nil {
		return nil, err
	}
	sem := media.NewSemaphore(media.MemoryBudget)
	mstore, err := media.NewLocalStore(filepath.Join(cfg.DataDir, media.UploadsDir))
	if err != nil {
		return nil, fmt.Errorf("media store: %w", err)
	}
	a := &app{mstore: mstore}
	a.builder, err = site.NewBuilder(site.BuilderDeps{
		Queries: q, Live: pp.live, Platforms: pp.catalog,
		Assets: media.NewGenerator(mstore, sem), BaseURL: cfg.BaseURL, Logger: logger,
	})
	if err == nil {
		a.importer, err = seed.NewImporter(seed.Deps{
			DB: pool, Media: media.NewProcessor(mstore, sem),
			Logger: logger, MaxUploadBytes: cfg.Uploads.MaxBytes,
		})
	}
	if err == nil {
		a.sched, err = buildScheduler(pool, q, pp, a.builder, holder, logger)
	}
	if err == nil {
		a.handlers, err = buildHandlers(cfg, logger, q, pp, mstore, holder)
	}
	if err != nil {
		return nil, joinClose(err, mstore)
	}
	return a, nil
}

// buildProviders follows contract 2.1 steps 1–6.
func buildProviders(cfg *config.Config, logger *slog.Logger, q *dbq.Queries) (providerParts, error) {
	mediaKey, err := cfg.DeriveKey(config.KeyInfoMediaProxy, 32)
	if err != nil {
		return providerParts{}, fmt.Errorf("derive media proxy key: %w", err)
	}
	client, err := provider.NewHTTPClient(provider.ClientOptions{
		ProxyURL: cfg.Providers.HTTPProxy.Reveal(), UserAgent: provider.UserAgent(version),
	})
	if err != nil {
		return providerParts{}, fmt.Errorf("provider http client: %w", err)
	}
	registrar, err := imgproxy.NewRegistrar(q, mediaKey, imageHosts())
	if err != nil {
		return providerParts{}, fmt.Errorf("image registrar: %w", err)
	}
	dp, err := discord.NewProvider(discord.Options{
		APIBase: cfg.Providers.Discord.APIBase, Client: client,
		Images: registrar, Logger: logger,
	})
	if err != nil {
		return providerParts{}, fmt.Errorf("discord provider: %w", err)
	}
	kp, err := kook.NewProvider(kook.Options{APIBase: cfg.Providers.KOOK.APIBase, Client: client})
	if err != nil {
		return providerParts{}, fmt.Errorf("kook provider: %w", err)
	}
	registry, err := provider.NewRegistry(dp, kp)
	if err != nil {
		return providerParts{}, fmt.Errorf("provider registry: %w", err)
	}
	catalog, err := provider.LoadPresets()
	if err != nil {
		return providerParts{}, fmt.Errorf("platform presets: %w", err)
	}
	return providerParts{
		client: client, registry: registry, live: provider.NewLiveStore(),
		catalog: catalog, registrar: registrar,
	}, nil
}

// buildScheduler follows contract 2.1 step 11.
func buildScheduler(pool *pgxpool.Pool, q *dbq.Queries, pp providerParts, b *site.Builder,
	holder *site.Holder, logger *slog.Logger,
) (*jobs.Scheduler, error) {
	rebuild := func(ctx context.Context) error { return b.Rebuild(ctx, holder) }
	refresh, err := jobs.NewRefreshJob(jobs.RefreshDeps{
		Store: q, Registry: pp.registry, Live: pp.live,
		OnChange: func(ctx context.Context) {
			if err := rebuild(ctx); err != nil {
				logger.Warn("page rebuild after provider refresh failed", slog.Any("error", err))
			}
		}, Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("refresh job: %w", err)
	}
	sched, err := jobs.NewScheduler(jobs.NewPGLeaderLock(pool, jobs.LeaderLockKey), logger,
		refresh,
		jobs.Job{
			Name: "page-boundary", Interval: pageBoundaryInterval,
			Run: func(ctx context.Context) error { return b.RebuildIfDue(ctx, holder) },
		},
		jobs.Job{Name: "page-rebuild", Interval: pageRebuildInterval, Run: rebuild},
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler: %w", err)
	}
	return sched, nil
}

// buildHandlers follows contract 2.1 step 12.
func buildHandlers(cfg *config.Config, logger *slog.Logger, q *dbq.Queries, pp providerParts,
	mstore *media.LocalStore, holder *site.Holder,
) (publicHandlers, error) {
	goSrc, err := golink.NewDBSource(q, pp.catalog)
	if err != nil {
		return publicHandlers{}, fmt.Errorf("golink source: %w", err)
	}
	goHandler, err := golink.NewHandler(golink.HandlerOptions{
		Resolver: golink.NewResolver(goSrc, nil),
		Snapshot: holder.Current, BaseURL: cfg.BaseURL, Hook: golink.NoopClickHook{}, Logger: logger,
	})
	if err != nil {
		return publicHandlers{}, fmt.Errorf("golink handler: %w", err)
	}
	proxy, err := imgproxy.NewHandler(imgproxy.HandlerOptions{Store: q, Client: pp.client, Hosts: imageHosts(), Logger: logger})
	if err != nil {
		return publicHandlers{}, fmt.Errorf("image proxy handler: %w", err)
	}
	files := func() *media.SiteFiles { return holder.Current().Files }
	return publicHandlers{
		favicon:  media.SiteFilesHandler(media.FileFavicon, files),
		robots:   media.SiteFilesHandler(media.FileRobots, files),
		manifest: media.SiteFilesHandler(media.FileManifest, files),
		goLink:   goHandler,
		uploads:  media.UploadsHandler(mstore, q, logger),
		imgProxy: proxy,
		qr:       media.QRHandler(mstore, q, logger),
	}, nil
}

// apply copies the handlers into the HTTP dependencies.
func (h publicHandlers) apply(d *httpapi.Deps) {
	d.Favicon, d.Robots, d.Manifest = h.favicon, h.robots, h.manifest
	d.GoLink, d.Uploads, d.ImgProxy, d.QR = h.goLink, h.uploads, h.imgProxy, h.qr
}

// importSeed imports cfg.SeedFile when set (the importer logs the
// result). A broken seed file is fatal; other errors (database) are
// retried by the startup loop.
func (a *app) importSeed(cfg *config.Config) func(context.Context) error {
	if cfg.SeedFile == "" {
		return nil
	}
	return func(ctx context.Context) error {
		if _, err := a.importer.Import(ctx, cfg.SeedFile); err != nil {
			return fatalIf(fmt.Errorf("seed import: %w", err), seed.ErrInvalidSeed)
		}
		return nil
	}
}

// loadSnapshot builds and publishes the first full snapshot.
func (a *app) loadSnapshot(holder *site.Holder) func(context.Context) (*site.Snapshot, error) {
	return func(ctx context.Context) (*site.Snapshot, error) {
		if err := a.builder.Rebuild(ctx, holder); err != nil {
			return nil, err
		}
		return holder.Current(), nil
	}
}

type closer interface{ Close() error }

func joinClose(err error, c closer) error {
	if cerr := c.Close(); cerr != nil {
		return fmt.Errorf("%w (close: %w)", err, cerr)
	}
	return err
}

// imageHosts is the media proxy allow-list, shared by registration and
// by the /media/p handler (which re-checks every stored row).
func imageHosts() map[string]map[string][]string {
	return map[string]map[string][]string{discord.ProviderKind: discord.ImageHosts(), kook.ProviderKind: {}}
}
