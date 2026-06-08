package pipeline

import "time"

// UploadRecordView is the flat DTO returned by upload-record queries.
type UploadRecordView struct {
	ID                     string                `json:"id"`
	RunID                  string                `json:"run_id"`
	ProjectID              string                `json:"project_id,omitempty"`
	SocialAccountID        string                `json:"social_account_id,omitempty"`
	StepIndex              int                   `json:"step_index"`
	Status                 string                `json:"status"`
	Provider               string                `json:"provider"`
	PlatformTarget         string                `json:"platform_target,omitempty"`
	Title                  string                `json:"title"`
	Description            string                `json:"description"`
	Tags                   []string              `json:"tags"`
	Hashtags               []string              `json:"hashtags"`
	Visibility             string                `json:"visibility"`
	MadeForKids            bool                  `json:"made_for_kids"`
	AgeRestriction         string                `json:"age_restriction"`
	CategoryID             string                `json:"category_id"`
	CategoryLabel          string                `json:"category_label"`
	CommentsEnabled        bool                  `json:"comments_enabled"`
	PlaylistNames          []string              `json:"playlist_names"`
	ScheduledAt            *time.Time            `json:"scheduled_at,omitempty"`
	ExternalRef            string                `json:"external_ref,omitempty"`
	ExternalID             string                `json:"external_id,omitempty"`
	ThumbnailAssetID       string                `json:"thumbnail_asset_id,omitempty"`
	Attempts               int                   `json:"attempts"`
	Error                  string                `json:"error,omitempty"`
	ErrorScreenshotAssetID string                `json:"error_screenshot_asset_id,omitempty"`
	MetadataOverridden     bool                  `json:"metadata_overridden"`
	AudienceConfidence     float64               `json:"audience_confidence"`
	AudienceReasoning      string                `json:"audience_reasoning,omitempty"`
	Hook                   string                `json:"hook,omitempty"`
	ScreenshotTrail        []ScreenshotEntryView `json:"screenshot_trail,omitempty"`
	StartedAt              *time.Time            `json:"started_at,omitempty"`
	FinishedAt             *time.Time            `json:"finished_at,omitempty"`
	CreatedAt              time.Time             `json:"created_at"`
	UpdatedAt              time.Time             `json:"updated_at"`
	// Analytics (Phase 2).
	LastKnownViews    int64      `json:"last_known_views,omitempty"`
	LastKnownLikes    int64      `json:"last_known_likes,omitempty"`
	LastKnownComments int64      `json:"last_known_comments,omitempty"`
	LastKnownShares   int64      `json:"last_known_shares,omitempty"`
	LastFetchedAt     *time.Time `json:"last_fetched_at,omitempty"`
	FetchError        string     `json:"fetch_error,omitempty"`
}

// ScreenshotEntryView is one frame in the per-stage debug trail.
type ScreenshotEntryView struct {
	Stage     string `json:"stage"`
	ObjectKey string `json:"object_key"`
}

// AccountUploadStats aggregates per-account counters so the project upload
// dashboard can render a badge per SocialAccount without N queries.
type AccountUploadStats struct {
	SocialAccountID string     `json:"social_account_id"`
	Total           int        `json:"total"`
	Uploaded        int        `json:"uploaded"`
	Published       int        `json:"published"`
	Failed          int        `json:"failed"`
	LastUploadAt    *time.Time `json:"last_upload_at,omitempty"`
}
