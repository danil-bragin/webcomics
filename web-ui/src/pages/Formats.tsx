import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type FormatRow } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { CardSkeletonGrid } from "@/components/ui/skeleton";

// Scope filter tabs.
const SCOPES: { id: "all" | "system" | "user"; key: string }[] = [
  { id: "all",    key: "formats.scope.all" },
  { id: "system", key: "formats.scope.system" },
  { id: "user",   key: "formats.scope.user" },
];

// Map (width,height) → human aspect label so cards show "9:16" not "1080×1920".
function aspectLabel(w?: number, h?: number): string {
  if (!w || !h) return "—";
  const r = w / h;
  if (Math.abs(r - 1) < 0.05) return "1:1";
  if (Math.abs(r - 9/16) < 0.05) return "9:16";
  if (Math.abs(r - 16/9) < 0.05) return "16:9";
  if (Math.abs(r - 4/3) < 0.05) return "4:3";
  return `${w}×${h}`;
}

export function Formats() {
  const { t } = useTranslation();
  const nav = useNavigate();
  const qc = useQueryClient();
  const [scope, setScope] = useState<"all" | "system" | "user">("all");

  const q = useQuery({ queryKey: ["formats"], queryFn: () => api.listFormats() });
  const del = useMutation({
    mutationFn: (id: string) => api.deleteFormat(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["formats"] }),
  });

  const all = ((q.data ?? []) as unknown as FormatRow[]);
  const filtered = scope === "all" ? all : all.filter((f) => f.scope === scope);

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">{t("formats.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("formats.subtitle")}</p>
        </div>
        <Button onClick={() => nav("/formats/new")}>+ {t("formats.newFormat")}</Button>
      </div>

      <div className="flex gap-2 border-b border-border">
        {SCOPES.map((s) => (
          <button
            key={s.id}
            onClick={() => setScope(s.id)}
            className={`px-4 py-2 text-sm border-b-2 -mb-px whitespace-nowrap ${
              scope === s.id
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            {t(s.key)} <span className="opacity-60 text-xs">({s.id === "all" ? all.length : all.filter(f => f.scope === s.id).length})</span>
          </button>
        ))}
      </div>

      {q.isLoading && <CardSkeletonGrid count={6} cols={3} />}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filtered.map((f) => (
          <FormatCard key={f.id} fmt={f} onDelete={() => del.mutate(f.id)} />
        ))}
      </div>

      {!q.isLoading && filtered.length === 0 ? (
        <p className="text-sm text-muted-foreground py-8 text-center">{t("formats.empty")}</p>
      ) : null}
    </div>
  );
}

function FormatCard({ fmt, onDelete }: { fmt: FormatRow; onDelete: () => void }) {
  const { t } = useTranslation();
  const ar = aspectLabel(fmt.width, fmt.height);
  return (
    <Card className="flex flex-col group relative">
      <div className={`h-0.5 w-full ${fmt.scope === "system" ? "bg-sky-500" : "bg-amber-500"}`} />

      <CardHeader className="pb-2">
        <div className="flex items-start gap-3">
          <span className="text-3xl leading-none shrink-0">{fmt.icon || "🎨"}</span>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2">
              <CardTitle className="truncate text-base">{fmt.name}</CardTitle>
              <span className="text-[10px] uppercase tracking-wide text-muted-foreground shrink-0">{fmt.scope}</span>
            </div>
            <div className="text-[11px] text-muted-foreground mt-0.5 flex gap-2 flex-wrap">
              <span className="px-1.5 py-0.5 rounded bg-secondary/40">{ar}</span>
              <span>{fmt.fps ?? 30}fps</span>
              <span>·</span>
              <span>{fmt.transition || "none"}</span>
              {fmt.image_model ? (<><span>·</span><span className="truncate">{fmt.image_model.split("/").pop()}</span></>) : null}
            </div>
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex-1 flex flex-col gap-2 pb-0">
        <p className="text-xs text-muted-foreground line-clamp-3 min-h-[3em]">
          {fmt.description || <span className="italic opacity-60">{t("formats.noDescription")}</span>}
        </p>

        {fmt.image_prompt_prefix ? (
          <div>
            <p className="text-[10px] uppercase tracking-wide text-muted-foreground mb-0.5">{t("formats.injectsPrefix")}</p>
            <p className="text-[11px] text-muted-foreground italic line-clamp-2">{fmt.image_prompt_prefix}…</p>
          </div>
        ) : null}
      </CardContent>

      <div className="px-6 pb-3 pt-3 mt-auto flex gap-2">
        <Link
          to={`/?format=${fmt.id}`}
          className="flex-1 text-center text-sm font-medium px-3 py-2 rounded bg-primary text-primary-foreground hover:brightness-110"
        >
          {t("formats.use")} →
        </Link>
        <Link
          to={`/formats/${fmt.id}/edit`}
          className="text-sm px-3 py-2 rounded border border-border hover:bg-secondary/40"
          title={fmt.is_system ? t("formats.systemViewOnly") : t("common.edit")}
        >
          {fmt.is_system ? t("formats.fork") : "✎"}
        </Link>
        {!fmt.is_system ? (
          <button
            onClick={() => {
              if (confirm(t("formats.confirmDelete", { name: fmt.name }))) onDelete();
            }}
            className="text-sm px-3 py-2 rounded border border-border text-red-400 hover:bg-red-500/20"
            title={t("common.delete")}
          >
            ✕
          </button>
        ) : null}
      </div>
    </Card>
  );
}
