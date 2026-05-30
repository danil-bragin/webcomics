import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type PresetView, type PresetCategory } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";

// Visual preset builder. Each step is a typed block with a per-step form so
// users can drag-add/remove without touching JSON. Falls back to a JSON
// inspector for power users who want the raw output.

type StepType = "script" | "image" | "audio" | "music" | "caption" | "assemble" | "upload";

const STEP_TYPES: { type: StepType; icon: string; label: string; help: string }[] = [
  { type: "script",   icon: "📝", label: "Script",   help: "LLM panel-by-panel script" },
  { type: "image",    icon: "🖼", label: "Images",   help: "Render each panel as an image" },
  { type: "audio",    icon: "🎙", label: "Voice",    help: "TTS voiceover of captions" },
  { type: "music",    icon: "🎵", label: "Music",    help: "Pick a background track" },
  { type: "caption",  icon: "📰", label: "Caption",  help: "Social-post metadata generation" },
  { type: "assemble", icon: "🎬", label: "Assemble", help: "Stitch panels + audio into MP4" },
  { type: "upload",   icon: "☁",  label: "Upload",  help: "Push to YouTube / social platform" },
];

type StepConfig = {
  type: StepType;
  system_prompt?: string;
  model?: string;
  provider?: string;
  params?: Record<string, unknown>;
};

type PresetForm = {
  name: string;
  description: string;
  category: PresetCategory;
  icon: string;
  steps: StepConfig[];
  sample_prompts: string[];
  format_id: string;
  defaults: Record<string, unknown>;
  max_cost_usd: number;
};

const EMPTY: PresetForm = {
  name: "",
  description: "",
  category: "custom",
  icon: "✨",
  steps: [
    { type: "script", params: { panel_count: 3 } },
    { type: "image" },
    { type: "assemble", params: { width: 1080, height: 1080, fps: 30, panel_duration_ms: 2500, transition: "crossfade" } },
  ],
  sample_prompts: [""],
  format_id: "",
  defaults: {},
  max_cost_usd: 0.10,
};

export function PresetEditor() {
  const { t } = useTranslation();
  const { id } = useParams();
  const nav = useNavigate();
  const isNew = !id || id === "new";

  const q = useQuery({
    queryKey: ["preset", id],
    queryFn: () => api.getPreset(id!),
    enabled: !isNew,
  });

  const [form, setForm] = useState<PresetForm>(EMPTY);

  useEffect(() => {
    if (!q.data) return;
    setForm(presetToForm(q.data));
  }, [q.data]);

  const save = useMutation({
    mutationFn: async () => {
      const body = {
        name: form.name,
        description: form.description,
        category: form.category,
        icon: form.icon,
        // OpenAPI codegen StepConfig type is stale (missing "caption" + "music"
        // alongside other domain step types). Cast through any until we re-gen.
        steps: form.steps as any,
        sample_prompts: form.sample_prompts.filter((s) => s.trim() !== ""),
        format_id: form.format_id || undefined,
        defaults: form.defaults,
        max_cost_usd: form.max_cost_usd,
      };
      if (isNew) {
        const r = await api.createPreset(body);
        return r.id;
      }
      await api.updatePreset(id!, body);
      return id!;
    },
    onSuccess: () => nav("/presets"),
  });

  return (
    <div className="max-w-5xl mx-auto p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          {isNew ? t("presets.editor.newTitle") : t("presets.editor.editTitle")}
        </h1>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => nav("/presets")}>{t("common.cancel")}</Button>
          <Button disabled={!form.name || save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? t("projectDefaults.saving") : t("common.save")}
          </Button>
        </div>
      </div>

      {/* Identity */}
      <Card>
        <CardHeader><CardTitle>{t("presets.editor.identity")}</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <div className="grid grid-cols-[80px_1fr_160px] gap-2">
            <Field label={t("presets.editor.icon")}>
              <input value={form.icon} onChange={(e) => setForm({ ...form, icon: e.target.value })}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-2xl text-center" />
            </Field>
            <Field label={t("common.name")}>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="my-shorts-preset"
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
            <Field label={t("presets.editor.category")}>
              <select value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value as PresetCategory })}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
                <option value="custom">custom</option>
                <option value="meme">meme</option>
                <option value="shorts">shorts</option>
                <option value="story">story</option>
                <option value="demo">demo</option>
              </select>
            </Field>
          </div>
          <Field label={t("common.description")}>
            <Textarea rows={2} value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder={t("presets.editor.descriptionPlaceholder")} />
          </Field>
          <div className="grid grid-cols-2 gap-2">
            <Field label={t("presets.editor.maxCost")}>
              <input type="number" step="0.01" min={0} value={form.max_cost_usd}
                onChange={(e) => setForm({ ...form, max_cost_usd: Number(e.target.value) })}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
            <Field label={t("presets.editor.formatId")}>
              <input value={form.format_id} onChange={(e) => setForm({ ...form, format_id: e.target.value })}
                placeholder={t("common.none")}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
          </div>
        </CardContent>
      </Card>

      {/* Pipeline steps */}
      <Card>
        <CardHeader>
          <CardTitle>{t("presets.editor.pipeline")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {form.steps.map((s, i) => (
            <StepBlock key={i} idx={i} step={s} total={form.steps.length}
              onUpdate={(next) => setForm({ ...form, steps: form.steps.map((x, j) => j === i ? next : x) })}
              onRemove={() => setForm({ ...form, steps: form.steps.filter((_, j) => j !== i) })}
              onMove={(dir) => {
                const idx2 = i + dir;
                if (idx2 < 0 || idx2 >= form.steps.length) return;
                const next = [...form.steps];
                [next[i], next[idx2]] = [next[idx2], next[i]];
                setForm({ ...form, steps: next });
              }}
            />
          ))}
          <div className="flex flex-wrap gap-2 pt-2 border-t border-border">
            <span className="text-xs text-muted-foreground mr-2 self-center">+ {t("presets.editor.addStep")}:</span>
            {STEP_TYPES.map((st) => (
              <button key={st.type}
                onClick={() => setForm({ ...form, steps: [...form.steps, { type: st.type }] })}
                className="text-xs px-2 py-1 rounded border border-border bg-secondary/30 hover:bg-secondary/60 flex items-center gap-1"
                title={st.help}
              >
                <span>{st.icon}</span> {st.label}
              </button>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Sample prompts */}
      <Card>
        <CardHeader>
          <CardTitle>{t("presets.editor.samplePrompts")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <p className="text-xs text-muted-foreground">{t("presets.editor.samplePromptsHint")}</p>
          {form.sample_prompts.map((s, i) => (
            <div key={i} className="flex gap-2">
              <input value={s}
                onChange={(e) => setForm({ ...form, sample_prompts: form.sample_prompts.map((x, j) => j === i ? e.target.value : x) })}
                placeholder={t("presets.editor.samplePlaceholder")}
                className="h-8 flex-1 rounded border border-border bg-secondary/30 px-2 text-sm" />
              <button onClick={() => setForm({ ...form, sample_prompts: form.sample_prompts.filter((_, j) => j !== i) })}
                className="text-xs px-2 rounded border border-border text-red-400">✕</button>
            </div>
          ))}
          <Button variant="outline" onClick={() => setForm({ ...form, sample_prompts: [...form.sample_prompts, ""] })}>
            + {t("presets.editor.addSample")}
          </Button>
        </CardContent>
      </Card>

      {/* Raw JSON inspector */}
      <details>
        <summary className="cursor-pointer text-sm text-muted-foreground">
          {t("presets.editor.rawJson")}
        </summary>
        <pre className="mt-2 text-[11px] bg-secondary/20 p-2 rounded max-h-64 overflow-auto">
          {JSON.stringify(form, null, 2)}
        </pre>
      </details>
    </div>
  );
}

function StepBlock({ idx, step, total, onUpdate, onRemove, onMove }: {
  idx: number;
  step: StepConfig;
  total: number;
  onUpdate: (next: StepConfig) => void;
  onRemove: () => void;
  onMove: (dir: -1 | 1) => void;
}) {
  const meta = STEP_TYPES.find((x) => x.type === step.type) ?? STEP_TYPES[0];
  const params = step.params ?? {};
  const setParam = (k: string, v: unknown) => {
    const next = { ...params };
    if (v === "" || v === undefined || v === null) delete next[k]; else next[k] = v;
    onUpdate({ ...step, params: next });
  };
  return (
    <div className="rounded border border-border p-3 space-y-2 bg-secondary/10">
      <div className="flex items-center gap-2">
        <span className="text-2xl">{meta.icon}</span>
        <div className="flex-1">
          <div className="font-medium text-sm">#{idx + 1} {meta.label}</div>
          <div className="text-[11px] text-muted-foreground">{meta.help}</div>
        </div>
        <button onClick={() => onMove(-1)} disabled={idx === 0}
          className="text-xs px-2 rounded border border-border disabled:opacity-40">▲</button>
        <button onClick={() => onMove(1)} disabled={idx === total - 1}
          className="text-xs px-2 rounded border border-border disabled:opacity-40">▼</button>
        <button onClick={onRemove}
          className="text-xs px-2 rounded border border-border text-red-400">✕</button>
      </div>

      <div className="grid grid-cols-2 gap-2 text-xs">
        {step.type === "script" || step.type === "image" || step.type === "audio" || step.type === "caption" || step.type === "music" ? (
          <Field label="model">
            <input value={step.model ?? ""} onChange={(e) => onUpdate({ ...step, model: e.target.value || undefined })}
              placeholder={defaultModel(step.type)}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2" />
          </Field>
        ) : null}
        {step.type === "script" ? (
          <Field label="panel_count">
            <input type="number" min={1} max={20} value={(params.panel_count as number) ?? ""}
              onChange={(e) => setParam("panel_count", e.target.value ? Number(e.target.value) : undefined)}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2" />
          </Field>
        ) : null}
        {step.type === "script" ? (
          <Field label="system_prompt" full>
            <Textarea rows={2} value={step.system_prompt ?? ""}
              onChange={(e) => onUpdate({ ...step, system_prompt: e.target.value || undefined })}
              placeholder="(optional)" />
          </Field>
        ) : null}
        {step.type === "image" ? (
          <Field label="style_reference">
            <select value={(params.style_reference as string) ?? "none"}
              onChange={(e) => setParam("style_reference", e.target.value)}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2">
              <option value="none">none</option>
              <option value="anchor">anchor</option>
              <option value="previous">previous</option>
            </select>
          </Field>
        ) : null}
        {step.type === "assemble" ? (
          <>
            <Field label="width">
              <input type="number" value={(params.width as number) ?? ""}
                onChange={(e) => setParam("width", e.target.value ? Number(e.target.value) : undefined)}
                className="h-8 w-full rounded border border-border bg-secondary/30 px-2" />
            </Field>
            <Field label="height">
              <input type="number" value={(params.height as number) ?? ""}
                onChange={(e) => setParam("height", e.target.value ? Number(e.target.value) : undefined)}
                className="h-8 w-full rounded border border-border bg-secondary/30 px-2" />
            </Field>
            <Field label="fps">
              <input type="number" value={(params.fps as number) ?? ""}
                onChange={(e) => setParam("fps", e.target.value ? Number(e.target.value) : undefined)}
                className="h-8 w-full rounded border border-border bg-secondary/30 px-2" />
            </Field>
            <Field label="panel_duration_ms">
              <input type="number" value={(params.panel_duration_ms as number) ?? ""}
                onChange={(e) => setParam("panel_duration_ms", e.target.value ? Number(e.target.value) : undefined)}
                className="h-8 w-full rounded border border-border bg-secondary/30 px-2" />
            </Field>
            <Field label="transition">
              <select value={(params.transition as string) ?? "crossfade"}
                onChange={(e) => setParam("transition", e.target.value)}
                className="h-8 w-full rounded border border-border bg-secondary/30 px-2">
                <option value="none">none</option>
                <option value="crossfade">crossfade</option>
                <option value="fade">fade</option>
                <option value="slide">slide</option>
              </select>
            </Field>
          </>
        ) : null}
      </div>
    </div>
  );
}

function Field({ label, full, children }: { label: string; full?: boolean; children: React.ReactNode }) {
  return (
    <label className={`block ${full ? "col-span-2" : ""}`}>
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1 block">{label}</span>
      {children}
    </label>
  );
}

function defaultModel(t: string): string {
  switch (t) {
    case "script": return "openai/gpt-4o-mini";
    case "image": return "fal-ai/flux/schnell";
    case "audio": return "eleven_flash_v2_5";
    default: return "";
  }
}

function presetToForm(p: PresetView): PresetForm {
  return {
    name: p.name,
    description: p.description ?? "",
    category: (p.category ?? "custom") as PresetCategory,
    icon: p.icon ?? "✨",
    steps: (p.steps ?? []) as StepConfig[],
    sample_prompts: p.sample_prompts ?? [""],
    format_id: p.format_id ?? "",
    defaults: (p.defaults ?? {}) as Record<string, unknown>,
    max_cost_usd: p.max_cost_usd ?? 0,
  };
}
