package scheduler

import (
	"context"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
)

// View is the flat read DTO for scheduler rows.
type View struct {
	ID              string     `json:"id"`
	RunID           string     `json:"run_id"`
	RunPrompt       string     `json:"run_prompt"`
	RunVideoAssetID string     `json:"run_video_asset_id,omitempty"`
	RunCostUSD      float64    `json:"run_cost_usd,omitempty"`
	RunStatus       string     `json:"run_status,omitempty"`
	SocialAccountID string     `json:"social_account_id"`
	AccountLabel    string     `json:"account_label"`
	AccountPlatform string     `json:"account_platform"`
	ScheduledAt     time.Time  `json:"scheduled_at"`
	Status          string     `json:"status"`
	ExternalRef     string     `json:"external_ref,omitempty"`
	Error           string     `json:"error,omitempty"`
	FiredAt         *time.Time `json:"fired_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// AccountWindowStats reports how many uploads are queued/in-flight/completed
// inside the rolling window for a given account.
type AccountWindowStats struct {
	SocialAccountID string     `json:"social_account_id"`
	LimitN          int        `json:"limit_n"`
	WindowHours     int        `json:"window_hours"`
	CountInWindow   int        `json:"count_in_window"`
	IsAtLimit       bool       `json:"is_at_limit"`
	NextFreeSlot    *time.Time `json:"next_free_slot,omitempty"`
}

type ReadModel interface {
	List(ctx context.Context, filter ListFilter) ([]View, error)
	Get(ctx context.Context, id string) (View, error)
	WindowStatsForAccount(ctx context.Context, accountID string, now time.Time) (AccountWindowStats, error)
}

type ListFilter struct {
	AccountID string
	Status    string
	Since     *time.Time
	Until     *time.Time
	Limit     int
}

// ----- ListScheduled -----

type ListScheduled struct{ Filter ListFilter }

func (ListScheduled) IsQuery() {}

type ListScheduledHandler struct{ m ReadModel }

func (h ListScheduledHandler) Handle(ctx context.Context, q ListScheduled) ([]View, error) {
	return h.m.List(ctx, q.Filter)
}

func ListScheduledOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[ListScheduled, []View](r, ListScheduledHandler{m: m})
}

// ----- GetSlotAvailability -----

type GetSlotAvailability struct {
	AccountID string
	TargetAt  time.Time
}

func (GetSlotAvailability) IsQuery() {}

type GetSlotAvailabilityHandler struct{ m ReadModel }

func (h GetSlotAvailabilityHandler) Handle(ctx context.Context, q GetSlotAvailability) (AccountWindowStats, error) {
	// The "next slot" check uses TargetAt as the window centre.
	return h.m.WindowStatsForAccount(ctx, q.AccountID, q.TargetAt)
}

func GetSlotAvailabilityOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[GetSlotAvailability, AccountWindowStats](r, GetSlotAvailabilityHandler{m: m})
}
