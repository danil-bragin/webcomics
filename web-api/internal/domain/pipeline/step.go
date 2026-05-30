package pipeline

import (
	"encoding/json"
	"time"
)

// Legacy aliases. New code uses AttemptStatus directly.
type StepStatus = AttemptStatus

const (
	StepPending   = AttemptPending
	StepRunning   = AttemptRunning
	StepCompleted = AttemptCompleted
	StepFailed    = AttemptFailed
)

// Step is a slot in the pipeline — its identity is (run_id, step_index,
// step_type) and it does not change once created. All execution state lives
// in attempts. The active attempt is the one driving downstream behaviour
// and shown by default in the UI.
//
// is_stale is set when an upstream step's active version moves past the
// version the active attempt consumed. It signals the UI "regenerate me to
// catch up" without auto-running anything.
type Step struct {
	id              StepID
	index           int
	stepType        StepType
	currentVersion  int
	isStale         bool
	attempts        []*StepAttempt
	activeAttemptID AttemptID
}

func newStep(idx int, cfg StepConfig) *Step {
	return &Step{
		id:             NewStepID(),
		index:          idx,
		stepType:       cfg.Type,
		currentVersion: 0,
	}
}

func (s *Step) ID() StepID                 { return s.id }
func (s *Step) Index() int                 { return s.index }
func (s *Step) Type() StepType             { return s.stepType }
func (s *Step) CurrentVersion() int        { return s.currentVersion }
func (s *Step) IsStale() bool              { return s.isStale }
func (s *Step) Attempts() []*StepAttempt   { return s.attempts }
func (s *Step) ActiveAttemptID() AttemptID { return s.activeAttemptID }

// ActiveAttempt returns the attempt currently driving this step. nil if the
// step has never run.
func (s *Step) ActiveAttempt() *StepAttempt {
	for _, a := range s.attempts {
		if a.id == s.activeAttemptID {
			return a
		}
	}
	if len(s.attempts) > 0 {
		return s.attempts[len(s.attempts)-1]
	}
	return nil
}

// Status returns the active attempt's status or "pending" if none.
func (s *Step) Status() AttemptStatus {
	if a := s.ActiveAttempt(); a != nil {
		return a.status
	}
	return AttemptPending
}

// Convenience passthroughs to the active attempt so callers that don't care
// about versioning can keep using the old shape.
func (s *Step) Input() json.RawMessage {
	if a := s.ActiveAttempt(); a != nil {
		return a.input
	}
	return nil
}
func (s *Step) Outputs() json.RawMessage {
	if a := s.ActiveAttempt(); a != nil {
		return a.outputs
	}
	return json.RawMessage("[]")
}
func (s *Step) Provider() string {
	if a := s.ActiveAttempt(); a != nil {
		return a.provider
	}
	return ""
}
func (s *Step) Model() string {
	if a := s.ActiveAttempt(); a != nil {
		return a.model
	}
	return ""
}
func (s *Step) CostUSD() float64 {
	if a := s.ActiveAttempt(); a != nil {
		return a.costUSD
	}
	return 0
}
func (s *Step) PanelsExpected() int {
	if a := s.ActiveAttempt(); a != nil {
		return a.panelsExpected
	}
	return 1
}
func (s *Step) PanelsCompleted() int {
	if a := s.ActiveAttempt(); a != nil {
		return a.panelsCompleted
	}
	return 0
}
func (s *Step) Error() string {
	if a := s.ActiveAttempt(); a != nil {
		return a.errorMsg
	}
	return ""
}
func (s *Step) StartedAt() *time.Time {
	if a := s.ActiveAttempt(); a != nil {
		return a.startedAt
	}
	return nil
}
func (s *Step) FinishedAt() *time.Time {
	if a := s.ActiveAttempt(); a != nil {
		return a.finishedAt
	}
	return nil
}

// addAttempt appends a new attempt, bumps the version, marks it active,
// and returns it. Called by Run when starting a step or regenerating it.
func (s *Step) addAttempt(input, paramsOverride json.RawMessage, upstream map[int]int, expectedPanels int, provider, model string) *StepAttempt {
	s.currentVersion++
	a := newAttempt(s.id, s.currentVersion, input, paramsOverride, upstream, expectedPanels, provider, model)
	s.attempts = append(s.attempts, a)
	s.activeAttemptID = a.id
	s.isStale = false
	return a
}

func (s *Step) markStale()  { s.isStale = true }
func (s *Step) clearStale() { s.isStale = false }

// ReconstituteStep rebuilds a step from row data + already-loaded attempts.
func ReconstituteStep(
	id StepID, idx int, t StepType,
	currentVersion int, isStale bool,
	activeAttemptID AttemptID,
	attempts []*StepAttempt,
) *Step {
	return &Step{
		id: id, index: idx, stepType: t,
		currentVersion: currentVersion, isStale: isStale,
		activeAttemptID: activeAttemptID,
		attempts:        attempts,
	}
}
