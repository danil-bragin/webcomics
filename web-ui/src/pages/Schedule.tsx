import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { api, type ScheduledUploadView, type SocialAccountView } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";

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

  // Group by account.
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
        return (
          <Card key={accountId}>
            <CardHeader className="py-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm">
                  {acct?.label || accountId.slice(0, 8)} · <span className="opacity-60">{acct?.platform}</span>
                </CardTitle>
                <span className="text-xs text-muted-foreground tabular-nums">
                  {items.length} {t("schedule.queued", "in queue")}
                </span>
              </div>
            </CardHeader>
            <CardContent className="pt-0">
              <ul className="divide-y divide-border">
                {items.map((r) => (
                  <li key={r.id} className="py-2 flex items-center gap-2 text-sm">
                    <span className="text-xs tabular-nums w-36 shrink-0">
                      {new Date(r.scheduled_at).toLocaleString(i18n.resolvedLanguage)}
                    </span>
                    <Link to={`/runs/${r.run_id}`} className="flex-1 truncate hover:underline" title={r.run_prompt}>
                      {r.run_prompt || r.run_id.slice(0, 8)}
                    </Link>
                    <StatusBadge status={r.status} />
                    {r.external_ref ? (
                      <a href={r.external_ref} target="_blank" rel="noreferrer" className="text-[11px] text-primary underline">
                        link
                      </a>
                    ) : null}
                    {r.status === "pending" ? (
                      <Button variant="outline" className="h-7 px-2 text-[11px] text-red-400"
                        onClick={() => { if (confirm(t("schedule.confirmCancel", "Cancel this scheduled upload?"))) cancel.mutate(r.id); }}>
                        {t("schedule.cancel", "cancel")}
                      </Button>
                    ) : null}
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        );
      })}
    </div>
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
