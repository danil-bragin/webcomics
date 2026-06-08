package write

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/audiolib"
)

type AudioRepository struct{ tx pgx.Tx }

func NewAudioRepository(tx pgx.Tx) *AudioRepository {
	return &AudioRepository{tx: tx}
}

const upsAudioTrack = `
INSERT INTO audio_tracks
  (id, kind, title, tags, mood, duration_ms, object_key, bucket,
   source, source_ref, attribution, scope, project_id, bytes, created_at, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (id) DO UPDATE SET
  kind = EXCLUDED.kind,
  title = EXCLUDED.title,
  tags = EXCLUDED.tags,
  mood = EXCLUDED.mood,
  duration_ms = EXCLUDED.duration_ms,
  object_key = EXCLUDED.object_key,
  bucket = EXCLUDED.bucket,
  source = EXCLUDED.source,
  source_ref = EXCLUDED.source_ref,
  attribution = EXCLUDED.attribution,
  scope = EXCLUDED.scope,
  project_id = EXCLUDED.project_id,
  bytes = EXCLUDED.bytes`

func (r *AudioRepository) Save(ctx context.Context, t *audiolib.Track) error {
	var projectID any
	if t.ProjectID() != "" {
		projectID = t.ProjectID()
	}
	_, err := r.tx.Exec(ctx, upsAudioTrack,
		t.ID().String(), string(t.Kind()), t.Title(), t.Tags(), t.Mood(),
		t.DurationMs(), t.ObjectKey(), t.Bucket(),
		string(t.Source()), t.SourceRef(), t.Attribution(),
		string(t.Scope()), projectID, t.Bytes(),
		t.CreatedAt(), t.CreatedBy())
	return err
}

const selAudioTrack = `
SELECT id, kind, title, tags, mood, duration_ms, object_key, bucket,
       source, source_ref, attribution, scope, COALESCE(project_id,''),
       bytes, created_at, created_by
FROM audio_tracks WHERE id = $1 FOR UPDATE`

func (r *AudioRepository) Get(ctx context.Context, id audiolib.TrackID) (*audiolib.Track, error) {
	var (
		tid, kind, title, mood, objectKey, bucket  string
		source, sourceRef, attribution, scope, pid string
		createdBy                                  string
		tags                                       []string
		durationMs                                 int
		bytes                                      int64
		createdAt                                  time.Time
	)
	err := r.tx.QueryRow(ctx, selAudioTrack, id.String()).Scan(
		&tid, &kind, &title, &tags, &mood, &durationMs, &objectKey, &bucket,
		&source, &sourceRef, &attribution, &scope, &pid, &bytes,
		&createdAt, &createdBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, audiolib.ErrTrackNotFound
		}
		return nil, err
	}
	return audiolib.Reconstitute(
		audiolib.TrackID(tid), audiolib.Kind(kind), title, tags, mood,
		durationMs, objectKey, bucket, audiolib.Source(source), sourceRef, attribution,
		audiolib.Scope(scope), pid, bytes, createdAt, createdBy,
	), nil
}

func (r *AudioRepository) Delete(ctx context.Context, id audiolib.TrackID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM audio_tracks WHERE id = $1`, id.String())
	return err
}
