package write

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/formats"
)

type FormatsRepository struct{ tx pgx.Tx }

func NewFormatsRepository(tx pgx.Tx) *FormatsRepository {
	return &FormatsRepository{tx: tx}
}

const upsFormat = `
INSERT INTO format_presets
  (id, name, description, scope, icon, script_system_suffix,
   image_prompt_prefix, image_prompt_suffix, image_model, style_reference,
   fps, width, height, codec, panel_duration_ms, transition, is_system, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, now())
ON CONFLICT (id) DO UPDATE SET
  name                 = EXCLUDED.name,
  description          = EXCLUDED.description,
  scope                = EXCLUDED.scope,
  icon                 = EXCLUDED.icon,
  script_system_suffix = EXCLUDED.script_system_suffix,
  image_prompt_prefix  = EXCLUDED.image_prompt_prefix,
  image_prompt_suffix  = EXCLUDED.image_prompt_suffix,
  image_model          = EXCLUDED.image_model,
  style_reference      = EXCLUDED.style_reference,
  fps                  = EXCLUDED.fps,
  width                = EXCLUDED.width,
  height               = EXCLUDED.height,
  codec                = EXCLUDED.codec,
  panel_duration_ms    = EXCLUDED.panel_duration_ms,
  transition           = EXCLUDED.transition,
  is_system            = EXCLUDED.is_system,
  updated_at           = now()`

func (r *FormatsRepository) Save(ctx context.Context, f *formats.Format) error {
	_, err := r.tx.Exec(ctx, upsFormat,
		f.ID, f.Name, f.Description, f.Scope, f.Icon,
		f.ScriptSystemSuffix, f.ImagePromptPrefix, f.ImagePromptSuffix,
		f.ImageModel, f.StyleReference,
		f.FPS, f.Width, f.Height, f.Codec, f.PanelDurationMs, f.Transition,
		f.Scope == "system",
	)
	return err
}

const selFormat = `
SELECT id, name, description, scope, icon, script_system_suffix,
       image_prompt_prefix, image_prompt_suffix, image_model, style_reference,
       fps, width, height, codec, panel_duration_ms, transition
FROM format_presets WHERE id = $1`

func (r *FormatsRepository) GetByID(ctx context.Context, id string) (*formats.Format, error) {
	var f formats.Format
	err := r.tx.QueryRow(ctx, selFormat, id).Scan(
		&f.ID, &f.Name, &f.Description, &f.Scope, &f.Icon,
		&f.ScriptSystemSuffix, &f.ImagePromptPrefix, &f.ImagePromptSuffix,
		&f.ImageModel, &f.StyleReference,
		&f.FPS, &f.Width, &f.Height, &f.Codec, &f.PanelDurationMs, &f.Transition,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("format: not found")
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *FormatsRepository) Delete(ctx context.Context, id string) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM format_presets WHERE id = $1 AND is_system = FALSE`, id)
	return err
}
