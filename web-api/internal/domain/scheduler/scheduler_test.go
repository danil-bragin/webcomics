package scheduler

import (
	"testing"
	"time"
)

func mkSlot(t time.Time) SlotPoint { return SlotPoint{ScheduledAt: t, Status: StatusPending} }

func TestCanSchedule_NoLimit(t *testing.T) {
	now := time.Now()
	rl := RateLimit{LimitN: 0, WindowHours: 24, MinGapSec: 60}
	if err := CanSchedule(now, []SlotPoint{mkSlot(now)}, rl); err != nil {
		t.Errorf("LimitN=0 must disable check, got %v", err)
	}
}

func TestCanSchedule_BlockAtLimit(t *testing.T) {
	now := time.Now()
	rl := RateLimit{LimitN: 2, WindowHours: 24, MinGapSec: 60}
	existing := []SlotPoint{
		mkSlot(now.Add(-2 * time.Hour)),
		mkSlot(now.Add(-1 * time.Hour)),
	}
	err := CanSchedule(now, existing, rl)
	if err != ErrLimitExceeded {
		t.Errorf("expected ErrLimitExceeded, got %v", err)
	}
}

func TestCanSchedule_OutsideWindow(t *testing.T) {
	now := time.Now()
	rl := RateLimit{LimitN: 2, WindowHours: 24, MinGapSec: 60}
	existing := []SlotPoint{
		mkSlot(now.Add(-25 * time.Hour)), // outside window
		mkSlot(now.Add(-1 * time.Hour)),
	}
	if err := CanSchedule(now, existing, rl); err != nil {
		t.Errorf("only 1 slot in window, should pass, got %v", err)
	}
}

func TestCanSchedule_GapViolation(t *testing.T) {
	now := time.Now()
	rl := RateLimit{LimitN: 10, WindowHours: 24, MinGapSec: 60}
	existing := []SlotPoint{mkSlot(now.Add(30 * time.Second))}
	if err := CanSchedule(now, existing, rl); err != ErrGapViolation {
		t.Errorf("30s apart with 60s gap must block, got %v", err)
	}
}

func TestCanSchedule_IgnoresCancelled(t *testing.T) {
	now := time.Now()
	rl := RateLimit{LimitN: 2, WindowHours: 24, MinGapSec: 0}
	existing := []SlotPoint{
		{ScheduledAt: now.Add(-time.Hour), Status: StatusCancelled},
		{ScheduledAt: now.Add(-2 * time.Hour), Status: StatusFailed},
		{ScheduledAt: now.Add(-3 * time.Hour), Status: StatusCompleted},
	}
	if err := CanSchedule(now, existing, rl); err != nil {
		t.Errorf("only 1 counted (completed), should pass, got %v", err)
	}
}

func TestSuggestNextFreeSlot_EmptyReturnsNotBefore(t *testing.T) {
	now := time.Now()
	rl := RateLimit{LimitN: 5, WindowHours: 24, MinGapSec: 60}
	got := SuggestNextFreeSlot(now, nil, rl)
	if !got.Equal(now.UTC()) {
		t.Errorf("expected %v got %v", now.UTC(), got)
	}
}

func TestSuggestNextFreeSlot_PastBlocker(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	rl := RateLimit{LimitN: 1, WindowHours: 1, MinGapSec: 60}
	existing := []SlotPoint{mkSlot(now)}
	got := SuggestNextFreeSlot(now, existing, rl)
	// Window=1h, so next free is after blocker + window + gap
	if !got.After(now) {
		t.Errorf("expected after %v, got %v", now, got)
	}
}

func TestScheduledUpload_Transitions(t *testing.T) {
	su, err := NewScheduledUpload("run-1", "acct-1", time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if su.Status() != StatusPending {
		t.Errorf("init status %s", su.Status())
	}
	if err := su.MarkInFlight(time.Now()); err != nil {
		t.Errorf("inflight: %v", err)
	}
	if err := su.MarkInFlight(time.Now()); err != ErrInvalidStatus {
		t.Errorf("double inflight should err, got %v", err)
	}
	if err := su.MarkCompleted(time.Now(), "https://youtu.be/abc"); err != nil {
		t.Errorf("complete: %v", err)
	}
	if err := su.Cancel(); err != ErrInvalidStatus {
		t.Errorf("cancel after complete must err")
	}
}

func TestScheduledUpload_CancelPending(t *testing.T) {
	su, _ := NewScheduledUpload("r", "a", time.Now().Add(time.Hour), nil)
	if err := su.Cancel(); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if su.Status() != StatusCancelled {
		t.Errorf("status %s", su.Status())
	}
}
