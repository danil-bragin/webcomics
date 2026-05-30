-- Audio library: bg music, SFX, ambient loops, voice samples.
--
-- Tracks live in MinIO under library/audio/{kind}/{id}.mp3 — this table is the
-- index that the music/audio workers consult to pick by mood / kind / scope.
-- scope='global' rows are shared across projects; scope='project' rows belong
-- to one project_id and only show up for runs in that project.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE audio_tracks (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,                 -- music | sfx | ambient | voice
    title         TEXT NOT NULL,
    tags          TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    mood          TEXT NOT NULL DEFAULT '',      -- carefree|epic|chill|sneaky|playful|energetic|smooth|...
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    object_key    TEXT NOT NULL,
    bucket        TEXT NOT NULL DEFAULT 'webcomics',
    source        TEXT NOT NULL,                 -- manual | url | pixabay
    source_ref    TEXT NOT NULL DEFAULT '',      -- original URL / pixabay id
    attribution   TEXT NOT NULL DEFAULT '',
    scope         TEXT NOT NULL,                 -- global | project
    project_id    TEXT REFERENCES projects(id) ON DELETE CASCADE,
    bytes         BIGINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_audio_tracks_lookup ON audio_tracks (kind, scope, mood);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_audio_tracks_project ON audio_tracks (project_id) WHERE project_id IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_audio_tracks_created_at ON audio_tracks (created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE audio_tracks
  ADD CONSTRAINT audio_tracks_kind_chk CHECK (kind IN ('music','sfx','ambient','voice')),
  ADD CONSTRAINT audio_tracks_scope_chk CHECK (scope IN ('global','project')),
  ADD CONSTRAINT audio_tracks_source_chk CHECK (source IN ('manual','url','pixabay')),
  ADD CONSTRAINT audio_tracks_project_scope_chk
      CHECK ((scope = 'global' AND project_id IS NULL) OR (scope = 'project' AND project_id IS NOT NULL));
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS audio_tracks;
-- +goose StatementEnd
