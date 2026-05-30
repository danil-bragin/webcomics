-- Seed 5 canonical presets so a fresh install has discoverable starting
-- points (instead of test-spam). UPSERT on name so re-running the migration
-- against an existing install only updates metadata, never duplicates.

-- +goose Up

-- +goose StatementBegin
INSERT INTO pipeline_templates
  (id, name, description, category, icon, steps, sample_prompts, defaults, max_cost_usd, is_test)
VALUES
(
  '00000000-0000-0000-0000-000000000101',
  'meme-3panel',
  '3-panel meme video. Sharp punchline. Sub-$0.05 per render. Best for daily content cadence.',
  'meme',
  '😂',
  '[
    {"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":3}},
    {"type":"image","model":"fal-ai/flux/schnell"},
    {"type":"assemble","params":{"width":1080,"height":1080,"fps":30,"transition":"crossfade","panel_duration_ms":2500}}
  ]'::jsonb,
  '[
    "a hipster cat opens a coffee shop on Mars",
    "office worker discovers the printer is sentient",
    "lazy dog tries to become a fitness influencer"
  ]'::jsonb,
  '{"language":"en"}'::jsonb,
  0.10, FALSE
),
(
  '00000000-0000-0000-0000-000000000102',
  'shorts-voice-music',
  '9:16 vertical, TTS voiceover + background music. YouTube Shorts / TikTok / Reels ready.',
  'shorts',
  '📱',
  '[
    {"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":3}},
    {"type":"image","model":"fal-ai/flux/schnell"},
    {"type":"music"},
    {"type":"audio"},
    {"type":"assemble","params":{"width":1080,"height":1920,"fps":30,"transition":"crossfade","panel_duration_ms":3000}}
  ]'::jsonb,
  '[
    "kazakh shepherd plays dombra under the stars",
    "tiny barista perfects the impossible latte art",
    "ninja sloth steals all the smartphones in a city"
  ]'::jsonb,
  '{"language":"en","subtitles":{"enabled":true,"style":"bottom_karaoke","position":"bottom"}}'::jsonb,
  0.50, FALSE
),
(
  '00000000-0000-0000-0000-000000000103',
  'story-8panel-narrated',
  '8-panel narrative arc with full voice narration. Slower pacing, longer-form (≈25s).',
  'story',
  '📖',
  '[
    {"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":8}},
    {"type":"image","model":"fal-ai/flux/schnell","params":{"style_reference":"previous"}},
    {"type":"audio"},
    {"type":"assemble","params":{"width":1080,"height":1080,"fps":30,"transition":"fade","panel_duration_ms":3000}}
  ]'::jsonb,
  '[
    "a lost robot learns to play the violin",
    "two strangers share an umbrella in a thunderstorm",
    "old lighthouse keeper writes a letter to the sea"
  ]'::jsonb,
  '{"language":"en","style_reference":"previous"}'::jsonb,
  1.00, FALSE
),
(
  '00000000-0000-0000-0000-000000000104',
  'silent-demo',
  '3-panel silent montage. No audio, no music. Fastest cheapest preset for prompt iteration.',
  'demo',
  '🤫',
  '[
    {"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":3}},
    {"type":"image","model":"fal-ai/flux/schnell"},
    {"type":"assemble","params":{"width":1080,"height":1080,"fps":30,"transition":"none","panel_duration_ms":2000}}
  ]'::jsonb,
  '[
    "abstract neon shapes morph through cyberpunk cityscape",
    "tea ceremony in slow motion close-up",
    "sand dunes shifting from sunrise to night"
  ]'::jsonb,
  '{"language":"en"}'::jsonb,
  0.05, FALSE
),
(
  '00000000-0000-0000-0000-000000000105',
  'fixture-minimal',
  'Single-panel fixture preset reused by integration tests. Do not delete.',
  'demo',
  '🧪',
  '[
    {"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":1}},
    {"type":"image","model":"fal-ai/flux/schnell"},
    {"type":"assemble","params":{"width":512,"height":512,"fps":24,"transition":"none","panel_duration_ms":1000}}
  ]'::jsonb,
  '[]'::jsonb,
  '{}'::jsonb,
  0.01, TRUE
)
ON CONFLICT (id) DO UPDATE SET
  name           = EXCLUDED.name,
  description    = EXCLUDED.description,
  category       = EXCLUDED.category,
  icon           = EXCLUDED.icon,
  steps          = EXCLUDED.steps,
  sample_prompts = EXCLUDED.sample_prompts,
  defaults       = EXCLUDED.defaults,
  max_cost_usd   = EXCLUDED.max_cost_usd,
  is_test        = EXCLUDED.is_test,
  updated_at     = now();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM pipeline_templates WHERE id IN (
  '00000000-0000-0000-0000-000000000101',
  '00000000-0000-0000-0000-000000000102',
  '00000000-0000-0000-0000-000000000103',
  '00000000-0000-0000-0000-000000000104',
  '00000000-0000-0000-0000-000000000105'
);
-- +goose StatementEnd
