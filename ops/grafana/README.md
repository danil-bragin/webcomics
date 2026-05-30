# Grafana dashboard

`webcomics-dashboard.json` — pre-built dashboard for Webcomics' Prometheus
metrics (`/metrics` endpoint on `web-api`).

## Import

1. Add Prometheus as a datasource in Grafana, pointing at your Prometheus
   instance (scrape `web-api:8080/metrics`).
2. Dashboards → New → Import → paste the JSON.
3. When prompted, pick the Prometheus datasource for `DS_PROM`.

## Panels

| Panel | Metric |
|---|---|
| Total runs (completed) | `sum(pipeline_runs_total{status="completed"})` |
| Total cost (USD) | `sum(pipeline_step_cost_usd_total)` |
| Failed runs | `sum(pipeline_runs_total{status=~"failed\|cancelled"})` |
| Commands/sec | `sum(rate(pipeline_commands_total[5m]))` |
| Command p95 latency | `histogram_quantile(0.95, …pipeline_command_duration_seconds_bucket…)` |
| Commands by status | `sum by (status) (rate(pipeline_commands_total[5m]))` |
| Steps by type/status | `sum by (step_type, status) (rate(pipeline_steps_total[5m]))` |
| Cost by provider | `sum by (provider) (pipeline_step_cost_usd_total)` |

## Scrape config snippet

```yaml
scrape_configs:
  - job_name: webcomics
    metrics_path: /metrics
    static_configs:
      - targets: ['web-api:8080']
```
