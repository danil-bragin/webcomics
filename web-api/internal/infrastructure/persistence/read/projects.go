package read

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pq "github.com/example/dddcqrs/internal/app/query/projects"
)

type ProjectsModel struct{ pool *pgxpool.Pool }

func NewProjectsModel(pool *pgxpool.Pool) *ProjectsModel {
	return &ProjectsModel{pool: pool}
}

const qProject = `
SELECT p.id, p.name, p.description, COALESCE(p.defaults,'{}'::jsonb), p.archived, p.created_at, p.updated_at,
       COALESCE((SELECT count(*) FROM pipeline_runs WHERE project_id = p.id), 0) AS runs_count,
       COALESCE((SELECT count(*) FROM pipeline_upload_records WHERE project_id = p.id
                  AND status IN ('uploaded','published')), 0) AS uploaded_count
FROM projects p WHERE p.id = $1`

func (m *ProjectsModel) GetProject(ctx context.Context, id string) (pq.ProjectView, error) {
	var v pq.ProjectView
	var defRaw []byte
	err := m.pool.QueryRow(ctx, qProject, id).Scan(
		&v.ID, &v.Name, &v.Description, &defRaw, &v.Archived, &v.CreatedAt, &v.UpdatedAt,
		&v.RunsCount, &v.UploadedCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	v.Defaults = json.RawMessage(defRaw)
	return v, err
}

func (m *ProjectsModel) ListProjects(ctx context.Context) ([]pq.ProjectView, error) {
	const q = `
SELECT p.id, p.name, p.description, COALESCE(p.defaults,'{}'::jsonb), p.archived, p.created_at, p.updated_at,
       COALESCE(rc.cnt, 0)  AS runs_count,
       COALESCE(uc.cnt, 0)  AS uploaded_count
FROM projects p
LEFT JOIN (SELECT project_id, count(*) AS cnt FROM pipeline_runs GROUP BY project_id) rc ON rc.project_id = p.id
LEFT JOIN (SELECT project_id, count(*) AS cnt FROM pipeline_upload_records
           WHERE status IN ('uploaded','published') GROUP BY project_id) uc ON uc.project_id = p.id
ORDER BY p.updated_at DESC`
	rows, err := m.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pq.ProjectView{}
	for rows.Next() {
		var v pq.ProjectView
		var defRaw []byte
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &defRaw, &v.Archived,
			&v.CreatedAt, &v.UpdatedAt, &v.RunsCount, &v.UploadedCount); err != nil {
			return nil, err
		}
		v.Defaults = json.RawMessage(defRaw)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *ProjectsModel) GetProjectDetail(ctx context.Context, id string) (pq.ProjectDetailView, error) {
	var d pq.ProjectDetailView
	p, err := m.GetProject(ctx, id)
	if err != nil {
		return d, err
	}
	d.Project = p
	if d.Characters, err = m.ListCharacters(ctx, id); err != nil {
		return d, err
	}
	if d.Environments, err = m.ListEnvironments(ctx, id); err != nil {
		return d, err
	}
	plot, err := m.GetPlotByProject(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return d, err
	}
	d.Plot = plot
	if d.SocialAccounts, err = m.ListSocialAccounts(ctx, id); err != nil {
		return d, err
	}
	return d, nil
}

// ListSocialAccounts returns the accounts LINKED to the given project (via
// project_social_account_links). Each row carries the is_default flag.
func (m *ProjectsModel) ListSocialAccounts(ctx context.Context, projectID string) ([]pq.SocialAccountView, error) {
	const q = `SELECT a.id, a.platform, a.label, a.firefox_profile_path, COALESCE(a.extra,'{}'::jsonb),
	                  COALESCE(a.status,'active'), a.last_used_at, a.cooldown_until, COALESCE(a.failure_streak,0),
	                  COALESCE(a.default_visibility,'unlisted'), COALESCE(a.default_made_for_kids,false),
	                  COALESCE(a.default_category_id,'22'), COALESCE(a.default_category_label,'People & Blogs'),
	                  COALESCE(a.daily_upload_limit,15), COALESCE(a.limit_window_hours,24),
	                  COALESCE(a.is_verified,false), COALESCE(a.min_gap_seconds,60),
	                  a.created_at, a.updated_at, l.is_default
		FROM project_social_account_links l
		JOIN social_accounts a ON a.id = l.social_account_id
		WHERE l.project_id = $1
		ORDER BY l.is_default DESC, l.created_at ASC`
	rows, err := m.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pq.SocialAccountView{}
	for rows.Next() {
		var v pq.SocialAccountView
		var extraRaw []byte
		if err := rows.Scan(&v.ID, &v.Platform, &v.Label, &v.FirefoxProfilePath, &extraRaw,
			&v.Status, &v.LastUsedAt, &v.CooldownUntil, &v.FailureStreak,
			&v.DefaultVisibility, &v.DefaultMadeForKids, &v.DefaultCategoryID, &v.DefaultCategoryLabel,
			&v.DailyUploadLimit, &v.LimitWindowHours, &v.IsVerified, &v.MinGapSeconds,
			&v.CreatedAt, &v.UpdatedAt, &v.IsDefault); err != nil {
			return nil, err
		}
		v.ProjectID = projectID
		v.Extra = json.RawMessage(extraRaw)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListAllSocialAccounts returns every account regardless of project linkage.
// Powers the new /social page (global library).
func (m *ProjectsModel) ListAllSocialAccounts(ctx context.Context, filterPlatform string) ([]pq.SocialAccountView, error) {
	q := `SELECT a.id, a.platform, a.label, a.firefox_profile_path, COALESCE(a.extra,'{}'::jsonb),
	             COALESCE(a.status,'active'), a.last_used_at, a.cooldown_until, COALESCE(a.failure_streak,0),
	             COALESCE(a.default_visibility,'unlisted'), COALESCE(a.default_made_for_kids,false),
	             COALESCE(a.default_category_id,'22'), COALESCE(a.default_category_label,'People & Blogs'),
	             COALESCE(a.daily_upload_limit,15), COALESCE(a.limit_window_hours,24),
	             COALESCE(a.is_verified,false), COALESCE(a.min_gap_seconds,60),
	             a.created_at, a.updated_at,
	             (SELECT COUNT(*) FROM project_social_account_links WHERE social_account_id = a.id) AS project_count,
	             (SELECT COUNT(*) FROM pipeline_upload_records WHERE social_account_id = a.id) AS upload_count
		FROM social_accounts a`
	args := []any{}
	if filterPlatform != "" {
		q += ` WHERE a.platform = $1`
		args = append(args, filterPlatform)
	}
	q += ` ORDER BY a.created_at DESC`
	rows, err := m.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pq.SocialAccountView{}
	for rows.Next() {
		var v pq.SocialAccountView
		var extraRaw []byte
		if err := rows.Scan(&v.ID, &v.Platform, &v.Label, &v.FirefoxProfilePath, &extraRaw,
			&v.Status, &v.LastUsedAt, &v.CooldownUntil, &v.FailureStreak,
			&v.DefaultVisibility, &v.DefaultMadeForKids, &v.DefaultCategoryID, &v.DefaultCategoryLabel,
			&v.DailyUploadLimit, &v.LimitWindowHours, &v.IsVerified, &v.MinGapSeconds,
			&v.CreatedAt, &v.UpdatedAt, &v.ProjectCount, &v.UploadCount); err != nil {
			return nil, err
		}
		v.Extra = json.RawMessage(extraRaw)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *ProjectsModel) ListCharacters(ctx context.Context, projectID string) ([]pq.CharacterView, error) {
	const q = `SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
		FROM characters WHERE project_id = $1 ORDER BY created_at ASC`
	rows, err := m.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pq.CharacterView{}
	for rows.Next() {
		var v pq.CharacterView
		var traitsRaw []byte
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Name, &v.Description, &traitsRaw, &v.RefAssetIDs, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Traits = json.RawMessage(traitsRaw)
		if v.RefAssetIDs == nil {
			v.RefAssetIDs = []string{}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *ProjectsModel) GetCharacter(ctx context.Context, id string) (pq.CharacterView, error) {
	const q = `SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
		FROM characters WHERE id = $1`
	var v pq.CharacterView
	var traitsRaw []byte
	err := m.pool.QueryRow(ctx, q, id).Scan(&v.ID, &v.ProjectID, &v.Name, &v.Description, &traitsRaw, &v.RefAssetIDs, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	v.Traits = json.RawMessage(traitsRaw)
	if v.RefAssetIDs == nil {
		v.RefAssetIDs = []string{}
	}
	return v, err
}

func (m *ProjectsModel) ListEnvironments(ctx context.Context, projectID string) ([]pq.EnvironmentView, error) {
	const q = `SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
		FROM environments WHERE project_id = $1 ORDER BY created_at ASC`
	rows, err := m.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pq.EnvironmentView{}
	for rows.Next() {
		var v pq.EnvironmentView
		var traitsRaw []byte
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Name, &v.Description, &traitsRaw, &v.RefAssetIDs, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Traits = json.RawMessage(traitsRaw)
		if v.RefAssetIDs == nil {
			v.RefAssetIDs = []string{}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *ProjectsModel) GetEnvironment(ctx context.Context, id string) (pq.EnvironmentView, error) {
	const q = `SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
		FROM environments WHERE id = $1`
	var v pq.EnvironmentView
	var traitsRaw []byte
	err := m.pool.QueryRow(ctx, q, id).Scan(&v.ID, &v.ProjectID, &v.Name, &v.Description, &traitsRaw, &v.RefAssetIDs, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	v.Traits = json.RawMessage(traitsRaw)
	if v.RefAssetIDs == nil {
		v.RefAssetIDs = []string{}
	}
	return v, err
}

func (m *ProjectsModel) GetPlotByProject(ctx context.Context, projectID string) (*pq.PlotView, error) {
	const q = `SELECT id, project_id, name, premise, COALESCE(beats,'[]'::jsonb), created_at, updated_at
		FROM plots WHERE project_id = $1`
	var v pq.PlotView
	var beatsRaw []byte
	err := m.pool.QueryRow(ctx, q, projectID).Scan(&v.ID, &v.ProjectID, &v.Name, &v.Premise, &beatsRaw, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(beatsRaw, &v.Beats)
	if v.Beats == nil {
		v.Beats = []pq.PlotBeatView{}
	}
	return &v, nil
}

// avoid unused import warning if all errors compile out.
var _ = time.Time{}
