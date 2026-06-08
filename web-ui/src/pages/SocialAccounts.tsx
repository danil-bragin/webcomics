import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type SocialAccountView, type FxSession, type InspectSession } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardSkeletonGrid } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast";

// Selenium-based platforms. Each connect flow opens a Firefox container the
// operator logs into manually; the cookie profile is saved to disk and used
// by the upload worker. Default daily upload caps reflect each platform's
// soft-ban thresholds for new/unverified accounts.
const PLATFORMS: { id: string; label: string; live: boolean; defaultLimit: number }[] = [
  { id: "youtube_selenium",   label: "YouTube",   live: true, defaultLimit: 15 },
  { id: "instagram_selenium", label: "Instagram", live: true, defaultLimit: 25 },
  { id: "tiktok_selenium",    label: "TikTok",    live: true, defaultLimit: 10 },
  { id: "facebook_selenium",  label: "Facebook",  live: true, defaultLimit: 25 },
];

function fmtAgo(ts?: string | null): string {
  if (!ts) return "—";
  const d = new Date(ts);
  const diffMs = Date.now() - d.getTime();
  const min = Math.floor(diffMs / 60000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

function statusBadge(s?: string): { text: string; variant: "default" | "warn" | "danger" | "info" } {
  switch (s) {
    case "needs_relogin": return { text: "needs re-login", variant: "warn" };
    case "banned":        return { text: "banned",         variant: "danger" };
    case "disabled":      return { text: "disabled",       variant: "info" };
    default:              return { text: "active",         variant: "default" };
  }
}

export function SocialAccounts() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const toast = useToast();
  const [platform, setPlatform] = useState<string>("youtube_selenium");
  const [addOpen, setAddOpen] = useState(false);

  const accounts = useQuery<SocialAccountView[]>({
    queryKey: ["social-accounts", platform],
    queryFn: () => api.listSocialAccountsGlobal(platform),
    refetchInterval: 8000,
  });

  const del = useMutation({
    mutationFn: (id: string) => api.deleteSocialAccountGlobal(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["social-accounts"] }); toast.push("success", t("social.deleted", "Account deleted")); },
    onError: (e: Error) => toast.push("error", e.message),
  });

  const active = PLATFORMS.find((p) => p.id === platform);

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">{t("social.title", "Social Accounts")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("social.subtitle", "Global library — link to projects on demand. Same channel works across many projects.")}
          </p>
        </div>
        {active?.live ? (
          <Button onClick={() => setAddOpen(true)}>
            + {t("social.connect", "Connect")} {active.label}
          </Button>
        ) : null}
      </div>

      <div className="flex gap-2 border-b border-border">
        {PLATFORMS.map((p) => (
          <button
            key={p.id}
            onClick={() => p.live && setPlatform(p.id)}
            disabled={!p.live}
            className={`px-3 py-2 text-sm border-b-2 -mb-px transition ${
              platform === p.id
                ? "border-primary text-primary"
                : p.live
                  ? "border-transparent text-muted-foreground hover:text-foreground"
                  : "border-transparent text-muted-foreground/40 cursor-not-allowed"
            }`}
          >
            {p.label}
            {!p.live ? <span className="ml-1 text-[9px] uppercase opacity-60">{t("social.soon", "soon")}</span> : null}
          </button>
        ))}
      </div>

      {!active?.live ? (
        <p className="text-sm text-muted-foreground text-center py-10">
          {t("social.platformSoon", "This platform isn't wired up yet. YouTube is the only live integration today.")}
        </p>
      ) : accounts.isLoading ? (
        <CardSkeletonGrid count={4} cols={2} />
      ) : (accounts.data ?? []).length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center space-y-2">
            <p className="text-sm text-muted-foreground">
              {t("social.empty", "No accounts yet. Connect one to get started.")}
            </p>
            <Button onClick={() => setAddOpen(true)}>+ {t("social.connect", "Connect")} {active.label}</Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {(accounts.data ?? []).map((a) => (
            <AccountCard key={a.id} account={a} onDelete={() => {
              if (confirm(t("social.confirmDelete", "Delete account '{{name}}'? Unlinks from every project; upload history is preserved.", { name: a.label || "untitled" }))) {
                del.mutate(a.id);
              }
            }} />
          ))}
        </div>
      )}

      {addOpen ? (
        <AddAccountModal
          platform={platform}
          onClose={() => setAddOpen(false)}
          onCreated={() => {
            setAddOpen(false);
            qc.invalidateQueries({ queryKey: ["social-accounts"] });
            toast.push("success", t("social.connected", "Account connected"));
          }}
        />
      ) : null}
    </div>
  );
}

function AccountCard({ account, onDelete }: { account: SocialAccountView; onDelete: () => void }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const toast = useToast();
  const s = statusBadge(account.status);
  const cooldown = account.cooldown_until && new Date(account.cooldown_until).getTime() > Date.now();

  const [limit, setLimit] = useState<number>(account.daily_upload_limit ?? 15);
  const [windowH, setWindowH] = useState<number>(account.limit_window_hours ?? 24);
  const [verified, setVerified] = useState<boolean>(account.is_verified ?? false);
  const [minGap, setMinGap] = useState<number>(account.min_gap_seconds ?? 60);
  const saveLimits = useMutation({
    mutationFn: () => api.patchSocialAccountLimits(account.id, {
      daily_upload_limit: limit,
      limit_window_hours: windowH,
      is_verified: verified,
      min_gap_seconds: minGap,
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["social-accounts"] }); toast.push("success", t("social.limitsSaved", "Limits saved")); },
    onError: (e: Error) => toast.push("error", e.message),
  });
  // Verified toggle suggests bumping limit (one-click ack).
  function onVerifiedToggle(next: boolean) {
    setVerified(next);
    if (next && limit < 100) setLimit(100);
    if (!next && limit > 15) setLimit(15);
  }

  return (
    <Card>
      <CardContent className="py-4 space-y-2">
        <div className="flex items-start justify-between gap-2">
          <div className="flex-1 min-w-0">
            <p className="font-medium truncate">{account.label || t("social.untitled", "untitled")}</p>
            <p className="text-[11px] text-muted-foreground truncate">{account.firefox_profile_path}</p>
          </div>
          <Badge variant={s.variant === "warn" ? "info" : s.variant === "danger" ? "danger" : "default"}>{s.text}</Badge>
        </div>
        <div className="grid grid-cols-3 gap-2 text-[11px] text-muted-foreground">
          <div>
            <p className="uppercase tracking-wide opacity-70">{t("social.lastUsed", "last used")}</p>
            <p className="text-foreground tabular-nums">{fmtAgo(account.last_used_at)}</p>
          </div>
          <div>
            <p className="uppercase tracking-wide opacity-70">{t("social.linkedTo", "projects")}</p>
            <p className="text-foreground tabular-nums">{account.project_count ?? 0}</p>
          </div>
          <div>
            <p className="uppercase tracking-wide opacity-70">{t("social.uploads", "uploads")}</p>
            <p className="text-foreground tabular-nums">{account.upload_count ?? 0}</p>
          </div>
        </div>
        {cooldown ? (
          <p className="text-[10px] text-amber-400">
            {t("social.cooldownUntil", "cooldown until {{ts}}", { ts: new Date(account.cooldown_until!).toLocaleString() })}
          </p>
        ) : null}

        {/* Rate-limit config */}
        <details className="text-[11px] border-t border-border/40 pt-2">
          <summary className="cursor-pointer text-muted-foreground">{t("social.limits", "Rate limits")}</summary>
          <div className="grid grid-cols-2 gap-2 pt-2">
            <label className="space-y-0.5">
              <span className="uppercase tracking-wide text-muted-foreground">{t("social.dailyLimit", "Daily limit")}</span>
              <input type="number" min={0} value={limit} onChange={(e) => setLimit(Number(e.target.value))}
                className="h-7 w-full rounded border border-border bg-secondary/30 px-2 tabular-nums" />
            </label>
            <label className="space-y-0.5">
              <span className="uppercase tracking-wide text-muted-foreground">{t("social.windowH", "Window (h)")}</span>
              <input type="number" min={1} max={168} value={windowH} onChange={(e) => setWindowH(Number(e.target.value))}
                className="h-7 w-full rounded border border-border bg-secondary/30 px-2 tabular-nums" />
            </label>
            <label className="space-y-0.5">
              <span className="uppercase tracking-wide text-muted-foreground">{t("social.minGap", "Min gap (s)")}</span>
              <input type="number" min={0} value={minGap} onChange={(e) => setMinGap(Number(e.target.value))}
                className="h-7 w-full rounded border border-border bg-secondary/30 px-2 tabular-nums" />
            </label>
            <label className="flex items-center gap-2 pt-4">
              <input type="checkbox" checked={verified} onChange={(e) => onVerifiedToggle(e.target.checked)} />
              <span>{t("social.verified", "Verified")}</span>
            </label>
          </div>
          <div className="flex justify-end pt-2">
            <Button className="h-7 px-2 text-[11px]" disabled={saveLimits.isPending} onClick={() => saveLimits.mutate()}>
              {saveLimits.isPending ? "…" : t("common.save", "Save")}
            </Button>
          </div>
        </details>

        <UploadMethodsPanel account={account} />
        <InspectSessionPanel accountId={account.id} label={account.label} />

        <div className="flex justify-end gap-1 pt-1">
          <Button variant="outline" className="h-7 px-2 text-[11px]" onClick={onDelete}>{t("common.delete", "delete")}</Button>
        </div>
      </CardContent>
    </Card>
  );
}

// UploadMethodsPanel shows which upload methods a YouTube account supports and
// lets the user connect the official API path (OAuth). API is preferred in auto
// mode (no ban risk); Selenium is the fallback.
function UploadMethodsPanel({ account }: { account: SocialAccountView }) {
  const isYouTube = (account.platform || "").startsWith("youtube");
  if (!isYouTube) return null;
  const used = account.api_uploads_used ?? 0;
  const limit = account.api_uploads_limit ?? 6;
  return (
    <div className="border-t border-border/40 pt-2 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] text-muted-foreground">Upload methods</span>
        <div className="flex items-center gap-1">
          <span className={`text-[10px] rounded px-1.5 py-0.5 border ${account.has_api ? "border-emerald-500/50 text-emerald-400" : "border-border text-muted-foreground"}`}>
            API {account.has_api ? "✓" : "—"}
          </span>
          <span className={`text-[10px] rounded px-1.5 py-0.5 border ${account.has_selenium ? "border-emerald-500/50 text-emerald-400" : "border-border text-muted-foreground"}`}>
            Selenium {account.has_selenium ? "✓" : "—"}
          </span>
        </div>
      </div>
      <div className="flex items-center justify-between gap-2">
        {account.has_api ? (
          <span className="text-[11px] text-muted-foreground">
            {account.oauth_channel_title ? `${account.oauth_channel_title} · ` : ""}
            API quota: <span className="tabular-nums text-foreground">{used}/{limit}</span> today
          </span>
        ) : (
          <span className="text-[11px] text-muted-foreground">Connect API for ban-safe uploads</span>
        )}
        <a
          href={`/api/youtube-oauth/start?account_id=${account.id}`}
          target="_blank" rel="noreferrer"
          className="h-7 px-2 text-[11px] inline-flex items-center rounded border border-border hover:border-primary/50">
          {account.has_api ? "Reconnect API ↗" : "Connect via API ↗"}
        </a>
      </div>
    </div>
  );
}

// InspectSessionPanel lets the user open a live, viewable Firefox running on
// this account's saved profile — to watch the session and confirm the channel
// is still logged in. Backed by a jlesage/firefox container over noVNC.
function InspectSessionPanel({ accountId, label }: { accountId: string; label?: string }) {
  const { t } = useTranslation();
  const toast = useToast();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  // Poll status only while the panel is open OR a session may be running.
  const status = useQuery<InspectSession>({
    queryKey: ["inspect-session", accountId],
    queryFn: () => api.getInspectSession(accountId),
    refetchInterval: open ? 2000 : false,
  });
  const sess = status.data;
  const ready = sess?.status === "ready" && sess.vnc_url;

  const start = useMutation({
    mutationFn: () => api.startInspectSession(accountId),
    onSuccess: (s) => {
      setOpen(true);
      qc.setQueryData(["inspect-session", accountId], s);
      qc.invalidateQueries({ queryKey: ["inspect-session", accountId] });
    },
    onError: (e: Error) => toast.push("error", e.message),
  });
  const stop = useMutation({
    mutationFn: () => api.stopInspectSession(accountId),
    onSuccess: () => {
      setOpen(false);
      qc.setQueryData(["inspect-session", accountId], { status: "none" });
    },
    onError: (e: Error) => toast.push("error", e.message),
  });

  return (
    <div className="border-t border-border/40 pt-2 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] text-muted-foreground">
          {t("social.session", "Live session")}
          {ready ? <span className="ml-2 inline-block h-2 w-2 rounded-full bg-emerald-500 align-middle" /> : null}
        </span>
        <div className="flex items-center gap-1">
          {ready && sess?.vnc_url ? (
            <>
              <a href={sess.vnc_url} target="_blank" rel="noreferrer"
                 className="h-7 px-2 text-[11px] inline-flex items-center rounded border border-border hover:border-primary/50">
                {t("social.openTab", "Open in tab ↗")}
              </a>
              <Button variant="outline" className="h-7 px-2 text-[11px]" onClick={() => setOpen((v) => !v)}>
                {open ? t("social.hide", "Hide") : t("social.watch", "Watch")}
              </Button>
              <Button variant="outline" className="h-7 px-2 text-[11px]" disabled={stop.isPending} onClick={() => stop.mutate()}>
                {stop.isPending ? "…" : t("social.stop", "Stop")}
              </Button>
            </>
          ) : (
            <Button className="h-7 px-2 text-[11px]" disabled={start.isPending}
              onClick={() => start.mutate()}>
              {start.isPending ? t("social.starting", "Starting…") : t("social.openSession", "Open session")}
            </Button>
          )}
        </div>
      </div>
      {start.isPending ? (
        <p className="text-[10px] text-muted-foreground">{t("social.bootingFirefox", "Booting Firefox with this profile (~10–30s)…")}</p>
      ) : null}
      {sess?.status === "error" ? (
        <p className="text-[10px] text-red-400">{sess.error || "failed to start session"}</p>
      ) : null}
      {open && ready && sess?.vnc_url ? (
        <div className="space-y-1">
          <iframe
            title={`firefox-${label || accountId}`}
            src={sess.vnc_url}
            className="w-full h-[520px] rounded border border-border bg-black"
          />
          <p className="text-[10px] text-muted-foreground">
            {t("social.sessionHint", "Live view of the logged-in browser. Navigate to YouTube Studio to verify access. Stop when done.")}
          </p>
        </div>
      ) : null}
    </div>
  );
}

function AddAccountModal({ platform, onClose, onCreated }: { platform: string; onClose: () => void; onCreated: () => void }) {
  const { t } = useTranslation();
  const toast = useToast();
  const [label, setLabel] = useState("");
  const [session, setSession] = useState<FxSession | null>(null);
  const pollRef = useRef<number | null>(null);

  const start = useMutation({
    mutationFn: () => api.fxStart({ platform, label: label || undefined }),
    onSuccess: (s) => setSession(s),
    onError: (e: Error) => toast.push("error", e.message),
  });

  const platformMeta = PLATFORMS.find((p) => p.id === platform);
  const finish = useMutation({
    mutationFn: async () => {
      const res = await api.fxFinish(session!.id, { label: label || undefined });
      // Apply platform-default daily limit on first connect (best-effort).
      if (res?.social_account_id && platformMeta) {
        try {
          await api.patchSocialAccountLimits(res.social_account_id, { daily_upload_limit: platformMeta.defaultLimit });
        } catch { /* ignore — limits can be edited from the card */ }
      }
      return res;
    },
    onSuccess: () => onCreated(),
    onError: (e: Error) => toast.push("error", e.message),
  });

  // Poll session every 3s so the modal reflects the status (waiting → ready → finished).
  useEffect(() => {
    if (!session) return;
    pollRef.current = window.setInterval(async () => {
      try {
        const s = await api.fxGet(session.id);
        setSession(s);
        if (s.status === "ready") return;
        if (s.status === "finished") {
          onCreated();
        }
      } catch { /* ignore */ }
    }, 3000);
    return () => { if (pollRef.current) window.clearInterval(pollRef.current); };
  }, [session?.id]);

  const fxStatusBadge = useMemo(() => {
    if (!session) return null;
    return (
      <Badge variant={session.status === "ready" ? "default" : "info"}>
        {session.status}
      </Badge>
    );
  }, [session?.status]);

  return (
    <div className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4" onClick={onClose}>
      <div className="bg-card border border-border rounded-lg w-full max-w-md p-5 space-y-4" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">{t("social.modalTitle", "Connect account")}</h2>
          <button onClick={onClose} className="text-sm opacity-60 hover:opacity-100">×</button>
        </div>
        <div className="space-y-2">
          <label className="block text-xs uppercase tracking-wide text-muted-foreground">
            {t("social.labelField", "Label (optional)")}
          </label>
          <input
            value={label} onChange={(e) => setLabel(e.target.value)}
            placeholder="MemeMachine"
            className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
            disabled={!!session}
          />
        </div>
        {!session ? (
          <Button className="w-full" disabled={start.isPending} onClick={() => start.mutate()}>
            {start.isPending ? t("social.starting", "Opening browser…") : t("social.startLogin", "Open Firefox")}
          </Button>
        ) : (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">{t("social.session", "session")}</span>
              {fxStatusBadge}
            </div>
            {session.vnc_url ? (
              <a href={session.vnc_url} target="_blank" rel="noreferrer"
                 className="block w-full text-center px-3 py-2 rounded bg-primary text-primary-foreground text-sm hover:brightness-110">
                {t("social.openVnc", "Open browser to log in →")}
              </a>
            ) : (
              <p className="text-xs text-muted-foreground">{t("social.waitingBrowser", "Waiting for browser to start…")}</p>
            )}
            <p className="text-[11px] text-muted-foreground">
              {t("social.flowHint", "Sign in to your YouTube channel in the embedded browser, then click Finish below to save the session.")}
            </p>
            <Button className="w-full" disabled={finish.isPending} onClick={() => finish.mutate()}>
              {finish.isPending ? t("social.finishing", "Saving…") : t("social.finish", "Finish & save")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
