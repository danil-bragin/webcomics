package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/samber/do/v2"
)

// Unified field names. Same vocabulary used by Python (structlog) and Node
// (renderer-node/src/log.ts) so grepping for `run_id=X` across all runtimes
// yields a coherent timeline.
const (
	KeyService    = "service"
	KeyWorker     = "worker"
	KeyRunID      = "run_id"
	KeyStepIndex  = "step_index"
	KeyStepType   = "step_type"
	KeyPanelIndex = "panel_index"
)

// contextKey type prevents collisions; only this package can set/get.
type contextKey int

const ctxKeyRunID contextKey = iota + 1

// WithRunID returns a context carrying the given run id, so downstream code
// can call ctx.Value(...) — or use ContextLogger to get a pre-bound *slog.Logger.
func WithRunID(ctx context.Context, runID string) context.Context {
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRunID, runID)
}

// RunIDFrom returns the run id bound to the context (empty if none).
func RunIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRunID).(string)
	return v
}

// ContextLogger binds standard pipeline fields from ctx onto the given logger.
func ContextLogger(ctx context.Context, log *slog.Logger) *slog.Logger {
	if id := RunIDFrom(ctx); id != "" {
		return log.With(KeyRunID, id)
	}
	return log
}

func New(do.Injector) (*slog.Logger, error) {
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	l = l.With(KeyService, "web-api")
	slog.SetDefault(l)
	return l, nil
}
