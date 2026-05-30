import { useEffect, useRef, useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { useQueries } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type RunView, type AssetView } from "@/api/client";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { fmtDuration, fmtMoney, statusVariant } from "@/lib/format";

// Side-by-side comparison page. `?ids=A,B,C` selects which runs to load.
// All videos play in sync (master is the leftmost). Pause one → pauses all.
export function Compare() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const ids = (params.get("ids") || "").split(",").map((s) => s.trim()).filter(Boolean);

  const qs = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["run", id],
      queryFn: () => api.getRun(id),
      refetchInterval: 0,
    })),
  });

  const runs: RunView[] = qs.map((q) => q.data).filter(Boolean) as RunView[];

  if (ids.length === 0) {
    return (
      <div className="max-w-3xl mx-auto p-6 space-y-3">
        <p className="text-sm text-muted-foreground">
          {t("compare.noRunsSelected")} <Link to="/runs" className="underline">/runs</Link>
        </p>
      </div>
    );
  }

  return (
    <div className="max-w-[1600px] mx-auto p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">{t("compare.titleCount", { count: ids.length })}</h1>
        <Link to="/runs" className="text-xs text-muted-foreground underline">{t("compare.backToRuns")}</Link>
      </div>
      <SyncedVideoGrid runs={runs} />
      <PromptRow runs={runs} />
      <PanelGrid runs={runs} />
      <MetaRow runs={runs} />
    </div>
  );
}

function SyncedVideoGrid({ runs }: { runs: RunView[] }) {
  const { t } = useTranslation();
  const refs = useRef<(HTMLVideoElement | null)[]>([]);
  const urls = useVideoUrls(runs);

  const playAll = () => refs.current.forEach((v) => v?.play().catch(() => {}));
  const pauseAll = () => refs.current.forEach((v) => v?.pause());
  const seek = (t: number) =>
    refs.current.forEach((v) => {
      if (v) v.currentTime = t;
    });

  return (
    <div className="space-y-2">
      <div className="flex gap-2 items-center">
        <button className="text-xs px-2 py-1 rounded bg-primary text-primary-foreground" onClick={playAll}>
          ▶ {t("compare.playAll")}
        </button>
        <button className="text-xs px-2 py-1 rounded bg-secondary" onClick={pauseAll}>
          ⏸ {t("compare.pauseAll")}
        </button>
        <button className="text-xs px-2 py-1 rounded bg-secondary" onClick={() => seek(0)}>
          ⏮ {t("compare.reset")}
        </button>
      </div>
      <div className={`grid gap-3 ${gridCols(runs.length)}`}>
        {runs.map((r, i) => (
          <div key={r.id} className="space-y-1">
            {urls[i] ? (
              <video
                ref={(el) => (refs.current[i] = el)}
                src={urls[i]!}
                className="w-full aspect-square object-cover rounded bg-black"
                controls
                playsInline
                preload="auto"
              />
            ) : (
              <div className="aspect-square bg-secondary/40 rounded animate-pulse" />
            )}
            <div className="flex items-center justify-between text-xs">
              <Badge variant={statusVariant(r.status)}>{r.status}</Badge>
              <span className="tabular-nums">{fmtMoney(r.total_cost_usd)}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function PromptRow({ runs }: { runs: RunView[] }) {
  const { t } = useTranslation();
  if (runs.length === 0) return null;
  const same = runs.every((r) => r.prompt === runs[0].prompt);
  return (
    <Card>
      <CardContent className="py-3">
        <p className="text-[10px] uppercase tracking-wide text-muted-foreground">
          {same ? t("compare.samePrompt") : t("compare.differentPrompts")}
        </p>
        {same ? (
          <p className="text-sm">{runs[0].prompt}</p>
        ) : (
          <ul className="text-sm space-y-1 mt-1">
            {runs.map((r) => <li key={r.id}>– {r.prompt}</li>)}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function MetaRow({ runs }: { runs: RunView[] }) {
  const { t } = useTranslation();
  return (
    <div className={`grid gap-3 ${gridCols(runs.length)}`}>
      {runs.map((r) => {
        const cfg = r.config_snapshot ?? [];
        const img = cfg.find((s: any) => s.type === "image");
        const script = cfg.find((s: any) => s.type === "script");
        const assemble = cfg.find((s: any) => s.type === "assemble");
        const pc = Number((script?.params as any)?.panel_count ?? r.steps[0]?.panels_expected ?? 0);
        const perPanel = Number((assemble?.params as any)?.panel_duration_ms ?? 0);
        const styleRef = String((img?.params as any)?.style_reference ?? "none");
        return (
          <Card key={r.id}>
            <CardContent className="py-3 space-y-1 text-xs">
              <KV k={t("compare.imageModel")} v={img?.model || ""} />
              <KV k={t("compare.styleRef")} v={styleRef} />
              <KV k={t("compare.scriptModel")} v={script?.model || ""} />
              <KV k={t("compare.panels")} v={String(pc)} />
              <KV k={t("compare.videoDuration")} v={`${((pc * perPanel) / 1000).toFixed(1)}s`} />
              <KV k={t("compare.runDuration")} v={fmtDuration(r.started_at, r.finished_at)} />
              <KV k={t("compare.cost")} v={fmtMoney(r.total_cost_usd)} />
              <Link to={`/runs/${r.id}`} className="text-[10px] text-primary underline">{t("compare.openDetail")}</Link>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

function PanelGrid({ runs }: { runs: RunView[] }) {
  const { t } = useTranslation();
  const panelsByRun = runs.map((r) => {
    const imgStep = r.steps.find((s) => s.type === "image");
    const assets = (r.assets ?? []).filter((a) => a.kind === "panel_image" && a.step_id === imgStep?.id);
    const outputs = Array.isArray(imgStep?.outputs) ? (imgStep!.outputs as any[]) : [];
    return outputs.map((o: any) => {
      const a = assets.find((x: AssetView) => x.object_key === o.object_key);
      return { index: o.index, asset: a };
    });
  });

  const maxPanels = Math.max(0, ...panelsByRun.map((p) => p.length));

  return (
    <div className="space-y-2">
      <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("compare.panelGridHint")}</p>
      <div className="space-y-2">
        {Array.from({ length: maxPanels }).map((_, panelIdx) => (
          <div key={panelIdx} className={`grid gap-2 ${gridCols(runs.length)}`}>
            {runs.map((_, runIdx) => {
              const entry = panelsByRun[runIdx]?.[panelIdx];
              return <PanelTile key={runIdx} asset={entry?.asset} />;
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

function PanelTile({ asset }: { asset?: AssetView }) {
  const [url, setURL] = useState<string | null>(null);
  useEffect(() => {
    if (!asset) return;
    let alive = true;
    api.getAssetUrl(asset.id).then((r) => alive && setURL(r.url)).catch(() => {});
    return () => { alive = false; };
  }, [asset?.id]);
  if (!asset) return <div className="aspect-square bg-secondary/10 rounded" />;
  if (!url) return <div className="aspect-square bg-secondary/40 rounded animate-pulse" />;
  return <img src={url} className="aspect-square w-full object-cover rounded" />;
}

function KV({ k, v }: { k: string; v?: string }) {
  if (!v) return null;
  return (
    <div className="flex items-baseline gap-1">
      <span className="text-muted-foreground">{k}:</span>
      <span className="truncate">{v}</span>
    </div>
  );
}

function gridCols(n: number): string {
  if (n <= 1) return "grid-cols-1";
  if (n === 2) return "grid-cols-2";
  if (n === 3) return "grid-cols-3";
  if (n === 4) return "grid-cols-2 lg:grid-cols-4";
  return "grid-cols-2 lg:grid-cols-3 xl:grid-cols-4";
}

function useVideoUrls(runs: RunView[]): (string | null)[] {
  const [urls, setURLs] = useState<(string | null)[]>([]);
  useEffect(() => {
    let alive = true;
    Promise.all(
      runs.map(async (r) => {
        const v = (r.assets ?? []).find((a) => a.kind === "video");
        if (!v) return null;
        try {
          const out = await api.getAssetUrl(v.id);
          return out.url;
        } catch {
          return null;
        }
      }),
    ).then((u) => alive && setURLs(u));
    return () => { alive = false; };
  }, [runs.map((r) => r.id).join(",")]);
  return urls;
}
