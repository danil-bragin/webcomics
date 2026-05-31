import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type ProviderBalance } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { fmtMoney } from "@/lib/format";

// Status → swatch color for the runs-by-status breakdown bar.
const STATUS_COLOR: Record<string, string> = {
  completed: "bg-emerald-500",
  running: "bg-blue-500",
  queued: "bg-sky-400",
  failed: "bg-red-500",
  cancelled: "bg-zinc-500",
  awaiting_action: "bg-amber-500",
};

export function Dashboard() {
  const { t, i18n } = useTranslation();
  const q = useQuery({ queryKey: ["stats"], queryFn: api.getStats, refetchInterval: 5000 });
  const b = useQuery({ queryKey: ["balances"], queryFn: api.getBalances, refetchInterval: 15000 });
  const s = q.data;
  if (!s) return <p className="p-6 text-sm text-muted-foreground">{t("common.loading")}</p>;

  const runsTotal = Object.values(s.runs_by_status).reduce((a, b) => a + b, 0);
  const completed = s.runs_by_status["completed"] ?? 0;
  const failed = (s.runs_by_status["failed"] ?? 0) + (s.runs_by_status["cancelled"] ?? 0);
  const inFlight = (s.runs_by_status["running"] ?? 0) + (s.runs_by_status["queued"] ?? 0);
  const avgCost = completed > 0 ? s.total_cost_usd / completed : 0;

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      {/* Top KPIs */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Stat label={t("dashboard.totalCost")} value={fmtMoney(s.total_cost_usd)}
          hint={completed > 0 ? `${fmtMoney(avgCost)} ${t("dashboard.avgPerRun")}` : undefined} />
        <Stat label={t("dashboard.runs")} value={String(runsTotal)}
          hint={inFlight > 0 ? `${inFlight} ${t("dashboard.inFlight")}` : undefined} />
        <Stat label={t("dashboard.completed")} value={String(completed)}
          hint={runsTotal > 0 ? `${Math.round((completed / runsTotal) * 100)}%` : undefined} />
        <Stat label={t("dashboard.failedCancelled")} value={String(failed)}
          hint={runsTotal > 0 && failed > 0 ? `${Math.round((failed / runsTotal) * 100)}%` : undefined} />
      </div>

      {/* Runs by status — single stacked bar w/ legend */}
      {runsTotal > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("dashboard.runsByStatus")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="h-3 rounded overflow-hidden flex">
              {Object.entries(s.runs_by_status).map(([status, n]) => {
                const pct = (n / runsTotal) * 100;
                if (pct === 0) return null;
                return (
                  <div
                    key={status}
                    className={STATUS_COLOR[status] ?? "bg-secondary"}
                    style={{ width: `${pct}%` }}
                    title={`${status}: ${n}`}
                  />
                );
              })}
            </div>
            <div className="flex flex-wrap gap-3 text-xs">
              {Object.entries(s.runs_by_status).filter(([, n]) => n > 0).map(([status, n]) => (
                <span key={status} className="flex items-center gap-1.5">
                  <span className={`inline-block w-3 h-3 rounded-sm ${STATUS_COLOR[status] ?? "bg-secondary"}`} />
                  <span className="text-muted-foreground">{t(`runs.status.${status}`, status)}</span>
                  <span className="tabular-nums font-medium">{n}</span>
                </span>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : null}

      {/* Provider balances */}
      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.providerBalances")}</CardTitle>
        </CardHeader>
        <CardContent>
          {b.isLoading ? (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {[0, 1, 2].map((i) => <div key={i} className="h-24 rounded bg-secondary/30 animate-pulse" />)}
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {(b.data?.providers ?? []).map((p) => <BalanceTile key={p.name} p={p} />)}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Cost by provider — horizontal bars */}
      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.costByProvider")}</CardTitle>
        </CardHeader>
        <CardContent>
          {s.cost_by_provider.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">{t("dashboard.noDataYet")}</p>
          ) : (
            <div className="space-y-2">
              {s.cost_by_provider.map((p) => {
                const max = Math.max(1, ...s.cost_by_provider.map((x) => x.total_cost_usd));
                const pct = (p.total_cost_usd / max) * 100;
                return (
                  <div key={p.provider}>
                    <div className="flex justify-between text-xs mb-0.5">
                      <span>{p.provider}</span>
                      <span className="tabular-nums">{fmtMoney(p.total_cost_usd)}</span>
                    </div>
                    <div className="h-2 rounded bg-secondary/40 overflow-hidden">
                      <div className="h-full bg-primary" style={{ width: `${pct}%` }} />
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Cost by day chart */}
      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.costLast14")}</CardTitle>
        </CardHeader>
        <CardContent>
          <CostByDayChart days={s.cost_by_day} />
        </CardContent>
      </Card>
    </div>
  );

  function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
    return (
      <Card>
        <CardContent className="py-4">
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold tabular-nums">{value}</p>
          {hint ? <p className="text-[10px] text-muted-foreground mt-0.5">{hint}</p> : null}
        </CardContent>
      </Card>
    );
  }

  function BalanceTile({ p }: { p: ProviderBalance }) {
    if (p.error) {
      return (
        <Card>
          <CardContent className="py-4">
            <p className="text-xs text-muted-foreground uppercase">{p.name}</p>
            <p className="text-sm text-red-400 mt-1">{p.error}</p>
          </CardContent>
        </Card>
      );
    }
    const isUSD = p.currency === "usd";
    const fmt = (n: number) =>
      isUSD ? `$${n.toFixed(n < 1 ? 4 : 2)}` : `${Math.round(n).toLocaleString()} ${p.unit_label}`;
    const pct = p.limit > 0 ? (p.used / p.limit) * 100 : 0;
    const reset = p.reset_unix ? new Date(p.reset_unix * 1000) : null;

    return (
      <Card>
        <CardContent className="py-4 space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs text-muted-foreground uppercase">{p.name}</p>
            {p.limit > 0 ? (
              <span className="text-[10px] text-muted-foreground">{t("dashboard.percentUsed", { pct: pct.toFixed(0) })}</span>
            ) : null}
          </div>
          <p className="text-2xl font-semibold tabular-nums">{fmt(p.remaining)}</p>
          <p className="text-[11px] text-muted-foreground">
            {p.limit > 0 ? t("dashboard.usedOf", { used: fmt(p.used), limit: fmt(p.limit) }) : t("dashboard.remaining")}
          </p>
          {p.limit > 0 ? (
            <div className="h-1.5 rounded bg-secondary/40 overflow-hidden">
              <div
                className={pct > 90 ? "h-full bg-red-500" : pct > 70 ? "h-full bg-amber-500" : "h-full bg-emerald-500"}
                style={{ width: `${Math.min(100, pct)}%` }}
              />
            </div>
          ) : null}
          {reset ? (
            <p className="text-[10px] text-muted-foreground">{t("dashboard.resets", { date: reset.toLocaleDateString(i18n.resolvedLanguage) })}</p>
          ) : null}
        </CardContent>
      </Card>
    );
  }

  function CostByDayChart({ days }: { days: { date: string; total_cost_usd: number }[] }) {
    // Pad to last 14 calendar days even when the API returns sparse data so
    // the bar layout stays stable instead of jumping around.
    const today = new Date();
    const series: { date: string; v: number }[] = [];
    const byDate = new Map(days.map((d) => [d.date, d.total_cost_usd]));
    for (let i = 13; i >= 0; i--) {
      const d = new Date(today);
      d.setDate(today.getDate() - i);
      const iso = d.toISOString().slice(0, 10);
      series.push({ date: iso, v: byDate.get(iso) ?? 0 });
    }
    const total = series.reduce((acc, x) => acc + x.v, 0);
    const max = Math.max(0.001, ...series.map((x) => x.v));

    if (total === 0) {
      return (
        <p className="text-sm text-muted-foreground text-center py-4">{t("dashboard.noCostYet")}</p>
      );
    }

    return (
      <div>
        <div className="flex items-end gap-1 h-32 border-b border-border/40">
          {series.map((d) => {
            const h = (d.v / max) * 100;
            return (
              <div key={d.date} className="flex-1 h-full flex flex-col items-center justify-end" title={`${d.date}: ${fmtMoney(d.v)}`}>
                {d.v > 0 ? (
                  <div className="w-full bg-blue-500/70 rounded-t" style={{ height: `${Math.max(2, h)}%` }} />
                ) : (
                  <div className="w-full bg-secondary/20 rounded-t" style={{ height: "1px" }} />
                )}
              </div>
            );
          })}
        </div>
        <div className="flex justify-between mt-1 text-[10px] text-muted-foreground">
          <span>{series[0].date.slice(5)}</span>
          <span>{t("dashboard.totalSpend", { value: fmtMoney(total) })}</span>
          <span>{series[series.length - 1].date.slice(5)}</span>
        </div>
      </div>
    );
  }
}
