import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { api, type ScheduledUploadView, type SocialAccountView } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";
import { fmtMoney } from "@/lib/format";

const STATUSES = ["pending", "in_flight", "completed", "failed", "cancelled"] as const;

export function Schedule() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const toast = useToast();
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [accountFilter, setAccountFilter] = useState<string>("");

  const accounts = useQuery<SocialAccountView[]>({
    queryKey: ["social-accounts-all"],
    queryFn: () => api.listSocialAccountsGlobal(),
  });
  const rows = useQuery<ScheduledUploadView[]>({
    queryKey: ["scheduled", statusFilter, accountFilter],
    queryFn: () => api.listScheduled({
      status: statusFilter || undefined,
      account_id: accountFilter || undefined,
      limit: 500,
    }),
    refetchInterval: 10_000,
  });

  const cancel = useMutation({
    mutationFn: (id: string) => api.cancelScheduled(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["scheduled"] }); toast.push("success", t("schedule.cancelled", "Cancelled")); },
    onError: (e: Error) => toast.push("error", e.message),
  });

  const groups = useMemo(() => {
    const byAccount = new Map<string, ScheduledUploadView[]>();
    for (const r of (rows.data ?? [])) {
      const arr = byAccount.get(r.social_account_id) ?? [];
      arr.push(r);
      byAccount.set(r.social_account_id, arr);
    }
    return [...byAccount.entries()];
  }, [rows.data]);

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">{t("schedule.pageTitle", "Планировщик")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("schedule.pageSubtitle", "All scheduled uploads across linked social accounts.")}
          </p>
        </div>
        <div className="flex gap-2">
          <select value={accountFilter} onChange={(e) => setAccountFilter(e.target.value)}
            className="h-9 rounded border border-border bg-secondary/30 px-2 text-sm">
            <option value="">{t("schedule.allAccounts", "All accounts")}</option>
            {(accounts.data ?? []).map((a) => (
              <option key={a.id} value={a.id}>{a.label || "—"} · {a.platform}</option>
            ))}
          </select>
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}
            className="h-9 rounded border border-border bg-secondary/30 px-2 text-sm">
            <option value="">{t("schedule.allStatuses", "All statuses")}</option>
            {STATUSES.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
      </div>

      {rows.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : (rows.data ?? []).length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {t("schedule.empty", "No scheduled uploads. Schedule one from a run page.")}
          </CardContent>
        </Card>
      ) : groups.map(([accountId, items]) => {
        const acct = (accounts.data ?? []).find((a) => a.id === accountId);
        const limit = acct?.daily_upload_limit ?? 15;
        const windowH = acct?.limit_window_hours ?? 24;
        const liveCount = items.filter((r) => r.status === "pending" || r.status === "in_flight" || r.status === "completed").length;
        const utilPct = limit > 0 ? Math.min(100, Math.round((liveCount / limit) * 100)) : 0;
        return (
          <Card key={accountId}>
            <CardHeader className="py-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm">
                  {acct?.label || accountId.slice(0, 8)} · <span className="opacity-60">{acct?.platform}</span>
                </CardTitle>
                <div className="flex items-center gap-2 text-[11px] text-muted-foreground tabular-nums">
                  <span>{liveCount}/{limit} · {windowH}h</span>
                  <div className="w-24 h-1.5 rounded bg-secondary/40 overflow-hidden">
                    <div className={utilPct >= 100 ? "h-full bg-red-500" : utilPct > 80 ? "h-full bg-amber-500" : "h-full bg-emerald-500"}
                      style={{ width: `${utilPct}%` }} />
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent className="pt-0">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {items.map((r) => (
                  <ScheduledCard key={r.id} row={r} lang={i18n.resolvedLanguage}
                    onCancel={() => { if (confirm(t("schedule.confirmCancel", "Cancel this scheduled upload?"))) cancel.mutate(r.id); }} />
                ))}
              </div>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

function ScheduledCard({ row, onCancel, lang }: { row: ScheduledUploadView; onCancel: () => void; lang?: string }) {
  const { t } = useTranslation();
  return (
    <div className="relative rounded-lg border border-border bg-card overflow-hidden flex flex-col group">
      <Link to={`/runs/${row.run_id}`} className="block relative">
        <VideoThumb assetId={row.run_video_asset_id} />
      </Link>
      <div className="p-3 space-y-1.5">
        <div className="flex items-center justify-between gap-2">
          <StatusBadge status={row.status} />
          {row.run_cost_usd != null ? (
            <span className="text-xs tabular-nums text-muted-foreground">{fmtMoney(row.run_cost_usd)}</span>
          ) : null}
        </div>
        <p className="text-sm truncate" title={row.run_prompt}>{row.run_prompt || row.run_id.slice(0, 8)}</p>
        <p className="text-[10px] text-muted-foreground tabular-nums">
          {new Date(row.scheduled_at).toLocaleString(lang)}
        </p>
        {row.external_ref ? (
          <a href={row.external_ref} target="_blank" rel="noreferrer"
            className="block text-[11px] text-primary truncate hover:underline">
            {row.external_ref}
          </a>
        ) : null}
        {row.error ? <p className="text-[10px] text-red-400 line-clamp-2">{row.error}</p> : null}
      </div>
      {row.status === "pending" ? (
        <button
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); onCancel(); }}
          className="absolute top-2 right-2 w-6 h-6 rounded bg-background/80 border border-border opacity-0 group-hover:opacity-100 transition-opacity inline-flex items-center justify-center text-xs text-red-400 hover:bg-red-500/10"
          title={t("schedule.cancel", "cancel")}
          aria-label="cancel"
        >×</button>
      ) : null}
    </div>
  );
}

function VideoThumb({ assetId }: { assetId?: string }) {
  const { t } = useTranslation();
  const [url, setURL] = useState<string | null>(null);
  const ref = useRef<HTMLVideoElement | null>(null);
  useEffect(() => {
    if (!assetId) return;
    let alive = true;
    api.getAssetUrl(assetId).then((r) => alive && setURL(r.url)).catch(() => {});
    return () => { alive = false; };
  }, [assetId]);
  const onLoadedMetadata = () => {
    const v = ref.current;
    if (!v) return;
    const target = Math.min(1, Math.max(0.1, (v.duration || 2) * 0.25));
    try { v.currentTime = target; } catch { /* ignore */ }
  };
  if (!assetId) {
    return (
      <div className="aspect-square bg-secondary/20 flex items-center justify-center text-xs text-muted-foreground">
        {t("runs.noVideo", "no video")}
      </div>
    );
  }
  if (!url) return <div className="aspect-square bg-secondary/40 animate-pulse" />;
  return (
    <video ref={ref} src={url} className="aspect-square w-full object-cover bg-card"
      muted playsInline preload="metadata" onLoadedMetadata={onLoadedMetadata} />
  );
}

function StatusBadge({ status }: { status: ScheduledUploadView["status"] }) {
  const variant: Parameters<typeof Badge>[0]["variant"] =
    status === "completed" ? "default"
      : status === "in_flight" ? "info"
      : status === "failed" ? "danger"
      : status === "cancelled" ? "info"
      : "info";
  return <Badge variant={variant}>{status}</Badge>;
}
