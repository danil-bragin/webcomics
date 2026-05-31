import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, type RunView, type StepView, type AssetView, type AttemptView, type UploadRecordView } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";
import { fmtDuration, fmtMoney, statusVariant } from "@/lib/format";
import { useToast } from "@/components/ui/toast";
import { useEffect, useMemo, useState } from "react";

// Cached presigned-URL fetcher. Presigned URLs live 5 min on the API side, so
// staleTime=4m means tab switches + parent re-renders reuse the same URL
// instead of firing a fresh round trip per panel. Without this, opening a run
// with 5 panels triggered 5 presign + 5 image fetches every mount, which felt
// like the page was hanging on first paint.
function useAssetURL(assetId: string | undefined): string | null {
  const q = useQuery({
    queryKey: ["asset-url", assetId],
    queryFn: () => api.getAssetUrl(assetId!),
    enabled: !!assetId,
    staleTime: 4 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  return q.data?.url ?? null;
}

function StepTimeline({ run }: { run: RunView }) {
  const start = run.started_at ? new Date(run.started_at).getTime() : 0;
  const end = run.finished_at ? new Date(run.finished_at).getTime() : Date.now();
  const total = Math.max(1, end - start);
  const colors = ["bg-blue-500", "bg-amber-500", "bg-emerald-500", "bg-purple-500", "bg-pink-500", "bg-cyan-500"];
  return (
    <div className="space-y-1">
      <div className="flex h-3 w-full rounded overflow-hidden bg-secondary/40">
        {run.steps.map((s, idx) => {
          const sStart = s.started_at ? new Date(s.started_at).getTime() : start;
          const sEnd = s.finished_at ? new Date(s.finished_at).getTime() : end;
          const width = Math.max(0, ((sEnd - sStart) / total) * 100);
          if (width === 0) return null;
          return (
            <div
              key={s.id}
              className={colors[idx % colors.length]}
              style={{ width: `${width}%` }}
              title={`${s.type} · ${(((sEnd - sStart) / 1000)).toFixed(2)}s`}
            />
          );
        })}
      </div>
      <div className="flex gap-3 text-[10px] text-muted-foreground flex-wrap">
        {run.steps.map((s, idx) => (
          <span key={s.id} className="flex items-center gap-1">
            <span className={`inline-block w-2 h-2 rounded ${colors[idx % colors.length]}`} />
            {s.type}
          </span>
        ))}
      </div>
    </div>
  );
}

// --- per-step renderers ---

function ScriptStepBody({ step }: { step: StepView }) {
  const { t } = useTranslation();
  const input: any = step.input ?? {};
  const panels: { index: number; prompt: string; caption?: string }[] = Array.isArray(step.outputs) ? step.outputs : [];
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3 text-xs">
        <KV k="model" v={input.model || step.model} />
        <KV k="provider" v={step.provider} />
        {input.params?.panel_count ? <KV k="panel_count" v={String(input.params.panel_count)} /> : null}
      </div>
      {input.prompt ? (
        <Field label={t("runs.userPrompt")}>
          <p className="text-sm">{input.prompt}</p>
        </Field>
      ) : null}
      {input.system_prompt ? (
        <Field label={t("runs.systemPrompt")}>
          <p className="text-xs text-muted-foreground whitespace-pre-wrap">{input.system_prompt}</p>
        </Field>
      ) : null}
      {panels.length > 0 ? (
        <Field label={t("runs.outputPanels", { count: panels.length })}>
          <div className="space-y-2">
            {panels.map((p) => (
              <div key={p.index} className="rounded border border-border p-2 bg-secondary/20">
                <div className="text-[10px] text-muted-foreground mb-1">panel {p.index}</div>
                {p.caption ? <p className="text-sm font-medium">{p.caption}</p> : null}
                <p className="text-xs text-muted-foreground mt-1">{p.prompt}</p>
              </div>
            ))}
          </div>
        </Field>
      ) : null}
    </div>
  );
}

function ImageStepBody({ step, assets }: { step: StepView; assets: AssetView[] }) {
  const input: any = step.input ?? {};
  const outputs: { index: number; object_key: string }[] = Array.isArray(step.outputs) ? step.outputs : [];
  const inputPanels: { index: number; prompt: string; caption?: string }[] = input.panels ?? [];
  // Match per-panel input prompt with the produced asset.
  const panelRows = outputs.map((o) => {
    const inP = inputPanels.find((p) => p.index === o.index) ?? { index: o.index, prompt: "" };
    const asset = assets.find((a) => a.object_key === o.object_key);
    return { index: o.index, prompt: inP.prompt, caption: inP.caption, asset };
  });
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3 text-xs">
        <KV k="model" v={input.model || step.model} />
        <KV k="provider" v={step.provider} />
        {input.params?.image_size ? <KV k="image_size" v={input.params.image_size} /> : null}
        {input.params?.num_inference_steps ? <KV k="num_inference_steps" v={String(input.params.num_inference_steps)} /> : null}
      </div>
      <Field label={`Panels — ${step.panels_completed}/${step.panels_expected}`}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          {panelRows.map((r) => (
            <PanelTile key={r.index} index={r.index} prompt={r.prompt} caption={r.caption} asset={r.asset} />
          ))}
        </div>
      </Field>
    </div>
  );
}

function PanelTile({ index, prompt, caption, asset }: { index: number; prompt: string; caption?: string; asset?: AssetView }) {
  const url = useAssetURL(asset?.id);
  const [lightbox, setLightbox] = useState(false);
  const sizeKB = asset && asset.bytes > 0 ? `${(asset.bytes / 1024).toFixed(0)} KB` : null;
  return (
    <div className="rounded border border-border overflow-hidden bg-secondary/20 group relative">
      {url ? (
        <button onClick={() => setLightbox(true)} className="block w-full">
          <img src={url} alt={`panel ${index}`} className="aspect-square w-full object-cover" />
        </button>
      ) : (
        <div className="aspect-square bg-secondary/40 animate-pulse" />
      )}
      {url ? (
        <a
          href={url}
          download={`panel-${index}.png`}
          className="absolute top-1 right-1 w-7 h-7 rounded bg-background/80 border border-border opacity-0 group-hover:opacity-100 transition-opacity inline-flex items-center justify-center text-xs"
          title="download"
          aria-label="download"
        >
          ⬇
        </a>
      ) : null}
      <div className="p-2 space-y-1">
        <div className="flex items-center justify-between text-[10px] text-muted-foreground">
          <span>panel {index}</span>
          {sizeKB ? <span>{sizeKB}</span> : null}
        </div>
        {caption ? <p className="text-xs font-medium">{caption}</p> : null}
        <p className="text-[11px] text-muted-foreground leading-snug">{prompt}</p>
      </div>
      {lightbox && url ? (
        <div
          className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center p-4"
          onClick={() => setLightbox(false)}
        >
          <img src={url} alt={`panel ${index} full`} className="max-w-full max-h-full object-contain" />
          <button
            onClick={(e) => { e.stopPropagation(); setLightbox(false); }}
            className="absolute top-4 right-4 w-10 h-10 rounded-full bg-background/80 text-foreground text-lg"
          >
            ✕
          </button>
          <a
            href={url}
            download={`panel-${index}.png`}
            onClick={(e) => e.stopPropagation()}
            className="absolute top-4 right-16 px-3 h-10 rounded bg-primary text-primary-foreground inline-flex items-center text-sm"
          >
            ⬇ download
          </a>
        </div>
      ) : null}
    </div>
  );
}

function AudioStepBody({ step, assets }: { step: StepView; assets: AssetView[] }) {
  const { t } = useTranslation();
  const input: any = step.input ?? {};
  const captions: string[] = input.captions ?? [];
  const audio = assets.find((a) => a.kind === "audio" && a.step_id === step.id);
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3 text-xs">
        <KV k="model" v={input.model || step.model} />
        <KV k="provider" v={step.provider} />
      </div>
      {captions.length > 0 ? (
        <Field label={t("runs.sourceCaptions")}>
          <ul className="text-xs space-y-1 list-disc pl-4">
            {captions.map((c, i) => <li key={i}>{c}</li>)}
          </ul>
        </Field>
      ) : null}
      {audio ? <AudioPlayer assetId={audio.id} /> : null}
    </div>
  );
}

function AssembleStepBody({ step, assets }: { step: StepView; assets: AssetView[] }) {
  const { t } = useTranslation();
  const input: any = step.input ?? {};
  const panels: { index: number; object_key: string; duration_ms: number; transition: string }[] = input.panels ?? [];
  const video = assets.find((a) => a.kind === "video" && a.step_id === step.id);
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-3 text-xs">
        <KV k="width" v={`${input.width}`} />
        <KV k="height" v={`${input.height}`} />
        <KV k="fps" v={`${input.fps}`} />
      </div>
      <Field label={t("runs.composition")}>
        <div className="text-xs text-muted-foreground space-y-1">
          {panels.map((p) => (
            <div key={p.index}>
              {t("runs.panelLine", { idx: p.index, dur: (p.duration_ms / 1000).toFixed(1), trans: p.transition })}
            </div>
          ))}
          {input.audio_key ? <div>+ {t("runs.audioTrack")}: {input.audio_key}</div> : null}
          {input.music_key ? <div>+ {t("runs.musicTrack")}: {input.music_key}</div> : null}
        </div>
      </Field>
      {video ? <VideoPlayer assetId={video.id} /> : null}
    </div>
  );
}

function UploadStepBody({ step }: { step: StepView }) {
  const { t } = useTranslation();
  const input: any = step.input ?? {};
  const outputs: { external_ref: string }[] = Array.isArray(step.outputs) ? step.outputs : [];
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3 text-xs">
        <KV k="provider" v={input.provider} />
        <KV k="video_key" v={input.video_key} />
      </div>
      {outputs.map((o, i) => (
        <Field key={i} label={t("runs.externalRef")}>
          <code className="text-xs">{o.external_ref}</code>
        </Field>
      ))}
    </div>
  );
}

function GenericStepBody({ step }: { step: StepView }) {
  return (
    <details className="text-xs">
      <summary className="cursor-pointer text-muted-foreground">raw step data</summary>
      <pre className="mt-1 max-h-64 overflow-auto bg-secondary/30 p-2 rounded">
        {JSON.stringify({ input: step.input, outputs: step.outputs }, null, 2)}
      </pre>
    </details>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="text-[11px] uppercase tracking-wide text-muted-foreground mb-1">{label}</p>
      {children}
    </div>
  );
}

function KV({ k, v }: { k: string; v?: string }) {
  if (!v) return null;
  return (
    <div>
      <span className="text-muted-foreground">{k}: </span>
      <span className="tabular-nums">{v}</span>
    </div>
  );
}

function StepCard({ runId, step, assets, busy }: { runId: string; step: StepView; assets: AssetView[]; busy: boolean }) {
  const matchingAssets = assets.filter((a) => a.step_id === step.id);
  const [regenOpen, setRegenOpen] = useState(false);
  const [attemptIdx, setAttemptIdx] = useState<number | null>(null);
  const body = (() => {
    switch (step.type) {
      case "script": return <ScriptStepBody step={step} />;
      case "image": return <ImageStepBody step={step} assets={matchingAssets} />;
      case "audio":
      case "music": return <AudioStepBody step={step} assets={matchingAssets} />;
      case "assemble": return <AssembleStepBody step={step} assets={matchingAssets} />;
      case "upload": return <UploadStepBody step={step} />;
      default: return <GenericStepBody step={step} />;
    }
  })();
  const attempts = step.attempts ?? [];
  const showAttemptIdx = attemptIdx ?? attempts.length - 1;
  const selectedAttempt = attempts[showAttemptIdx];
  const browsingHistory = attemptIdx !== null && attemptIdx !== attempts.length - 1;
  return (
    <Card className={step.is_stale ? "border-amber-500/40" : undefined}>
      <CardHeader>
        <div className="flex items-center justify-between w-full">
          <div className="flex items-center gap-2">
            <CardTitle>#{step.index} · {step.type}</CardTitle>
            <span className="text-[10px] text-muted-foreground tabular-nums">v{step.current_version}</span>
            {step.is_stale ? <Badge variant="warning">stale</Badge> : null}
          </div>
          <div className="flex items-center gap-3">
            <Badge variant={statusVariant(step.status)}>{step.status}</Badge>
            <span className="text-sm tabular-nums text-muted-foreground">
              {fmtDuration(step.started_at, step.finished_at)} · {fmtMoney(step.cost_usd)}
            </span>
            <RegenButton
              runId={runId}
              step={step}
              open={regenOpen}
              setOpen={setRegenOpen}
              busy={busy}
            />
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {step.is_stale ? (
          <p className="text-xs text-amber-400">
            Upstream step changed since this attempt ran. Regenerate to refresh.
          </p>
        ) : null}
        {step.error ? <p className="text-sm text-red-400">{step.error}</p> : null}
        {attempts.length > 1 ? (
          <AttemptHistoryRow
            attempts={attempts}
            activeId={step.active_attempt_id}
            selectedIdx={showAttemptIdx}
            onSelect={(i) => setAttemptIdx(i === attempts.length - 1 ? null : i)}
          />
        ) : null}
        {browsingHistory && selectedAttempt ? (
          <AttemptViewer attempt={selectedAttempt} />
        ) : (
          body
        )}
        <details className="text-xs">
          <summary className="cursor-pointer text-muted-foreground">raw JSON</summary>
          <pre className="mt-1 max-h-64 overflow-auto bg-secondary/30 p-2 rounded text-[10px]">
            {JSON.stringify({ input: step.input, outputs: step.outputs }, null, 2)}
          </pre>
        </details>
      </CardContent>
    </Card>
  );
}

function AttemptHistoryRow({ attempts, activeId, selectedIdx, onSelect }:
  { attempts: AttemptView[]; activeId?: string; selectedIdx: number; onSelect: (i: number) => void }) {
  // Render the attempt chain as a connected graph of dots so v1 → v2 → v3
  // reads as a version tree, not a row of buttons. The active version (the one
  // downstream consumes) gets a thicker ring; the selected one for browsing is
  // outlined in foreground.
  const colorFor = (status: string) => {
    if (status === "completed") return "bg-emerald-500";
    if (status === "failed") return "bg-red-500";
    if (status === "running") return "bg-blue-500";
    if (status === "queued") return "bg-sky-400";
    return "bg-zinc-500";
  };
  return (
    <div className="space-y-1">
      <p className="text-[10px] uppercase tracking-wide text-muted-foreground">history</p>
      <div className="flex items-center gap-0">
        {attempts.map((a, i) => {
          const isActive = a.id === activeId;
          const isSelected = i === selectedIdx;
          const last = i === attempts.length - 1;
          return (
            <div key={a.id} className="flex items-center">
              <button
                onClick={() => onSelect(i)}
                title={`v${a.attempt_no} · ${a.status}${a.error ? ` — ${a.error.slice(0, 100)}` : ""}`}
                className={
                  "relative flex flex-col items-center gap-0.5 group"
                }
              >
                <span
                  className={
                    "h-6 w-6 rounded-full flex items-center justify-center text-[10px] font-medium text-white tabular-nums " +
                    colorFor(a.status) +
                    (isActive ? " ring-2 ring-foreground/80" : "") +
                    (isSelected && !isActive ? " ring-2 ring-foreground/40" : "")
                  }
                >
                  {a.attempt_no}
                </span>
                <span className={"text-[9px] " + (isSelected ? "text-foreground" : "text-muted-foreground")}>
                  v{a.attempt_no}
                  {isActive ? " ←" : ""}
                </span>
              </button>
              {!last ? (
                <div className="w-6 h-px bg-border self-start mt-3" />
              ) : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function AttemptViewer({ attempt }: { attempt: AttemptView }) {
  return (
    <div className="space-y-2 text-xs">
      <p className="text-amber-400">Showing historical attempt v{attempt.attempt_no} — switch back to latest to act.</p>
      {attempt.error ? <p className="text-red-400">{attempt.error}</p> : null}
      {attempt.params_override ? (
        <Field label="params_override">
          <pre className="bg-secondary/30 p-2 rounded text-[10px]">
            {JSON.stringify(attempt.params_override, null, 2)}
          </pre>
        </Field>
      ) : null}
      <Field label="outputs">
        <pre className="bg-secondary/30 p-2 rounded text-[10px] max-h-64 overflow-auto">
          {JSON.stringify(attempt.outputs, null, 2)}
        </pre>
      </Field>
    </div>
  );
}

function RegenButton({ runId, step, open, setOpen, busy }:
  { runId: string; step: StepView; open: boolean; setOpen: (b: boolean) => void; busy: boolean }) {
  const qc = useQueryClient();
  const [sysPrompt, setSysPrompt] = useState("");
  const [model, setModel] = useState("");
  const [paramsJSON, setParamsJSON] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const regen = useMutation({
    mutationFn: (body: { params?: Record<string, unknown> }) => api.regenerateStep(runId, step.index, body),
    onSuccess: () => {
      setOpen(false);
      setSysPrompt(""); setModel(""); setParamsJSON(""); setErr(null);
      qc.invalidateQueries({ queryKey: ["run", runId] });
    },
    onError: (e: Error) => setErr(e.message),
  });
  const inProgress = step.status === "running" || step.status === "pending";
  const blocked = busy && !inProgress;
  return (
    <div className="relative">
      <Button
        variant="outline"
        className="h-7 px-2 text-xs"
        disabled={inProgress || blocked || regen.isPending}
        onClick={() => setOpen(!open)}
      >
        regenerate
      </Button>
      {open ? (
        <div className="absolute right-0 top-9 z-10 w-[420px] rounded-md border border-border bg-card p-3 shadow-lg space-y-2">
          <div className="text-xs text-muted-foreground">
            Bumps the step to a new version. Downstream steps will be marked stale.
          </div>
          {step.type === "script" ? (
            <div>
              <label className="text-[11px] text-muted-foreground">system_prompt override</label>
              <Textarea
                rows={3}
                value={sysPrompt}
                onChange={(e) => setSysPrompt(e.target.value)}
                placeholder="(leave blank to keep current)"
                className="font-mono text-[11px]"
              />
            </div>
          ) : null}
          <div>
            <label className="text-[11px] text-muted-foreground">model override</label>
            <input
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="(leave blank to keep current)"
              className="h-8 w-full rounded-md border border-border bg-secondary/30 px-2 text-xs"
            />
          </div>
          <div>
            <label className="text-[11px] text-muted-foreground">extra params JSON</label>
            <Textarea
              rows={3}
              value={paramsJSON}
              onChange={(e) => setParamsJSON(e.target.value)}
              placeholder='e.g. {"transition":"slide","panel_duration_ms":2000}'
              className="font-mono text-[10px]"
            />
          </div>
          {err ? <p className="text-xs text-red-400">{err}</p> : null}
          <div className="flex justify-end gap-2">
            <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => setOpen(false)}>cancel</Button>
            <Button
              className="h-7 px-2 text-xs"
              disabled={regen.isPending}
              onClick={() => {
                setErr(null);
                const params: Record<string, unknown> = {};
                if (sysPrompt) params.system_prompt = sysPrompt;
                if (model) params.model = model;
                if (paramsJSON.trim()) {
                  try {
                    Object.assign(params, JSON.parse(paramsJSON));
                  } catch (e) {
                    setErr("invalid JSON: " + (e as Error).message);
                    return;
                  }
                }
                regen.mutate({ params: Object.keys(params).length ? params : undefined });
              }}
            >
              {regen.isPending ? "regenerating…" : "regenerate"}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function VideoPlayer({ assetId }: { assetId: string }) {
  const url = useAssetURL(assetId);
  if (!url) return <div className="aspect-video rounded bg-secondary/30 animate-pulse" />;
  return <video src={url} controls className="w-full rounded" />;
}

function AudioPlayer({ assetId }: { assetId: string }) {
  const url = useAssetURL(assetId);
  if (!url) return <div className="h-10 rounded bg-secondary/30 animate-pulse" />;
  return <audio src={url} controls className="w-full" />;
}

export function RunDetail() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const retry = useMutation({
    mutationFn: () => api.retryRun(id),
    onSuccess: (r) => { toast.push("success", t("runs.retried", "Run retried")); navigate(`/runs/${r.id}`); },
    onError: (e: Error) => toast.push("error", e.message),
  });
  const cancel = useMutation({
    mutationFn: () => api.cancelRun(id),
    onSuccess: () => toast.push("info", t("runs.cancelled", "Run cancelled")),
    onError: (e: Error) => toast.push("error", e.message),
  });
  const del = useMutation({
    mutationFn: () => api.deleteRun(id),
    onSuccess: () => { toast.push("success", t("runs.deleted", "Run deleted")); navigate("/runs"); },
    onError: (e: Error) => toast.push("error", e.message),
  });
  const q = useQuery<RunView>({
    queryKey: ["run", id],
    queryFn: () => api.getRun(id),
    refetchInterval: 0,
  });
  const [streamRun, setStreamRun] = useState<RunView | null>(null);
  useEffect(() => {
    if (!id) return;
    const es = new EventSource(`/api/runs/${id}/events`);
    es.addEventListener("state", (e) => {
      try {
        setStreamRun(JSON.parse((e as MessageEvent).data));
      } catch {}
    });
    es.addEventListener("error", () => es.close());
    return () => es.close();
  }, [id]);
  const qc = useQueryClient();
  const assemble = useMutation({
    mutationFn: () => api.requestAssemble(id, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["run", id] }),
  });
  const run = streamRun ?? q.data;
  const busy = useMemo(() => run?.status === "running" || run?.status === "queued", [run?.status]);
  if (!run) return <p className="p-6 text-sm text-muted-foreground">{t("timeline.loadingRun")}</p>;
  const assembleIdx = run.steps.findIndex((s) => s.type === "assemble");
  const canAssemble = !busy && assembleIdx >= 0 && run.status !== "cancelled";
  const upstreamReady = assembleIdx > 0 ? run.steps[assembleIdx - 1]?.status === "completed" : true;
  const showAssembleCTA = run.status === "awaiting_action" && upstreamReady;
  return (
    <div className="max-w-5xl mx-auto p-6 space-y-4">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between w-full">
            <div className="flex-1 min-w-0">
              {run.project_id ? (
                <Link to={`/projects/${run.project_id}`}
                  className="text-[11px] uppercase tracking-wide text-muted-foreground hover:text-foreground inline-flex items-center gap-1">
                  ← {run.project_name || "project"}
                </Link>
              ) : null}
              <CardTitle className="mt-1">{run.prompt}</CardTitle>
            </div>
            <div className="flex items-center gap-3">
              <Badge variant={statusVariant(run.status)}>{t(`runs.status.${run.status}`, run.status)}</Badge>
              {run.auto_assemble === false ? <Badge variant="info">{t("runs.manualAssemble")}</Badge> : null}
              <span className="text-sm tabular-nums" title={run.max_cost_usd > 0 ? `cap ${fmtMoney(run.max_cost_usd)}` : "no cap"}>
                {fmtMoney(run.total_cost_usd)}
                {run.max_cost_usd > 0 ? <span className="text-muted-foreground"> / {fmtMoney(run.max_cost_usd)}</span> : null}
              </span>
              <span className="text-sm text-muted-foreground">
                {fmtDuration(run.started_at, run.finished_at)}
              </span>
              {canAssemble ? (
                <Link to={`/runs/${id}/timeline`} className="text-xs underline text-muted-foreground">
                  {t("runs.editTimeline")}
                </Link>
              ) : null}
              {(run.status === "running" || run.status === "queued" || run.status === "awaiting_action") ? (
                <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
                  {t("common.cancel")}
                </Button>
              ) : null}
              {(run.status === "failed" || run.status === "cancelled" || run.status === "completed") ? (
                <>
                  <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => retry.mutate()} disabled={retry.isPending}>
                    {t("runs.reRun")}
                  </Button>
                  <Button variant="outline" className="h-7 px-2 text-xs text-red-400"
                    onClick={() => {
                      if (confirm(t("runs.confirmDelete"))) del.mutate();
                    }}
                    disabled={del.isPending}>
                    {del.isPending ? t("common.loading") : t("common.delete")}
                  </Button>
                </>
              ) : null}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-xs text-muted-foreground mb-2">
            {t("runs.stepProgress", { current: Math.min(run.current_step_index + 1, run.expected_steps), total: run.expected_steps })}
          </p>
          <StepTimeline run={run} />
          {run.error ? <p className="mt-2 text-sm text-red-400">{run.error}</p> : null}
          {showAssembleCTA ? (
            <div className="mt-3 flex items-center justify-between rounded border border-blue-500/30 bg-blue-500/5 p-2 text-xs">
              <span>{t("runs.pipelinePaused")}</span>
              <Button
                className="h-7 px-2 text-xs"
                disabled={assemble.isPending}
                onClick={() => assemble.mutate()}
              >
                {assemble.isPending ? t("runs.starting") : t("runs.assembleNow")}
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>
      {run.steps.map((s) => (
        <StepCard key={s.id} runId={id} step={s} assets={run.assets ?? []} busy={!!busy} />
      ))}
      <UploadRecordsCard runId={id} />
    </div>
  );
}

function UploadRecordsCard({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery<UploadRecordView[]>({
    queryKey: ["uploads-by-run", runId],
    queryFn: () => api.listRunUploadRecords(runId),
    refetchInterval: 6000,
  });
  const records = q.data ?? [];
  if (records.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("runs.uploads")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {records.map((r) => (
          <UploadRecordCard
            key={r.id}
            rec={r}
            onChange={() => qc.invalidateQueries({ queryKey: ["uploads-by-run", runId] })}
          />
        ))}
      </CardContent>
    </Card>
  );
}

const STATUS_VARIANT: Record<UploadRecordView["status"], "default" | "info" | "success" | "warning" | "danger"> = {
  pending: "default",
  metadata_ready: "info",
  pending_review: "warning",
  approved: "info",
  rejected: "default",
  uploading: "info",
  uploaded: "info",
  published: "success",
  failed: "danger",
};

function UploadRecordCard({ rec, onChange }: { rec: UploadRecordView; onChange: () => void }) {
  const { t } = useTranslation();
  const screenshotURL = useAssetURL(rec.error_screenshot_asset_id);
  const [editing, setEditing] = useState(false);
  const publish = useMutation({ mutationFn: () => api.publishUploadRecord(rec.id), onSuccess: onChange });
  const approve = useMutation({ mutationFn: () => api.approveUploadRecord(rec.id), onSuccess: onChange });
  const reject = useMutation({ mutationFn: () => api.rejectUploadRecord(rec.id), onSuccess: onChange });
  const audienceLabel = rec.made_for_kids ? "kids" : "not for kids";
  const confidencePct = Math.round((rec.audience_confidence ?? 0) * 100);
  return (
    <div className="rounded border border-border p-3 space-y-2">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="flex items-center gap-2 min-w-0">
          <Badge variant={STATUS_VARIANT[rec.status] ?? "default"}>{rec.status}</Badge>
          <span className="text-xs text-muted-foreground">{rec.provider}</span>
          {rec.platform_target ? (
            <span className="text-[10px] rounded bg-secondary/30 px-1.5 py-0.5">{rec.platform_target}</span>
          ) : null}
          <span className="text-xs text-muted-foreground">visibility: {rec.visibility}</span>
          <span className="text-xs text-muted-foreground" title={rec.audience_reasoning}>
            audience: {audienceLabel} ({confidencePct}%)
          </span>
          {rec.metadata_overridden ? <Badge variant="info">manual</Badge> : null}
        </div>
        <div className="flex items-center gap-2">
          {rec.external_ref ? (
            <a href={rec.external_ref} target="_blank" rel="noreferrer" className="text-xs underline">
              Open
            </a>
          ) : null}
          {rec.status === "pending_review" ? (
            <>
              <Button className="h-7 px-2 text-xs" onClick={() => approve.mutate()}>{t("runs.approve")}</Button>
              <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => reject.mutate()}>{t("runs.reject")}</Button>
            </>
          ) : null}
          {rec.status === "uploaded" ? (
            <Button className="h-7 px-2 text-xs" onClick={() => publish.mutate()}>{t("runs.publishPublic")}</Button>
          ) : null}
          <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => setEditing((v) => !v)}>
            {editing ? t("common.cancel") : t("common.edit")}
          </Button>
        </div>
      </div>
      {rec.hook ? (
        <p className="text-[11px] text-muted-foreground italic">Hook: {rec.hook}</p>
      ) : null}
      {rec.audience_reasoning ? (
        <p className="text-[11px] text-muted-foreground">Audience reasoning: {rec.audience_reasoning}</p>
      ) : null}
      {editing ? (
        <UploadMetadataEditor rec={rec} onSaved={() => { setEditing(false); onChange(); }} />
      ) : (
        <>
          <div className="text-sm font-medium" title={rec.title}>
            {rec.title || <span className="text-muted-foreground italic">no title</span>}
          </div>
          {rec.description ? (
            <p className="text-xs text-muted-foreground line-clamp-4 whitespace-pre-line">{rec.description}</p>
          ) : null}
          {rec.tags.length > 0 ? (
            <div className="flex flex-wrap gap-1">
              {rec.tags.map((t) => (
                <span key={t} className="text-[10px] rounded bg-secondary/30 px-1.5 py-0.5">{t}</span>
              ))}
            </div>
          ) : null}
        </>
      )}
      {rec.error ? (
        <div className="text-xs text-red-400 whitespace-pre-line">{rec.error}</div>
      ) : null}
      {screenshotURL ? (
        <a href={screenshotURL} target="_blank" rel="noreferrer">
          <img src={screenshotURL} className="max-h-40 rounded border border-border" alt="failure screenshot" />
        </a>
      ) : null}
      {(rec.screenshot_trail ?? []).length > 0 ? (
        <ScreenshotTrailStrip trail={rec.screenshot_trail!} />
      ) : null}
    </div>
  );
}

function ScreenshotTrailStrip({ trail }: { trail: { stage: string; object_key: string }[] }) {
  const [active, setActive] = useState(0);
  const [urls, setUrls] = useState<(string | null)[]>(trail.map(() => null));
  useEffect(() => {
    let alive = true;
    Promise.all(trail.map((t) => api.getScreenshotUrl(t.object_key).then((r) => r.url).catch(() => "")))
      .then((arr) => { if (alive) setUrls(arr); });
    return () => { alive = false; };
  }, [trail.map((t) => t.object_key).join(",")]);
  const cur = trail[active];
  return (
    <div className="space-y-2 mt-2">
      <p className="text-[11px] uppercase text-muted-foreground">Selenium debug trail ({trail.length} frames)</p>
      <div className="flex gap-1 overflow-x-auto pb-1">
        {trail.map((t, i) => (
          <button key={t.object_key + i}
            onClick={() => setActive(i)}
            title={t.stage}
            className={`shrink-0 text-[9px] uppercase rounded border px-2 py-1 transition-colors ${
              active === i ? "border-primary bg-primary/10" : "border-border hover:border-primary/40"
            } ${t.stage.includes("fail") ? "text-red-400" : ""}`}>
            {i + 1}. {t.stage.replace(/^\d+-(step|fail)-/, "")}
          </button>
        ))}
      </div>
      {urls[active] ? (
        <a href={urls[active]!} target="_blank" rel="noreferrer">
          <img src={urls[active]!} alt={cur.stage}
            className="w-full max-h-80 object-contain rounded border border-border bg-black" />
        </a>
      ) : <div className="h-32 bg-secondary/30 animate-pulse rounded" />}
      <p className="text-[10px] text-muted-foreground">{cur.stage}</p>
    </div>
  );
}

function UploadMetadataEditor({ rec, onSaved }: { rec: UploadRecordView; onSaved: () => void }) {
  const { t } = useTranslation();
  const [title, setTitle] = useState(rec.title);
  const [description, setDescription] = useState(rec.description);
  const [tags, setTags] = useState((rec.tags ?? []).join(", "));
  const [hashtags, setHashtags] = useState((rec.hashtags ?? []).join(", "));
  const [visibility, setVisibility] = useState(rec.visibility);
  const [madeForKids, setMadeForKids] = useState(rec.made_for_kids);
  const [commentsEnabled, setCommentsEnabled] = useState(rec.comments_enabled);
  const [ageRestriction, setAgeRestriction] = useState(rec.age_restriction);
  const save = useMutation({
    mutationFn: () => api.editUploadMetadata(rec.id, {
      title, description,
      tags: tags.split(/[,;\n]+/).map((s) => s.trim()).filter(Boolean),
      hashtags: hashtags.split(/[,;\n]+/).map((s) => s.trim()).filter(Boolean),
      visibility,
      made_for_kids: madeForKids,
      age_restriction: ageRestriction,
      category_id: rec.category_id,
      category_label: rec.category_label,
      comments_enabled: commentsEnabled,
      playlist_names: rec.playlist_names,
    }),
    onSuccess: onSaved,
  });
  return (
    <div className="rounded border border-border bg-secondary/10 p-3 space-y-2 text-sm">
      <input value={title} onChange={(e) => setTitle(e.target.value)}
        placeholder={t("runs.uploadTitle")}
        className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
      <textarea value={description} onChange={(e) => setDescription(e.target.value)}
        rows={4} placeholder={t("common.description")}
        className="w-full rounded border border-border bg-secondary/30 px-2 py-1 text-xs" />
      <input value={tags} onChange={(e) => setTags(e.target.value)}
        placeholder={t("runs.tagsPlaceholder")}
        className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs" />
      <input value={hashtags} onChange={(e) => setHashtags(e.target.value)}
        placeholder={t("runs.hashtagsPlaceholder")}
        className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs" />
      <div className="flex items-center gap-3 flex-wrap text-xs">
        <label className="flex items-center gap-1">
          visibility
          <select value={visibility} onChange={(e) => setVisibility(e.target.value as UploadRecordView["visibility"])}
            className="h-7 rounded border border-border bg-secondary/30 px-1">
            <option value="public">public</option>
            <option value="unlisted">unlisted</option>
            <option value="private">private</option>
          </select>
        </label>
        <label className="flex items-center gap-1">
          age
          <select value={ageRestriction} onChange={(e) => setAgeRestriction(e.target.value as UploadRecordView["age_restriction"])}
            className="h-7 rounded border border-border bg-secondary/30 px-1">
            <option value="none">none</option>
            <option value="18plus">18+</option>
          </select>
        </label>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={madeForKids} onChange={(e) => setMadeForKids(e.target.checked)} />
          made for kids
        </label>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={commentsEnabled} onChange={(e) => setCommentsEnabled(e.target.checked)} />
          comments
        </label>
      </div>
      <div className="flex justify-end">
        <Button className="h-7 px-3 text-xs" disabled={save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "saving…" : "save"}
        </Button>
      </div>
    </div>
  );
}
