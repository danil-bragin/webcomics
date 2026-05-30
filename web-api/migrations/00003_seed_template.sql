-- +goose Up
-- +goose StatementBegin
-- Seed one default template so the UI works on a fresh database.
INSERT INTO pipeline_templates (id, name, steps, created_at, updated_at)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'default-meme-3panel',
    '[
        {"type":"script","model":"openai/gpt-4o-mini","system_prompt":"Write a 3-panel meme webcomic script. Output strict JSON {\"panels\":[{\"index\":int,\"prompt\":\"...\",\"caption\":\"...\"}]}","params":{"panel_count":3}},
        {"type":"image","model":"fal-ai/flux/schnell","params":{"image_size":"square_hd","num_inference_steps":4}},
        {"type":"assemble","params":{"width":1080,"height":1080,"fps":30,"panel_duration_ms":2500,"transition":"crossfade"}}
    ]'::jsonb,
    now(),
    now()
)
ON CONFLICT (id) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM pipeline_templates WHERE id = '00000000-0000-0000-0000-000000000001';
-- +goose StatementEnd
