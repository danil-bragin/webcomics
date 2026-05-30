import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type PresetView, type PresetCategory } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

const CATEGORIES: { id: PresetCategory | "all"; icon: string; key: string }[] = [
  { id: "all",    icon: "✦", key: "presets.cat.all" },
  { id: "meme",   icon: "😂", key: "presets.cat.meme" },
  { id: "shorts", icon: "📱", key: "presets.cat.shorts" },
  { id: "story",  icon: "📖", key: "presets.cat.story" },
  { id: "demo",   icon: "🤫", key: "presets.cat.demo" },
  { id: "custom", icon: "⚙",  key: "presets.cat.custom" },
];

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
  const [cat, setCat] = useState<PresetCategory | "all">("all");

  const q = useQuery({
    queryKey: ["presets", cat],
    queryFn: () => api.listPresets({ category: cat === "all" ? undefined : cat }),
  });

  const del = useMutation({
    mutationFn: (id: string) => api.deletePreset(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["presets"] }),
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

      {q.isLoading && <p className="text-sm text-muted-foreground">{t("common.loading")}</p>}

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
    <Card className="flex flex-col">
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <span className="text-2xl leading-none">{preset.icon || "📄"}</span>
            <span className="truncate">{preset.name}</span>
          </CardTitle>
          {preset.category ? (
            <Badge variant="info" className="text-[10px] capitalize">{preset.category}</Badge>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="flex-1 flex flex-col gap-3">
        {preset.description ? (
          <p className="text-xs text-muted-foreground line-clamp-3">{preset.description}</p>
        ) : null}

        {/* Step chain — visual pipeline glyphs */}
        <div className="flex items-center gap-1 flex-wrap text-base" title={steps.map(s => s.type).join(" → ")}>
          {steps.map((s, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="rounded bg-secondary/40 w-7 h-7 inline-flex items-center justify-center" title={s.type}>
                {STEP_ICON[s.type] || "·"}
              </span>
              {i < steps.length - 1 ? <span className="text-muted-foreground text-xs">→</span> : null}
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

        <div className="mt-auto pt-2 flex items-center gap-2 justify-between text-[11px] text-muted-foreground">
          <div className="flex items-center gap-2">
            <span title={t("presets.maxCost")}>${preset.max_cost_usd.toFixed(2)}</span>
            <span>·</span>
            <span>{t("presets.stepsCount", { count: steps.length })}</span>
          </div>
          <div className="flex items-center gap-1">
            <Link to={`/presets/${preset.id}/edit`}
              className="text-xs px-2 py-1 rounded border border-border hover:bg-secondary/40">
              {t("common.edit")}
            </Link>
            <button
              onClick={() => {
                if (confirm(t("presets.confirmDelete", { name: preset.name }))) onDelete();
              }}
              className="text-xs px-2 py-1 rounded border border-border text-red-400 hover:bg-secondary/40"
            >
              ✕
            </button>
            <Link to={`/?preset=${preset.id}`}
              className="text-xs px-3 py-1 rounded bg-primary text-primary-foreground">
              {t("presets.use")} →
            </Link>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
