package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/app"
	"github.com/example/dddcqrs/internal/app/bus"
	audiocmd "github.com/example/dddcqrs/internal/app/command/audiolib"
	"github.com/example/dddcqrs/internal/infrastructure/config"
	miniostore "github.com/example/dddcqrs/internal/infrastructure/storage/minio"
	httpiface "github.com/example/dddcqrs/internal/interfaces/http"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
	"github.com/example/dddcqrs/internal/platform/balances"
	"github.com/example/dddcqrs/internal/platform/metrics"
	"github.com/example/dddcqrs/internal/platform/postgres"
	"github.com/example/dddcqrs/internal/platform/metricsticker"
	"github.com/example/dddcqrs/internal/platform/redis"
	"github.com/example/dddcqrs/internal/platform/scheduler"
	"github.com/example/dddcqrs/internal/domain/uploadmetrics"
)

// balancesAdapter satisfies httpiface.BalancesProvider so the http package
// doesn't need to import the balances package.
type balancesAdapter struct{ c *balances.Client }

func (a balancesAdapter) Snapshot(ctx context.Context) any { return a.c.Snapshot(ctx) }

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	i := app.Build(cfg)
	log := do.MustInvoke[*slog.Logger](i)
	reg := do.MustInvoke[*bus.Registry](i)
	store := do.MustInvoke[*miniostore.Store](i)
	met := do.MustInvoke[*metrics.Metrics](i)

	wp := do.MustInvoke[*postgres.WritePool](i)
	rc := do.MustInvoke[*redis.Client](i)
	ready := func(ctx context.Context) error {
		if err := wp.Pool.Ping(ctx); err != nil {
			return err
		}
		return rc.HealthCheck()
	}
	srv := &http.Server{
		Addr: ":" + cfg.HTTPPort,
		Handler: httpiface.NewServer(reg, store, met.Handler(), cfg.APIKey, ready,
			balancesAdapter{c: do.MustInvoke[*balances.Client](i)}).
			WithUploads(wp.Pool, cfg.MinIOBucket).
			WithMusicLibrary(store.Client()).
			WithAudioLibrary(httpiface.NewAudioLibHandler(reg, store,
				do.MustInvoke[audiocmd.PixabaySearcher](i))).
			WithFirefoxLogin(httpiface.FirefoxLoginConfig{
				HostProfilesDir:  os.Getenv("FIREFOX_PROFILES_DIR"),
				WorkerMountPoint: "/profiles",
				Image:            "jlesage/firefox:latest",
			}).
			Router(),
	}
	// In-process scheduler tick loop. Single goroutine — runs alongside the
	// HTTP server, cancelled by the same SIGINT/SIGTERM path.
	schedCtx, cancelSched := context.WithCancel(context.Background())
	schRunner := scheduler.New(do.MustInvoke[uow.Manager](i), rc.Client, log)
	go schRunner.Run(schedCtx)

	// Metrics ticker: polls upload counts at configurable cadence.
	mtRunner := metricsticker.New(
		do.MustInvoke[uow.Manager](i),
		reg,
		do.MustInvoke[uploadmetrics.Fetcher](i),
		rc.Client,
		log,
	)
	go mtRunner.Run(schedCtx)

	go func() {
		log.Info("http listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancelSched()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = i.Shutdown()
}
