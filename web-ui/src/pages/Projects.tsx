import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type ProjectView } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";
import { CardSkeletonGrid } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast";

export function Projects() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery<ProjectView[]>({ queryKey: ["projects"], queryFn: api.listProjects });
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const create = useMutation({
    mutationFn: () => api.createProject({ name, description: desc }),
    onSuccess: () => {
      setName(""); setDesc("");
      qc.invalidateQueries({ queryKey: ["projects"] });
      toast.push("success", t("projects.created", "Project created"));
    },
    onError: (e: Error) => toast.push("error", e.message),
  });

  const items = (q.data ?? []).filter((p): p is ProjectView => p != null);

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("projects.newProject")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("projects.projectName")}
            className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
          />
          <Textarea
            rows={2}
            value={desc}
            onChange={(e) => setDesc(e.target.value)}
            placeholder={t("projects.optionalDescription")}
          />
          <div className="flex justify-end">
            <Button disabled={!name || create.isPending} onClick={() => create.mutate()}>
              {create.isPending ? t("projects.creating") : t("common.create")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <div>
        <div className="flex items-baseline justify-between mb-3">
          <h2 className="text-lg font-semibold">{t("projects.title")}</h2>
          <span className="text-xs text-muted-foreground">{t("projects.totalCount", { count: items.length })}</span>
        </div>

        {q.isLoading ? (
          <CardSkeletonGrid count={6} cols={3} />
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("projects.noProjectsHint")}</p>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {items.map((p) => (
              <ProjectCard key={p.id} project={p} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function ProjectCard({ project: p }: { project: ProjectView }) {
  const { t, i18n } = useTranslation();
  const initial = (p.name?.trim()?.[0] ?? "?").toUpperCase();
  const updated = new Date(p.updated_at).toLocaleDateString(i18n.resolvedLanguage, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
  return (
    <Link
      to={`/projects/${p.id}`}
      className="group rounded-lg border border-border bg-card hover:border-primary/50 hover:bg-secondary/20 transition-colors flex flex-col overflow-hidden"
    >
      <div className="p-4 flex items-start gap-3">
        <div className="w-10 h-10 rounded-md bg-primary/15 text-primary flex items-center justify-center text-base font-semibold shrink-0">
          {initial}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold truncate" title={p.name}>
              {p.name}
            </h3>
            {p.archived ? (
              <Badge variant="info" className="text-[10px] px-1.5 py-0">
                {t("projects.archived")}
              </Badge>
            ) : null}
          </div>
          <p className="text-xs text-muted-foreground line-clamp-2 mt-0.5 min-h-[2em]">
            {p.description || <span className="italic opacity-60">{t("projects.noDescription")}</span>}
          </p>
        </div>
      </div>

      <div className="px-4 pb-2 flex items-center gap-3 text-[11px] text-muted-foreground">
        <span title={t("projects.totalRuns")}>📽 {t("projects.runsCount", { count: p.runs_count ?? 0 })}</span>
        <span className={p.uploaded_count ? "text-green-400" : ""} title={t("projects.uploadedTitle")}>
          ☁ {t("projects.uploadedCount", { count: p.uploaded_count ?? 0 })}
        </span>
      </div>
      <div className="mt-auto px-4 py-2 border-t border-border bg-secondary/10 flex items-center justify-between text-[11px] text-muted-foreground">
        <span>{t("projects.updatedAt", { date: updated })}</span>
        <span className="text-primary opacity-0 group-hover:opacity-100 transition-opacity">
          {t("common.open")} →
        </span>
      </div>
    </Link>
  );
}
