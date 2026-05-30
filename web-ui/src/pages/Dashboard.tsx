import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type ProviderBalance } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { fmtMoney } from "@/lib/format";

export function Dashboard() {
  const { t, i18n } = useTranslation();
  const q = useQuery({ queryKey: ["stats"], queryFn: api.getStats, refetchInterval: 5000 });
  const b = useQuery({ queryKey: ["balances"], queryFn: api.getBalances, refetchInterval: 15000 });
  const s = q.data;
  if (!s) return <p className="p-6 text-sm text-muted-foreground">{t("common.loading")}</p>;

  const runsTotal = Object.values(s.runs_by_status).reduce((a, b) => a + b, 0);
  const maxDay = Math.max(1, ...s.cost_by_day.map((d) => d.total_cost_usd));

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Stat label={t("dashboard.totalCost")} value={fmtMoney(s.total_cost_usd)} />
        <Stat label={t("dashboard.runs")} value={String(runsTotal)} />
        <Stat label={t("dashboard.completed")} value={String(s.runs_by_status["completed"] ?? 0)} />
        <Stat
          label={t("dashboard.failedCancelled")}
          value={String((s.runs_by_status["failed"] ?? 0) + (s.runs_by_status["cancelled"] ?? 0))}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.providerBalances")}</CardTitle>
        </CardHeader>
        <CardContent>
          {b.isLoading ? (
            <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {(b.data?.providers ?? []).map((p) => <BalanceTile key={p.name} p={p} />)}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.costByProvider")}</CardTitle>
        </CardHeader>
        <CardContent>
          {s.cost_by_provider.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("dashboard.noData")}</p>
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

      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.costLast14")}</CardTitle>
        </CardHeader>
        <CardContent>
          {s.cost_by_day.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("dashboard.noData")}</p>
          ) : (
            <div className="flex items-end gap-1 h-32">
              {s.cost_by_day.map((d) => {
                const h = (d.total_cost_usd / maxDay) * 100;
                return (
                  <div key={d.date} className="flex-1 flex flex-col items-center" title={`${d.date}: ${fmtMoney(d.total_cost_usd)}`}>
                    <div className="w-full bg-blue-500/70 rounded-t" style={{ height: `${Math.max(2, h)}%` }} />
                    <span className="text-[10px] mt-1 text-muted-foreground">{d.date.slice(5)}</span>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );

  function Stat({ label, value }: { label: string; value: string }) {
    return (
      <Card>
        <CardContent className="py-4">
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold tabular-nums">{value}</p>
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
}
