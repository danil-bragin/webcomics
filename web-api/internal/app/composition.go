// Package app is the composition root. It builds the do injector, wires every
// layer, configures the bus middleware pipelines, and registers all command
// and query handlers. Entry points (http/grpc/consumer) ask the injector for
// the *bus.Registry and dispatch through it.
package app

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/app/command"
	audiocmd "github.com/example/dddcqrs/internal/app/command/audiolib"
	formatcmd "github.com/example/dddcqrs/internal/app/command/formats"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
	schcmd "github.com/example/dddcqrs/internal/app/command/scheduler"
	appmw "github.com/example/dddcqrs/internal/app/middleware"
	"github.com/example/dddcqrs/internal/app/query"
	audioq "github.com/example/dddcqrs/internal/app/query/audiolib"
	formatq "github.com/example/dddcqrs/internal/app/query/formats"
	pipeq "github.com/example/dddcqrs/internal/app/query/pipeline"
	projq "github.com/example/dddcqrs/internal/app/query/projects"
	schq "github.com/example/dddcqrs/internal/app/query/scheduler"
	"github.com/example/dddcqrs/internal/infrastructure/audiosource"
	"github.com/example/dddcqrs/internal/infrastructure/config"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/read"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
	"github.com/example/dddcqrs/internal/infrastructure/storage/minio"
	"github.com/example/dddcqrs/internal/platform/balances"
	"github.com/example/dddcqrs/internal/platform/logger"
	"github.com/example/dddcqrs/internal/platform/metrics"
	"github.com/example/dddcqrs/internal/platform/postgres"
	"github.com/example/dddcqrs/internal/platform/redis"
)

// Build constructs the injector and registers all providers shared by every
// entry point. It does NOT start servers — each cmd/* does that.
func Build(cfg *config.Config) *do.RootScope {
	i := do.New()

	do.ProvideValue(i, cfg)
	do.Provide(i, logger.New)

	// Platform: two pools (write=master, read=replica).
	do.Provide(i, postgres.NewWritePool)
	do.Provide(i, postgres.NewReadPool)
	do.Provide(i, redis.NewClient)

	// UoW manager bound to the WRITE pool.
	do.Provide(i, func(inj do.Injector) (uow.Manager, error) {
		wp := do.MustInvoke[*postgres.WritePool](inj)
		return uow.NewManager(wp.Pool), nil
	})

	// Read model bound to the READ pool.
	do.Provide(i, func(inj do.Injector) (query.ReadModel, error) {
		rp := do.MustInvoke[*postgres.ReadPool](inj)
		return read.NewModel(rp.Pool), nil
	})

	// Pipeline read model on the READ pool.
	do.Provide(i, func(inj do.Injector) (pipeq.ReadModel, error) {
		rp := do.MustInvoke[*postgres.ReadPool](inj)
		return read.NewPipelineModel(rp.Pool), nil
	})

	// Projects read model on the READ pool.
	do.Provide(i, func(inj do.Injector) (projq.ReadModel, error) {
		rp := do.MustInvoke[*postgres.ReadPool](inj)
		return read.NewProjectsModel(rp.Pool), nil
	})

	// Audio library read model on the READ pool.
	do.Provide(i, func(inj do.Injector) (audioq.ReadModel, error) {
		rp := do.MustInvoke[*postgres.ReadPool](inj)
		return read.NewAudioLibModel(rp.Pool), nil
	})

	// Formats read model on the READ pool.
	do.Provide(i, func(inj do.Injector) (formatq.ReadModel, error) {
		rp := do.MustInvoke[*postgres.ReadPool](inj)
		return read.NewFormatsModel(rp.Pool), nil
	})

	// Scheduler read model on the READ pool.
	do.Provide(i, func(inj do.Injector) (schq.ReadModel, error) {
		rp := do.MustInvoke[*postgres.ReadPool](inj)
		return read.NewSchedulerModel(rp), nil
	})

	// URL fetcher + Pixabay scraper for the audio library.
	do.Provide(i, func(inj do.Injector) (audiocmd.URLFetcher, error) {
		return audiosource.NewHTTPFetcher(), nil
	})
	do.Provide(i, func(inj do.Injector) (audiocmd.PixabaySearcher, error) {
		cfg := do.MustInvoke[*config.Config](inj)
		return audiosource.NewPixabayScraper(cfg.PixabayAPIKey), nil
	})

	// MinIO asset store.
	do.Provide(i, minio.New)

	// Prometheus metrics.
	do.Provide(i, metrics.New)

	// Balances dashboard client.
	do.Provide(i, balances.New)

	// The bus registry with separate command/query pipelines.
	do.Provide(i, func(inj do.Injector) (*bus.Registry, error) {
		log := do.MustInvoke[*slog.Logger](inj)
		reg := bus.NewRegistry()

		met := do.MustInvoke[*metrics.Metrics](inj)
		// Command pipeline: recover → metrics → logging → validation.
		reg.UseCommandMiddleware(
			appmw.Recover(),
			appmw.Metrics(met),
			appmw.Logging(log),
			appmw.Validation(),
		)
		// Query pipeline: recover → metrics → logging → validation. No transaction.
		reg.UseQueryMiddleware(
			appmw.Recover(),
			appmw.Metrics(met),
			appmw.Logging(log),
			appmw.Validation(),
		)

		// Register handlers.
		m := do.MustInvoke[uow.Manager](inj)
		rm := do.MustInvoke[query.ReadModel](inj)

		command.RegisterUserOnBus(reg, m)
		command.ActivateUserOnBus(reg, m)
		query.GetUserOnBus(reg, rm)
		query.ListUsersOnBus(reg, rm)

		// Pipeline.
		prm := do.MustInvoke[pipeq.ReadModel](inj)
		pipecmd.CreateRunOnBus(reg, m)
		pipecmd.RetryRunOnBus(reg, m)
		pipecmd.RegenerateStepOnBus(reg, m)
		pipecmd.RequestAssembleOnBus(reg, m)
		pipecmd.CleanupRunsOnBus(reg, m)
		pipecmd.RecordScriptCompletedOnBus(reg, m)
		pipecmd.RecordImageCompletedOnBus(reg, m)
		pipecmd.RecordAssembleCompletedOnBus(reg, m)
		pipecmd.RecordAudioCompletedOnBus(reg, m)
		pipecmd.RecordCaptionCompletedOnBus(reg, m)
		pipecmd.RecordUploadCompletedOnBus(reg, m)
		pipecmd.CreateUploadRecordOnBus(reg, m)
		pipecmd.MarkUploadRecordCompletedOnBus(reg, m)
		pipecmd.MarkUploadRecordFailedOnBus(reg, m)
		pipecmd.PromoteUploadToPublishedOnBus(reg, m)
		pipecmd.BackfillUploadMetadataOnBus(reg, m)
		pipecmd.EditUploadMetadataOnBus(reg, m)
		pipecmd.ApproveUploadRecordOnBus(reg, m)
		pipecmd.RejectUploadRecordOnBus(reg, m)
		pipecmd.RecordMusicCompletedOnBus(reg, m)
		pipecmd.RecordStepFailedOnBus(reg, m)
		pipecmd.CancelRunOnBus(reg, m)
		// Delete run cascades into MinIO + DB FKs.
		store2 := do.MustInvoke[*minio.Store](inj)
		wp := do.MustInvoke[*postgres.WritePool](inj)
		pipecmd.DeleteRunOnBus(reg, m, store2, pipecmd.PoolExecAdapter(wp.Pool))
		pipecmd.CreateTemplateOnBus(reg, m)
		pipecmd.UpdateTemplateOnBus(reg, m)
		pipecmd.DeleteTemplateOnBus(reg, m)
		pipeq.GetRunOnBus(reg, prm)
		pipeq.ListRunsOnBus(reg, prm)
		pipeq.GetTemplateOnBus(reg, prm)
		pipeq.ListTemplatesOnBus(reg, prm)
		pipeq.GetAssetRefOnBus(reg, prm)
		pipeq.GetStatsOnBus(reg, prm)
		pipeq.GetUploadRecordOnBus(reg, prm)
		pipeq.ListUploadRecordsByRunOnBus(reg, prm)
		pipeq.ListUploadRecordsByProjectOnBus(reg, prm)
		pipeq.AccountUploadStatsOnBus(reg, prm)

		// Audio library.
		alm := do.MustInvoke[audioq.ReadModel](inj)
		store := do.MustInvoke[*minio.Store](inj)
		fetcher := do.MustInvoke[audiocmd.URLFetcher](inj)
		pixabay := do.MustInvoke[audiocmd.PixabaySearcher](inj)
		// Formats marketplace.
		fmm := do.MustInvoke[formatq.ReadModel](inj)
		formatcmd.SaveFormatOnBus(reg, m)
		formatcmd.DeleteFormatOnBus(reg, m)
		formatq.ListFormatsOnBus(reg, fmm)
		formatq.GetFormatOnBus(reg, fmm)

		audiocmd.UploadTrackOnBus(reg, m, store)
		audiocmd.ImportFromURLOnBus(reg, m, store, fetcher)
		audiocmd.ImportFromPixabayOnBus(reg, m, store, pixabay)
		audiocmd.DeleteTrackOnBus(reg, m, store)
		audiocmd.RetagTrackOnBus(reg, m)
		audioq.ListTracksOnBus(reg, alm)
		audioq.GetTrackOnBus(reg, alm)
		audioq.PickTrackOnBus(reg, alm)

		// Projects.
		pjm := do.MustInvoke[projq.ReadModel](inj)
		projcmd.CreateProjectOnBus(reg, m)
		projcmd.UpdateProjectOnBus(reg, m)
		projcmd.DeleteProjectOnBus(reg, m)
		projcmd.UpsertCharacterOnBus(reg, m)
		projcmd.DeleteCharacterOnBus(reg, m)
		projcmd.UpsertEnvironmentOnBus(reg, m)
		projcmd.DeleteEnvironmentOnBus(reg, m)
		projcmd.UpsertPlotOnBus(reg, m)
		projcmd.UpsertSocialAccountOnBus(reg, m)
		projcmd.DeleteSocialAccountOnBus(reg, m)
		projcmd.LinkSocialAccountOnBus(reg, m)
		projcmd.UnlinkSocialAccountOnBus(reg, m)
		projcmd.SetDefaultSocialAccountOnBus(reg, m)
		projq.GetProjectOnBus(reg, pjm)
		projq.ListProjectsOnBus(reg, pjm)
		projq.GetProjectDetailOnBus(reg, pjm)
		projq.ListProjectSocialAccountsOnBus(reg, pjm)
		projq.ListSocialAccountsGlobalOnBus(reg, pjm)
		projcmd.SetSocialAccountLimitsOnBus(reg, m)

		// Scheduler.
		schm := do.MustInvoke[schq.ReadModel](inj)
		schcmd.ScheduleUploadOnBus(reg, m)
		schcmd.CancelScheduledUploadOnBus(reg, m)
		schcmd.RescheduleUploadOnBus(reg, m)
		schq.ListScheduledOnBus(reg, schm)
		schq.GetSlotAvailabilityOnBus(reg, schm)

		return reg, nil
	})

	return i
}

// WritePoolFrom is a small helper for cmd/* that need the raw write pool
// (e.g. the outbox relay).
func WritePoolFrom(i do.Injector) *pgxpool.Pool {
	return do.MustInvoke[*postgres.WritePool](i).Pool
}
