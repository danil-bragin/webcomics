// Package projects holds read-side queries + DTOs for the projects context.
package projects

import (
	"encoding/json"
	"time"
)

type ProjectView struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Defaults    json.RawMessage `json:"defaults"`
	Archived    bool            `json:"archived"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	RunsCount     int           `json:"runs_count"`
	UploadedCount int           `json:"uploaded_count"`
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
	Project        ProjectView          `json:"project"`
	Characters     []CharacterView      `json:"characters"`
	Environments   []EnvironmentView    `json:"environments"`
	SocialAccounts []SocialAccountView  `json:"social_accounts"`
	Plot           *PlotView            `json:"plot,omitempty"`
}

type SocialAccountView struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"project_id"`
	Platform           string          `json:"platform"`
	Label              string          `json:"label"`
	FirefoxProfilePath string          `json:"firefox_profile_path"`
	Extra              json.RawMessage `json:"extra"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}
