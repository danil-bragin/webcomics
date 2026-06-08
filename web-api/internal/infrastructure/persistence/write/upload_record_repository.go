package write

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/pipeline"
)

type UploadRecordRepository struct {
	tx pgx.Tx
}

func NewUploadRecordRepository(tx pgx.Tx) *UploadRecordRepository {
	return &UploadRecordRepository{tx: tx}
}

const upsUploadRecord = `
INSERT INTO pipeline_upload_records (
  id, run_id, project_id, social_account_id, step_index,
  status, provider, platform_target,
  title, description, tags, hashtags,
  visibility, made_for_kids, age_restriction,
  category_id, category_label, comments_enabled, playlist_names, scheduled_at,
  external_ref, external_id, thumbnail_asset_id,
  attempts, error, error_screenshot_asset_id,
  metadata_overridden, audience_confidence, audience_reasoning, hook,
  started_at, finished_at, created_at, updated_at,
  screenshot_trail
)
VALUES (
  $1, $2, NULLIF($3,''), NULLIF($4,''), $5,
  $6, $7, $8,
  $9, $10, $11, $12,
  $13, $14, $15,
  $16, $17, $18, $19, $20,
  $21, $22, NULLIF($23,''),
  $24, $25, NULLIF($26,''),
  $27, $28, $29, $30,
  $31, $32, $33, $34,
  $35
)
ON CONFLICT (id) DO UPDATE SET
  status = EXCLUDED.status,
  platform_target = EXCLUDED.platform_target,
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  tags = EXCLUDED.tags,
  hashtags = EXCLUDED.hashtags,
  visibility = EXCLUDED.visibility,
  made_for_kids = EXCLUDED.made_for_kids,
  age_restriction = EXCLUDED.age_restriction,
  category_id = EXCLUDED.category_id,
  category_label = EXCLUDED.category_label,
  comments_enabled = EXCLUDED.comments_enabled,
  playlist_names = EXCLUDED.playlist_names,
  scheduled_at = EXCLUDED.scheduled_at,
  external_ref = EXCLUDED.external_ref,
  external_id = EXCLUDED.external_id,
  thumbnail_asset_id = EXCLUDED.thumbnail_asset_id,
  attempts = EXCLUDED.attempts,
  error = EXCLUDED.error,
  error_screenshot_asset_id = EXCLUDED.error_screenshot_asset_id,
  metadata_overridden = EXCLUDED.metadata_overridden,
  audience_confidence = EXCLUDED.audience_confidence,
  audience_reasoning = EXCLUDED.audience_reasoning,
  hook = EXCLUDED.hook,
  started_at = EXCLUDED.started_at,
  finished_at = EXCLUDED.finished_at,
  updated_at = EXCLUDED.updated_at,
  screenshot_trail = EXCLUDED.screenshot_trail`

func (r *UploadRecordRepository) Save(ctx context.Context, u *pipeline.UploadRecord) error {
	m := u.Metadata()
	trail := u.ScreenshotTrail()
	if trail == nil {
		trail = []pipeline.ScreenshotEntry{}
	}
	trailJSON, _ := json.Marshal(trail)
	_, err := r.tx.Exec(ctx, upsUploadRecord,
		u.ID().String(), u.RunID(), u.ProjectID(), u.SocialAccountID(), u.StepIndex(),
		string(u.Status()), u.Provider(), u.PlatformTarget(),
		m.Title, m.Description, m.Tags, m.Hashtags,
		m.Visibility, m.MadeForKids, m.AgeRestriction,
		m.CategoryID, m.CategoryLabel, m.CommentsEnabled, m.PlaylistNames, m.ScheduledAt,
		u.ExternalRef(), u.ExternalID(), m.ThumbnailAssetID,
		u.Attempts(), u.ErrorMessage(), u.ErrorScreenshotAssetID(),
		u.MetadataOverridden(), u.AudienceConfidence(), u.AudienceReasoning(), u.Hook(),
		u.StartedAt(), u.FinishedAt(), u.CreatedAt(), u.UpdatedAt(),
		trailJSON)
	return err
}

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
WHERE id = $1
FOR UPDATE`

func scanUploadRecord(row pgx.Row) (*pipeline.UploadRecord, error) {
	var (
		id, runID, projectID, accountID, statusStr, provider, platformTarget string
		stepIndex                                                            int
		title, description                                                   string
		tags, hashtags, playlistNames                                        []string
		visibility, ageRestriction, categoryID, categoryLab                  string
		madeForKids, commentsEnabled                                         bool
		scheduledAt                                                          *time.Time
		externalRef, externalID, thumbnailAssetID                            string
		attempts                                                             int
		errMsg, shotAssetID                                                  string
		metadataOverridden                                                   bool
		audienceConfidence                                                   float64
		audienceReasoning, hook                                              string
		trailRaw                                                             []byte
		startedAt, finishedAt                                                *time.Time
		createdAt, updatedAt                                                 time.Time
	)
	if err := row.Scan(&id, &runID, &projectID, &accountID, &stepIndex,
		&statusStr, &provider, &platformTarget,
		&title, &description, &tags, &hashtags,
		&visibility, &madeForKids, &ageRestriction,
		&categoryID, &categoryLab, &commentsEnabled, &playlistNames, &scheduledAt,
		&externalRef, &externalID, &thumbnailAssetID,
		&attempts, &errMsg, &shotAssetID,
		&metadataOverridden, &audienceConfidence, &audienceReasoning, &hook,
		&trailRaw,
		&startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var trail []pipeline.ScreenshotEntry
	_ = json.Unmarshal(trailRaw, &trail)
	meta := pipeline.UploadMetadata{
		Title: title, Description: description, Tags: tags, Hashtags: hashtags,
		Visibility: visibility, MadeForKids: madeForKids, AgeRestriction: ageRestriction,
		CategoryID: categoryID, CategoryLabel: categoryLab,
		CommentsEnabled: commentsEnabled, PlaylistNames: playlistNames,
		ScheduledAt: scheduledAt, ThumbnailAssetID: thumbnailAssetID,
	}
	rec := pipeline.ReconstituteUploadRecordFull(
		pipeline.UploadRecordID(id), runID, projectID, accountID, provider, platformTarget,
		stepIndex, pipeline.UploadRecordStatus(statusStr), meta,
		metadataOverridden, audienceConfidence, audienceReasoning, hook,
		externalRef, externalID, attempts, errMsg, shotAssetID,
		startedAt, finishedAt, createdAt, updatedAt,
	)
	if trail != nil {
		rec.SetScreenshotTrail(trail)
	}
	return rec, nil
}

func (r *UploadRecordRepository) GetByID(ctx context.Context, id pipeline.UploadRecordID) (*pipeline.UploadRecord, error) {
	row := r.tx.QueryRow(ctx, selUploadRecord, id.String())
	rec, err := scanUploadRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pipeline.ErrUploadRecordNotFound
	}
	return rec, err
}

const listUploadRecordsByRun = `
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

func (r *UploadRecordRepository) CountByAccountProviderSince(ctx context.Context, accountID, provider string, since time.Time) (int, error) {
	var n int
	err := r.tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM pipeline_upload_records
		   WHERE social_account_id = $1 AND provider = $2
		     AND created_at >= $3 AND status NOT IN ('failed','rejected')`,
		accountID, provider, since).Scan(&n)
	return n, err
}

func (r *UploadRecordRepository) ListByRun(ctx context.Context, runID string) ([]*pipeline.UploadRecord, error) {
	rows, err := r.tx.Query(ctx, listUploadRecordsByRun, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*pipeline.UploadRecord{}
	for rows.Next() {
		rec, err := scanUploadRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
