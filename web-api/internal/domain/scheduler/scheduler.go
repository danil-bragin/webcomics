// Package scheduler holds the pure-domain entity + invariant checks for
// scheduled uploads. No SQL / no transport — that lives in the app + infra
// layers. The HTTP / command-handler asks `CanSchedule` whether a slot is
// free, given a precomputed slice of slots in the relevant window.
package scheduler

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrLimitExceeded = errors.New("scheduler: account upload limit exceeded for this window")
	ErrGapViolation  = errors.New("scheduler: too close to another scheduled upload (min gap)")
	ErrInvalidStatus = errors.New("scheduler: invalid status transition")
	ErrPastTime      = errors.New("scheduler: scheduled_at must be in the future")
)

type ID string

func NewID() ID         { return ID(uuid.NewString()) }
func (id ID) String() string { return string(id) }

type Status string

const (
	StatusPending   Status = "pending"
	StatusInFlight  Status = "in_flight"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusInFlight, StatusCompleted, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// RateLimit captures the per-account window: at most N uploads in W hours,
// with at least MinGap seconds between two consecutive slots. Zero N disables
// rate limiting; zero W defaults to 24h.
type RateLimit struct {
	LimitN      int
	WindowHours int
	MinGapSec   int
}

func (r RateLimit) Window() time.Duration {
	if r.WindowHours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(r.WindowHours) * time.Hour
}

// ScheduledUpload is the aggregate root. All mutators return errors instead
// of panicking so the command layer can map them to HTTP 4xx.
type ScheduledUpload struct {
	id              ID
	runID           string
	socialAccountID string
	scheduledAt     time.Time
	status          Status
	externalRef     string
	error           string
	metadata        map[string]any
	createdAt       time.Time
	updatedAt       time.Time
	firedAt         *time.Time
	completedAt     *time.Time
}

func NewScheduledUpload(runID, accountID string, scheduledAt time.Time, metadata map[string]any) (*ScheduledUpload, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, errors.New("scheduler: run_id required")
	}
	if strings.TrimSpace(accountID) == "" {
		return nil, errors.New("scheduler: social_account_id required")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	now := time.Now().UTC()
	return &ScheduledUpload{
		id:              NewID(),
		runID:           runID,
		socialAccountID: accountID,
		scheduledAt:     scheduledAt.UTC(),
		status:          StatusPending,
		metadata:        metadata,
		createdAt:       now,
		updatedAt:       now,
	}, nil
}

func Reconstitute(id ID, runID, accountID string, scheduledAt time.Time, status Status,
	externalRef, errMsg string, metadata map[string]any,
	createdAt, updatedAt time.Time, firedAt, completedAt *time.Time) *ScheduledUpload {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if !status.Valid() {
		status = StatusPending
	}
	return &ScheduledUpload{
		id: id, runID: runID, socialAccountID: accountID, scheduledAt: scheduledAt.UTC(),
		status: status, externalRef: externalRef, error: errMsg, metadata: metadata,
		createdAt: createdAt, updatedAt: updatedAt, firedAt: firedAt, completedAt: completedAt,
	}
}

// Getters.
func (s *ScheduledUpload) ID() ID                    { return s.id }
func (s *ScheduledUpload) RunID() string             { return s.runID }
func (s *ScheduledUpload) SocialAccountID() string   { return s.socialAccountID }
func (s *ScheduledUpload) ScheduledAt() time.Time    { return s.scheduledAt }
func (s *ScheduledUpload) Status() Status            { return s.status }
func (s *ScheduledUpload) ExternalRef() string       { return s.externalRef }
func (s *ScheduledUpload) Error() string             { return s.error }
func (s *ScheduledUpload) Metadata() map[string]any  { return s.metadata }
func (s *ScheduledUpload) CreatedAt() time.Time      { return s.createdAt }
func (s *ScheduledUpload) UpdatedAt() time.Time      { return s.updatedAt }
func (s *ScheduledUpload) FiredAt() *time.Time       { return s.firedAt }
func (s *ScheduledUpload) CompletedAt() *time.Time   { return s.completedAt }

// State transitions.

func (s *ScheduledUpload) MarkInFlight(now time.Time) error {
	if s.status != StatusPending {
		return ErrInvalidStatus
	}
	s.status = StatusInFlight
	t := now.UTC()
	s.firedAt = &t
	s.updatedAt = t
	return nil
}

func (s *ScheduledUpload) MarkCompleted(now time.Time, externalRef string) error {
	if s.status != StatusInFlight && s.status != StatusPending {
		return ErrInvalidStatus
	}
	s.status = StatusCompleted
	s.externalRef = externalRef
	t := now.UTC()
	s.completedAt = &t
	s.updatedAt = t
	return nil
}

func (s *ScheduledUpload) MarkFailed(now time.Time, errMsg string) error {
	if s.status == StatusCancelled || s.status == StatusCompleted {
		return ErrInvalidStatus
	}
	s.status = StatusFailed
	s.error = errMsg
	t := now.UTC()
	s.completedAt = &t
	s.updatedAt = t
	return nil
}

func (s *ScheduledUpload) Cancel() error {
	if s.status == StatusCompleted || s.status == StatusCancelled {
		return ErrInvalidStatus
	}
	s.status = StatusCancelled
	s.updatedAt = time.Now().UTC()
	return nil
}

func (s *ScheduledUpload) Reschedule(at time.Time) error {
	if s.status != StatusPending {
		return ErrInvalidStatus
	}
	s.scheduledAt = at.UTC()
	s.updatedAt = time.Now().UTC()
	return nil
}

// SlotPoint is a minimal projection of an existing scheduled upload — only
// what `CanSchedule` needs. The repo materialises these from a SELECT.
type SlotPoint struct {
	ScheduledAt time.Time
	Status      Status // ignored in count when Cancelled/Failed
}

// CanSchedule returns nil if a new upload at `targetAt` would fit under the
// rate limit for `existing` slots (already in DB, within ±window of targetAt).
// Caller is responsible for the SELECT; this stays pure for unit testing.
//
// Rules:
//   - Skip cancelled/failed rows from the count.
//   - LimitN <= 0 disables the limit entirely (return nil).
//   - Two slots within MinGapSec of each other are an error.
func CanSchedule(targetAt time.Time, existing []SlotPoint, rl RateLimit) error {
	if rl.LimitN <= 0 {
		return nil
	}
	// Count uploads inside [target - window, target + window].
	window := rl.Window()
	low := targetAt.Add(-window)
	high := targetAt.Add(window)
	count := 0
	for _, p := range existing {
		if p.Status == StatusCancelled || p.Status == StatusFailed {
			continue
		}
		if p.ScheduledAt.Before(low) || p.ScheduledAt.After(high) {
			continue
		}
		count++
		if rl.MinGapSec > 0 {
			gap := p.ScheduledAt.Sub(targetAt)
			if gap < 0 {
				gap = -gap
			}
			if gap < time.Duration(rl.MinGapSec)*time.Second {
				return ErrGapViolation
			}
		}
	}
	if count >= rl.LimitN {
		return ErrLimitExceeded
	}
	return nil
}

// SuggestNextFreeSlot finds the earliest time >= notBefore where CanSchedule
// would succeed against `existing`. Walks existing rows in chronological order
// and proposes slots after each blocking neighbour + MinGap. Bounded scan: at
// most LimitN * 2 iterations.
func SuggestNextFreeSlot(notBefore time.Time, existing []SlotPoint, rl RateLimit) time.Time {
	if rl.LimitN <= 0 {
		return notBefore
	}
	// Filter + sort live rows.
	live := make([]SlotPoint, 0, len(existing))
	for _, p := range existing {
		if p.Status == StatusCancelled || p.Status == StatusFailed {
			continue
		}
		live = append(live, p)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].ScheduledAt.Before(live[j].ScheduledAt) })

	candidate := notBefore.UTC()
	gap := time.Duration(rl.MinGapSec) * time.Second
	for tries := 0; tries < rl.LimitN*2+8; tries++ {
		if err := CanSchedule(candidate, live, rl); err == nil {
			return candidate
		}
		// Find the upload whose window we're conflicting with — push past it.
		var blocker *time.Time
		for i := range live {
			diff := live[i].ScheduledAt.Sub(candidate)
			if diff < 0 {
				diff = -diff
			}
			if diff < gap || withinWindow(live[i].ScheduledAt, candidate, rl.Window()) {
				t := live[i].ScheduledAt
				if blocker == nil || t.After(*blocker) {
					blocker = &t
				}
			}
		}
		if blocker == nil {
			candidate = candidate.Add(gap)
			continue
		}
		candidate = blocker.Add(gap)
	}
	return candidate
}

func withinWindow(t, target time.Time, window time.Duration) bool {
	diff := t.Sub(target)
	if diff < 0 {
		diff = -diff
	}
	return diff <= window
}
