// Package metricsticker runs an in-process loop that polls upload metrics on
// a configurable cadence (UPLOAD_METRICS_INTERVAL_HOURS env, default 6h).
//
// Per tick:
//  1. SELECT due rows (last_fetched_at NULL or older than `interval`).
//  2. For YT — fetch in-process via Data API + RecordMetricsSnapshot.
//  3. For IG/TT/FB — XADD pipeline.metrics.requested for the Python worker
//     (which has Firefox); the worker publishes pipeline.metrics.completed
//     that a Go consumer maps back into RecordMetricsSnapshot.
package metricsticker

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/dddcqrs/internal/app/bus"
	umcmd "github.com/example/dddcqrs/internal/app/command/uploadmetrics"
	"github.com/example/dddcqrs/internal/domain/uploadmetrics"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

const (
	defaultIntervalHours = 6
	envInterval          = "UPLOAD_METRICS_INTERVAL_HOURS"
	batchSize            = 100
	metricsRequest       = "pipeline.metrics.requested"
)

type Runner struct {
	uow      uow.Manager
	reg      *bus.Registry
	ytFetch  uploadmetrics.Fetcher
	redis    *redis.Client
	log      *slog.Logger
	interval time.Duration
}

func New(u uow.Manager, reg *bus.Registry, ytFetch uploadmetrics.Fetcher, r *redis.Client, log *slog.Logger) *Runner {
	hours := defaultIntervalHours
	if raw := os.Getenv(envInterval); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			hours = n
		}
	}
	return &Runner{
		uow:      u,
		reg:      reg,
		ytFetch:  ytFetch,
		redis:    r,
		log:      log,
		interval: time.Duration(hours) * time.Hour,
	}
}

func (r *Runner) Interval() time.Duration { return r.interval }

func (r *Runner) Run(ctx context.Context) {
	r.log.Info("metrics ticker started", "interval", r.interval)
	// Initial tick after a short delay so the API is fully up.
	first := time.NewTimer(30 * time.Second)
	select {
	case <-ctx.Done():
		first.Stop()
		return
	case <-first.C:
	}
	if err := r.tick(ctx); err != nil {
		r.log.Error("metrics tick failed", "err", err)
	}
	// Subsequent ticks: at most every minute, but the SQL cutoff uses the
	// configured interval so we only fetch when due. Keeping the loop tight
	// lets newly-uploaded rows get their first snapshot promptly.
	tk := time.NewTicker(time.Minute)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			r.log.Info("metrics ticker stopping")
			return
		case <-tk.C:
			if err := r.tick(ctx); err != nil {
				r.log.Error("metrics tick failed", "err", err)
			}
		}
	}
}

func (r *Runner) tick(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-r.interval)
	var due []uow.MetricsDueRow
	err := r.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rows, err := u.Repositories().Metrics().ListUploadsDueForMetrics(ctx, cutoff, batchSize)
		if err != nil {
			return err
		}
		due = rows
		return nil
	})
	if err != nil {
		return err
	}
	for _, d := range due {
		if d.Platform == "youtube_selenium" {
			r.fetchYouTube(ctx, d)
			continue
		}
		// XADD for selenium-based providers.
		payload, _ := json.Marshal(map[string]any{
			"upload_record_id":  d.ID,
			"external_ref":      d.ExternalRef,
			"platform":          d.Platform,
			"social_account_id": d.SocialAccountID,
			"profile_path":      d.ProfilePath,
		})
		if err := r.redis.XAdd(ctx, &redis.XAddArgs{
			Stream: metricsRequest,
			Values: map[string]any{"payload": string(payload)},
		}).Err(); err != nil {
			r.log.Error("XADD metrics.requested failed", "id", d.ID, "err", err)
		}
	}
	return nil
}

func (r *Runner) fetchYouTube(ctx context.Context, d uow.MetricsDueRow) {
	if r.ytFetch == nil {
		return
	}
	snap, err := r.ytFetch.Fetch(ctx, d.ExternalRef, "")
	if err != nil {
		r.log.Warn("yt metrics fetch failed", "id", d.ID, "err", err)
		_, _ = bus.Dispatch[umcmd.RecordMetricsFailureResult](ctx, r.reg, umcmd.RecordMetricsFailure{
			UploadRecordID: d.ID, Error: err.Error(),
		})
		return
	}
	snap.UploadRecordID = d.ID
	_, err = bus.Dispatch[umcmd.RecordMetricsSnapshotResult](ctx, r.reg, umcmd.RecordMetricsSnapshot{
		UploadRecordID: d.ID,
		Views:          snap.Views, Likes: snap.Likes, Comments: snap.Comments, Shares: snap.Shares,
		Raw: snap.Raw,
	})
	if err != nil {
		r.log.Error("record snapshot failed", "id", d.ID, "err", err)
	}
}
