package pipeline

import (
	"encoding/json"
	"time"
)

type AttemptStatus string

const (
	AttemptPending   AttemptStatus = "pending"
	AttemptRunning   AttemptStatus = "running"
	AttemptCompleted AttemptStatus = "completed"
	AttemptFailed    AttemptStatus = "failed"
)

// StepAttempt is one execution of a step. A step may have many attempts when
// the user regenerates it; the active one drives downstream behaviour and
// shows in the UI as the "current" output.
//
// upstreamVersions records what version of each upstream step this attempt
// consumed, e.g. {"0": 2, "1": 1}. When an upstream's active version moves
// past the recorded number, the step that owns this attempt becomes stale.
type StepAttempt struct {
	id               AttemptID
	stepID           StepID
	attemptNo        int
	status           AttemptStatus
	input            json.RawMessage
	outputs          json.RawMessage
	paramsOverride   json.RawMessage
	upstreamVersions map[int]int
	provider         string
	model            string
	costUSD          float64
	panelsExpected   int
	panelsCompleted  int
	errorMsg         string
	startedAt        *time.Time
	finishedAt       *time.Time
	createdAt        time.Time
}

func newAttempt(stepID StepID, attemptNo int, input json.RawMessage, paramsOverride json.RawMessage, upstream map[int]int, expectedPanels int, provider, model string) *StepAttempt {
	if expectedPanels < 1 {
		expectedPanels = 1
	}
	now := time.Now().UTC()
	return &StepAttempt{
		id:               NewAttemptID(),
		stepID:           stepID,
		attemptNo:        attemptNo,
		status:           AttemptRunning,
		input:            input,
		outputs:          json.RawMessage("[]"),
		paramsOverride:   paramsOverride,
		upstreamVersions: upstream,
		provider:         provider,
		model:            model,
		panelsExpected:   expectedPanels,
		startedAt:        &now,
		createdAt:        now,
	}
}

// ReconstituteAttempt rebuilds an attempt from a persisted row.
func ReconstituteAttempt(
	id AttemptID, stepID StepID, attemptNo int, status AttemptStatus,
	input, outputs, paramsOverride json.RawMessage,
	upstreamVersions map[int]int,
	provider, model string, costUSD float64,
	panelsExpected, panelsCompleted int,
	errorMsg string,
	startedAt, finishedAt *time.Time,
	createdAt time.Time,
) *StepAttempt {
	if len(outputs) == 0 {
		outputs = json.RawMessage("[]")
	}
	if upstreamVersions == nil {
		upstreamVersions = map[int]int{}
	}
	return &StepAttempt{
		id: id, stepID: stepID, attemptNo: attemptNo, status: status,
		input: input, outputs: outputs, paramsOverride: paramsOverride,
		upstreamVersions: upstreamVersions,
		provider:         provider, model: model, costUSD: costUSD,
		panelsExpected: panelsExpected, panelsCompleted: panelsCompleted,
		errorMsg: errorMsg, startedAt: startedAt, finishedAt: finishedAt,
		createdAt: createdAt,
	}
}

func (a *StepAttempt) ID() AttemptID                   { return a.id }
func (a *StepAttempt) StepID() StepID                  { return a.stepID }
func (a *StepAttempt) AttemptNo() int                  { return a.attemptNo }
func (a *StepAttempt) Status() AttemptStatus           { return a.status }
func (a *StepAttempt) Input() json.RawMessage          { return a.input }
func (a *StepAttempt) Outputs() json.RawMessage        { return a.outputs }
func (a *StepAttempt) ParamsOverride() json.RawMessage { return a.paramsOverride }
func (a *StepAttempt) UpstreamVersions() map[int]int   { return a.upstreamVersions }
func (a *StepAttempt) Provider() string                { return a.provider }
func (a *StepAttempt) Model() string                   { return a.model }
func (a *StepAttempt) CostUSD() float64                { return a.costUSD }
func (a *StepAttempt) PanelsExpected() int             { return a.panelsExpected }
func (a *StepAttempt) PanelsCompleted() int            { return a.panelsCompleted }
func (a *StepAttempt) Error() string                   { return a.errorMsg }
func (a *StepAttempt) StartedAt() *time.Time           { return a.startedAt }
func (a *StepAttempt) FinishedAt() *time.Time          { return a.finishedAt }
func (a *StepAttempt) CreatedAt() time.Time            { return a.createdAt }
