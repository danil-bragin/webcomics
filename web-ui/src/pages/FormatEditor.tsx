import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type FormatRow } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";

const TRANSITIONS = ["none", "crossfade", "fade", "slide", "wipe"];
const STYLE_REFS = ["none", "anchor", "previous"];
const RESOLUTIONS = [
  { label: "Square 1080×1080",       w: 1080, h: 1080 },
  { label: "Portrait 1080×1920 (Shorts)", w: 1080, h: 1920 },
  { label: "Landscape 1920×1080",    w: 1920, h: 1080 },
  { label: "Landscape 1440×1080",    w: 1440, h: 1080 },
  { label: "Square 4K 2160×2160",    w: 2160, h: 2160 },
];

type Form = Omit<FormatRow, "is_system" | "created_at" | "updated_at"> & { is_system?: boolean };

const EMPTY: Form = {
  id: "",
  name: "",
  description: "",
  scope: "user",
  icon: "🎨",
  script_system_suffix: "",
  image_prompt_prefix: "",
  image_prompt_suffix: "",
  image_model: "fal-ai/flux/schnell",
  style_reference: "none",
  fps: 30,
  width: 1080,
  height: 1080,
  codec: "h264",
  panel_duration_ms: 2500,
  transition: "crossfade",
};

export function FormatEditor() {
  const { t } = useTranslation();
  const { id } = useParams();
  const nav = useNavigate();
  const isNew = !id || id === "new";

  const q = useQuery({
    queryKey: ["format", id],
    queryFn: () => api.getFormat(id!),
    enabled: !isNew,
  });

  const [form, setForm] = useState<Form>(EMPTY);
  const [sampleText, setSampleText] = useState("a hipster cat opens a coffee shop on Mars");
  const [composed, setComposed] = useState("");
  const [forked, setForked] = useState(false);

  useEffect(() => {
    if (!q.data) return;
    const f = q.data as FormatRow;
    // If loading a system format, switch into "fork mode" so save creates a
    // user-scoped copy instead of overwriting the system row.
    if (f.is_system) {
      setForked(true);
      setForm({ ...f, scope: "user", id: f.id + "-fork" });
    } else {
      setForm({ ...f });
    }
  }, [q.data]);

  // Recompute the static preview on prompt/prefix/suffix change.
  useEffect(() => {
    setComposed(((form.image_prompt_prefix ?? "") + sampleText + (form.image_prompt_suffix ?? "")).trim());
  }, [form.image_prompt_prefix, form.image_prompt_suffix, sampleText]);

  const save = useMutation({
    mutationFn: async () => {
      const body = { ...form };
      if (isNew || forked) {
        const r = await api.createFormat(body);
        return r.id;
      }
      await api.updateFormat(id!, body);
      return id!;
    },
    onSuccess: () => nav("/formats"),
  });

  const isSystemReadOnly = q.data?.is_system && !forked;

  return (
    <div className="max-w-5xl mx-auto p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          {isNew ? t("formats.editor.newTitle") : forked ? t("formats.editor.forkTitle") : t("formats.editor.editTitle")}
        </h1>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => nav("/formats")}>{t("common.cancel")}</Button>
          <Button disabled={!form.name || save.isPending || isSystemReadOnly} onClick={() => save.mutate()}>
            {save.isPending ? t("projectDefaults.saving") : forked ? t("formats.editor.saveAsFork") : t("common.save")}
          </Button>
        </div>
      </div>

      {q.data?.is_system && !forked ? (
        <div className="rounded border border-amber-500/30 bg-amber-500/10 p-3 text-sm">
          {t("formats.editor.systemNotice")}{" "}
          <button onClick={() => {
            setForked(true);
            setForm({ ...form, scope: "user", id: form.id + "-fork" });
          }} className="underline">
            {t("formats.editor.forkNow")} →
          </button>
        </div>
      ) : null}

      {/* Identity */}
      <Card>
        <CardHeader><CardTitle>{t("formats.editor.identity")}</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <div className="grid grid-cols-[80px_1fr_160px] gap-2">
            <Field label={t("presets.editor.icon")}>
              <input value={form.icon ?? ""} onChange={(e) => setForm({ ...form, icon: e.target.value })}
                disabled={!!isSystemReadOnly}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-2xl text-center" />
            </Field>
            <Field label={t("common.name")}>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="my-format"
                disabled={!!isSystemReadOnly}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
            <Field label="ID">
              <input value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })}
                disabled={!isNew && !forked}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
          </div>
          <Field label={t("common.description")}>
            <Textarea rows={2} value={form.description}
              disabled={!!isSystemReadOnly}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder={t("formats.editor.descriptionPlaceholder")} />
          </Field>
        </CardContent>
      </Card>

      {/* Render dims */}
      <Card>
        <CardHeader><CardTitle>{t("formats.editor.renderSection")}</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <div className="grid grid-cols-3 gap-2">
            <Field label={t("formats.editor.aspect")}>
              <select
                disabled={!!isSystemReadOnly}
                value={`${form.width}x${form.height}`}
                onChange={(e) => {
                  const [w, h] = e.target.value.split("x").map(Number);
                  setForm({ ...form, width: w, height: h });
                }}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm"
              >
                {RESOLUTIONS.map((r) => (
                  <option key={r.label} value={`${r.w}x${r.h}`}>{r.label}</option>
                ))}
              </select>
            </Field>
            <Field label={t("studio.fps")}>
              <select value={String(form.fps ?? 30)} onChange={(e) => setForm({ ...form, fps: Number(e.target.value) })}
                disabled={!!isSystemReadOnly}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
                <option value="24">24</option>
                <option value="30">30</option>
                <option value="60">60</option>
              </select>
            </Field>
            <Field label="codec">
              <select value={form.codec} onChange={(e) => setForm({ ...form, codec: e.target.value })}
                disabled={!!isSystemReadOnly}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
                <option value="h264">h264</option>
                <option value="h265">h265</option>
              </select>
            </Field>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <Field label={t("studio.panelDurationMs")}>
              <input type="number" value={form.panel_duration_ms ?? 2500}
                disabled={!!isSystemReadOnly}
                onChange={(e) => setForm({ ...form, panel_duration_ms: Number(e.target.value) })}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
            <Field label={t("studio.transition")}>
              <select value={form.transition} onChange={(e) => setForm({ ...form, transition: e.target.value })}
                disabled={!!isSystemReadOnly}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
                {TRANSITIONS.map((t) => <option key={t} value={t}>{t}</option>)}
              </select>
            </Field>
          </div>
        </CardContent>
      </Card>

      {/* Image influence */}
      <Card>
        <CardHeader><CardTitle>{t("formats.editor.imageInfluence")}</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <p className="text-xs text-muted-foreground">{t("formats.editor.imageInfluenceHint")}</p>
          <Field label={t("formats.editor.prefix")}>
            <Textarea rows={2} value={form.image_prompt_prefix ?? ""}
              disabled={!!isSystemReadOnly}
              onChange={(e) => setForm({ ...form, image_prompt_prefix: e.target.value })}
              placeholder="manga style, black ink linework, ..." className="font-mono text-xs" />
          </Field>
          <Field label={t("formats.editor.suffix")}>
            <Textarea rows={2} value={form.image_prompt_suffix ?? ""}
              disabled={!!isSystemReadOnly}
              onChange={(e) => setForm({ ...form, image_prompt_suffix: e.target.value })}
              placeholder=", traditional manga panel composition" className="font-mono text-xs" />
          </Field>
          <div className="grid grid-cols-2 gap-2">
            <Field label={t("studio.imageModel")}>
              <input value={form.image_model ?? ""}
                disabled={!!isSystemReadOnly}
                onChange={(e) => setForm({ ...form, image_model: e.target.value })}
                placeholder="fal-ai/flux/schnell"
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
            </Field>
            <Field label={t("studio.styleConsistency")}>
              <select value={form.style_reference ?? "none"}
                disabled={!!isSystemReadOnly}
                onChange={(e) => setForm({ ...form, style_reference: e.target.value })}
                className="h-9 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
                {STYLE_REFS.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
            </Field>
          </div>

          {/* Static preview */}
          <div className="rounded border border-border bg-secondary/10 p-2 mt-2 space-y-1">
            <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("formats.editor.preview")}</p>
            <input value={sampleText} onChange={(e) => setSampleText(e.target.value)}
              placeholder={t("formats.editor.samplePromptPlaceholder")}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs" />
            <p className="font-mono text-xs text-muted-foreground italic line-clamp-3">
              → {composed || t("formats.editor.composedEmpty")}
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Script influence */}
      <Card>
        <CardHeader><CardTitle>{t("formats.editor.scriptInfluence")}</CardTitle></CardHeader>
        <CardContent>
          <p className="text-xs text-muted-foreground mb-2">{t("formats.editor.scriptInfluenceHint")}</p>
          <Textarea rows={3} value={form.script_system_suffix ?? ""}
            disabled={!!isSystemReadOnly}
            onChange={(e) => setForm({ ...form, script_system_suffix: e.target.value })}
            placeholder="Use snappy dialogue and Japanese-style sound effects..."
            className="font-mono text-xs" />
        </CardContent>
      </Card>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1 block">{label}</span>
      {children}
    </label>
  );
}
