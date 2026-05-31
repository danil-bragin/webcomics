package write

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/scheduler"
)

// SchedulerRepository persists scheduled_uploads inside a UoW tx.
type SchedulerRepository struct{ tx pgx.Tx }

func NewSchedulerRepository(tx pgx.Tx) *SchedulerRepository {
	return &SchedulerRepository{tx: tx}
}

var ErrScheduledUploadNotFound = errors.New("scheduler: row not found")

const upsScheduled = `
INSERT INTO scheduled_uploads
  (id, run_id, social_account_id, scheduled_at, status, external_ref, error, metadata,
   created_at, updated_at, fired_at, completed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (id) DO UPDATE SET
  scheduled_at = EXCLUDED.scheduled_at,
  status       = EXCLUDED.status,
  external_ref = EXCLUDED.external_ref,
  error        = EXCLUDED.error,
  metadata     = EXCLUDED.metadata,
  updated_at   = EXCLUDED.updated_at,
  fired_at     = EXCLUDED.fired_at,
  completed_at = EXCLUDED.completed_at`

func (r *SchedulerRepository) Save(ctx context.Context, s *scheduler.ScheduledUpload) error {
	md, _ := json.Marshal(s.Metadata())
	_, err := r.tx.Exec(ctx, upsScheduled,
		s.ID().String(), s.RunID(), s.SocialAccountID(), s.ScheduledAt(), string(s.Status()),
		s.ExternalRef(), s.Error(), md,
		s.CreatedAt(), s.UpdatedAt(), s.FiredAt(), s.CompletedAt())
	return err
}

const selScheduled = `
SELECT id, run_id, social_account_id, scheduled_at, status, external_ref, error,
       COALESCE(metadata,'{}'::jsonb), created_at, updated_at, fired_at, completed_at
FROM scheduled_uploads WHERE id = $1 FOR UPDATE`

func (r *SchedulerRepository) Get(ctx context.Context, id scheduler.ID) (*scheduler.ScheduledUpload, error) {
	row := r.tx.QueryRow(ctx, selScheduled, id.String())
	return scanScheduledRow(row.Scan)
}

func (r *SchedulerRepository) Delete(ctx context.Context, id scheduler.ID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM scheduled_uploads WHERE id = $1`, id.String())
	return err
}

// ListSlotsInWindow returns slot points for an account within ±window of
// targetAt. Used by the limit-check resolver. Cancelled / failed rows are
// included; the domain layer filters them.
func (r *SchedulerRepository) ListSlotsInWindow(ctx context.Context, accountID string, targetAt time.Time, window time.Duration) ([]scheduler.SlotPoint, error) {
	low := targetAt.Add(-window)
	high := targetAt.Add(window)
	rows, err := r.tx.Query(ctx,
		`SELECT scheduled_at, status FROM scheduled_uploads
		 WHERE social_account_id = $1 AND scheduled_at BETWEEN $2 AND $3
		 ORDER BY scheduled_at ASC`,
		accountID, low, high)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []scheduler.SlotPoint{}
	for rows.Next() {
		var at time.Time
		var st string
		if err := rows.Scan(&at, &st); err != nil {
			return nil, err
		}
		out = append(out, scheduler.SlotPoint{ScheduledAt: at, Status: scheduler.Status(st)})
	}
	return out, rows.Err()
}

// ListPendingDue fetches up to `limit` pending rows whose scheduled_at <= now.
// FOR UPDATE SKIP LOCKED so multiple scheduler workers don't race.
func (r *SchedulerRepository) ListPendingDue(ctx context.Context, now time.Time, limit int) ([]*scheduler.ScheduledUpload, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.tx.Query(ctx,
		`SELECT `+schedCols+`
		   FROM scheduled_uploads
		  WHERE status = 'pending' AND scheduled_at <= $1
		  ORDER BY scheduled_at ASC
		  LIMIT $2
		  FOR UPDATE SKIP LOCKED`,
		now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*scheduler.ScheduledUpload{}
	for rows.Next() {
		s, err := scanScheduledRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FindByRunAndExternalRef is how the upload-completion handler matches a
// completed XADD payload back to its scheduled row.
func (r *SchedulerRepository) FindByRunPending(ctx context.Context, runID string) (*scheduler.ScheduledUpload, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT `+schedCols+` FROM scheduled_uploads
		   WHERE run_id = $1 AND status = 'in_flight'
		   ORDER BY fired_at DESC LIMIT 1 FOR UPDATE`,
		runID)
	return scanScheduledRow(row.Scan)
}

const schedCols = `id, run_id, social_account_id, scheduled_at, status, external_ref, error,
       COALESCE(metadata,'{}'::jsonb), created_at, updated_at, fired_at, completed_at`

func scanScheduledRow(scan func(...any) error) (*scheduler.ScheduledUpload, error) {
	var (
		id, runID, accountID, status, externalRef, errMsg string
		scheduledAt, createdAt, updatedAt                 time.Time
		firedAt, completedAt                              *time.Time
		mdRaw                                             []byte
	)
	if err := scan(&id, &runID, &accountID, &scheduledAt, &status, &externalRef, &errMsg,
		&mdRaw, &createdAt, &updatedAt, &firedAt, &completedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrScheduledUploadNotFound
		}
		return nil, err
	}
	md := map[string]any{}
	_ = json.Unmarshal(mdRaw, &md)
	return scheduler.Reconstitute(
		scheduler.ID(id), runID, accountID, scheduledAt, scheduler.Status(status),
		externalRef, errMsg, md, createdAt, updatedAt, firedAt, completedAt,
	), nil
}
