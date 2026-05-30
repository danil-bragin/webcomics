-- Format presets — visual+composition recipes (manga, webtoon, noir, …).
-- Previously hardcoded in domain/formats/library.go. Migrating to DB so users
-- can fork system formats, edit prompt prefixes/suffixes, and see how each
-- format influences the image+script prompts at run time.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE format_presets (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    scope                 TEXT NOT NULL DEFAULT 'user',
    icon                  TEXT NOT NULL DEFAULT '',
    script_system_suffix  TEXT NOT NULL DEFAULT '',
    image_prompt_prefix   TEXT NOT NULL DEFAULT '',
    image_prompt_suffix   TEXT NOT NULL DEFAULT '',
    image_model           TEXT NOT NULL DEFAULT '',
    style_reference       TEXT NOT NULL DEFAULT '',
    fps                   INTEGER NOT NULL DEFAULT 30,
    width                 INTEGER NOT NULL DEFAULT 1080,
    height                INTEGER NOT NULL DEFAULT 1080,
    codec                 TEXT NOT NULL DEFAULT 'h264',
    panel_duration_ms     INTEGER NOT NULL DEFAULT 2500,
    transition            TEXT NOT NULL DEFAULT 'crossfade',
    is_system             BOOLEAN NOT NULL DEFAULT FALSE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_format_presets_scope ON format_presets (scope);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE format_presets
    ADD CONSTRAINT format_presets_scope_chk CHECK (scope IN ('system','user'));
-- +goose StatementEnd

-- Seed 12 canonical formats from the old hardcoded library. is_system=TRUE so
-- the UI can mark them read-only by default with a "Fork" affordance.
-- +goose StatementBegin
INSERT INTO format_presets
  (id, name, description, scope, icon, script_system_suffix,
   image_prompt_prefix, image_prompt_suffix, image_model, style_reference,
   fps, width, height, codec, panel_duration_ms, transition, is_system)
VALUES
('slideshow','Slideshow',
 'Plain image-to-image transitions. No specific art-style cues. Closest to the original default.',
 'system','🖼','',
 '','','','none',
 30,1080,1080,'h264',2500,'crossfade',TRUE),

('manga','Manga',
 'Black ink, halftone screentones, dramatic eyes, sound effects. Vertical portrait.',
 'system','🗯','Use snappy dialogue and Japanese-style sound effects (BANG! WHOOSH!). Describe each panel in a dramatic black-and-white manga aesthetic with motion lines.',
 'manga style, black ink linework, halftone screentone shading, dramatic motion lines, expressive eyes, ',
 ', traditional manga panel composition',
 'fal-ai/flux-2/edit','anchor',
 24,1080,1920,'h264',3500,'wipe',TRUE),

('webtoon','Webtoon / Manhwa',
 'Full-color soft digital painting. Vertical scroll feel, expressive faces, frequent close-ups.',
 'system','📱','Compose each panel as if seen scrolling vertically. Favor close-ups and emotional beats. Use modern conversational dialogue.',
 'webtoon manhwa style, soft digital painting, smooth shading, vibrant colors, expressive faces, ',
 ', clean vertical composition',
 'fal-ai/flux-2/edit','anchor',
 30,1080,1920,'h264',4000,'fade',TRUE),

('american_superhero','American Superhero',
 'Bold ink, primary colors, dynamic action poses, halftone shading. Bronze-age comic feel.',
 'system','💥','Write punchy action-comic dialogue. Use bold exclamations and dynamic poses. Captions in third-person omniscient narration.',
 'American comic book art style, bold ink outlines, primary colors red blue yellow, halftone shading, dynamic action pose, ',
 ', dramatic comic panel composition',
 'fal-ai/flux-2/edit','anchor',
 30,1080,1080,'h264',3000,'slide',TRUE),

('ligne_claire','Ligne Claire (Franco-Belgian)',
 'Clean uniform line weights, flat colors, no shading, detailed backgrounds. Tintin / Asterix lineage.',
 'system','📜','Write witty Franco-Belgian comic dialogue with detailed scenery descriptions in each panel.',
 'ligne claire style, clean uniform black ink lines, flat colors no shading, detailed background, Hergé Tintin style, ',
 ', classic European comic composition',
 'fal-ai/flux-2/edit','anchor',
 30,1440,1080,'h264',3000,'crossfade',TRUE),

('indie_alt','Indie / Alternative',
 'Scratchy hand-drawn linework, muted earthtones, mundane subjects. Daniel Clowes / R. Crumb feel.',
 'system','🖋','Write deadpan, character-driven dialogue. Quiet observational moments. Avoid action — favor introspection.',
 'indie alternative comic art, scratchy hand-drawn linework, muted earthtone palette, awkward composition, ',
 ', quiet observational scene',
 'fal-ai/flux/schnell','none',
 30,1080,1080,'h264',3500,'fade',TRUE),

('graphic_novel','Graphic Novel (Painted)',
 'Full painted illustration, cinematic lighting, complex composition. Premium look.',
 'system','🎨','Write each panel like a cinematic still. Detailed visual setup, atmospheric lighting cues. Sparse dialogue.',
 'graphic novel painted illustration, cinematic lighting, rich color palette, detailed atmospheric composition, ',
 ', premium graphic novel page composition',
 'fal-ai/nano-banana-pro/edit','anchor',
 30,1920,1080,'h264',4500,'crossfade',TRUE),

('newspaper_strip','Newspaper Strip',
 'Cartoony, dialogue-heavy 3-4 panel horizontal strip. Calvin & Hobbes / Garfield vibe.',
 'system','📰','Write a 3-4 panel setup-punchline strip. Snappy dialogue. End on a visual gag.',
 'newspaper comic strip cartoon style, simple cartoony lines, flat colors, expressive characters, ',
 ', clean strip panel composition',
 'fal-ai/flux/schnell','anchor',
 30,1920,1080,'h264',3000,'slide',TRUE),

('noir','Noir',
 'High contrast black and white, harsh shadows. Lighter moderation profile.',
 'system','🌑','Write light noir narration in first person. Short clipped sentences. Atmospheric, not violent.',
 'noir comic illustration, high contrast black and white, soft chiaroscuro shadows, expressive faces, cozy mystery vibe, ',
 ', clean panel composition, family-friendly',
 'fal-ai/flux/schnell','anchor',
 24,1080,1920,'h264',4000,'fade',TRUE),

('watercolor','Watercolor Storybook',
 'Soft watercolor washes, hand-lettered captions, gentle children''s book feel.',
 'system','🎨','Write gentle storybook narration. Wonder-filled descriptions. Lyrical captions.',
 'soft watercolor illustration, gentle wash, hand-drawn lines, children''s storybook style, ',
 ', whimsical composition',
 'fal-ai/flux/schnell','anchor',
 30,1080,1080,'h264',4000,'fade',TRUE),

('pixel_retro','Pixel / Retro Game',
 'Chunky pixel art, limited palette, 16-bit JRPG feel.',
 'system','👾','Write 16-bit JRPG cutscene dialogue. Short text-box style lines. Use stage directions like [BATTLE START].',
 'pixel art style, chunky pixels, limited 16-bit color palette, retro video game aesthetic, ',
 ', classic JRPG composition',
 'fal-ai/flux/schnell','anchor',
 30,1080,1080,'h264',3000,'wipe',TRUE),

('cinematic_3d','Cinematic 3D',
 'Rendered CGI with soft global illumination. Pixar-adjacent premium look.',
 'system','🎬','Write cinematic scene descriptions with explicit camera direction. Wide → medium → close-up arcs.',
 'cinematic 3D rendered illustration, soft global illumination, Pixar-quality, detailed character expression, ',
 ', cinematic widescreen composition',
 'fal-ai/nano-banana-pro/edit','anchor',
 30,1920,1080,'h264',4000,'crossfade',TRUE)

ON CONFLICT (id) DO UPDATE SET
  name                  = EXCLUDED.name,
  description           = EXCLUDED.description,
  icon                  = EXCLUDED.icon,
  script_system_suffix  = EXCLUDED.script_system_suffix,
  image_prompt_prefix   = EXCLUDED.image_prompt_prefix,
  image_prompt_suffix   = EXCLUDED.image_prompt_suffix,
  image_model           = EXCLUDED.image_model,
  style_reference       = EXCLUDED.style_reference,
  fps                   = EXCLUDED.fps,
  width                 = EXCLUDED.width,
  height                = EXCLUDED.height,
  codec                 = EXCLUDED.codec,
  panel_duration_ms     = EXCLUDED.panel_duration_ms,
  transition            = EXCLUDED.transition,
  is_system             = EXCLUDED.is_system,
  updated_at            = now();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS format_presets;
-- +goose StatementEnd
