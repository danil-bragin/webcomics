package read

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	pipelineq "github.com/example/dddcqrs/internal/app/query/pipeline"
)

const selUploadRecord = `
SELECT id, run_id, COALESCE(project_id,''), COALESCE(social_account_id,''), step_index,
       status, provider, COALESCE(platform_target,''),
       title, description, tags, hashtags,
       visibility, made_for_kids, age_restriction,
       category_id, category_label, comments_enabled, playlist_names, scheduled_at,
       external_ref, external_id, COALESCE(thumbnail_asset_id,''),
       attempts, error, COALESCE(error_screenshot_asset_id,''),
       COALESCE(metadata_overridden,false), COALESCE(audience_confidence,0),
       COALESCE(audience_reasoning,''), COALESCE(hook,''),
       COALESCE(screenshot_trail, '[]'::jsonb),
       started_at, finished_at, created_at, updated_at
FROM pipeline_upload_records
WHERE id = $1`

const listUploadByRun = `
SELECT id, run_id, COALESCE(project_id,''), COALESCE(social_account_id,''), step_index,
       status, provider, COALESCE(platform_target,''),
       title, description, tags, hashtags,
       visibility, made_for_kids, age_restriction,
       category_id, category_label, comments_enabled, playlist_names, scheduled_at,
       external_ref, external_id, COALESCE(thumbnail_asset_id,''),
       attempts, error, COALESCE(error_screenshot_asset_id,''),
       COALESCE(metadata_overridden,false), COALESCE(audience_confidence,0),
       COALESCE(audience_reasoning,''), COALESCE(hook,''),
       COALESCE(screenshot_trail, '[]'::jsonb),
       started_at, finished_at, created_at, updated_at
FROM pipeline_upload_records
WHERE run_id = $1
ORDER BY created_at ASC`

const listUploadByProject = `
SELECT id, run_id, COALESCE(project_id,''), COALESCE(social_account_id,''), step_index,
       status, provider, COALESCE(platform_target,''),
       title, description, tags, hashtags,
       visibility, made_for_kids, age_restriction,
       category_id, category_label, comments_enabled, playlist_names, scheduled_at,
       external_ref, external_id, COALESCE(thumbnail_asset_id,''),
       attempts, error, COALESCE(error_screenshot_asset_id,''),
       COALESCE(metadata_overridden,false), COALESCE(audience_confidence,0),
       COALESCE(audience_reasoning,''), COALESCE(hook,''),
       COALESCE(screenshot_trail, '[]'::jsonb),
       started_at, finished_at, created_at, updated_at
FROM pipeline_upload_records
WHERE project_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

const listUploadByAccount = `
SELECT id, run_id, COALESCE(project_id,''), COALESCE(social_account_id,''), step_index,
       status, provider, COALESCE(platform_target,''),
       title, description, tags, hashtags,
       visibility, made_for_kids, age_restriction,
       category_id, category_label, comments_enabled, playlist_names, scheduled_at,
       external_ref, external_id, COALESCE(thumbnail_asset_id,''),
       attempts, error, COALESCE(error_screenshot_asset_id,''),
       COALESCE(metadata_overridden,false), COALESCE(audience_confidence,0),
       COALESCE(audience_reasoning,''), COALESCE(hook,''),
       COALESCE(screenshot_trail, '[]'::jsonb),
       started_at, finished_at, created_at, updated_at
FROM pipeline_upload_records
WHERE social_account_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

func scanUploadView(row pgx.Row) (pipelineq.UploadRecordView, error) {
	var v pipelineq.UploadRecordView
	var trailRaw []byte
	var scheduledAt, startedAt, finishedAt *time.Time
	if err := row.Scan(&v.ID, &v.RunID, &v.ProjectID, &v.SocialAccountID, &v.StepIndex,
		&v.Status, &v.Provider, &v.PlatformTarget,
		&v.Title, &v.Description, &v.Tags, &v.Hashtags,
		&v.Visibility, &v.MadeForKids, &v.AgeRestriction,
		&v.CategoryID, &v.CategoryLabel, &v.CommentsEnabled, &v.PlaylistNames, &scheduledAt,
		&v.ExternalRef, &v.ExternalID, &v.ThumbnailAssetID,
		&v.Attempts, &v.Error, &v.ErrorScreenshotAssetID,
		&v.MetadataOverridden, &v.AudienceConfidence,
		&v.AudienceReasoning, &v.Hook,
		&trailRaw,
		&startedAt, &finishedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return v, err
	}
	_ = json.Unmarshal(trailRaw, &v.ScreenshotTrail)
	v.ScheduledAt = scheduledAt
	v.StartedAt = startedAt
	v.FinishedAt = finishedAt
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if v.Hashtags == nil {
		v.Hashtags = []string{}
	}
	if v.PlaylistNames == nil {
		v.PlaylistNames = []string{}
	}
	return v, nil
}

func (m *PipelineModel) GetUploadRecord(ctx context.Context, id string) (pipelineq.UploadRecordView, error) {
	row := m.pool.QueryRow(ctx, selUploadRecord, id)
	v, err := scanUploadView(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

func (m *PipelineModel) ListUploadRecordsByRun(ctx context.Context, runID string) ([]pipelineq.UploadRecordView, error) {
	rows, err := m.pool.Query(ctx, listUploadByRun, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.UploadRecordView{}
	for rows.Next() {
		v, err := scanUploadView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *PipelineModel) ListUploadRecordsByProject(ctx context.Context, projectID string, limit, offset int) ([]pipelineq.UploadRecordView, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.pool.Query(ctx, listUploadByProject, projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.UploadRecordView{}
	for rows.Next() {
		v, err := scanUploadView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *PipelineModel) ListUploadRecordsByAccount(ctx context.Context, socialAccountID string, limit, offset int) ([]pipelineq.UploadRecordView, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.pool.Query(ctx, listUploadByAccount, socialAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.UploadRecordView{}
	for rows.Next() {
		v, err := scanUploadView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const accountStatsQuery = `
SELECT COALESCE(social_account_id,'') AS aid,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE status = 'uploaded') AS uploaded,
       COUNT(*) FILTER (WHERE status = 'published') AS published,
       COUNT(*) FILTER (WHERE status = 'failed') AS failed,
       MAX(finished_at) AS last_upload_at
FROM pipeline_upload_records
WHERE project_id = $1 AND social_account_id IS NOT NULL
GROUP BY social_account_id`

func (m *PipelineModel) AccountUploadStats(ctx context.Context, projectID string) ([]pipelineq.AccountUploadStats, error) {
	rows, err := m.pool.Query(ctx, accountStatsQuery, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.AccountUploadStats{}
	for rows.Next() {
		var s pipelineq.AccountUploadStats
		var lastUpload *time.Time
		if err := rows.Scan(&s.SocialAccountID, &s.Total, &s.Uploaded, &s.Published, &s.Failed, &lastUpload); err != nil {
			return nil, err
		}
		s.LastUploadAt = lastUpload
		out = append(out, s)
	}
	return out, rows.Err()
}
