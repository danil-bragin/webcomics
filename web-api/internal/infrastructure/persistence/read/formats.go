package read

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	fq "github.com/example/dddcqrs/internal/app/query/formats"
)

type FormatsModel struct{ pool *pgxpool.Pool }

func NewFormatsModel(pool *pgxpool.Pool) *FormatsModel {
	return &FormatsModel{pool: pool}
}

const formatCols = `
SELECT id, name, description, scope, icon,
       script_system_suffix, image_prompt_prefix, image_prompt_suffix,
       image_model, style_reference,
       fps, width, height, codec, panel_duration_ms, transition,
       is_system, created_at, updated_at
FROM format_presets`

func scanFormat(scan func(...any) error) (fq.FormatView, error) {
	var v fq.FormatView
	var createdAt, updatedAt time.Time
	err := scan(&v.ID, &v.Name, &v.Description, &v.Scope, &v.Icon,
		&v.ScriptSystemSuffix, &v.ImagePromptPrefix, &v.ImagePromptSuffix,
		&v.ImageModel, &v.StyleReference,
		&v.FPS, &v.Width, &v.Height, &v.Codec, &v.PanelDurationMs, &v.Transition,
		&v.IsSystem, &createdAt, &updatedAt)
	v.CreatedAt = createdAt.Format(time.RFC3339)
	v.UpdatedAt = updatedAt.Format(time.RFC3339)
	return v, err
}

func (m *FormatsModel) GetFormat(ctx context.Context, id string) (fq.FormatView, error) {
	row := m.pool.QueryRow(ctx, formatCols+" WHERE id = $1", id)
	v, err := scanFormat(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

func (m *FormatsModel) ListFormats(ctx context.Context) ([]fq.FormatView, error) {
	rows, err := m.pool.Query(ctx, formatCols+" ORDER BY is_system DESC, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []fq.FormatView{}
	for rows.Next() {
		v, err := scanFormat(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
