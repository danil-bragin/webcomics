import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { api, StepConfig } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/input";

const DEFAULT_STEPS: StepConfig[] = [
  { type: "script", model: "openai/gpt-4o-mini", params: { panel_count: 3 } },
  { type: "image", model: "fal-ai/flux/schnell", params: {} },
  { type: "assemble", params: { width: 1080, height: 1080, fps: 30, panel_duration_ms: 2500, transition: "crossfade" } },
];

export function Templates() {
  const { t: tt, i18n } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["templates"], queryFn: api.listTemplates });
  const [name, setName] = useState("default-meme-3panel");
  const [stepsJSON, setStepsJSON] = useState(JSON.stringify(DEFAULT_STEPS, null, 2));
  const [err, setErr] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => {
      let steps: StepConfig[];
      try {
        steps = JSON.parse(stepsJSON);
      } catch (e) {
        throw new Error("invalid JSON: " + (e as Error).message);
      }
      return api.createTemplate({ name, steps, max_cost_usd: 0 });
    },
    onError: (e) => setErr((e as Error).message),
    onSuccess: () => {
      setErr(null);
      qc.invalidateQueries({ queryKey: ["templates"] });
    },
  });

  return (
    <div className="max-w-4xl mx-auto p-6 space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{tt("templates.newTemplate")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={tt("common.name")} />
          <Textarea rows={14} value={stepsJSON} onChange={(e) => setStepsJSON(e.target.value)} className="font-mono text-xs" />
          {err ? <p className="text-sm text-red-400">{err}</p> : null}
          <div className="flex justify-end">
            <Button onClick={() => create.mutate()} disabled={create.isPending}>
              {create.isPending ? tt("templates.saving") : tt("common.save")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{tt("templates.countLabel", { count: q.data?.length ?? 0 })}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {q.data?.map((tpl) => (
            <div key={tpl.id} className="rounded border border-border p-3">
              <p className="font-medium">{tpl.name}</p>
              <p className="text-xs text-muted-foreground">
                {tt("templates.stepsUpdated", {
                  count: tpl.steps.length,
                  date: new Date(tpl.updated_at).toLocaleString(i18n.resolvedLanguage),
                })}
              </p>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
