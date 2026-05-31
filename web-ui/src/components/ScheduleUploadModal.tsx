import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type AccountWindowStats, type SocialAccountView } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/toast";

// Returns yyyy-mm-ddThh:mm in browser-local time, suitable for <input type=datetime-local>.
function defaultSlot(): string {
  const d = new Date(Date.now() + 60 * 60 * 1000); // 1h from now
  d.setSeconds(0, 0);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function localToISO(local: string): string {
  // Browser interprets datetime-local as the user's local time.
  return new Date(local).toISOString();
}

function isoToLocal(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function ScheduleUploadModal({
  runId, accounts, defaultAccountId, onClose, onScheduled, initialAt, runVideoKey, runCaptions, runPrompt,
}: {
  runId: string;
  accounts: SocialAccountView[];
  defaultAccountId?: string;
  onClose: () => void;
  onScheduled: (id: string) => void;
  initialAt?: string;
  runVideoKey?: string;
  runCaptions?: string[];
  runPrompt?: string;
}) {
  const { t } = useTranslation();
  const toast = useToast();
  const [accountId, setAccountId] = useState(defaultAccountId || accounts[0]?.id || "");
  const [when, setWhen] = useState<string>(initialAt ? isoToLocal(initialAt) : defaultSlot());
  const [visibility, setVisibility] = useState<"public" | "unlisted" | "private">("public");
  const [blocked, setBlocked] = useState<string | null>(null);
  const [nextFree, setNextFree] = useState<string | null>(null);

  const atISO = useMemo(() => {
    try { return localToISO(when); } catch { return ""; }
  }, [when]);

  // Live availability check (debounced 400ms).
  const [debouncedAt, setDebouncedAt] = useState(atISO);
  useEffect(() => {
    const h = setTimeout(() => setDebouncedAt(atISO), 400);
    return () => clearTimeout(h);
  }, [atISO]);

  const avail = useQuery<AccountWindowStats>({
    queryKey: ["schedule-avail", accountId, debouncedAt],
    queryFn: () => api.scheduleAvailability(accountId, debouncedAt),
    enabled: !!accountId && !!debouncedAt,
    staleTime: 5_000,
  });

  const submit = useMutation({
    mutationFn: () => {
      const metadata: Record<string, unknown> = {
        video_key: runVideoKey,
        params: {
          platform: accounts.find((a) => a.id === accountId)?.platform ?? "youtube_selenium",
          visibility,
          category_id: "22",
          category_label: "People & Blogs",
          made_for_kids: false,
          tags: ["meme", "comic", "shorts", "manga"],
          title: runPrompt ?? "",
          description: (runCaptions ?? []).join("\n"),
        },
        captions: {
          youtube: {
            title: runPrompt ?? "",
            description: (runCaptions ?? []).join("\n"),
            tags: ["meme", "comic", "shorts", "manga"],
            hashtags: ["#shorts", "#meme", "#manga"],
          },
        },
      };
      return api.createScheduled({
        run_id: runId,
        social_account_id: accountId,
        scheduled_at: atISO,
        metadata,
      });
    },
    onSuccess: (res) => {
      toast.push("success", t("schedule.created", "Scheduled"));
      onScheduled(res.id);
    },
    onError: async (e: unknown) => {
      // 409 with next_free_slot body for limit conflicts.
      const msg = e instanceof Error ? e.message : String(e);
      try {
        const m = msg.match(/\{.*\}/s);
        if (m) {
          const body = JSON.parse(m[0]);
          if (body.next_free_slot) {
            setBlocked(body.error || t("schedule.blocked", "Limit exceeded"));
            setNextFree(body.next_free_slot);
            return;
          }
        }
      } catch { /* ignore */ }
      toast.push("error", msg);
    },
  });

  return (
    <div className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4" onClick={onClose}>
      <div className="bg-card border border-border rounded-lg w-full max-w-md p-5 space-y-3" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">{t("schedule.title", "Schedule upload")}</h2>
          <button onClick={onClose} className="text-sm opacity-60 hover:opacity-100">×</button>
        </div>
        <label className="block space-y-1">
          <span className="text-xs uppercase tracking-wide text-muted-foreground">{t("schedule.account", "Account")}</span>
          <select value={accountId} onChange={(e) => setAccountId(e.target.value)}
            className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>{a.label || "—"} · {a.platform}</option>
            ))}
          </select>
        </label>
        <label className="block space-y-1">
          <span className="text-xs uppercase tracking-wide text-muted-foreground">{t("schedule.when", "When")}</span>
          <input type="datetime-local" value={when} onChange={(e) => { setWhen(e.target.value); setBlocked(null); }}
            className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </label>
        <label className="block space-y-1">
          <span className="text-xs uppercase tracking-wide text-muted-foreground">{t("schedule.visibility", "Visibility")}</span>
          <select value={visibility} onChange={(e) => setVisibility(e.target.value as typeof visibility)}
            className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
            <option value="public">public</option>
            <option value="unlisted">unlisted</option>
            <option value="private">private</option>
          </select>
        </label>

        {avail.data ? (
          <div className="flex items-center justify-between text-[11px]">
            <span className="text-muted-foreground">
              {t("schedule.windowUsage", "{{count}}/{{limit}} in {{hrs}}h window", {
                count: avail.data.count_in_window,
                limit: avail.data.limit_n,
                hrs: avail.data.window_hours,
              })}
            </span>
            {avail.data.is_at_limit ? (
              <Badge variant="info">{t("schedule.atLimit", "at limit")}</Badge>
            ) : null}
          </div>
        ) : null}

        {blocked ? (
          <div className="rounded border border-amber-500/40 bg-amber-500/10 text-amber-200 text-xs p-2 space-y-1">
            <p>{blocked}</p>
            {nextFree ? (
              <button onClick={() => { setWhen(isoToLocal(nextFree)); setBlocked(null); }}
                className="text-amber-100 underline">
                {t("schedule.useSuggested", "Use suggested")}: {new Date(nextFree).toLocaleString()}
              </button>
            ) : null}
          </div>
        ) : null}

        <div className="flex justify-end gap-2 pt-2">
          <Button variant="outline" onClick={onClose}>{t("common.cancel", "Cancel")}</Button>
          <Button disabled={submit.isPending || !atISO} onClick={() => submit.mutate()}>
            {submit.isPending ? "…" : t("schedule.submit", "Schedule")}
          </Button>
        </div>
      </div>
    </div>
  );
}
