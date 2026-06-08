// Package projects holds read-side queries + DTOs for the projects context.
package projects

import (
	"encoding/json"
	"time"
)

type ProjectView struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Defaults      json.RawMessage `json:"defaults"`
	Archived      bool            `json:"archived"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	RunsCount     int             `json:"runs_count"`
	UploadedCount int             `json:"uploaded_count"`
}

type CharacterView struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Traits      json.RawMessage `json:"traits"`
	RefAssetIDs []string        `json:"ref_asset_ids"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type EnvironmentView struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Traits      json.RawMessage `json:"traits"`
	RefAssetIDs []string        `json:"ref_asset_ids"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type PlotBeatView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

type PlotView struct {
	ID        string         `json:"id"`
	ProjectID string         `json:"project_id"`
	Name      string         `json:"name"`
	Premise   string         `json:"premise"`
	Beats     []PlotBeatView `json:"beats"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ProjectDetailView is the "everything for /projects/:id" payload.
type ProjectDetailView struct {
	Project        ProjectView         `json:"project"`
	Characters     []CharacterView     `json:"characters"`
	Environments   []EnvironmentView   `json:"environments"`
	SocialAccounts []SocialAccountView `json:"social_accounts"`
	Plot           *PlotView           `json:"plot,omitempty"`
}

// SocialAccountView is the read DTO for a SocialAccount. ProjectID + IsDefault
// are populated only when the view is fetched inside a project context (e.g.
// ListSocialAccounts(projectID)); for the global library list those stay zero
// and ProjectCount / UploadCount carry usage stats instead.
type SocialAccountView struct {
	ID                   string          `json:"id"`
	ProjectID            string          `json:"project_id,omitempty"`
	Platform             string          `json:"platform"`
	Label                string          `json:"label"`
	FirefoxProfilePath   string          `json:"firefox_profile_path"`
	Extra                json.RawMessage `json:"extra"`
	Status               string          `json:"status"`
	LastUsedAt           *time.Time      `json:"last_used_at,omitempty"`
	CooldownUntil        *time.Time      `json:"cooldown_until,omitempty"`
	FailureStreak        int             `json:"failure_streak"`
	DefaultVisibility    string          `json:"default_visibility"`
	DefaultMadeForKids   bool            `json:"default_made_for_kids"`
	DefaultCategoryID    string          `json:"default_category_id"`
	DefaultCategoryLabel string          `json:"default_category_label"`
	DailyUploadLimit     int             `json:"daily_upload_limit"`
	LimitWindowHours     int             `json:"limit_window_hours"`
	IsVerified           bool            `json:"is_verified"`
	MinGapSeconds        int             `json:"min_gap_seconds"`
	IsDefault            bool            `json:"is_default,omitempty"`
	ProjectCount         int             `json:"project_count,omitempty"`
	UploadCount          int             `json:"upload_count,omitempty"`
	// Upload-method capabilities. HasSelenium = Firefox profile present;
	// HasAPI = a YouTube API OAuth refresh token is stored. The refresh token
	// itself is NEVER exposed — it's scrubbed from Extra by the read mapper.
	HasSelenium       bool   `json:"has_selenium"`
	HasAPI            bool   `json:"has_api"`
	OAuthChannelTitle string `json:"oauth_channel_title,omitempty"`
	// API daily quota (videos.insert ≈ 1600 units, 10k/day ⇒ ~6/day).
	APIUploadsUsed  int       `json:"api_uploads_used"`
	APIUploadsLimit int       `json:"api_uploads_limit"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
