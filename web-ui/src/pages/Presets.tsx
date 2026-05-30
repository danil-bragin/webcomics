import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type PresetView, type PresetCategory } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { CardSkeletonGrid } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast";

const CATEGORIES: { id: PresetCategory | "all"; icon: string; key: string }[] = [
  { id: "all",    icon: "✦", key: "presets.cat.all" },
  { id: "meme",   icon: "😂", key: "presets.cat.meme" },
  { id: "shorts", icon: "📱", key: "presets.cat.shorts" },
  { id: "story",  icon: "📖", key: "presets.cat.story" },
  { id: "demo",   icon: "🤫", key: "presets.cat.demo" },
  { id: "custom", icon: "⚙",  key: "presets.cat.custom" },
];

// Per-category accent color for the card top stripe — fast visual scan.
const CATEGORY_COLOR: Record<string, string> = {
  meme:   "bg-amber-400",
  shorts: "bg-pink-500",
  story:  "bg-emerald-500",
  demo:   "bg-sky-400",
  custom: "bg-zinc-500",
};

// Step glyphs render as a visual pipeline chain on each card so the user can
// glance "this one has voice + music" without reading JSON.
const STEP_ICON: Record<string, string> = {
  script: "📝",
  image: "🖼",
  audio: "🎙",
  music: "🎵",
  caption: "📰",
  assemble: "🎬",
  upload: "☁",
};

export function Presets() {
  const { t } = useTranslation();
  const nav = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const [cat, setCat] = useState<PresetCategory | "all">("all");

  const q = useQuery({
    queryKey: ["presets", cat],
    queryFn: () => api.listPresets({ category: cat === "all" ? undefined : cat }),
  });

  const del = useMutation({
    mutationFn: (id: string) => api.deletePreset(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["presets"] });
      toast.push("success", t("presets.deleted", "Preset deleted"));
    },
    onError: (e: Error) => toast.push("error", e.message),
  });

  const presets = q.data ?? [];

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">{t("presets.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("presets.subtitle")}</p>
        </div>
        <Button onClick={() => nav("/presets/new")}>+ {t("presets.newPreset")}</Button>
      </div>

      <div className="flex gap-2 border-b border-border overflow-x-auto">
        {CATEGORIES.map((c) => (
          <button
            key={c.id}
            onClick={() => setCat(c.id)}
            className={`px-4 py-2 text-sm border-b-2 -mb-px whitespace-nowrap flex items-center gap-1 ${
              cat === c.id
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <span>{c.icon}</span> {t(c.key)}
          </button>
        ))}
      </div>

      {q.isLoading && <CardSkeletonGrid count={6} cols={3} />}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {presets.map((p) => (
          <PresetCard key={p.id} preset={p} onDelete={() => del.mutate(p.id)} />
        ))}
      </div>

      {!q.isLoading && presets.length === 0 ? (
        <p className="text-sm text-muted-foreground py-8 text-center">{t("presets.empty")}</p>
      ) : null}
    </div>
  );
}

function PresetCard({ preset, onDelete }: { preset: PresetView; onDelete: () => void }) {
  const { t } = useTranslation();
  const steps = (preset.steps ?? []) as { type: string }[];

  return (
    <Card className="flex flex-col group relative overflow-hidden">
      {/* Subtle category accent stripe top */}
      {preset.category ? (
        <div className={`h-0.5 w-full ${CATEGORY_COLOR[preset.category] ?? "bg-secondary"}`} />
      ) : null}

      <CardHeader className="pb-2">
        <div className="flex items-start gap-3">
          <span className="text-3xl leading-none shrink-0">{preset.icon || "📄"}</span>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2">
              <CardTitle className="truncate text-base">{preset.name}</CardTitle>
              {preset.category ? (
                <span className="text-[10px] uppercase tracking-wide text-muted-foreground shrink-0">
                  {preset.category}
                </span>
              ) : null}
            </div>
            <div className="text-[11px] text-muted-foreground mt-0.5">
              {t("presets.stepsCount", { count: steps.length })}
              {preset.max_cost_usd > 0 ? <> · ≤ ${preset.max_cost_usd.toFixed(2)}</> : null}
            </div>
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex-1 flex flex-col gap-3 pb-0">
        {preset.description ? (
          <p className="text-xs text-muted-foreground line-clamp-3 min-h-[3em]">{preset.description}</p>
        ) : (
          <p className="text-xs italic opacity-50 min-h-[3em]">{t("presets.noDescription")}</p>
        )}

        {/* Step chain — visual pipeline glyphs */}
        <div className="flex items-center gap-1 flex-wrap" title={steps.map(s => s.type).join(" → ")}>
          {steps.map((s, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="rounded bg-secondary/40 w-7 h-7 inline-flex items-center justify-center" title={s.type}>
                {STEP_ICON[s.type] || "·"}
              </span>
              {i < steps.length - 1 ? <span className="text-muted-foreground/60 text-xs">→</span> : null}
            </span>
          ))}
        </div>

        {preset.sample_prompts && preset.sample_prompts.length > 0 ? (
          <div className="space-y-1">
            <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{t("presets.samplePrompts")}</p>
            <ul className="space-y-0.5">
              {preset.sample_prompts.slice(0, 3).map((s, i) => (
                <li key={i} className="text-xs text-muted-foreground italic line-clamp-1">— {s}</li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>

      {/* Primary action — full-width Use button as CTA */}
      <div className="px-6 pb-3 pt-3 mt-auto">
        <Link
          to={`/?preset=${preset.id}`}
          className="block w-full text-center text-sm font-medium px-3 py-2 rounded bg-primary text-primary-foreground hover:brightness-110"
        >
          {t("presets.use")} →
        </Link>
      </div>

      {/* Secondary actions reveal on hover so the card looks clean by default */}
      <div className="absolute top-2 right-2 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <Link
          to={`/presets/${preset.id}/edit`}
          title={t("common.edit")}
          aria-label={t("common.edit")}
          className="w-7 h-7 inline-flex items-center justify-center rounded bg-background/80 border border-border text-xs hover:bg-secondary/60"
        >
          ✎
        </Link>
        <button
          onClick={() => {
            if (confirm(t("presets.confirmDelete", { name: preset.name }))) onDelete();
          }}
          title={t("common.delete")}
          aria-label={t("common.delete")}
          className="w-7 h-7 inline-flex items-center justify-center rounded bg-background/80 border border-border text-xs text-red-400 hover:bg-red-500/20"
        >
          ✕
        </button>
      </div>
    </Card>
  );
}
