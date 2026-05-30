import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { api, type RunView, type AssetView } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

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

type TransitionType = "none" | "fade" | "crossfade" | "slide" | "push" | "zoom" | "wipe";
type EasingName = "linear" | "ease-in" | "ease-out" | "ease-in-out" | "cubic";
type Direction = "left" | "right" | "up" | "down";

const TRANSITIONS: TransitionType[] = ["none", "fade", "crossfade", "slide", "push", "zoom", "wipe"];
const EASINGS: EasingName[] = ["linear", "ease-in", "ease-out", "ease-in-out", "cubic"];
const DIRECTIONS: Direction[] = ["left", "right", "up", "down"];

import { RESOLUTION_PRESETS, FPS_PRESETS } from "@/lib/options";

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

function PanelThumb({ asset, label }: { asset?: AssetView; label: string }) {
  const url = useAssetURL(asset?.id);
  return (
    <div className="aspect-square w-full rounded border border-border overflow-hidden bg-secondary/30 relative">
      {url ? <img src={url} className="w-full h-full object-cover" /> : <div className="w-full h-full animate-pulse" />}
      <span className="absolute top-1 left-1 text-[10px] bg-black/60 text-white px-1 rounded">{label}</span>
    </div>
  );
}

function SortablePanel({ p, idx, asset, onChange, onRemove }:
  { p: Panel; idx: number; asset?: AssetView; onChange: (next: Panel) => void; onRemove: () => void }) {
  const { t } = useTranslation();
  const id = `panel-${idx}-${p.image_panel_index}`;
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const style = { transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.5 : 1 };
  return (
    <div ref={setNodeRef} style={style} className="rounded border border-border bg-card p-3 space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <button {...attributes} {...listeners} className="cursor-grab text-muted-foreground text-xs">⋮⋮</button>
          <Badge variant="info">{t("timeline.slot", { idx: idx + 1 })}</Badge>
          <span className="text-[10px] text-muted-foreground">{t("timeline.sourcePanel", { idx: p.image_panel_index })}</span>
        </div>
        <Button variant="outline" className="h-6 px-2 text-[10px]" onClick={onRemove}>{t("timeline.remove")}</Button>
      </div>
      <PanelThumb asset={asset} label={`#${p.image_panel_index}`} />
      <div className="grid grid-cols-2 gap-2 text-xs">
        <NumField label={t("timeline.durationMs")} value={p.duration_ms} min={500} max={20000} step={100}
          onChange={(v) => onChange({ ...p, duration_ms: v })} />
        <SelectField label={t("studio.transition")} value={p.transition_in.type} options={TRANSITIONS}
          onChange={(v) => onChange({ ...p, transition_in: { ...p.transition_in, type: v as TransitionType } })} />
        <NumField label={t("timeline.transDurationMs")} value={p.transition_in.duration_ms} min={0} max={2000} step={20}
          onChange={(v) => onChange({ ...p, transition_in: { ...p.transition_in, duration_ms: v } })} />
        <SelectField label={t("timeline.easing")} value={p.transition_in.easing} options={EASINGS}
          onChange={(v) => onChange({ ...p, transition_in: { ...p.transition_in, easing: v as EasingName } })} />
        <SelectField label={t("timeline.direction")} value={p.transition_in.direction} options={DIRECTIONS}
          onChange={(v) => onChange({ ...p, transition_in: { ...p.transition_in, direction: v as Direction } })} />
      </div>
      <details className="text-xs">
        <summary className="cursor-pointer text-muted-foreground">{t("timeline.kenBurns")}</summary>
        <div className="mt-1 grid grid-cols-2 gap-2">
          <NumField label={t("timeline.zoomStart")} value={p.effects[0]?.zoom_start ?? 1.0} min={0.5} max={2} step={0.01}
            onChange={(v) => onChange({ ...p, effects: [{ ...(p.effects[0] ?? defaultPanel(0,0,"").effects[0]), zoom_start: v }] })} />
          <NumField label={t("timeline.zoomEnd")} value={p.effects[0]?.zoom_end ?? 1.08} min={0.5} max={2} step={0.01}
            onChange={(v) => onChange({ ...p, effects: [{ ...(p.effects[0] ?? defaultPanel(0,0,"").effects[0]), zoom_end: v }] })} />
          <NumField label={t("timeline.panXEnd")} value={p.effects[0]?.pan_x_end ?? 0} min={-200} max={200} step={1}
            onChange={(v) => onChange({ ...p, effects: [{ ...(p.effects[0] ?? defaultPanel(0,0,"").effects[0]), pan_x_end: v }] })} />
          <NumField label={t("timeline.panYEnd")} value={p.effects[0]?.pan_y_end ?? -10} min={-200} max={200} step={1}
            onChange={(v) => onChange({ ...p, effects: [{ ...(p.effects[0] ?? defaultPanel(0,0,"").effects[0]), pan_y_end: v }] })} />
        </div>
      </details>
      <details className="text-xs">
        <summary className="cursor-pointer text-muted-foreground">{t("timeline.caption")}</summary>
        <div className="mt-1 space-y-1">
          <Textarea rows={2} value={p.caption.text}
            onChange={(e) => onChange({ ...p, caption: { ...p.caption, text: e.target.value } })}
            placeholder={t("timeline.captionEmpty")}
            className="font-mono text-[10px]" />
          <SelectField label={t("projectDefaults.position")} value={p.caption.position} options={["top","bottom","center"]}
            onChange={(v) => onChange({ ...p, caption: { ...p.caption, position: v as Panel["caption"]["position"] } })} />
        </div>
      </details>
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
  const q = useQuery<RunView>({ queryKey: ["run", id], queryFn: () => api.getRun(id) });

  const [panels, setPanels] = useState<Panel[]>([]);
  const [fps, setFps] = useState(30);
  const [resolution, setResolution] = useState(RESOLUTION_PRESETS[0]);
  const [codec, setCodec] = useState<"h264" | "h265">("h264");
  const [submitErr, setSubmitErr] = useState<string | null>(null);

  // Seed panel rows from run state on first load.
  const sourcePanels = useMemo(() => {
    if (!q.data) return [] as { index: number; objectKey: string; caption: string; assetId?: string }[];
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
    setPanels(sourcePanels.map((p) => defaultPanel(p.index, (p as any).defaultDur ?? 2500, p.caption)));
    const assemble = q.data?.steps.find((s) => s.type === "assemble");
    const input: any = assemble?.input ?? {};
    if (input.fps) setFps(Number(input.fps));
    if (input.width && input.height) {
      const match = RESOLUTION_PRESETS.find((p) => p.w === Number(input.width) && p.h === Number(input.height));
      if (match) setResolution(match);
    }
  }, [sourcePanels, panels.length, q.data]);

  const submit = useMutation({
    mutationFn: () => {
      const body = {
        params: {
          fps,
          width: resolution.w,
          height: resolution.h,
          codec,
          timeline: { panels },
        },
      };
      return api.requestAssemble(id, body as any);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["run", id] });
      nav(`/runs/${id}`);
    },
    onError: (e: Error) => setSubmitErr(e.message),
  });

  const sensors = useSensors(useSensor(PointerSensor), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }));
  const onDragEnd = (e: DragEndEvent) => {
    if (!e.over || e.active.id === e.over.id) return;
    setPanels((curr) => {
      const oldIdx = curr.findIndex((p, i) => `panel-${i}-${p.image_panel_index}` === e.active.id);
      const newIdx = curr.findIndex((p, i) => `panel-${i}-${p.image_panel_index}` === e.over!.id);
      if (oldIdx < 0 || newIdx < 0) return curr;
      return arrayMove(curr, oldIdx, newIdx);
    });
  };

  const totalDur = panels.reduce((sum, p) => sum + p.duration_ms, 0);
  const items = panels.map((p, i) => `panel-${i}-${p.image_panel_index}`);
  const assetsByIdx = new Map(sourcePanels.map((p) => [p.index, p.assetId] as const));

  const tle = useTranslation().t;
  if (!q.data) return <p className="p-6 text-sm text-muted-foreground">{tle("timeline.loadingRun")}</p>;
  if (sourcePanels.length === 0) return <p className="p-6 text-sm text-muted-foreground">{tle("timeline.waitImageStep")}</p>;

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-4">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>{tle("timeline.title")} — {q.data.prompt.slice(0, 60)}…</CardTitle>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground tabular-nums">
                {tle("timeline.totalLabel", { seconds: (totalDur / 1000).toFixed(1), panels: panels.length })}
              </span>
              <Button variant="outline" className="h-7 px-2 text-xs" onClick={() => nav(`/runs/${id}`)}>{tle("common.back")}</Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-4 gap-2 text-xs">
            <SelectField label={tle("studio.fps")} value={String(fps)} options={FPS_PRESETS.map(String)} onChange={(v) => setFps(Number(v))} />
            <SelectField
              label={tle("studio.resolution")}
              value={resolution.label}
              options={RESOLUTION_PRESETS.map((r) => r.label)}
              onChange={(v) => setResolution(RESOLUTION_PRESETS.find((r) => r.label === v) ?? RESOLUTION_PRESETS[0])}
            />
            <SelectField label="codec" value={codec} options={["h264", "h265"]} onChange={(v) => setCodec(v as "h264" | "h265")} />
            <div className="flex items-end justify-end">
              <Button
                className="h-8 px-3 text-xs"
                disabled={submit.isPending || panels.length === 0}
                onClick={() => { setSubmitErr(null); submit.mutate(); }}
              >
                {submit.isPending ? tle("timeline.rendering") : tle("timeline.renderWithTimeline")}
              </Button>
            </div>
          </div>
          {submitErr ? <p className="text-xs text-red-400">{submitErr}</p> : null}
          <div className="flex items-center justify-between">
            <p className="text-xs text-muted-foreground">{tle("timeline.dragHint")}</p>
            <Button
              variant="outline"
              className="h-7 px-2 text-xs"
              onClick={() => setPanels(sourcePanels.map((p) => defaultPanel(p.index, 2500, p.caption)))}
            >
              {tle("timeline.reset")}
            </Button>
          </div>
        </CardContent>
      </Card>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={items} strategy={rectSortingStrategy}>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
            {panels.map((p, i) => (
              <SortablePanel
                key={`${i}-${p.image_panel_index}`}
                p={p}
                idx={i}
                asset={(q.data!.assets ?? []).find((a) => a.id === assetsByIdx.get(p.image_panel_index))}
                onChange={(next) => setPanels((curr) => curr.map((c, j) => (j === i ? next : c)))}
                onRemove={() => setPanels((curr) => curr.filter((_, j) => j !== i))}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>
    </div>
  );
}
