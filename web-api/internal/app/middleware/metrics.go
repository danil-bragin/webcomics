package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/platform/metrics"
)

// StepCompletion is implemented by Record*Completed commands so the Metrics
// middleware can fan out step-typed counters without each handler doing it.
type StepCompletion interface {
	GetStepType() string
	GetProvider() string
	GetCostUSD() float64
}

// StepFailure is implemented by RecordStepFailed.
type StepFailure interface {
	GetStepType() string
}

// RunTerminal is implemented by commands that put the run into a terminal
// state (run.completed / run.failed / run.cancelled).
type RunTerminal interface {
	GetTerminalStatus() string
}

// Metrics records pipeline_commands_total, pipeline_command_duration_seconds,
// and (when applicable) pipeline_steps_total / pipeline_step_cost_usd_total /
// pipeline_runs_total.
func Metrics(m *metrics.Metrics) bus.Middleware {
	return func(next bus.Handler) bus.Handler {
		return func(ctx context.Context, msg any) (any, error) {
			start := time.Now()
			res, err := next(ctx, msg)
			dur := time.Since(start).Seconds()
			cmd := fmt.Sprintf("%T", msg)
			m.CommandDuration.WithLabelValues(cmd).Observe(dur)
			status := "ok"
			if err != nil {
				status = "err"
			}
			m.CommandsTotal.WithLabelValues(cmd, status).Inc()

			if err == nil {
				if sc, ok := msg.(StepCompletion); ok {
					st := sc.GetStepType()
					m.StepsTotal.WithLabelValues(st, "completed").Inc()
					if c := sc.GetCostUSD(); c > 0 {
						p := sc.GetProvider()
						if p == "" {
							p = "unknown"
						}
						m.StepCostUSDTotal.WithLabelValues(p, st).Add(c)
					}
				} else if sf, ok := msg.(StepFailure); ok {
					m.StepsTotal.WithLabelValues(sf.GetStepType(), "failed").Inc()
				}
				if rt, ok := msg.(RunTerminal); ok {
					if s := rt.GetTerminalStatus(); s != "" {
						m.RunsTotal.WithLabelValues(s).Inc()
					}
				}
			}
			return res, err
		}
	}
}
