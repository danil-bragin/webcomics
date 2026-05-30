// Package metrics exposes Prometheus instrumentation. Mounted on the same HTTP
// server as /api/* but at /metrics so the existing chi router serves both.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/do/v2"
)

type Metrics struct {
	registry *prometheus.Registry

	CommandsTotal    *prometheus.CounterVec
	CommandDuration  *prometheus.HistogramVec
	RunsTotal        *prometheus.CounterVec
	StepsTotal       *prometheus.CounterVec
	StepCostUSDTotal *prometheus.CounterVec
}

func New(do.Injector) (*Metrics, error) {
	r := prometheus.NewRegistry()

	commands := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_commands_total",
		Help: "Number of bus commands dispatched, by command type and outcome.",
	}, []string{"command", "status"})

	commandDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pipeline_command_duration_seconds",
		Help:    "Latency of bus command dispatches.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"command"})

	runs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_runs_total",
		Help: "Number of pipeline runs by terminal status.",
	}, []string{"status"})

	steps := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_steps_total",
		Help: "Number of pipeline step completions by step type and status.",
	}, []string{"step_type", "status"})

	cost := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_step_cost_usd_total",
		Help: "Cumulative USD spent on pipeline step provider calls.",
	}, []string{"provider", "step_type"})

	r.MustRegister(commands, commandDuration, runs, steps, cost)

	return &Metrics{
		registry:         r,
		CommandsTotal:    commands,
		CommandDuration:  commandDuration,
		RunsTotal:        runs,
		StepsTotal:       steps,
		StepCostUSDTotal: cost,
	}, nil
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) HealthCheck() error { return nil }
func (m *Metrics) Shutdown() error    { return nil }
