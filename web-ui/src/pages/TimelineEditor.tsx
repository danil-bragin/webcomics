import { Fragment, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { api, type RunView, type AssetView } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";
import { RESOLUTION_PRESETS, FPS_PRESETS } from "@/lib/options";

type TransitionType = "none" | "fade" | "crossfade" | "slide" | "push" | "zoom" | "wipe";
type EasingName = "linear" | "ease-in" | "ease-out" | "ease-in-out" | "cubic";
type Direction = "left" | "right" | "up" | "down";

type Panel = {
  image_panel_index: number;
  duration_ms: number;
  transition_in: {
    type: TransitionType;
    duration_ms: number;
    easing: EasingName;
    direction: Direction;
  };
  effects: { type: "ken_burns"; zoom_start: number; zoom_end: number; pan_x_end: number; pan_y_end: number }[];
  caption: { text: string; position: "top" | "bottom" | "center" };
};

const TRANSITIONS: TransitionType[] = ["none", "fade", "crossfade", "slide", "push", "zoom", "wipe"];
const EASINGS: EasingName[] = ["linear", "ease-in", "ease-out", "ease-in-out", "cubic"];
const DIRECTIONS: Direction[] = ["left", "right", "up", "down"];

// Colour each transition type so the seam between two clips reads at a glance.
const TRANSITION_COLOR: Record<TransitionType, string> = {
  none: "bg-zinc-700",
  fade: "bg-amber-500",
  crossfade: "bg-emerald-500",
  slide: "bg-sky-500",
  push: "bg-indigo-500",
  zoom: "bg-fuchsia-500",
  wipe: "bg-orange-500",
};

const MIN_PANEL_PX = 80;
const PX_PER_SECOND = 60;

function defaultPanel(index: number, durationMs: number, caption: string): Panel {
  return {
    image_panel_index: index,
    duration_ms: durationMs,
    transition_in: { type: "crossfade", duration_ms: 280, easing: "ease-in-out", direction: "left" },
    effects: [{ type: "ken_burns", zoom_start: 1.0, zoom_end: 1.08, pan_x_end: 0, pan_y_end: -10 }],
    caption: { text: caption, position: "bottom" },
  };
}

function useAssetURL(assetId: string | undefined) {
  const [url, setURL] = useState<string | null>(null);
  useEffect(() => {
    if (!assetId) return;
    let alive = true;
    api.getAssetUrl(assetId).then((r) => alive && setURL(r.url));
    return () => { alive = false; };
  }, [assetId]);
  return url;
}

function TrackTile({
  panel, idx, asset, selected, widthPx, onSelect, onShift,
}: {
  panel: Panel;
  idx: number;
  asset?: AssetView;
  selected: boolean;
  widthPx: number;
  onSelect: () => void;
  onShift: (delta: number) => void;
}) {
  const url = useAssetURL(asset?.id);
  const seconds = (panel.duration_ms / 1000).toFixed(1);
  return (
    <div
      onClick={onSelect}
      className={`group relative shrink-0 h-24 rounded overflow-hidden border cursor-pointer transition-shadow ${
        selected ? "border-primary ring-2 ring-primary/40" : "border-border hover:border-primary/60"
      }`}
      style={{ width: `${Math.max(MIN_PANEL_PX, widthPx)}px` }}
      title={`#${panel.image_panel_index} · ${seconds}s`}
    >
      {url ? (
        <img src={url} className="absolute inset-0 w-full h-full object-cover" alt="" />
      ) : (
        <div className="absolute inset-0 bg-secondary/40 animate-pulse" />
      )}
      <div className="absolute inset-x-0 top-0 px-1.5 py-0.5 bg-gradient-to-b from-black/70 to-transparent flex items-center justify-between text-[10px] text-white">
        <span className="tabular-nums">#{idx + 1}</span>
        <span className="tabular-nums">{seconds}s</span>
      </div>
      <div className="absolute inset-x-0 bottom-0 flex justify-between opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={(e) => { e.stopPropagation(); onShift(-1); }}
          className="px-1.5 py-0.5 text-[10px] bg-black/60 text-white"
          title="move left"
          disabled={idx === 0}
        >‹</button>
        <button
          onClick={(e) => { e.stopPropagation(); onShift(1); }}
          className="px-1.5 py-0.5 text-[10px] bg-black/60 text-white"
          title="move right"
        >›</button>
      </div>
    </div>
  );
}

function TransitionSeam({ panel }: { panel: Panel }) {
  const t = panel.transition_in;
  const ms = t.duration_ms;
  // Width scales with transition duration so a long crossfade reads visibly
  // wider than a snap cut. Caps at ~30px so it doesn't dominate the track.
  const w = Math.min(30, Math.max(8, ms / 25));
  return (
    <div
      className={`shrink-0 h-24 ${TRANSITION_COLOR[t.type]} opacity-80 flex items-center justify-center text-[9px] text-white/90`}
      style={{ width: `${w}px` }}
      title={`${t.type} ${ms}ms ${t.easing}`}
    >
      {t.type === "none" ? "|" : t.type[0].toUpperCase()}
    </div>
  );
}

function TimeRuler({ totalMs }: { totalMs: number }) {
  const seconds = Math.ceil(totalMs / 1000);
  const ticks: number[] = [];
  for (let i = 0; i <= seconds; i++) ticks.push(i);
  return (
    <div
      className="relative h-5 border-b border-border/60"
      style={{ width: `${seconds * PX_PER_SECOND}px`, minWidth: "100%" }}
    >
      {ticks.map((s) => (
        <div
          key={s}
          className="absolute top-0 bottom-0 border-l border-border/40 text-[9px] text-muted-foreground pl-0.5 tabular-nums"
          style={{ left: `${s * PX_PER_SECOND}px` }}
        >
          {s}s
        </div>
      ))}
    </div>
  );
}

function PanelInspector({
  panel, sourceCount, onChange, onRemove,
}: {
  panel: Panel;
  sourceCount: number;
  onChange: (next: Panel) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const t_in = panel.transition_in;
  const eff = panel.effects[0] ?? defaultPanel(0, 0, "").effects[0];
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">
          {t("timeline.slot", { idx: panel.image_panel_index + 1 })} ·{" "}
          <span className="text-muted-foreground">{t("timeline.sourcePanel", { idx: panel.image_panel_index })}</span>
        </h3>
        <Button variant="outline" className="h-7 px-2 text-xs" onClick={onRemove}>
          {t("timeline.remove")}
        </Button>
      </div>

      <section className="space-y-1.5">
        <h4 className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("timeline.durationMs")}</h4>
        <input
          type="range" min={500} max={10000} step={100}
          value={panel.duration_ms}
          onChange={(e) => onChange({ ...panel, duration_ms: Number(e.target.value) })}
          className="w-full"
        />
        <div className="flex justify-between text-[10px] text-muted-foreground tabular-nums">
          <span>0.5s</span>
          <span className="font-medium text-foreground">{(panel.duration_ms / 1000).toFixed(1)}s</span>
          <span>10s</span>
        </div>
      </section>

      <section className="space-y-1.5">
        <h4 className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("studio.transition")}</h4>
        <div className="grid grid-cols-4 gap-1">
          {TRANSITIONS.map((tt) => (
            <button
              key={tt}
              onClick={() => onChange({ ...panel, transition_in: { ...t_in, type: tt } })}
              className={`h-7 rounded text-[10px] border ${
                t_in.type === tt ? `${TRANSITION_COLOR[tt]} text-white border-transparent` : "border-border hover:border-primary/60"
              }`}
            >
              {tt}
            </button>
          ))}
        </div>
        <div className="grid grid-cols-2 gap-2 pt-1">
          <NumField label={t("timeline.transDurationMs")} value={t_in.duration_ms} min={0} max={2000} step={20}
            onChange={(v) => onChange({ ...panel, transition_in: { ...t_in, duration_ms: v } })} />
          <SelectField label={t("timeline.easing")} value={t_in.easing} options={EASINGS}
            onChange={(v) => onChange({ ...panel, transition_in: { ...t_in, easing: v as EasingName } })} />
          {(t_in.type === "slide" || t_in.type === "push" || t_in.type === "wipe") ? (
            <SelectField label={t("timeline.direction")} value={t_in.direction} options={DIRECTIONS}
              onChange={(v) => onChange({ ...panel, transition_in: { ...t_in, direction: v as Direction } })} />
          ) : null}
        </div>
      </section>

      <section className="space-y-1.5">
        <h4 className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("timeline.kenBurns")}</h4>
        <div className="grid grid-cols-2 gap-2">
          <NumField label={t("timeline.zoomStart")} value={eff.zoom_start} min={0.5} max={2} step={0.01}
            onChange={(v) => onChange({ ...panel, effects: [{ ...eff, zoom_start: v }] })} />
          <NumField label={t("timeline.zoomEnd")} value={eff.zoom_end} min={0.5} max={2} step={0.01}
            onChange={(v) => onChange({ ...panel, effects: [{ ...eff, zoom_end: v }] })} />
          <NumField label={t("timeline.panXEnd")} value={eff.pan_x_end} min={-200} max={200} step={1}
            onChange={(v) => onChange({ ...panel, effects: [{ ...eff, pan_x_end: v }] })} />
          <NumField label={t("timeline.panYEnd")} value={eff.pan_y_end} min={-200} max={200} step={1}
            onChange={(v) => onChange({ ...panel, effects: [{ ...eff, pan_y_end: v }] })} />
        </div>
      </section>

      <section className="space-y-1.5">
        <h4 className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("timeline.caption")}</h4>
        <Textarea rows={2} value={panel.caption.text}
          onChange={(e) => onChange({ ...panel, caption: { ...panel.caption, text: e.target.value } })}
          placeholder={t("timeline.captionEmpty")}
          className="font-mono text-[11px]" />
        <SelectField label={t("projectDefaults.position")} value={panel.caption.position} options={["top", "bottom", "center"]}
          onChange={(v) => onChange({ ...panel, caption: { ...panel.caption, position: v as Panel["caption"]["position"] } })} />
      </section>

      {sourceCount > 1 ? (
        <p className="text-[10px] text-muted-foreground pt-2 border-t border-border/40">
          {t("timeline.swapSourceHint", "Click another tile on the track to edit it. Use ‹ › to reorder.")}
        </p>
      ) : null}
    </div>
  );
}

function NumField({ label, value, min, max, step, onChange }:
  { label: string; value: number; min: number; max: number; step: number; onChange: (n: number) => void }) {
  return (
    <label className="space-y-0.5">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <input type="number" value={value} min={min} max={max} step={step}
        onChange={(e) => onChange(Number(e.target.value))}
        className="h-7 w-full rounded border border-border bg-secondary/30 px-2 text-xs tabular-nums" />
    </label>
  );
}

function SelectField({ label, value, options, onChange }:
  { label: string; value: string; options: readonly string[]; onChange: (v: string) => void }) {
  return (
    <label className="space-y-0.5">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <select value={value} onChange={(e) => onChange(e.target.value)}
        className="h-7 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </label>
  );
}

export function TimelineEditor() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const qc = useQueryClient();
  const { t } = useTranslation();
  const q = useQuery<RunView>({ queryKey: ["run", id], queryFn: () => api.getRun(id) });

  const [panels, setPanels] = useState<Panel[]>([]);
  const [fps, setFps] = useState(30);
  const [resolution, setResolution] = useState(RESOLUTION_PRESETS[0]);
  const [codec, setCodec] = useState<"h264" | "h265">("h264");
  const [submitErr, setSubmitErr] = useState<string | null>(null);
  const [selectedIdx, setSelectedIdx] = useState(0);

  const sourcePanels = useMemo(() => {
    if (!q.data) return [] as { index: number; objectKey: string; caption: string; assetId?: string; defaultDur: number }[];
    const imageStep = q.data.steps.find((s) => s.type === "image");
    const scriptStep = q.data.steps.find((s) => s.type === "script");
    const assemble = q.data.steps.find((s) => s.type === "assemble");
    const defaultDur = (assemble?.input as any)?.params?.panel_duration_ms ?? 2500;
    const scriptOut = Array.isArray(scriptStep?.outputs) ? (scriptStep!.outputs as any[]) : [];
    const captionByIdx = new Map<number, string>(scriptOut.map((p) => [Number(p.index), String(p.caption ?? "")]));
    const out = Array.isArray(imageStep?.outputs) ? (imageStep!.outputs as any[]) : [];
    return out.map((o) => {
      const idx = Number(o.index ?? 0);
      const asset = (q.data!.assets ?? []).find((a) => a.object_key === o.object_key);
      return { index: idx, objectKey: String(o.object_key ?? ""), caption: captionByIdx.get(idx) ?? "", assetId: asset?.id, defaultDur };
    });
  }, [q.data]);

  useEffect(() => {
    if (panels.length > 0 || sourcePanels.length === 0) return;
    setPanels(sourcePanels.map((p) => defaultPanel(p.index, p.defaultDur ?? 2500, p.caption)));
    const assemble = q.data?.steps.find((s) => s.type === "assemble");
    const input: any = assemble?.input ?? {};
    if (input.fps) setFps(Number(input.fps));
    if (input.width && input.height) {
      const match = RESOLUTION_PRESETS.find((p) => p.w === Number(input.width) && p.h === Number(input.height));
      if (match) setResolution(match);
    }
  }, [sourcePanels, panels.length, q.data]);

  const renderMut = useMutation({
    mutationFn: (mode: "preview" | "final") => {
      const isPreview = mode === "preview";
      // Preview shrinks the final dimensions ~half and forces h264 so the
      // user gets a draft in 1/3 the time before committing the full render.
      const w = isPreview ? Math.round(resolution.w / 2) : resolution.w;
      const h = isPreview ? Math.round(resolution.h / 2) : resolution.h;
      return api.requestAssemble(id, {
        params: {
          fps: isPreview ? Math.min(fps, 24) : fps,
          width: w, height: h,
          codec: isPreview ? "h264" : codec,
          timeline: { panels },
          preview: isPreview ? true : undefined,
        },
      } as any);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["run", id] });
      nav(`/runs/${id}`);
    },
    onError: (e: Error) => setSubmitErr(e.message),
  });

  const shift = (i: number, delta: number) => {
    setPanels((curr) => {
      const next = [...curr];
      const j = i + delta;
      if (j < 0 || j >= next.length) return curr;
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
    setSelectedIdx((s) => (s === i ? i + delta : s === i + delta ? i : s));
  };

  const totalMs = panels.reduce((sum, p) => sum + p.duration_ms, 0);
  const assetsByIdx = new Map(sourcePanels.map((p) => [p.index, p.assetId] as const));
  const selected = panels[selectedIdx];

  if (!q.data) return <p className="p-6 text-sm text-muted-foreground">{t("timeline.loadingRun")}</p>;
  if (sourcePanels.length === 0) return <p className="p-6 text-sm text-muted-foreground">{t("timeline.waitImageStep")}</p>;

  return (
    <div className="max-w-7xl mx-auto p-4 space-y-3">
      <Card>
        <CardHeader className="py-3">
          <div className="flex items-center justify-between gap-3">
            <CardTitle className="text-base truncate">
              {t("timeline.title")} — <span className="text-muted-foreground font-normal">{q.data.prompt.slice(0, 80)}</span>
            </CardTitle>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground tabular-nums whitespace-nowrap">
                {t("timeline.totalLabel", { seconds: (totalMs / 1000).toFixed(1), panels: panels.length })}
              </span>
              <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => nav(`/runs/${id}`)}>
                {t("common.back")}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3 pt-0">
          <div className="grid grid-cols-2 md:grid-cols-5 gap-2">
            <SelectField label={t("studio.fps")} value={String(fps)} options={FPS_PRESETS.map(String)}
              onChange={(v) => setFps(Number(v))} />
            <SelectField label={t("studio.resolution")} value={resolution.label}
              options={RESOLUTION_PRESETS.map((r) => r.label)}
              onChange={(v) => setResolution(RESOLUTION_PRESETS.find((r) => r.label === v) ?? RESOLUTION_PRESETS[0])} />
            <SelectField label="codec" value={codec} options={["h264", "h265"]}
              onChange={(v) => setCodec(v as "h264" | "h265")} />
            <Button
              variant="outline"
              className="h-9 mt-4 text-xs"
              disabled={renderMut.isPending || panels.length === 0}
              onClick={() => { setSubmitErr(null); renderMut.mutate("preview"); }}
              title={t("timeline.previewHint", "Quick low-res draft to verify the cut.")}
            >
              {renderMut.isPending && renderMut.variables === "preview" ? t("timeline.rendering") : t("timeline.preview", "Preview (draft)")}
            </Button>
            <Button
              className="h-9 mt-4 text-xs"
              disabled={renderMut.isPending || panels.length === 0}
              onClick={() => { setSubmitErr(null); renderMut.mutate("final"); }}
            >
              {renderMut.isPending && renderMut.variables === "final" ? t("timeline.rendering") : t("timeline.renderFinal", "Render final")}
            </Button>
          </div>
          {submitErr ? <p className="text-xs text-red-400">{submitErr}</p> : null}
        </CardContent>
      </Card>

      {/* TRACK */}
      <Card>
        <CardHeader className="py-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">{t("timeline.trackTitle", "Timeline")}</CardTitle>
            <Button variant="outline" className="h-6 px-2 text-[10px]"
              onClick={() => setPanels(sourcePanels.map((p) => defaultPanel(p.index, 2500, p.caption)))}>
              {t("timeline.reset")}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          <div className="overflow-x-auto">
            <div className="min-w-max">
              <TimeRuler totalMs={totalMs} />
              <div className="flex items-stretch mt-1">
                {panels.map((p, i) => {
                  const widthPx = (p.duration_ms / 1000) * PX_PER_SECOND;
                  return (
                    <Fragment key={`p-${i}-${p.image_panel_index}`}>
                      {i > 0 ? <TransitionSeam panel={p} /> : null}
                      <TrackTile
                        panel={p}
                        idx={i}
                        asset={(q.data!.assets ?? []).find((a) => a.id === assetsByIdx.get(p.image_panel_index))}
                        selected={selectedIdx === i}
                        widthPx={widthPx}
                        onSelect={() => setSelectedIdx(i)}
                        onShift={(d) => shift(i, d)}
                      />
                    </Fragment>
                  );
                })}
              </div>
            </div>
          </div>
          <p className="text-[10px] text-muted-foreground mt-2">
            {t("timeline.trackHint", "Each tile's width equals its duration. Coloured seams are transitions — click a tile to edit it below.")}
          </p>
        </CardContent>
      </Card>

      {/* INSPECTOR */}
      {selected ? (
        <Card>
          <CardContent className="pt-4">
            <PanelInspector
              panel={selected}
              sourceCount={panels.length}
              onChange={(next) => setPanels((curr) => curr.map((c, j) => (j === selectedIdx ? next : c)))}
              onRemove={() => {
                setPanels((curr) => curr.filter((_, j) => j !== selectedIdx));
                setSelectedIdx((s) => Math.max(0, Math.min(s, panels.length - 2)));
              }}
            />
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
