// Package pipeline holds read-side queries + DTOs for the pipeline context.
// Flat, jsonb-friendly DTOs only — no domain types.
package pipeline

import (
	"encoding/json"
	"time"
)

type RunView struct {
	ID               string          `json:"id"`
	TemplateID       string          `json:"template_id"`
	ProjectID        string          `json:"project_id,omitempty"`
	ProjectName      string          `json:"project_name,omitempty"`
	Prompt           string          `json:"prompt"`
	Status           string          `json:"status"`
	CurrentStepIndex int             `json:"current_step_index"`
	ExpectedSteps    int             `json:"expected_steps"`
	AutoAssemble     bool            `json:"auto_assemble"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	MaxCostUSD       float64         `json:"max_cost_usd"`
	Error            string          `json:"error,omitempty"`
	Language         string          `json:"language,omitempty"`
	ConfigSnapshot   json.RawMessage `json:"config_snapshot"`
	CreatedAt        time.Time       `json:"created_at"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
	Steps            []StepView      `json:"steps"`
	Assets           []AssetView     `json:"assets"`
	CostEntries      []CostEntryView `json:"cost_entries"`
}

type StepView struct {
	ID              string `json:"id"`
	Index           int    `json:"index"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	CurrentVersion  int    `json:"current_version"`
	IsStale         bool   `json:"is_stale"`
	ActiveAttemptID string `json:"active_attempt_id,omitempty"`
	// Flattened view of the active attempt — convenience for callers that
	// don't care about the version history.
	Input           json.RawMessage `json:"input"`
	Outputs         json.RawMessage `json:"outputs"`
	Provider        string          `json:"provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	CostUSD         float64         `json:"cost_usd"`
	PanelsExpected  int             `json:"panels_expected"`
	PanelsCompleted int             `json:"panels_completed"`
	Error           string          `json:"error,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	Attempts        []AttemptView   `json:"attempts"`
}

// FindActiveAttempt returns a pointer to the attempt whose ID matches
// ActiveAttemptID, or the latest one if no ID is set, or nil for an empty list.
func (s *StepView) FindActiveAttempt() *AttemptView {
	if len(s.Attempts) == 0 {
		return nil
	}
	if s.ActiveAttemptID != "" {
		for i := range s.Attempts {
			if s.Attempts[i].ID == s.ActiveAttemptID {
				return &s.Attempts[i]
			}
		}
	}
	return &s.Attempts[len(s.Attempts)-1]
}

type AttemptView struct {
	ID               string          `json:"id"`
	StepID           string          `json:"step_id"`
	AttemptNo        int             `json:"attempt_no"`
	Status           string          `json:"status"`
	Input            json.RawMessage `json:"input"`
	Outputs          json.RawMessage `json:"outputs"`
	ParamsOverride   json.RawMessage `json:"params_override,omitempty"`
	UpstreamVersions json.RawMessage `json:"upstream_versions,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	Model            string          `json:"model,omitempty"`
	CostUSD          float64         `json:"cost_usd"`
	PanelsExpected   int             `json:"panels_expected"`
	PanelsCompleted  int             `json:"panels_completed"`
	Error            string          `json:"error,omitempty"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type AssetView struct {
	ID        string    `json:"id"`
	StepID    string    `json:"step_id,omitempty"`
	AttemptID string    `json:"attempt_id,omitempty"`
	Kind      string    `json:"kind"`
	Bucket    string    `json:"bucket"`
	ObjectKey string    `json:"object_key"`
	Mime      string    `json:"mime"`
	Bytes     int64     `json:"bytes"`
	CreatedAt time.Time `json:"created_at"`
}

type CostEntryView struct {
	ID           string    `json:"id"`
	StepID       string    `json:"step_id,omitempty"`
	AttemptID    string    `json:"attempt_id,omitempty"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model,omitempty"`
	Units        float64   `json:"units"`
	UnitLabel    string    `json:"unit_label"`
	UnitCostUSD  float64   `json:"unit_cost_usd"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type RunSummary struct {
	ID                string     `json:"id"`
	TemplateID        string     `json:"template_id"`
	Prompt            string     `json:"prompt"`
	Status            string     `json:"status"`
	TotalCostUSD      float64    `json:"total_cost_usd"`
	CreatedAt         time.Time  `json:"created_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	VideoAssetID      string     `json:"video_asset_id,omitempty"`
	FirstImageAssetID string     `json:"first_image_asset_id,omitempty"`
}

type TemplateView struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Category      string          `json:"category,omitempty"`
	Icon          string          `json:"icon,omitempty"`
	Steps         json.RawMessage `json:"steps"`
	SamplePrompts []string        `json:"sample_prompts,omitempty"`
	FormatID      string          `json:"format_id,omitempty"`
	Defaults      json.RawMessage `json:"defaults,omitempty"`
	MaxCostUSD    float64         `json:"max_cost_usd"`
	IsTest        bool            `json:"is_test,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type AssetRef struct {
	ID        string `json:"id"`
	Bucket    string `json:"bucket"`
	ObjectKey string `json:"object_key"`
	Mime      string `json:"mime"`
}

type ProviderCost struct {
	Provider     string  `json:"provider"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type DayCost struct {
	Date         string  `json:"date"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type StatsView struct {
	RunsByStatus   map[string]int `json:"runs_by_status"`
	TotalCostUSD   float64        `json:"total_cost_usd"`
	CostByProvider []ProviderCost `json:"cost_by_provider"`
	CostByDay      []DayCost      `json:"cost_by_day"`
}
