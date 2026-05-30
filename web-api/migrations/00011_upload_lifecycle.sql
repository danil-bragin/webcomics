-- 00011 — upload lifecycle expansion: review gate + manual edits.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE pipeline_upload_records
    ADD COLUMN metadata_overridden BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN audience_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN audience_reasoning  TEXT NOT NULL DEFAULT '',
    ADD COLUMN hook                TEXT NOT NULL DEFAULT '',
    ADD COLUMN platform_target     TEXT NOT NULL DEFAULT '';
        -- youtube_shorts | youtube_long | instagram_reels | tiktok | twitter
-- +goose StatementEnd

-- New legal statuses: pending_review, approved, rejected, uploading, metadata_ready.
-- Status was already TEXT so no schema change needed.

-- +goose Down

-- +goose StatementBegin
ALTER TABLE pipeline_upload_records
    DROP COLUMN metadata_overridden,
    DROP COLUMN audience_confidence,
    DROP COLUMN audience_reasoning,
    DROP COLUMN hook,
    DROP COLUMN platform_target;
-- +goose StatementEnd
