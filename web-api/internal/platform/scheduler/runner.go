// Package scheduler runs an in-process tick loop that picks due
// scheduled_uploads rows and fires the matching pipeline.upload.requested
// Redis Stream events. Single goroutine, cooperative cancellation via ctx.
//
// Concurrency: SELECT FOR UPDATE SKIP LOCKED inside a UoW means several
// scheduler runners would not double-dispatch the same row. For MVP we run
// exactly one inside cmd/api; scale-out is a future cmd/scheduler binary.
package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
	domsched "github.com/example/dddcqrs/internal/domain/scheduler"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

const (
	tickInterval = 30 * time.Second
	batchSize    = 50
	uploadStream = "pipeline.upload.requested"
)

// Runner fires due scheduled uploads onto the Redis upload stream.
type Runner struct {
	uow   uow.Manager
	redis *redis.Client
	log   *slog.Logger
}

// fireable bundles the schedule row + its account context + the freshly-
// created UploadRecord id (so completion can map back without a JOIN).
type fireable struct {
	Row            *domsched.ScheduledUpload
	AccountFP      string
	Platform       string
	UploadRecordID string
}

func New(u uow.Manager, r *redis.Client, log *slog.Logger) *Runner {
	return &Runner{uow: u, redis: r, log: log}
}

// Run blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	tk := time.NewTicker(tickInterval)
	defer tk.Stop()
	r.log.Info("scheduler runner started", "interval", tickInterval)
	for {
		select {
		case <-ctx.Done():
			r.log.Info("scheduler runner stopping")
			return
		case <-tk.C:
			if err := r.tick(ctx); err != nil {
				r.log.Error("scheduler tick failed", "err", err)
			}
		}
	}
}

func (r *Runner) tick(ctx context.Context) error {
	var fireables []fireable

	// 1) Pick due rows + their account info INSIDE a single UoW. Mark each
	//    in_flight to prevent re-pick by another tick / restart. Build a
	//    side list of XADD payloads to publish AFTER commit so a Redis hiccup
	//    doesn't leave rows in_flight with no event.
	err := r.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		now := time.Now().UTC()
		rows, err := repos.Scheduler().ListPendingDue(ctx, now, batchSize)
		if err != nil {
			return err
		}
		for _, row := range rows {
			acct, err := repos.Projects().GetSocialAccount(ctx, projects.SocialAccountID(row.SocialAccountID()))
			if err != nil {
				r.log.Warn("scheduled row references missing account, marking failed",
					"id", row.ID(), "account", row.SocialAccountID())
				_ = row.MarkFailed(now, "social account missing")
				_ = repos.Scheduler().Save(ctx, row)
				continue
			}
			if err := row.MarkInFlight(now); err != nil {
				continue
			}
			if err := repos.Scheduler().Save(ctx, row); err != nil {
				return err
			}
			// Create pipeline_upload_records row so analytics ticker has a
			// target to poll once the worker reports an external_ref.
			// Metadata is sparse for scheduler-triggered uploads — the
			// caption/title comes from the schedule row's metadata.params.
			meta := pipeline.UploadMetadata{}
			if params, ok := row.Metadata()["params"].(map[string]any); ok {
				if v, ok := params["title"].(string); ok {
					meta.Title = v
				}
				if v, ok := params["description"].(string); ok {
					meta.Description = v
				}
				if v, ok := params["visibility"].(string); ok {
					meta.Visibility = v
				}
			}
			rec := pipeline.NewUploadRecord(row.RunID(), "", row.SocialAccountID(),
				acct.Platform(), 99, meta)
			if err := repos.UploadRecords().Save(ctx, rec); err != nil {
				return err
			}
			fireables = append(fireables, fireable{
				Row:            row,
				AccountFP:      acct.FirefoxProfilePath(),
				Platform:       acct.Platform(),
				UploadRecordID: rec.ID().String(),
			})
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 2) Outside the tx, XADD payloads. If publish fails, the row stays
	//    in_flight — operator can manually retry; we don't auto-rollback to
	//    avoid hot retry loops on persistent Redis outage.
	for _, f := range fireables {
		payload := buildUploadPayload(f.Row, f.AccountFP, f.Platform)
		raw, _ := json.Marshal(payload)
		if err := r.redis.XAdd(ctx, &redis.XAddArgs{
			Stream: uploadStream,
			Values: map[string]any{"payload": string(raw)},
		}).Err(); err != nil {
			r.log.Error("XADD upload failed", "id", f.Row.ID(), "err", err)
			continue
		}
		r.log.Info("scheduled upload fired",
			"id", f.Row.ID(), "run", f.Row.RunID(),
			"account", f.Row.SocialAccountID(), "platform", f.Platform)
	}
	return nil
}

// buildUploadPayload mirrors the existing pipeline.upload.requested shape so
// the upload worker doesn't need scheduler-specific code paths.
func buildUploadPayload(row *domsched.ScheduledUpload, profilePath, platform string) map[string]any {
	md := row.Metadata()
	if md == nil {
		md = map[string]any{}
	}
	videoKey, _ := md["video_key"].(string)
	captions := md["captions"]
	// Params snapshot: caller passes the same map shape the worker expects.
	params, _ := md["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	if _, ok := params["platform"]; !ok {
		params["platform"] = platform
	}
	return map[string]any{
		"run_id":               row.RunID(),
		"step_index":           99, // synthetic step idx for scheduled uploads
		"step_id":              "00000000-0000-0000-0000-000000000099",
		"attempt_id":           row.ID().String(),
		"video_key":            videoKey,
		"provider":             platform,
		"social_account_id":    row.SocialAccountID(),
		"firefox_profile_path": profilePath,
		"params":               params,
		"captions":             captions,
		"scheduled_at":         row.ScheduledAt().Format(time.RFC3339),
	}
}
