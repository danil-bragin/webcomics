import { useEffect, useState } from "react";
import { useNavigate, useSearchParams, Link } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type StepConfig, type TemplateView, type ElevenLabsVoice, type ProjectView, type ProjectDetailView, type FormatView, type PresetView } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/input";

const SCRIPT_MODELS = [
  "openai/gpt-4o-mini",
  "openai/gpt-4o",
  "anthropic/claude-3.5-sonnet",
  "google/gemini-2.0-flash-exp:free",
];
// Each entry: [slug, label including price/panel].
const IMAGE_MODELS: { slug: string; label: string; refs: boolean }[] = [
  { slug: "fal-ai/flux/schnell",          label: "Flux Schnell — $0.003 (text only)",       refs: false },
  { slug: "fal-ai/flux/dev",              label: "Flux Dev — $0.025 (text only)",           refs: false },
  { slug: "fal-ai/flux-2/edit",           label: "Flux 2 Edit dev — $0.013 (up to 10 refs)",refs: true },
  { slug: "fal-ai/flux-2-pro/edit",       label: "Flux 2 Edit pro — $0.032 (10 refs)",      refs: true },
  { slug: "fal-ai/flux-pro/kontext",      label: "Flux Kontext — $0.04 (1 ref)",            refs: true },
  { slug: "fal-ai/flux-pro/kontext/multi",label: "Flux Kontext multi — $0.04 (4 refs)",     refs: true },
  { slug: "fal-ai/gpt-image-1.5/edit",    label: "GPT Image 1.5 edit (low) — $0.013",       refs: true },
  { slug: "fal-ai/bytedance/seedream/v4/text-to-image", label: "Seedream v4 t2i — $0.03",   refs: false },
  { slug: "fal-ai/bytedance/seedream/v4/edit", label: "Seedream v4 edit — $0.03 (10 refs)", refs: true },
  { slug: "fal-ai/nano-banana-2",         label: "Nano Banana 2 — $0.08",                   refs: true },
  { slug: "fal-ai/nano-banana-pro/edit",  label: "Nano Banana Pro edit — $0.15 (top)",      refs: true },
];

import { RESOLUTION_PRESETS, FPS_PRESETS, CODEC_PRESETS, SCRIPT_MODELS as SHARED_SCRIPT_MODELS, IMAGE_MODELS as SHARED_IMAGE_MODELS, STYLE_REFS as SHARED_STYLE_REFS } from "@/lib/options";

// Adapter: existing Studio code keeps Record<id, …> shape.
const RESOLUTION_OPTS = Object.fromEntries(RESOLUTION_PRESETS.map((r) => [r.id, r])) as Record<string, typeof RESOLUTION_PRESETS[number]>;
void FPS_PRESETS; void CODEC_PRESETS; void SHARED_SCRIPT_MODELS; void SHARED_IMAGE_MODELS; void SHARED_STYLE_REFS;

const STYLE_REFS: { value: string; label: string }[] = [
  { value: "none",     label: "None — parallel, each panel independent" },
  { value: "anchor",   label: "Anchor — panel 0 referenced by all others" },
  { value: "previous", label: "Previous — cumulative (panel N gets refs 0..N-1)" },
];

export function Studio() {
  // i18n shorthand: keep tt() since this file uses many local `t` shadowed by the existing template list var.
  const { t: tt } = useTranslation();
  const nav = useNavigate();
  const [prompt, setPrompt] = useState("");
  const [templateId, setTemplateId] = useState("");

  const [panelCount, setPanelCount] = useState(3);
  const [targetDurationMs, setTargetDurationMs] = useState(9000);
  const [enableAudio, setEnableAudio] = useState(false);
  const [autoAssemble, setAutoAssemble] = useState(true);
  const [voiceId, setVoiceId] = useState("EXAVITQu4vr4xnSDxMaL"); // Sarah, default
  const [voiceModel, setVoiceModel] = useState("eleven_flash_v2_5");
  const [voiceSpeed, setVoiceSpeed] = useState(1.0);
  const [renderFps, setRenderFps] = useState(30);
  const [renderResolution, setRenderResolution] = useState("square_1080");
  const [renderCodec, setRenderCodec] = useState<"h264" | "h265">("h264");
  const [projectId, setProjectId] = useState("");
  const [language, setLanguage] = useState<"en" | "ru" | "fr">("en");
  const [pickedChars, setPickedChars] = useState<Set<string>>(new Set());
  const [pickedEnvs, setPickedEnvs] = useState<Set<string>>(new Set());
  const [usePlot, setUsePlot] = useState(false);
  const [formatId, setFormatId] = useState("");
  // Upload settings
  const [uploadEnabled, setUploadEnabled] = useState(false);
  const [uploadAccountId, setUploadAccountId] = useState("");
  const [scheduleAt, setScheduleAt] = useState(""); // local datetime string
  const [captionOverride, setCaptionOverride] = useState("");
  const [scriptModel, setScriptModel] = useState(SCRIPT_MODELS[0]);
  const [imageModel, setImageModel] = useState(IMAGE_MODELS[0].slug);
  const [styleRef, setStyleRef] = useState("none");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [advanced, setAdvanced] = useState(false);
  const [stepsJSON, setStepsJSON] = useState("");
  const [stepsError, setStepsError] = useState<string | null>(null);

  // Preset marketplace data — replaces the old flat template dropdown.
  const [searchParams, setSearchParams] = useSearchParams();
  const presetsQ = useQuery({
    queryKey: ["presets-studio"],
    queryFn: () => api.listPresets(),
  });
  const templates = presetsQ; // legacy alias for downstream hydrate effect
  const voices = useQuery<ElevenLabsVoice[]>({
    queryKey: ["elevenlabs-voices"],
    queryFn: api.listVoices,
    enabled: enableAudio,
    staleTime: 5 * 60 * 1000,
  });
  const projects = useQuery<ProjectView[]>({ queryKey: ["projects"], queryFn: api.listProjects });
  const formatsQ = useQuery<FormatView[]>({ queryKey: ["formats"], queryFn: api.listFormats, staleTime: 60 * 60 * 1000 });
  const projectDetail = useQuery<ProjectDetailView>({
    queryKey: ["project", projectId],
    queryFn: () => api.getProject(projectId),
    enabled: !!projectId,
  });

  useEffect(() => {
    if (!templateId || !templates.data) return;
    const t = templates.data.find((x) => x.id === templateId);
    if (!t) return;
    hydrateFromTemplate(t as unknown as TemplateView, {
      setPanelCount, setTargetDurationMs, setScriptModel, setImageModel,
      setSystemPrompt, setEnableAudio, setStepsJSON, setStyleRef,
    });
    // Preset-only: apply language + auto-fill sample prompt when one is set
    // on the preset and the user hasn't typed yet.
    const p = t as PresetView;
    if (p.defaults && typeof (p.defaults as any).language === "string") {
      setLanguage((p.defaults as any).language);
    }
  }, [templateId, templates.data]);

  // Pre-select preset from ?preset=id in the URL (deep-link from /presets).
  useEffect(() => {
    const presetId = searchParams.get("preset");
    if (presetId && presetId !== templateId) setTemplateId(presetId);
  }, [searchParams, templateId]);

  // Hydrate Studio from a project's saved defaults when picked.
  useEffect(() => {
    if (!projectId || !projectDetail.data) return;
    const d = (projectDetail.data.project?.defaults ?? {}) as Record<string, any>;
    if (typeof d.panel_count === "number") setPanelCount(d.panel_count);
    if (typeof d.target_duration_ms === "number") setTargetDurationMs(d.target_duration_ms);
    if (typeof d.enable_audio === "boolean") setEnableAudio(d.enable_audio);
    if (typeof d.auto_assemble === "boolean") setAutoAssemble(d.auto_assemble);
    if (typeof d.script_model === "string" && d.script_model) setScriptModel(d.script_model);
    if (typeof d.image_model === "string" && d.image_model) setImageModel(d.image_model);
    if (typeof d.style_reference === "string" && d.style_reference) setStyleRef(d.style_reference);
    if (typeof d.system_prompt === "string") setSystemPrompt(d.system_prompt);
    if (d.audio && typeof d.audio === "object") {
      if (d.audio.voice_id) setVoiceId(d.audio.voice_id);
      if (d.audio.model) setVoiceModel(d.audio.model);
      if (typeof d.audio.speed === "number") setVoiceSpeed(d.audio.speed);
    }
    if (d.assemble && typeof d.assemble === "object") {
      if (typeof d.assemble.fps === "number") setRenderFps(d.assemble.fps);
      if (typeof d.assemble.width === "number" && typeof d.assemble.height === "number") {
        const matched = Object.entries(RESOLUTION_OPTS).find(([, v]) =>
          v.w === d.assemble.width && v.h === d.assemble.height,
        );
        if (matched) setRenderResolution(matched[0]);
      }
      if (d.assemble.codec === "h264" || d.assemble.codec === "h265") setRenderCodec(d.assemble.codec);
    }
  }, [projectId, projectDetail.data]);

  const create = useMutation({
    mutationFn: (b: any) => api.createRun(b),
    onSuccess: (r) => nav(`/runs/${r.id}`),
  });

  const submit = () => {
    setStepsError(null);
    const overrides: any = {
      panel_count: panelCount,
      target_duration_ms: targetDurationMs,
      enable_audio: enableAudio,
      auto_assemble: autoAssemble,
      script_model: scriptModel,
      image_model: imageModel,
      style_reference: styleRef,
    };
    if (systemPrompt) overrides.system_prompt = systemPrompt;
    if (enableAudio) {
      overrides.audio = { voice_id: voiceId, model: voiceModel, speed: voiceSpeed };
    }
    overrides.assemble = {
      fps: renderFps,
      width: RESOLUTION_OPTS[renderResolution].w,
      height: RESOLUTION_OPTS[renderResolution].h,
      codec: renderCodec,
    };
    if (advanced && stepsJSON.trim()) {
      try {
        overrides.steps = JSON.parse(stepsJSON);
      } catch (e) {
        setStepsError("invalid JSON: " + (e as Error).message);
        return;
      }
    }
    const body: any = { prompt, template_id: templateId, overrides, language };
    if (projectId) {
      body.project_id = projectId;
      body.character_ids = Array.from(pickedChars);
      body.environment_ids = Array.from(pickedEnvs);
      body.use_plot = usePlot;
    }
    if (formatId) body.format_id = formatId;
    if (uploadEnabled) {
      const up: any = { enabled: true };
      if (uploadAccountId) up.social_account_ids = [uploadAccountId];
      if (scheduleAt) {
        const iso = new Date(scheduleAt).toISOString();
        up.scheduled_at = iso;
      }
      if (captionOverride) up.caption_override = captionOverride;
      overrides.upload = up;
    } else {
      overrides.upload = { enabled: false };
    }
    create.mutate(body);
  };

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{tt("studio.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">{tt("studio.formatLabel")}</label>
            <select
              value={formatId}
              onChange={(e) => setFormatId(e.target.value)}
              className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
            >
              <option value="">— inherit (no format)  —</option>
              {formatsQ.data?.map((f) => (
                <option key={f.id} value={f.id}>{f.name}</option>
              ))}
            </select>
            {formatId && formatsQ.data ? (
              <p className="text-xs text-muted-foreground mt-1">
                {formatsQ.data.find((f) => f.id === formatId)?.description}
              </p>
            ) : null}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">{tt("studio.projectOptional")}</label>
              <select
                value={projectId}
                onChange={(e) => { setProjectId(e.target.value); setPickedChars(new Set()); setPickedEnvs(new Set()); setUsePlot(false); }}
                className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
              >
                <option value="">— {tt("studio.standaloneRun")} —</option>
                {projects.data?.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">
                {tt("studio.languageLabel")}
                <span className="ml-1 text-[10px] opacity-60">{tt("studio.languageHint")}</span>
              </label>
              <select
                value={language}
                onChange={(e) => setLanguage(e.target.value as "en" | "ru" | "fr")}
                className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
              >
                <option value="en">English</option>
                <option value="ru">Русский</option>
                <option value="fr">Français</option>
              </select>
            </div>
          </div>

          {projectId && projectDetail.data ? (
            <div className="rounded border border-border bg-secondary/10 p-3 space-y-2 text-xs">
              <div>
                <p className="uppercase tracking-wide text-muted-foreground mb-1">{tt("projects.characters")}</p>
                {projectDetail.data.characters.length === 0
                  ? <p className="text-muted-foreground">{tt("studio.noCharactersYet")} <a href={`/projects/${projectId}`} className="underline">{tt("studio.addSome")}</a></p>
                  : (
                    <div className="flex flex-wrap gap-2">
                      {projectDetail.data.characters.map((c) => (
                        <label key={c.id} className={
                          "rounded border px-2 py-1 cursor-pointer " +
                          (pickedChars.has(c.id) ? "border-foreground bg-foreground/10" : "border-border")
                        }>
                          <input type="checkbox" className="mr-1" checked={pickedChars.has(c.id)}
                            onChange={(e) => {
                              setPickedChars((curr) => {
                                const n = new Set(curr);
                                if (e.target.checked) n.add(c.id); else n.delete(c.id);
                                return n;
                              });
                            }} />
                          {c.name}
                        </label>
                      ))}
                    </div>
                  )}
              </div>
              <div>
                <p className="uppercase tracking-wide text-muted-foreground mb-1">{tt("projects.environments")}</p>
                {projectDetail.data.environments.length === 0
                  ? <p className="text-muted-foreground">{tt("studio.noEnvironmentsYet")}</p>
                  : (
                    <div className="flex flex-wrap gap-2">
                      {projectDetail.data.environments.map((e) => (
                        <label key={e.id} className={
                          "rounded border px-2 py-1 cursor-pointer " +
                          (pickedEnvs.has(e.id) ? "border-foreground bg-foreground/10" : "border-border")
                        }>
                          <input type="checkbox" className="mr-1" checked={pickedEnvs.has(e.id)}
                            onChange={(ev) => {
                              setPickedEnvs((curr) => {
                                const n = new Set(curr);
                                if (ev.target.checked) n.add(e.id); else n.delete(e.id);
                                return n;
                              });
                            }} />
                          {e.name}
                        </label>
                      ))}
                    </div>
                  )}
              </div>
              {projectDetail.data.plot ? (
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={usePlot} onChange={(e) => setUsePlot(e.target.checked)} />
                  {tt("studio.includePlot")} <span className="text-muted-foreground">({projectDetail.data.plot.name})</span>
                </label>
              ) : null}
            </div>
          ) : null}

          <PresetPickerSection
            presets={(presetsQ.data ?? []) as PresetView[]}
            selectedId={templateId}
            onPick={(id) => {
              setTemplateId(id);
              const next = new URLSearchParams(searchParams);
              if (id) next.set("preset", id); else next.delete("preset");
              setSearchParams(next, { replace: true });
            }}
            onUseSample={(text) => setPrompt(text)}
          />
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">{tt("studio.promptLabel")}</label>
            <Textarea
              rows={4}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder={tt("studio.promptPlaceholder")}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{tt("studio.overrides")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <Slider
              label={`${tt("studio.panels")}: ${panelCount}`}
              min={1} max={10} step={1}
              value={panelCount}
              onChange={setPanelCount}
            />
            <Slider
              label={tt("studio.durationSlider", { total: (targetDurationMs / 1000).toFixed(1), per: (targetDurationMs / panelCount / 1000).toFixed(1) })}
              min={1000} max={30000} step={500}
              value={targetDurationMs}
              onChange={setTargetDurationMs}
            />
          </div>

          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enableAudio}
              onChange={(e) => setEnableAudio(e.target.checked)}
              className="h-4 w-4"
            />
            {tt("studio.addAudioStep")}
          </label>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              checked={autoAssemble}
              onChange={(e) => setAutoAssemble(e.target.checked)}
              className="h-4 w-4 mt-0.5"
            />
            <span>
              {tt("studio.autoAssemble")}
              <span className="block text-xs text-muted-foreground">
                {tt("studio.autoAssembleHint")}
              </span>
            </span>
          </label>

          <div className="rounded border border-border bg-secondary/10 p-3 space-y-2">
            <p className="text-xs uppercase tracking-wide text-muted-foreground">{tt("studio.autoUpload")}</p>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={uploadEnabled} onChange={(e) => setUploadEnabled(e.target.checked)} />
              {tt("studio.uploadAfterRender")}
            </label>
            {uploadEnabled ? (
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div>
                  <label className="text-[10px] uppercase text-muted-foreground mb-1 block">{tt("studio.socialAccount")}</label>
                  <select value={uploadAccountId} onChange={(e) => setUploadAccountId(e.target.value)}
                    className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
                    <option value="">— {tt("studio.pickFromProject")} —</option>
                    {projectDetail.data?.social_accounts?.map((a) => (
                      <option key={a.id} value={a.id}>{a.platform} · {a.label || "—"}</option>
                    ))}
                  </select>
                  {!projectId ? <p className="text-[10px] text-muted-foreground mt-1">{tt("studio.pickProjectHint")}</p> : null}
                </div>
                <div>
                  <label className="text-[10px] uppercase text-muted-foreground mb-1 block">{tt("studio.scheduleOptional")}</label>
                  <input type="datetime-local" value={scheduleAt} onChange={(e) => setScheduleAt(e.target.value)}
                    className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs" />
                </div>
                <div className="col-span-2">
                  <label className="text-[10px] uppercase text-muted-foreground mb-1 block">{tt("studio.captionOverride")}</label>
                  <Textarea rows={2} value={captionOverride} onChange={(e) => setCaptionOverride(e.target.value)}
                    placeholder={tt("studio.captionOverridePlaceholder")} />
                </div>
              </div>
            ) : null}
          </div>

          <div className="rounded border border-border bg-secondary/10 p-3 space-y-2">
            <p className="text-xs uppercase tracking-wide text-muted-foreground">{tt("studio.renderSettings")}</p>
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">{tt("studio.fps")}</label>
                <select
                  value={renderFps}
                  onChange={(e) => setRenderFps(Number(e.target.value))}
                  className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
                >
                  <option value={24}>24</option>
                  <option value={30}>30</option>
                  <option value={60}>60</option>
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">resolution</label>
                <select
                  value={renderResolution}
                  onChange={(e) => setRenderResolution(e.target.value)}
                  className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
                >
                  {Object.entries(RESOLUTION_OPTS).map(([k, v]) => (
                    <option key={k} value={k}>{v.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">codec</label>
                <select
                  value={renderCodec}
                  onChange={(e) => setRenderCodec(e.target.value as "h264" | "h265")}
                  className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
                >
                  <option value="h264">h264</option>
                  <option value="h265">h265</option>
                </select>
              </div>
            </div>
          </div>

          {enableAudio ? (
            <div className="rounded border border-border bg-secondary/10 p-3 space-y-2">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">{tt("studio.audioSettings")}</p>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-muted-foreground mb-1 block">{tt("studio.voice")}</label>
                  <select
                    value={voiceId}
                    onChange={(e) => setVoiceId(e.target.value)}
                    className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
                  >
                    {voices.data?.map((v) => (
                      <option key={v.voice_id} value={v.voice_id}>
                        {v.name}{v.category ? ` (${v.category})` : ""}
                      </option>
                    )) ?? <option value={voiceId}>{voiceId}</option>}
                  </select>
                </div>
                <div>
                  <label className="text-xs text-muted-foreground mb-1 block">{tt("studio.ttsModel")}</label>
                  <select
                    value={voiceModel}
                    onChange={(e) => setVoiceModel(e.target.value)}
                    className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
                  >
                    <option value="eleven_flash_v2_5">eleven_flash_v2_5 (fast)</option>
                    <option value="eleven_turbo_v2_5">eleven_turbo_v2_5</option>
                    <option value="eleven_multilingual_v2">eleven_multilingual_v2</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">
                  {tt("studio.speed")}: {voiceSpeed.toFixed(2)}x
                </label>
                <input
                  type="range"
                  min={0.7}
                  max={1.2}
                  step={0.05}
                  value={voiceSpeed}
                  onChange={(e) => setVoiceSpeed(Number(e.target.value))}
                  className="w-full"
                />
              </div>
            </div>
          ) : null}

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">{tt("studio.scriptModel")}</label>
              <select
                value={scriptModel}
                onChange={(e) => setScriptModel(e.target.value)}
                className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
              >
                {SCRIPT_MODELS.map((m) => <option key={m} value={m}>{m}</option>)}
              </select>
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">{tt("studio.imageModel")}</label>
              <select
                value={imageModel}
                onChange={(e) => setImageModel(e.target.value)}
                className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
              >
                {IMAGE_MODELS.map((m) => <option key={m.slug} value={m.slug}>{m.label}</option>)}
              </select>
            </div>
          </div>

          <div>
            <label className="text-sm text-muted-foreground mb-1 block">
              {tt("studio.styleConsistency")} {(() => {
                const m = IMAGE_MODELS.find((x) => x.slug === imageModel);
                return m && !m.refs && styleRef !== "none" ? (
                  <span className="text-amber-400 text-[10px] ml-1">{tt("studio.styleNoRefsWarn")}</span>
                ) : null;
              })()}
            </label>
            <select
              value={styleRef}
              onChange={(e) => setStyleRef(e.target.value)}
              className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
            >
              {STYLE_REFS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
            </select>
          </div>

          <div>
            <label className="text-sm text-muted-foreground mb-1 block">{tt("studio.systemPromptLabel")}</label>
            <Textarea
              rows={3}
              value={systemPrompt}
              onChange={(e) => setSystemPrompt(e.target.value)}
              placeholder={tt("studio.systemPromptPlaceholder")}
              className="font-mono text-xs"
            />
          </div>

          <details open={advanced}>
            <summary
              className="cursor-pointer text-sm text-muted-foreground select-none"
              onClick={(e) => { e.preventDefault(); setAdvanced((v) => !v); }}
            >
              {tt("studio.advancedToggle")}
            </summary>
            {advanced ? (
              <div className="mt-2">
                <Textarea
                  rows={12}
                  value={stepsJSON}
                  onChange={(e) => setStepsJSON(e.target.value)}
                  className="font-mono text-xs"
                />
                {stepsError ? <p className="text-sm text-red-400 mt-1">{stepsError}</p> : null}
              </div>
            ) : null}
          </details>

          {create.error ? <p className="text-sm text-red-400">{(create.error as Error).message}</p> : null}
          <div className="flex justify-end">
            <Button
              disabled={!prompt || !templateId || create.isPending}
              onClick={submit}
            >
              {create.isPending ? tt("studio.submitting") : tt("studio.generate")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// Preset picker section — replaces the cryptic <select> dropdown with a card
// that surfaces icon + name + step chain + sample prompts. Click "Change" to
// re-open the marketplace inline.
const STEP_GLYPH: Record<string, string> = {
  script: "📝", image: "🖼", audio: "🎙", music: "🎵",
  caption: "📰", assemble: "🎬", upload: "☁",
};

function PresetPickerSection({ presets, selectedId, onPick, onUseSample }: {
  presets: PresetView[];
  selectedId: string;
  onPick: (id: string) => void;
  onUseSample: (text: string) => void;
}) {
  const { t } = useTranslation();
  const [browsing, setBrowsing] = useState(!selectedId);
  const selected = presets.find((p) => p.id === selectedId);

  useEffect(() => {
    // Auto-close the browser once a preset is chosen so the prompt field
    // becomes the focus.
    if (selectedId) setBrowsing(false);
  }, [selectedId]);

  if (browsing || !selected) {
    return (
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-sm text-muted-foreground">{t("presets.title")}</label>
          <Link to="/presets" className="text-xs underline text-muted-foreground">{t("studio.openPresetsPage")}</Link>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2 max-h-72 overflow-y-auto pr-1">
          {presets.length === 0 ? (
            <p className="text-xs text-muted-foreground col-span-full">{t("presets.empty")}</p>
          ) : null}
          {presets.map((p) => {
            const steps = (p.steps ?? []) as { type: string }[];
            return (
              <button
                key={p.id}
                onClick={() => onPick(p.id)}
                className="text-left rounded border border-border bg-secondary/20 hover:border-primary/60 p-3 transition-colors"
              >
                <div className="flex items-start gap-2">
                  <span className="text-xl leading-none">{p.icon || "📄"}</span>
                  <div className="flex-1 min-w-0">
                    <div className="font-medium text-sm truncate">{p.name}</div>
                    {p.description ? (
                      <div className="text-[10px] text-muted-foreground line-clamp-2">{p.description}</div>
                    ) : null}
                  </div>
                </div>
                <div className="flex items-center gap-0.5 mt-2 text-base">
                  {steps.map((s, i) => (
                    <span key={i} className="rounded bg-secondary/40 w-5 h-5 inline-flex items-center justify-center text-[10px]" title={s.type}>
                      {STEP_GLYPH[s.type] ?? "·"}
                    </span>
                  ))}
                </div>
              </button>
            );
          })}
        </div>
      </div>
    );
  }

  const steps = (selected.steps ?? []) as { type: string }[];
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-sm text-muted-foreground">{t("presets.title")}</label>
        <button onClick={() => setBrowsing(true)} className="text-xs underline text-muted-foreground">
          {t("studio.changePreset")}
        </button>
      </div>
      <div className="rounded border border-border bg-secondary/10 p-3 space-y-2">
        <div className="flex items-start gap-3">
          <span className="text-3xl leading-none">{selected.icon || "📄"}</span>
          <div className="flex-1 min-w-0">
            <div className="font-medium">{selected.name}</div>
            {selected.description ? (
              <div className="text-xs text-muted-foreground mt-0.5">{selected.description}</div>
            ) : null}
          </div>
          {selected.category ? (
            <span className="text-[10px] rounded bg-secondary/40 px-1.5 py-0.5 uppercase">{selected.category}</span>
          ) : null}
        </div>
        <div className="flex items-center gap-1 text-base">
          {steps.map((s, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="rounded bg-secondary/40 w-7 h-7 inline-flex items-center justify-center" title={s.type}>
                {STEP_GLYPH[s.type] ?? "·"}
              </span>
              {i < steps.length - 1 ? <span className="text-muted-foreground text-xs">→</span> : null}
            </span>
          ))}
        </div>
        {selected.sample_prompts && selected.sample_prompts.length > 0 ? (
          <div className="border-t border-border pt-2">
            <div className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1">
              {t("presets.samplePrompts")} ({t("studio.clickToUse")})
            </div>
            <div className="flex flex-wrap gap-1">
              {selected.sample_prompts.map((s, i) => (
                <button key={i} onClick={() => onUseSample(s)}
                  className="text-[11px] text-left px-2 py-1 rounded bg-secondary/30 hover:bg-secondary/60 italic truncate max-w-full">
                  — {s}
                </button>
              ))}
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}

function Slider(props: { label: string; min: number; max: number; step: number; value: number; onChange: (n: number) => void }) {
  return (
    <div>
      <label className="text-xs text-muted-foreground mb-1 block">{props.label}</label>
      <input
        type="range"
        min={props.min}
        max={props.max}
        step={props.step}
        value={props.value}
        onChange={(e) => props.onChange(Number(e.target.value))}
        className="w-full"
      />
    </div>
  );
}

function hydrateFromTemplate(t: TemplateView, ctx: {
  setPanelCount: (n: number) => void;
  setTargetDurationMs: (n: number) => void;
  setScriptModel: (s: string) => void;
  setImageModel: (s: string) => void;
  setSystemPrompt: (s: string) => void;
  setEnableAudio: (b: boolean) => void;
  setStepsJSON: (s: string) => void;
  setStyleRef: (s: string) => void;
}) {
  const steps = t.steps as StepConfig[];
  const script = steps.find((s) => s.type === "script");
  const image = steps.find((s) => s.type === "image");
  const assemble = steps.find((s) => s.type === "assemble");
  const audio = steps.find((s) => s.type === "audio");
  const pc = Number((script?.params as any)?.panel_count ?? 3);
  const perPanel = Number((assemble?.params as any)?.panel_duration_ms ?? 2500);
  ctx.setPanelCount(pc);
  ctx.setTargetDurationMs(pc * perPanel);
  if (script?.model) ctx.setScriptModel(script.model);
  if (image?.model) ctx.setImageModel(image.model);
  ctx.setSystemPrompt(script?.system_prompt ?? "");
  ctx.setEnableAudio(!!audio);
  ctx.setStepsJSON(JSON.stringify(steps, null, 2));
  const sr = String((image?.params as any)?.style_reference ?? "none");
  ctx.setStyleRef(sr);
}
