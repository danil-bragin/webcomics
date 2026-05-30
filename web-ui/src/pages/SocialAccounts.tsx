import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type SocialAccountView, type FxSession } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardSkeletonGrid } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast";

// Platforms available on the global library page. YouTube is the only live
// one today; others render as "coming soon" tabs so the IA reads correctly.
const PLATFORMS: { id: string; label: string; live: boolean }[] = [
  { id: "youtube_selenium", label: "YouTube", live: true },
  { id: "telegram", label: "Telegram", live: false },
  { id: "instagram", label: "Instagram", live: false },
  { id: "tiktok", label: "TikTok", live: false },
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
  const s = statusBadge(account.status);
  const cooldown = account.cooldown_until && new Date(account.cooldown_until).getTime() > Date.now();
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
        <div className="flex justify-end gap-1 pt-1">
          <Button variant="outline" className="h-7 px-2 text-[11px]" onClick={onDelete}>{t("common.delete", "delete")}</Button>
        </div>
      </CardContent>
    </Card>
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

  const finish = useMutation({
    mutationFn: () => api.fxFinish(session!.id, { label: label || undefined }),
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
