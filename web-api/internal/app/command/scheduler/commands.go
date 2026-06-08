// Package scheduler holds write-side commands for scheduled uploads.
//
// Resolution chain in ScheduleUpload:
//  1. Load social account → rate-limit config (limit, window, gap).
//  2. Load existing slots in [target ± window].
//  3. Pure domain CanSchedule check.
//  4. On OK insert row; on ErrLimitExceeded / ErrGapViolation return next
//     free slot suggestion to the caller (no DB write).
package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/domain/scheduler"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// --- ScheduleUpload ---

type ScheduleUpload struct {
	RunID           string
	SocialAccountID string
	ScheduledAt     time.Time
	Metadata        map[string]any
}

func (ScheduleUpload) IsCommand() {}

type ScheduleUploadResult struct {
	ID            string
	NextFreeSlot  *time.Time // populated when LimitExceeded — caller can offer Use suggested
	BlockedReason string     // empty when scheduled OK
}

type ScheduleUploadHandler struct{ uow uow.Manager }

func NewScheduleUploadHandler(m uow.Manager) *ScheduleUploadHandler {
	return &ScheduleUploadHandler{uow: m}
}

func (h *ScheduleUploadHandler) Handle(ctx context.Context, cmd ScheduleUpload) (ScheduleUploadResult, error) {
	var out ScheduleUploadResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		acct, err := repos.Projects().GetSocialAccount(ctx, projects.SocialAccountID(cmd.SocialAccountID))
		if err != nil {
			return err
		}
		rl := scheduler.RateLimit{
			LimitN:      acct.DailyUploadLimit(),
			WindowHours: acct.LimitWindowHours(),
			MinGapSec:   acct.MinGapSeconds(),
		}
		slots, err := repos.Scheduler().ListSlotsInWindow(ctx, cmd.SocialAccountID, cmd.ScheduledAt, rl.Window())
		if err != nil {
			return err
		}
		if err := scheduler.CanSchedule(cmd.ScheduledAt, slots, rl); err != nil {
			next := scheduler.SuggestNextFreeSlot(cmd.ScheduledAt, slots, rl)
			out.NextFreeSlot = &next
			out.BlockedReason = err.Error()
			// Return a typed error so the HTTP layer can convert to 409.
			return errBlockedByLimit{Inner: err, NextFree: next}
		}
		su, err := scheduler.NewScheduledUpload(cmd.RunID, cmd.SocialAccountID, cmd.ScheduledAt, cmd.Metadata)
		if err != nil {
			return err
		}
		if err := repos.Scheduler().Save(ctx, su); err != nil {
			return err
		}
		out.ID = su.ID().String()
		return nil
	})
	return out, err
}

// errBlockedByLimit lets the HTTP layer distinguish "limit blocked, suggest
// next slot" from generic errors. ScheduleUploadResult.NextFreeSlot is the
// payload the UI shows; the error keeps the dispatch surface honest.
type errBlockedByLimit struct {
	Inner    error
	NextFree time.Time
}

func (e errBlockedByLimit) Error() string { return e.Inner.Error() }
func (e errBlockedByLimit) Unwrap() error { return e.Inner }

// IsBlockedByLimit reports whether err came from the limit check.
func IsBlockedByLimit(err error) (next time.Time, ok bool) {
	var blk errBlockedByLimit
	if errors.As(err, &blk) {
		return blk.NextFree, true
	}
	return time.Time{}, false
}

// --- CancelScheduledUpload ---

type CancelScheduledUpload struct{ ID string }

func (CancelScheduledUpload) IsCommand() {}

type CancelScheduledUploadResult struct{}

type CancelScheduledUploadHandler struct{ uow uow.Manager }

func NewCancelScheduledUploadHandler(m uow.Manager) *CancelScheduledUploadHandler {
	return &CancelScheduledUploadHandler{uow: m}
}

func (h *CancelScheduledUploadHandler) Handle(ctx context.Context, cmd CancelScheduledUpload) (CancelScheduledUploadResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Scheduler()
		su, err := repo.Get(ctx, scheduler.ID(cmd.ID))
		if err != nil {
			return err
		}
		if err := su.Cancel(); err != nil {
			return err
		}
		return repo.Save(ctx, su)
	})
	return CancelScheduledUploadResult{}, err
}

// --- RescheduleUpload ---

type RescheduleUpload struct {
	ID          string
	ScheduledAt time.Time
}

func (RescheduleUpload) IsCommand() {}

type RescheduleUploadResult struct {
	NextFreeSlot  *time.Time
	BlockedReason string
}

type RescheduleUploadHandler struct{ uow uow.Manager }

func NewRescheduleUploadHandler(m uow.Manager) *RescheduleUploadHandler {
	return &RescheduleUploadHandler{uow: m}
}

func (h *RescheduleUploadHandler) Handle(ctx context.Context, cmd RescheduleUpload) (RescheduleUploadResult, error) {
	var out RescheduleUploadResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Scheduler()
		su, err := repo.Get(ctx, scheduler.ID(cmd.ID))
		if err != nil {
			return err
		}
		acct, err := u.Repositories().Projects().GetSocialAccount(ctx, projects.SocialAccountID(su.SocialAccountID()))
		if err != nil {
			return err
		}
		rl := scheduler.RateLimit{
			LimitN:      acct.DailyUploadLimit(),
			WindowHours: acct.LimitWindowHours(),
			MinGapSec:   acct.MinGapSeconds(),
		}
		slots, err := repo.ListSlotsInWindow(ctx, su.SocialAccountID(), cmd.ScheduledAt, rl.Window())
		if err != nil {
			return err
		}
		// Exclude self from the count: filter slots whose status matches our row.
		// Practically: pull all then skip our current scheduled_at.
		filtered := make([]scheduler.SlotPoint, 0, len(slots))
		for _, p := range slots {
			if p.ScheduledAt.Equal(su.ScheduledAt()) && p.Status == su.Status() {
				continue // ours
			}
			filtered = append(filtered, p)
		}
		if err := scheduler.CanSchedule(cmd.ScheduledAt, filtered, rl); err != nil {
			next := scheduler.SuggestNextFreeSlot(cmd.ScheduledAt, filtered, rl)
			out.NextFreeSlot = &next
			out.BlockedReason = err.Error()
			return errBlockedByLimit{Inner: err, NextFree: next}
		}
		if err := su.Reschedule(cmd.ScheduledAt); err != nil {
			return err
		}
		return repo.Save(ctx, su)
	})
	return out, err
}

// --- SettleScheduledUpload ---
//
// Closes the in-flight scheduled_uploads row once the worker reports a terminal
// result for the run. Without this the row stays in_flight forever and the
// schedule UI can't tell uploaded from stuck. Best-effort: if no in_flight row
// exists for the run (e.g. an ad-hoc upload not driven by the scheduler) the
// command is a no-op and returns nil.

type SettleScheduledUpload struct {
	RunID       string
	Success     bool
	ExternalRef string
	Error       string
}

func (SettleScheduledUpload) IsCommand() {}

type SettleScheduledUploadResult struct{ Settled bool }

type SettleScheduledUploadHandler struct{ uow uow.Manager }

func NewSettleScheduledUploadHandler(m uow.Manager) *SettleScheduledUploadHandler {
	return &SettleScheduledUploadHandler{uow: m}
}

func (h *SettleScheduledUploadHandler) Handle(ctx context.Context, cmd SettleScheduledUpload) (SettleScheduledUploadResult, error) {
	var out SettleScheduledUploadResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Scheduler()
		su, err := repo.FindByRunPending(ctx, cmd.RunID)
		if err != nil {
			// No in-flight row → ad-hoc upload, nothing to settle. Swallow the
			// not-found so the upload result path stays clean.
			return nil
		}
		now := time.Now().UTC()
		if cmd.Success {
			if err := su.MarkCompleted(now, cmd.ExternalRef); err != nil {
				return err
			}
		} else {
			if err := su.MarkFailed(now, cmd.Error); err != nil {
				return err
			}
		}
		if err := repo.Save(ctx, su); err != nil {
			return err
		}
		out.Settled = true
		return nil
	})
	return out, err
}

// --- Bus registrations ---

func ScheduleUploadOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[ScheduleUpload, ScheduleUploadResult](r, NewScheduleUploadHandler(m))
}
func SettleScheduledUploadOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[SettleScheduledUpload, SettleScheduledUploadResult](r, NewSettleScheduledUploadHandler(m))
}
func CancelScheduledUploadOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CancelScheduledUpload, CancelScheduledUploadResult](r, NewCancelScheduledUploadHandler(m))
}
func RescheduleUploadOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RescheduleUpload, RescheduleUploadResult](r, NewRescheduleUploadHandler(m))
}
