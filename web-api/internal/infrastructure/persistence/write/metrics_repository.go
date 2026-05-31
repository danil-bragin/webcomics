package write

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/uploadmetrics"
)

// MetricsDueRow is the projection the metrics ticker needs from the repo.
// Defined here (instead of inside the uow pkg) to avoid an import cycle
// when other write repos want to reference it.
type MetricsDueRow struct {
	ID              string
	Platform        string
	ExternalRef     string
	SocialAccountID string
	ProfilePath     string
}

// MetricsRepository persists upload_metrics_snapshots + denormalises
// last-known counters onto pipeline_upload_records inside the bound UoW tx.
type MetricsRepository struct{ tx pgx.Tx }

func NewMetricsRepository(tx pgx.Tx) *MetricsRepository {
	return &MetricsRepository{tx: tx}
}

// InsertSnapshot writes one snapshot row + bumps last-known on the upload
// record + records fetched_at. Returns nil on success.
func (r *MetricsRepository) InsertSnapshot(ctx context.Context, s uploadmetrics.Snapshot) error {
	raw, _ := json.Marshal(s.Raw)
	if _, err := r.tx.Exec(ctx, `
		INSERT INTO upload_metrics_snapshots
		  (id, upload_record_id, fetched_at, views, likes, comments, shares, raw_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.ID.String(), s.UploadRecordID, s.FetchedAt, s.Views, s.Likes, s.Comments, s.Shares, raw,
	); err != nil {
		return err
	}
	if _, err := r.tx.Exec(ctx, `
		UPDATE pipeline_upload_records
		   SET last_known_views    = $2,
		       last_known_likes    = $3,
		       last_known_comments = $4,
		       last_known_shares   = $5,
		       last_fetched_at     = $6,
		       fetch_error         = '',
		       updated_at          = now()
		 WHERE id = $1`,
		s.UploadRecordID, s.Views, s.Likes, s.Comments, s.Shares, s.FetchedAt,
	); err != nil {
		return err
	}
	return nil
}

// MarkFetchFailed bumps the attempt counter + records the error string so
// the UI can surface why a particular upload hasn't refreshed.
func (r *MetricsRepository) MarkFetchFailed(ctx context.Context, uploadRecordID, errMsg string, now time.Time) error {
	_, err := r.tx.Exec(ctx, `
		UPDATE pipeline_upload_records
		   SET fetch_error         = $2,
		       fetch_attempt_count = fetch_attempt_count + 1,
		       last_fetched_at     = $3,
		       updated_at          = now()
		 WHERE id = $1`,
		uploadRecordID, errMsg, now,
	)
	return err
}

// ListUploadsDueForMetrics returns uploads whose last_fetched_at is older
// than `cutoff` (or NULL) and that have a non-empty external_ref. Includes
// the social account's firefox_profile_path so selenium fetchers can run
// without an extra round trip. Returns the uow.MetricsDueRow type (declared in
// the persistence/uow ports) so callers can use it without import cycles.
func (r *MetricsRepository) ListUploadsDueForMetrics(ctx context.Context, cutoff time.Time, limit int) ([]MetricsDueRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.tx.Query(ctx, `
		SELECT u.id, u.provider, u.external_ref, u.social_account_id,
		       COALESCE(a.firefox_profile_path,'')
		  FROM pipeline_upload_records u
		  LEFT JOIN social_accounts a ON a.id = u.social_account_id
		 WHERE u.external_ref <> ''
		   AND (u.last_fetched_at IS NULL OR u.last_fetched_at < $1)
		 ORDER BY u.last_fetched_at NULLS FIRST
		 LIMIT $2`,
		cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetricsDueRow{}
	for rows.Next() {
		var d MetricsDueRow
		if err := rows.Scan(&d.ID, &d.Platform, &d.ExternalRef, &d.SocialAccountID, &d.ProfilePath); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
