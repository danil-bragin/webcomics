package read

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	aq "github.com/example/dddcqrs/internal/app/query/audiolib"
)

type AudioLibModel struct{ pool *pgxpool.Pool }

func NewAudioLibModel(pool *pgxpool.Pool) *AudioLibModel {
	return &AudioLibModel{pool: pool}
}

const audioColumns = `
SELECT id, kind, title, tags, mood, duration_ms, object_key, bucket,
       source, source_ref, attribution, scope, COALESCE(project_id,''), bytes,
       created_at, created_by
FROM audio_tracks`

func scanTrack(rows pgx.Rows) (aq.TrackView, error) {
	var v aq.TrackView
	err := rows.Scan(
		&v.ID, &v.Kind, &v.Title, &v.Tags, &v.Mood, &v.DurationMs, &v.ObjectKey, &v.Bucket,
		&v.Source, &v.SourceRef, &v.Attribution, &v.Scope, &v.ProjectID, &v.Bytes,
		&v.CreatedAt, &v.CreatedBy,
	)
	return v, err
}

func (m *AudioLibModel) ListTracks(ctx context.Context, f aq.ListTracksFilter) ([]aq.TrackView, error) {
	args := []any{}
	conds := []string{}
	add := func(c string, v any) {
		args = append(args, v)
		conds = append(conds, strings.Replace(c, "?", phN(len(args)), 1))
	}
	if f.Kind != "" {
		add("kind = ?", f.Kind)
	}
	if f.Mood != "" {
		add("mood = ?", f.Mood)
	}
	switch f.Scope {
	case "global":
		conds = append(conds, "scope = 'global'")
	case "project":
		conds = append(conds, "scope = 'project'")
		if f.ProjectID != "" {
			add("project_id = ?", f.ProjectID)
		}
	default:
		// no scope filter — if project_id present, return global ∪ that project
		if f.ProjectID != "" {
			args = append(args, f.ProjectID)
			conds = append(conds, "(scope = 'global' OR project_id = "+phN(len(args))+")")
		}
	}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		conds = append(conds, "(LOWER(title) LIKE "+phN(len(args))+" OR EXISTS (SELECT 1 FROM unnest(tags) tg WHERE LOWER(tg) LIKE "+phN(len(args))+"))")
	}
	q := audioColumns
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += " LIMIT " + phN(len(args))
	}
	if f.Offset > 0 {
		args = append(args, f.Offset)
		q += " OFFSET " + phN(len(args))
	}
	rows, err := m.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []aq.TrackView{}
	for rows.Next() {
		v, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		if v.Tags == nil {
			v.Tags = []string{}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *AudioLibModel) GetTrack(ctx context.Context, id string) (aq.TrackView, error) {
	rows, err := m.pool.Query(ctx, audioColumns+" WHERE id = $1", id)
	if err != nil {
		return aq.TrackView{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return aq.TrackView{}, ErrNotFound
	}
	v, err := scanTrack(rows)
	if err != nil {
		return v, err
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	return v, nil
}

// PickTrack returns one random track matching the filter. project_id present
// means global ∪ that project; mood is preferred when supplied but falls back
// to any track of the kind if no mood match.
func (m *AudioLibModel) PickTrack(ctx context.Context, f aq.PickFilter) (*aq.TrackView, error) {
	if f.Kind == "" {
		return nil, errors.New("audiolib: PickTrack requires kind")
	}
	pick := func(mood string) (*aq.TrackView, error) {
		args := []any{f.Kind}
		conds := []string{"kind = $1"}
		if mood != "" {
			args = append(args, mood)
			conds = append(conds, "mood = $"+phN(len(args))[1:])
		}
		if f.ProjectID != "" {
			args = append(args, f.ProjectID)
			conds = append(conds, "(scope = 'global' OR project_id = $"+phN(len(args))[1:]+")")
		} else {
			conds = append(conds, "scope = 'global'")
		}
		q := audioColumns + " WHERE " + strings.Join(conds, " AND ") + " ORDER BY random() LIMIT 1"
		rows, err := m.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		if !rows.Next() {
			return nil, nil
		}
		v, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		if v.Tags == nil {
			v.Tags = []string{}
		}
		return &v, nil
	}
	if f.Mood != "" {
		if v, err := pick(f.Mood); err != nil || v != nil {
			return v, err
		}
	}
	return pick("")
}

func phN(n int) string {
	// caller computed param index, returns "$N"
	digits := []byte("0123456789")
	if n < 10 {
		return "$" + string(digits[n])
	}
	// for n>=10 use strconv-style; keep deps light
	out := []byte("$")
	buf := []byte{}
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	out = append(out, buf...)
	return string(out)
}
