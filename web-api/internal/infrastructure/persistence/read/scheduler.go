package read

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/example/dddcqrs/internal/platform/postgres"

	schq "github.com/example/dddcqrs/internal/app/query/scheduler"
	"github.com/example/dddcqrs/internal/domain/scheduler"
)

type SchedulerModel struct{ pool *postgres.ReadPool }

func NewSchedulerModel(pool *postgres.ReadPool) *SchedulerModel { return &SchedulerModel{pool: pool} }

// schedListCols also surfaces the run's latest video asset id (for thumbnails)
// + the run cost/status (so the /schedule card can echo the RunsList look).
// The video sub-select returns the most-recent video asset for the run.
const schedListCols = `s.id, s.run_id, COALESCE(r.prompt,''),
       COALESCE((
         SELECT pa.id FROM pipeline_assets pa
          WHERE pa.run_id = s.run_id AND pa.kind = 'video'
          ORDER BY pa.created_at DESC LIMIT 1
       ), '') AS run_video_asset_id,
       COALESCE(r.total_cost_usd, 0) AS run_cost,
       COALESCE(r.status, '') AS run_status,
       s.social_account_id,
       COALESCE(a.label,''), COALESCE(a.platform,''),
       s.scheduled_at, s.status, COALESCE(s.external_ref,''), COALESCE(s.error,''),
       s.fired_at, s.completed_at, s.created_at`

func (m *SchedulerModel) List(ctx context.Context, f schq.ListFilter) ([]schq.View, error) {
	q := `SELECT ` + schedListCols + `
		FROM scheduled_uploads s
		LEFT JOIN pipeline_runs    r ON r.id = s.run_id
		LEFT JOIN social_accounts  a ON a.id = s.social_account_id`
	args := []any{}
	conds := []string{}
	if f.AccountID != "" {
		args = append(args, f.AccountID)
		conds = append(conds, "s.social_account_id = $"+strconv.Itoa(len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "s.status = $"+strconv.Itoa(len(args)))
	}
	if f.Since != nil {
		args = append(args, *f.Since)
		conds = append(conds, "s.scheduled_at >= $"+strconv.Itoa(len(args)))
	}
	if f.Until != nil {
		args = append(args, *f.Until)
		conds = append(conds, "s.scheduled_at <= $"+strconv.Itoa(len(args)))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY s.scheduled_at ASC"
	if f.Limit > 0 {
		q += " LIMIT " + strconv.Itoa(f.Limit)
	}
	rows, err := m.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []schq.View{}
	for rows.Next() {
		var v schq.View
		if err := rows.Scan(&v.ID, &v.RunID, &v.RunPrompt,
			&v.RunVideoAssetID, &v.RunCostUSD, &v.RunStatus,
			&v.SocialAccountID,
			&v.AccountLabel, &v.AccountPlatform,
			&v.ScheduledAt, &v.Status, &v.ExternalRef, &v.Error,
			&v.FiredAt, &v.CompletedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *SchedulerModel) Get(ctx context.Context, id string) (schq.View, error) {
	var v schq.View
	row := m.pool.QueryRow(ctx,
		`SELECT `+schedListCols+`
		   FROM scheduled_uploads s
		   LEFT JOIN pipeline_runs    r ON r.id = s.run_id
		   LEFT JOIN social_accounts  a ON a.id = s.social_account_id
		  WHERE s.id = $1`, id)
	err := row.Scan(&v.ID, &v.RunID, &v.RunPrompt,
		&v.RunVideoAssetID, &v.RunCostUSD, &v.RunStatus,
		&v.SocialAccountID,
		&v.AccountLabel, &v.AccountPlatform,
		&v.ScheduledAt, &v.Status, &v.ExternalRef, &v.Error,
		&v.FiredAt, &v.CompletedAt, &v.CreatedAt)
	return v, err
}

// WindowStatsForAccount: read account limit + count live rows around `now`.
func (m *SchedulerModel) WindowStatsForAccount(ctx context.Context, accountID string, now time.Time) (schq.AccountWindowStats, error) {
	var s schq.AccountWindowStats
	s.SocialAccountID = accountID
	// Load account limit cfg
	row := m.pool.QueryRow(ctx,
		`SELECT COALESCE(daily_upload_limit,15), COALESCE(limit_window_hours,24)
		   FROM social_accounts WHERE id = $1`, accountID)
	if err := row.Scan(&s.LimitN, &s.WindowHours); err != nil {
		return s, err
	}
	window := time.Duration(s.WindowHours) * time.Hour
	if window <= 0 {
		window = 24 * time.Hour
	}
	rows, err := m.pool.Query(ctx,
		`SELECT scheduled_at, status FROM scheduled_uploads
		  WHERE social_account_id = $1
		    AND scheduled_at BETWEEN $2 AND $3`,
		accountID, now.Add(-window), now.Add(window))
	if err != nil {
		return s, err
	}
	defer rows.Close()
	slots := []scheduler.SlotPoint{}
	for rows.Next() {
		var at time.Time
		var st string
		if err := rows.Scan(&at, &st); err != nil {
			return s, err
		}
		slots = append(slots, scheduler.SlotPoint{ScheduledAt: at, Status: scheduler.Status(st)})
	}
	rl := scheduler.RateLimit{LimitN: s.LimitN, WindowHours: s.WindowHours}
	live := 0
	for _, p := range slots {
		if p.Status == scheduler.StatusCancelled || p.Status == scheduler.StatusFailed {
			continue
		}
		live++
	}
	s.CountInWindow = live
	s.IsAtLimit = s.LimitN > 0 && live >= s.LimitN
	if s.IsAtLimit {
		next := scheduler.SuggestNextFreeSlot(now, slots, rl)
		s.NextFreeSlot = &next
	}
	return s, nil
}

