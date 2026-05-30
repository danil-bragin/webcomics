import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, type CharacterView, type EnvironmentView, type PlotBeatView, type ProjectDetailView, type RunSummary, type FormatView, type SocialAccountView, type FxSession, type UploadRecordView, type AccountUploadStats } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";
import { RunCard } from "@/pages/RunsList";

// Upload helper — POST to /api/uploads/presign, PUT bytes directly to MinIO,
// return the asset_id that callers should attach to ref_asset_ids.
async function uploadRef(file: File, kind: "character_ref" | "environment_ref"): Promise<string> {
  const presign = await api.presignUpload({ kind, filename: file.name, content_type: file.type || "image/png" });
  const r = await fetch(presign.url, { method: "PUT", body: file, headers: { "Content-Type": presign.mime } });
  if (!r.ok) throw new Error(`upload failed: HTTP ${r.status}`);
  return presign.asset_id;
}

function useAssetURL(assetId: string | undefined) {
  const [url, setURL] = useState<string | null>(null);
  useEffect(() => {
    if (!assetId) return;
    let alive = true;
    api.getAssetUrl(assetId).then((r) => alive && setURL(r.url)).catch(() => alive && setURL(null));
    return () => { alive = false; };
  }, [assetId]);
  return url;
}

function RefThumb({ assetId, onRemove }: { assetId: string; onRemove?: () => void }) {
  const url = useAssetURL(assetId);
  return (
    <div className="relative">
      {url ? (
        <img src={url} className="w-20 h-20 object-cover rounded border border-border" />
      ) : (
        <div className="w-20 h-20 rounded border border-border bg-secondary/30 animate-pulse" />
      )}
      {onRemove ? (
        <button
          onClick={onRemove}
          className="absolute -top-1 -right-1 h-5 w-5 rounded-full bg-red-500 text-white text-xs"
        >×</button>
      ) : null}
    </div>
  );
}

function CharacterCard({ c, onSave, onDelete }:
  { c: CharacterView; onSave: (next: { name: string; description: string; ref_asset_ids: string[] }) => void; onDelete: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState(c.name);
  const [desc, setDesc] = useState(c.description);
  const [refs, setRefs] = useState<string[]>(c.ref_asset_ids ?? []);
  const fileRef = useRef<HTMLInputElement>(null);
  const upload = useMutation({
    mutationFn: (f: File) => uploadRef(f, "character_ref"),
    onSuccess: (id) => setRefs((curr) => [...curr, id]),
  });
  const dirty = name !== c.name || desc !== c.description || JSON.stringify(refs) !== JSON.stringify(c.ref_asset_ids ?? []);
  return (
    <div className="rounded border border-border p-3 space-y-2">
      <input value={name} onChange={(e) => setName(e.target.value)}
        className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm font-medium" />
      <Textarea rows={2} value={desc} onChange={(e) => setDesc(e.target.value)}
        placeholder={t("projects.characterPlaceholder")} />
      <div className="flex flex-wrap items-center gap-2">
        {refs.map((id) => (
          <RefThumb key={id} assetId={id} onRemove={() => setRefs((curr) => curr.filter((x) => x !== id))} />
        ))}
        <input ref={fileRef} type="file" accept="image/*" className="hidden"
          onChange={(e) => { const f = e.target.files?.[0]; if (f) upload.mutate(f); }} />
        <Button variant="outline" className="h-7 px-2 text-xs"
          onClick={() => fileRef.current?.click()} disabled={upload.isPending}>
          {upload.isPending ? t("projects.uploading") : t("projects.uploadRef")}
        </Button>
      </div>
      <div className="flex justify-between">
        <Button variant="outline" className="h-7 px-2 text-xs text-red-400" onClick={onDelete}>{t("common.delete")}</Button>
        <Button className="h-7 px-2 text-xs" disabled={!dirty}
          onClick={() => onSave({ name, description: desc, ref_asset_ids: refs })}>{t("common.save")}</Button>
      </div>
    </div>
  );
}

function EnvironmentCard({ e, onSave, onDelete }:
  { e: EnvironmentView; onSave: (next: { name: string; description: string; ref_asset_ids: string[] }) => void; onDelete: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState(e.name);
  const [desc, setDesc] = useState(e.description);
  const [refs, setRefs] = useState<string[]>(e.ref_asset_ids ?? []);
  const fileRef = useRef<HTMLInputElement>(null);
  const upload = useMutation({
    mutationFn: (f: File) => uploadRef(f, "environment_ref"),
    onSuccess: (id) => setRefs((curr) => [...curr, id]),
  });
  const dirty = name !== e.name || desc !== e.description || JSON.stringify(refs) !== JSON.stringify(e.ref_asset_ids ?? []);
  return (
    <div className="rounded border border-border p-3 space-y-2">
      <input value={name} onChange={(ev) => setName(ev.target.value)}
        className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm font-medium" />
      <Textarea rows={2} value={desc} onChange={(ev) => setDesc(ev.target.value)}
        placeholder={t("projects.environmentPlaceholder")} />
      <div className="flex flex-wrap items-center gap-2">
        {refs.map((id) => (
          <RefThumb key={id} assetId={id} onRemove={() => setRefs((curr) => curr.filter((x) => x !== id))} />
        ))}
        <input ref={fileRef} type="file" accept="image/*" className="hidden"
          onChange={(ev) => { const f = ev.target.files?.[0]; if (f) upload.mutate(f); }} />
        <Button variant="outline" className="h-7 px-2 text-xs"
          onClick={() => fileRef.current?.click()} disabled={upload.isPending}>
          {upload.isPending ? t("projects.uploading") : t("projects.uploadRef")}
        </Button>
      </div>
      <div className="flex justify-between">
        <Button variant="outline" className="h-7 px-2 text-xs text-red-400" onClick={onDelete}>{t("common.delete")}</Button>
        <Button className="h-7 px-2 text-xs" disabled={!dirty}
          onClick={() => onSave({ name, description: desc, ref_asset_ids: refs })}>{t("common.save")}</Button>
      </div>
    </div>
  );
}

function PlotEditor({ projectId, initialName, initialPremise, initialBeats }:
  { projectId: string; initialName: string; initialPremise: string; initialBeats: PlotBeatView[] }) {
  const { t } = useTranslation();
  const [name, setName] = useState(initialName || "Main");
  const [premise, setPremise] = useState(initialPremise);
  const [beats, setBeats] = useState<PlotBeatView[]>(initialBeats);
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: () => api.upsertPlot(projectId, { name, premise, beats }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", projectId] }),
  });
  return (
    <div className="space-y-2">
      <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("projects.plotName")}
        className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm font-medium" />
      <Textarea rows={3} value={premise} onChange={(e) => setPremise(e.target.value)} placeholder={t("projects.premisePlaceholder")} />
      <p className="text-[11px] uppercase text-muted-foreground">{t("projects.beats")}</p>
      <div className="space-y-2">
        {beats.map((b, i) => (
          <div key={i} className="flex gap-2">
            <input value={b.name} onChange={(e) => {
              const v = e.target.value;
              setBeats((curr) => curr.map((x, j) => j === i ? { ...x, name: v } : x));
            }} placeholder={t("projects.beatName")}
              className="h-8 w-1/3 rounded border border-border bg-secondary/30 px-2 text-xs" />
            <input value={b.description} onChange={(e) => {
              const v = e.target.value;
              setBeats((curr) => curr.map((x, j) => j === i ? { ...x, description: v } : x));
            }} placeholder={t("common.description")}
              className="h-8 flex-1 rounded border border-border bg-secondary/30 px-2 text-xs" />
            <Button variant="outline" className="h-8 px-2 text-xs"
              onClick={() => setBeats((curr) => curr.filter((_, j) => j !== i))}>×</Button>
          </div>
        ))}
      </div>
      <Button variant="outline" className="h-7 px-2 text-xs"
        onClick={() => setBeats((curr) => [...curr, { name: "", description: "", order: curr.length }])}>+ {t("projects.beat")}</Button>
      <div className="flex justify-end">
        <Button className="h-7 px-2 text-xs" disabled={save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? t("projectDefaults.saving") : t("projects.savePlot")}
        </Button>
      </div>
    </div>
  );
}

type ProjectDefaults = {
  format_id?: string;
  panel_count?: number;
  target_duration_ms?: number;
  enable_audio?: boolean;
  auto_assemble?: boolean;
  script_model?: string;
  image_model?: string;
  style_reference?: "none" | "anchor" | "previous";
  system_prompt?: string;
  audio?: { voice_id?: string; model?: string; speed?: number };
  assemble?: { fps?: number; width?: number; height?: number; codec?: "h264" | "h265"; panel_duration_ms?: number; transition?: string };
  music?: { preferred_mood?: string; track_id?: string };
  ambient?: { object_key?: string };
  language?: "en" | "ru" | "fr";
  subtitles?: {
    enabled?: boolean;
    style?: "bottom_karaoke" | "impact_meme" | "word_pop";
    position?: "top" | "bottom" | "center";
  };
};

import { RESOLUTION_PRESETS } from "@/lib/options";

function ProjectDefaultsCard({ projectId, initial }: { projectId: string; initial: ProjectDefaults }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [d, setD] = useState<ProjectDefaults>(initial);
  const formatsQ = useQuery<FormatView[]>({ queryKey: ["formats"], queryFn: api.listFormats, staleTime: 60 * 60 * 1000 });
  const voicesQ = useQuery({ queryKey: ["voices"], queryFn: api.listVoices, staleTime: 60 * 60 * 1000 });
  const musicQ = useQuery({ queryKey: ["music-library"], queryFn: api.listMusicLibrary, staleTime: 60 * 60 * 1000 });
  const audioLibQ = useQuery({
    queryKey: ["audio-tracks-all", projectId],
    queryFn: () => api.listAudioTracks({ project_id: projectId }),
    staleTime: 30 * 1000,
  });
  const ambientTracks = (audioLibQ.data ?? []).filter((t) => t.kind === "ambient");
  const dbMusicTracks = (audioLibQ.data ?? []).filter((t) => t.kind === "music");
  const save = useMutation({
    mutationFn: () => api.updateProject(projectId, { name: "", defaults: d } as any),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", projectId] }),
  });
  // Track resolution by index.
  const resIdx = (() => {
    if (d.assemble?.width && d.assemble?.height) {
      const i = RESOLUTION_PRESETS.findIndex((p) => p.w === d.assemble!.width && p.h === d.assemble!.height);
      return i >= 0 ? i : 0;
    }
    return 0;
  })();
  return (
    <div className="space-y-3 text-sm">
      <p className="text-xs text-muted-foreground">{t("projectDefaults.hint")}</p>
      <Field label={t("projectDefaults.defaultContentLanguage")}>
        <select
          value={d.language ?? "en"}
          onChange={(e) => setD({ ...d, language: (e.target.value || "en") as "en" | "ru" | "fr" })}
          className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm"
        >
          <option value="en">English</option>
          <option value="ru">Русский</option>
          <option value="fr">Français</option>
        </select>
        <p className="text-[10px] text-muted-foreground mt-1">
          {t("projectDefaults.languageHint")}
        </p>
      </Field>
      <Field label={t("projectDefaults.formatPreset")}>
        <select
          value={d.format_id ?? ""}
          onChange={(e) => setD({ ...d, format_id: e.target.value || undefined })}
          className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm"
        >
          <option value="">— {t("common.none")} —</option>
          {formatsQ.data?.map((f) => (
            <option key={f.id} value={f.id}>{f.name}</option>
          ))}
        </select>
        {d.format_id && formatsQ.data ? (
          <p className="text-[11px] text-muted-foreground mt-1">
            {formatsQ.data.find((f) => f.id === d.format_id)?.description}
          </p>
        ) : null}
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("projectDefaults.defaultPanels")}>
          <input type="number" min={1} max={20}
            value={d.panel_count ?? ""}
            onChange={(e) => setD({ ...d, panel_count: e.target.value ? Number(e.target.value) : undefined })}
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
        <Field label={t("projectDefaults.defaultDuration")}>
          <input type="number" min={1000} max={120000} step={500}
            value={d.target_duration_ms ?? ""}
            onChange={(e) => setD({ ...d, target_duration_ms: e.target.value ? Number(e.target.value) : undefined })}
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
      </div>
      <label className="flex items-center gap-2 text-xs">
        <input type="checkbox" checked={!!d.enable_audio} onChange={(e) => setD({ ...d, enable_audio: e.target.checked })} />
        {t("projectDefaults.enableAudio")}
      </label>
      <label className="flex items-center gap-2 text-xs">
        <input type="checkbox" checked={d.auto_assemble !== false}
          onChange={(e) => setD({ ...d, auto_assemble: e.target.checked })} />
        {t("projectDefaults.autoAssemble")}
      </label>

      <div className="grid grid-cols-2 gap-3">
        <Field label={t("studio.scriptModel")}>
          <input value={d.script_model ?? ""}
            onChange={(e) => setD({ ...d, script_model: e.target.value || undefined })}
            placeholder="openai/gpt-4o-mini"
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
        <Field label={t("studio.imageModel")}>
          <input value={d.image_model ?? ""}
            onChange={(e) => setD({ ...d, image_model: e.target.value || undefined })}
            placeholder="fal-ai/flux/schnell"
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
      </div>

      <Field label={t("studio.styleConsistency")}>
        <select value={d.style_reference ?? "none"}
          onChange={(e) => setD({ ...d, style_reference: e.target.value as ProjectDefaults["style_reference"] })}
          className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
          <option value="none">{t("common.none")}</option>
          <option value="anchor">{t("projectDefaults.styleAnchor")}</option>
          <option value="previous">{t("projectDefaults.stylePrevious")}</option>
        </select>
      </Field>

      <Field label={t("projectDefaults.systemPrompt")}>
        <Textarea rows={2} value={d.system_prompt ?? ""}
          onChange={(e) => setD({ ...d, system_prompt: e.target.value || undefined })}
          placeholder={t("projectDefaults.optional")} />
      </Field>

      <div className="rounded border border-border bg-secondary/10 p-2 space-y-2">
        <p className="text-[11px] uppercase text-muted-foreground">{t("projectDefaults.audioDefaults")}</p>
        <div className="grid grid-cols-3 gap-2">
          <Field label={t("studio.voice")}>
            <select value={d.audio?.voice_id ?? ""}
              onChange={(e) => setD({ ...d, audio: { ...d.audio, voice_id: e.target.value || undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">— {t("projectDefaults.defaultValue")} —</option>
              {(voicesQ.data ?? []).map((v) => (
                <option key={v.voice_id} value={v.voice_id}>
                  {v.name} {v.category ? `(${v.category})` : ""}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t("projectDefaults.model")}>
            <select value={d.audio?.model ?? ""}
              onChange={(e) => setD({ ...d, audio: { ...d.audio, model: e.target.value || undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">—</option>
              <option value="eleven_flash_v2_5">flash_v2_5</option>
              <option value="eleven_turbo_v2_5">turbo_v2_5</option>
              <option value="eleven_multilingual_v2">multilingual_v2</option>
            </select>
          </Field>
          <Field label={t("studio.speed")}>
            <input type="number" min={0.7} max={1.2} step={0.05}
              value={d.audio?.speed ?? ""}
              onChange={(e) => setD({ ...d, audio: { ...d.audio, speed: e.target.value ? Number(e.target.value) : undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs" />
          </Field>
        </div>
      </div>

      <div className="rounded border border-border bg-secondary/10 p-2 space-y-2">
        <p className="text-[11px] uppercase text-muted-foreground">{t("projectDefaults.musicDefaults")}</p>
        <div className="grid grid-cols-2 gap-2">
          <Field label={t("projectDefaults.preferredMood")}>
            <select value={d.music?.preferred_mood ?? ""}
              onChange={(e) => setD({ ...d, music: { ...d.music, preferred_mood: e.target.value || undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">— {t("projectDefaults.llmAutoPick")} —</option>
              {Array.from(new Set((musicQ.data ?? []).flatMap((t) => t.mood))).sort().map((m) => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </Field>
          <Field label={t("projectDefaults.fixedTrack")}>
            <select value={d.music?.track_id ?? ""}
              onChange={(e) => setD({ ...d, music: { ...d.music, track_id: e.target.value || undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">— {t("projectDefaults.llmAutoPick")} —</option>
              <optgroup label={t("projectDefaults.audioLibraryDB")}>
                {dbMusicTracks.map((t2) => (
                  <option key={t2.id} value={t2.id}>{t2.title} — {t2.mood} [{t2.scope}]</option>
                ))}
              </optgroup>
              <optgroup label={t("projectDefaults.staticManifest")}>
                {(musicQ.data ?? []).map((t2) => (
                  <option key={t2.id} value={t2.id}>{t2.title} — {t2.tempo} / {t2.genre.join(", ")}</option>
                ))}
              </optgroup>
            </select>
          </Field>
        </div>
        <p className="text-[10px] text-muted-foreground">
          {t("projectDefaults.tracksSummary", { db: dbMusicTracks.length, manifest: (musicQ.data ?? []).length })}{" "}
          <a href={`/library/audio?project_id=${projectId}&kind=music`} className="underline">
            {t("projects.manageLibrary")}
          </a>
        </p>
      </div>

      <div className="rounded border border-border bg-secondary/10 p-2 space-y-2">
        <p className="text-[11px] uppercase text-muted-foreground">{t("projects.ambientBed")}</p>
        <Field label={t("projects.ambientLoopLabel")}>
          <select value={d.ambient?.object_key ?? ""}
            onChange={(e) => setD({ ...d, ambient: { object_key: e.target.value || undefined } })}
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
            <option value="">— {t("common.none")} —</option>
            {ambientTracks.map((t2) => (
              <option key={t2.id} value={t2.object_key}>{t2.title} — {t2.mood} [{t2.scope}]</option>
            ))}
          </select>
        </Field>
        <p className="text-[10px] text-muted-foreground">
          {t("projectDefaults.ambientCount", { count: ambientTracks.length })}{" "}
          <a href={`/library/audio?project_id=${projectId}&kind=ambient`} className="underline">
            {t("projects.uploadMore")}
          </a>
        </p>
      </div>

      <div className="rounded border border-border bg-secondary/10 p-2 space-y-2">
        <p className="text-[11px] uppercase text-muted-foreground">{t("projectDefaults.renderDefaults")}</p>
        <div className="grid grid-cols-3 gap-2">
          <Field label={t("studio.fps")}>
            <select value={d.assemble?.fps ?? ""}
              onChange={(e) => setD({ ...d, assemble: { ...d.assemble, fps: e.target.value ? Number(e.target.value) : undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">—</option>
              <option value={24}>24</option>
              <option value={30}>30</option>
              <option value={60}>60</option>
            </select>
          </Field>
          <Field label={t("studio.resolution")}>
            <select value={d.assemble?.width ? String(resIdx) : ""}
              onChange={(e) => {
                if (e.target.value === "") {
                  setD({ ...d, assemble: { ...d.assemble, width: undefined, height: undefined } });
                  return;
                }
                const r = RESOLUTION_PRESETS[Number(e.target.value)];
                setD({ ...d, assemble: { ...d.assemble, width: r.w, height: r.h } });
              }}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">—</option>
              {RESOLUTION_PRESETS.map((r, i) => <option key={i} value={i}>{r.label}</option>)}
            </select>
          </Field>
          <Field label={t("projectDefaults.codec")}>
            <select value={d.assemble?.codec ?? ""}
              onChange={(e) => setD({ ...d, assemble: { ...d.assemble, codec: (e.target.value || undefined) as ProjectDefaults["assemble"] extends infer A ? A extends { codec?: infer C } ? C : never : never } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">—</option>
              <option value="h264">h264</option>
              <option value="h265">h265</option>
            </select>
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Field label={t("studio.panelDurationMs")}>
            <input type="number" min={1000} max={10000} step={250}
              value={d.assemble?.panel_duration_ms ?? ""}
              onChange={(e) => setD({ ...d, assemble: { ...d.assemble, panel_duration_ms: e.target.value ? Number(e.target.value) : undefined } })}
              placeholder="e.g. 4000"
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs" />
          </Field>
          <Field label={t("studio.transition")}>
            <select value={d.assemble?.transition ?? ""}
              onChange={(e) => setD({ ...d, assemble: { ...d.assemble, transition: e.target.value || undefined } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="">— {t("projectDefaults.defaultValue")} —</option>
              <option value="crossfade">{t("projectDefaults.crossfade")}</option>
              <option value="fade">{t("projectDefaults.fadeToBlack")}</option>
              <option value="slide">{t("projectDefaults.slide")}</option>
              <option value="none">{t("projectDefaults.hardCut")}</option>
            </select>
          </Field>
        </div>
      </div>

      <div className="rounded border border-border bg-secondary/10 p-2 space-y-2">
        <p className="text-[11px] uppercase text-muted-foreground">{t("projectDefaults.burnedSubtitles")}</p>
        <label className="flex items-center gap-2 text-xs">
          <input type="checkbox" checked={!!d.subtitles?.enabled}
            onChange={(e) => setD({ ...d, subtitles: { ...d.subtitles, enabled: e.target.checked } })} />
          {t("projectDefaults.renderSubtitles")}
        </label>
        <div className="grid grid-cols-2 gap-2">
          <Field label={t("projectDefaults.style")}>
            <select value={d.subtitles?.style ?? "bottom_karaoke"}
              onChange={(e) => setD({ ...d, subtitles: { ...d.subtitles, style: e.target.value as NonNullable<ProjectDefaults["subtitles"]>["style"] } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="bottom_karaoke">{t("projectDefaults.styleBottomKaraoke")}</option>
              <option value="impact_meme">{t("projectDefaults.styleImpactMeme")}</option>
              <option value="word_pop">{t("projectDefaults.styleWordPop")}</option>
            </select>
          </Field>
          <Field label={t("projectDefaults.position")}>
            <select value={d.subtitles?.position ?? "bottom"}
              onChange={(e) => setD({ ...d, subtitles: { ...d.subtitles, position: e.target.value as NonNullable<ProjectDefaults["subtitles"]>["position"] } })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-xs">
              <option value="bottom">{t("projectDefaults.positionBottom")}</option>
              <option value="top">{t("projectDefaults.positionTop")}</option>
              <option value="center">{t("projectDefaults.positionCenter")}</option>
            </select>
          </Field>
        </div>
      </div>

      <div className="flex justify-end">
        <Button className="h-8 px-3 text-xs" disabled={save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? t("projectDefaults.saving") : t("projectDefaults.saveDefaults")}
        </Button>
      </div>
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

export function ProjectDetail() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const q = useQuery<ProjectDetailView>({ queryKey: ["project", id], queryFn: () => api.getProject(id) });
  const runs = useQuery<RunSummary[]>({
    queryKey: ["project-runs", id],
    queryFn: () => api.listRuns({ project_id: id, limit: 100 }),
    enabled: !!id,
  });

  const [newCharName, setNewCharName] = useState("");
  const [newEnvName, setNewEnvName] = useState("");
  const [tab, setTab] = useState<"overview" | "runs">("overview");

  const createChar = useMutation({
    mutationFn: () => api.createCharacter(id, { name: newCharName }),
    onSuccess: () => { setNewCharName(""); qc.invalidateQueries({ queryKey: ["project", id] }); },
  });
  const createEnv = useMutation({
    mutationFn: () => api.createEnvironment(id, { name: newEnvName }),
    onSuccess: () => { setNewEnvName(""); qc.invalidateQueries({ queryKey: ["project", id] }); },
  });
  const updateChar = useMutation({
    mutationFn: (args: { id: string; body: Parameters<typeof api.updateCharacter>[1] }) =>
      api.updateCharacter(args.id, args.body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", id] }),
  });
  const deleteChar = useMutation({
    mutationFn: (cid: string) => api.deleteCharacter(cid),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", id] }),
  });
  const updateEnv = useMutation({
    mutationFn: (args: { id: string; body: Parameters<typeof api.updateEnvironment>[1] }) =>
      api.updateEnvironment(args.id, args.body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", id] }),
  });
  const deleteEnv = useMutation({
    mutationFn: (eid: string) => api.deleteEnvironment(eid),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", id] }),
  });

  if (!q.data) return <p className="p-6 text-sm text-muted-foreground">{t("common.loading")}</p>;
  const d = q.data;
  return (
    <div className="max-w-6xl mx-auto p-6 space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{d.project.name}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{d.project.description}</p>
        </CardContent>
      </Card>

      <div className="flex gap-1 border-b border-border">
        <TabBtn active={tab === "overview"} onClick={() => setTab("overview")}>{t("projects.overview")}</TabBtn>
        <TabBtn active={tab === "runs"} onClick={() => setTab("runs")}>
          {t("runs.title")} <span className="text-muted-foreground">({runs.data?.length ?? 0})</span>
        </TabBtn>
      </div>

      {tab === "runs" ? (
        runs.data && runs.data.length > 0 ? (
          <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
            {runs.data.map((r) => (
              <RunCard
                key={r.id}
                id={r.id}
                prompt={r.prompt}
                status={r.status}
                cost={r.total_cost_usd}
                createdAt={r.created_at}
                videoAssetId={r.video_asset_id}
              />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t("projects.noRunsYet")} <Link to="/" className="underline">{t("nav.studio")}</Link>
          </p>
        )
      ) : null}

      {tab === "overview" ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("projects.defaultSettings")}</CardTitle>
          </CardHeader>
          <CardContent>
            <ProjectDefaultsCard projectId={id} initial={(d.project.defaults ?? {}) as ProjectDefaults} />
          </CardContent>
        </Card>
      ) : null}

      {tab === "overview" ? (
        <>
          <Card>
            <CardHeader>
              <CardTitle>{t("projects.characters")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex gap-2">
                <input value={newCharName} onChange={(e) => setNewCharName(e.target.value)} placeholder={t("projects.newCharacter")}
                  className="h-8 flex-1 rounded border border-border bg-secondary/30 px-2 text-sm" />
                <Button className="h-8 px-2 text-xs" disabled={!newCharName || createChar.isPending} onClick={() => createChar.mutate()}>
                  + {t("projects.addCharacter")}
                </Button>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {d.characters.map((c) => (
                  <CharacterCard key={c.id} c={c}
                    onSave={(body) => updateChar.mutate({ id: c.id, body })}
                    onDelete={() => deleteChar.mutate(c.id)} />
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("projects.environments")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex gap-2">
                <input value={newEnvName} onChange={(e) => setNewEnvName(e.target.value)} placeholder={t("projects.newEnvironment")}
                  className="h-8 flex-1 rounded border border-border bg-secondary/30 px-2 text-sm" />
                <Button className="h-8 px-2 text-xs" disabled={!newEnvName || createEnv.isPending} onClick={() => createEnv.mutate()}>
                  + {t("projects.addEnvironment")}
                </Button>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {d.environments.map((e) => (
                  <EnvironmentCard key={e.id} e={e}
                    onSave={(body) => updateEnv.mutate({ id: e.id, body })}
                    onDelete={() => deleteEnv.mutate(e.id)} />
                ))}
              </div>
            </CardContent>
          </Card>

          <SocialAccountsSection projectId={id} accounts={d.social_accounts ?? []} />
          <UploadDefaultsCard projectId={id} defaults={(d.project.defaults ?? {}) as Record<string, unknown>} accounts={d.social_accounts ?? []} />
          <UploadHistoryCard projectId={id} accounts={d.social_accounts ?? []} />

          <Card>
            <CardHeader>
              <CardTitle>{t("projects.plot")}</CardTitle>
            </CardHeader>
            <CardContent>
              <PlotEditor
                projectId={id}
                initialName={d.plot?.name ?? "Main"}
                initialPremise={d.plot?.premise ?? ""}
                initialBeats={d.plot?.beats ?? []}
              />
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  );
}

const PLATFORMS = [
  { value: "youtube_selenium", label: "YouTube (selenium)" },
  { value: "twitter_selenium", label: "Twitter / X (selenium)" },
];

function SocialAccountsSection({ projectId, accounts }:
  { projectId: string; accounts: SocialAccountView[] }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const fxStatusQ = useQuery({ queryKey: ["fx-status"], queryFn: api.fxStatus, staleTime: 60_000 });
  const [loginOpen, setLoginOpen] = useState(false);
  const [loginPlatform, setLoginPlatform] = useState(PLATFORMS[0].value);
  const [loginLabel, setLoginLabel] = useState("");

  const del = useMutation({
    mutationFn: (id: string) => api.deleteSocialAccount(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", projectId] }),
  });

  const fxEnabled = !!fxStatusQ.data?.enabled;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle>{t("social.title")}</CardTitle>
          <div className="flex items-center gap-2">
            <select value={loginPlatform} onChange={(e) => setLoginPlatform(e.target.value)}
              className="h-8 rounded border border-border bg-secondary/30 px-2 text-xs">
              {PLATFORMS.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
            </select>
            <input value={loginLabel} onChange={(e) => setLoginLabel(e.target.value)}
              placeholder={t("social.labelPlaceholder")}
              className="h-8 rounded border border-border bg-secondary/30 px-2 text-xs w-44" />
            <Button className="h-8 px-3 text-xs"
              disabled={!fxEnabled || !loginLabel}
              title={fxEnabled ? "" : t("social.fxDisabledTitle")}
              onClick={() => setLoginOpen(true)}>
              + {t("social.signInNew")}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {!fxEnabled ? (
          <p className="text-xs text-amber-500">
            {t("social.embeddedDisabled")}
          </p>
        ) : null}
        {accounts.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("social.noAccountsHint")}
          </p>
        ) : null}
        <ul className="divide-y divide-border">
          {accounts.map((a) => (
            <li key={a.id} className="py-2 flex items-center gap-2 text-sm">
              <span className="text-[10px] uppercase text-muted-foreground w-32">{a.platform}</span>
              <span className="font-medium">{a.label || "—"}</span>
              <code className="text-xs text-muted-foreground flex-1 truncate">{a.firefox_profile_path}</code>
              <Button variant="outline" className="h-7 px-2 text-xs text-red-400" onClick={() => del.mutate(a.id)}>{t("common.delete")}</Button>
            </li>
          ))}
        </ul>
        {loginOpen ? (
          <FxLoginDialog
            projectId={projectId}
            platform={loginPlatform}
            label={loginLabel}
            onClose={(saved) => {
              setLoginOpen(false);
              if (saved) {
                setLoginLabel("");
                qc.invalidateQueries({ queryKey: ["project", projectId] });
              }
            }}
          />
        ) : null}
      </CardContent>
    </Card>
  );
}

function FxLoginDialog({ projectId, platform, label, onClose }: {
  projectId: string;
  platform: string;
  label: string;
  onClose: (saved: boolean) => void;
}) {
  const { t } = useTranslation();
  const [sess, setSess] = useState<FxSession | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [finishing, setFinishing] = useState(false);
  const finishedRef = useRef(false);

  useEffect(() => {
    let alive = true;
    api.fxStart({ project_id: projectId, platform, label })
      .then((s) => { if (alive) setSess(s); })
      .catch((e) => { if (alive) setErr(String(e?.message || e)); });
    return () => {
      alive = false;
      // Best-effort cleanup if the dialog unmounts before finish.
      if (!finishedRef.current && sess?.id) {
        api.fxCancel(sess.id).catch(() => {});
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const finish = async () => {
    if (!sess) return;
    setFinishing(true);
    try {
      await api.fxFinish(sess.id, { label });
      finishedRef.current = true;
      onClose(true);
    } catch (e: any) {
      setErr(String(e?.message || e));
      setFinishing(false);
    }
  };

  const cancel = async () => {
    if (sess) {
      finishedRef.current = true;
      await api.fxCancel(sess.id).catch(() => {});
    }
    onClose(false);
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4">
      <div className="bg-card border border-border rounded-lg shadow-xl w-[1100px] max-w-full h-[80vh] flex flex-col">
        <div className="flex items-center justify-between px-4 py-2 border-b border-border">
          <div className="flex items-center gap-2 text-sm">
            <strong>{t("social.embeddedLogin")}</strong>
            <span className="text-xs text-muted-foreground">{platform}</span>
            {sess ? <span className="text-xs text-muted-foreground">· {t("social.session")} {sess.id.slice(0, 8)}</span> : null}
            <span className={`text-[10px] uppercase ml-2 ${
              sess?.status === "ready" ? "text-green-400" :
              sess?.status === "error" ? "text-red-400" : "text-muted-foreground"
            }`}>{sess?.status ?? t("social.starting")}</span>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" className="h-7 px-3 text-xs" onClick={cancel}>{t("common.cancel")}</Button>
            <Button
              className="h-7 px-3 text-xs"
              disabled={!sess || sess.status !== "ready" || finishing}
              onClick={finish}
            >
              {finishing ? t("social.saving") : t("social.signedInSave")}
            </Button>
          </div>
        </div>
        <div className="flex-1 relative bg-black">
          {err ? (
            <div className="absolute inset-0 flex items-center justify-center text-red-400 text-sm p-4 text-center">{err}</div>
          ) : !sess || sess.status === "starting" ? (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">
              {t("social.spinningUp")}
            </div>
          ) : (
            <iframe
              src={sess.vnc_url}
              className="w-full h-full border-0"
              title="firefox-login"
              allow="clipboard-read; clipboard-write"
            />
          )}
        </div>
        <div className="px-4 py-2 border-t border-border text-[11px] text-muted-foreground">
          {t("social.signInHint")}
        </div>
      </div>
    </div>
  );
}

// --- Upload defaults card -----------------------------------------------

const YT_CATEGORIES: Array<{ id: string; label: string }> = [
  { id: "1", label: "Film & Animation" },
  { id: "2", label: "Autos & Vehicles" },
  { id: "10", label: "Music" },
  { id: "15", label: "Pets & Animals" },
  { id: "17", label: "Sports" },
  { id: "20", label: "Gaming" },
  { id: "22", label: "People & Blogs" },
  { id: "23", label: "Comedy" },
  { id: "24", label: "Entertainment" },
  { id: "25", label: "News & Politics" },
  { id: "26", label: "Howto & Style" },
  { id: "27", label: "Education" },
  { id: "28", label: "Science & Technology" },
];

type UploadDefaults = {
  social_account_id?: string;
  visibility?: "public" | "unlisted" | "private";
  made_for_kids?: boolean;
  age_restriction?: "none" | "18plus";
  category_id?: string;
  category_label?: string;
  comments_enabled?: boolean;
  tags?: string[];
  playlist_names?: string[];
  title?: string;
  description?: string;
  review_mode?: "auto" | "review";
  primary_format?: "shorts" | "long" | "square" | "reels" | "tiktok";
};

function UploadDefaultsCard({ projectId, defaults, accounts }:
  { projectId: string; defaults: Record<string, unknown>; accounts: SocialAccountView[] }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const initial = ((defaults as { upload?: UploadDefaults }).upload ?? {}) as UploadDefaults;
  const [d, setD] = useState<UploadDefaults>(initial);
  const [tagsRaw, setTagsRaw] = useState<string>((initial.tags ?? []).join(", "));
  const [playlistsRaw, setPlaylistsRaw] = useState<string>((initial.playlist_names ?? []).join(", "));
  const save = useMutation({
    mutationFn: () => api.updateProject(projectId, {
      name: "",
      defaults: { ...defaults, upload: { ...d,
        tags: tagsRaw.split(/[,;\n]+/).map((s) => s.trim()).filter(Boolean),
        playlist_names: playlistsRaw.split(/[,;\n]+/).map((s) => s.trim()).filter(Boolean),
      } },
    } as any),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", projectId] }),
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("upload.defaults")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">{t("upload.appliedAt")}</p>
        <div className="grid grid-cols-2 gap-3">
          <Field label={t("upload.boundAccount")}>
            <select value={d.social_account_id ?? ""}
              onChange={(e) => setD({ ...d, social_account_id: e.target.value || undefined })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
              <option value="">— {t("common.none")} —</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>{a.label || a.platform}</option>
              ))}
            </select>
          </Field>
          <Field label={t("upload.defaultVisibility")}>
            <select value={d.visibility ?? "unlisted"}
              onChange={(e) => setD({ ...d, visibility: e.target.value as UploadDefaults["visibility"] })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
              <option value="public">{t("upload.visibilityPublic")}</option>
              <option value="unlisted">{t("upload.visibilityUnlisted")}</option>
              <option value="private">{t("upload.visibilityPrivate")}</option>
            </select>
          </Field>
        </div>
        <Field label={t("upload.primaryFormat")}>
          <select value={d.primary_format ?? "shorts"}
            onChange={(e) => setD({ ...d, primary_format: e.target.value as UploadDefaults["primary_format"] })}
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
            <option value="shorts">YouTube Shorts (9:16, 1080×1920)</option>
            <option value="reels">Instagram Reels / TikTok (9:16)</option>
            <option value="long">YouTube long-form (16:9, 1920×1080)</option>
            <option value="square">Square (1:1, 1080×1080)</option>
          </select>
        </Field>
        <Field label={t("upload.reviewGate")}>
          <select value={d.review_mode ?? "review"}
            onChange={(e) => setD({ ...d, review_mode: e.target.value as UploadDefaults["review_mode"] })}
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
            <option value="review">{t("upload.reviewBefore")}</option>
            <option value="auto">{t("upload.autopilot")}</option>
          </select>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label={t("upload.category")}>
            <select value={d.category_id ?? "22"}
              onChange={(e) => {
                const cat = YT_CATEGORIES.find((c) => c.id === e.target.value)!;
                setD({ ...d, category_id: cat.id, category_label: cat.label });
              }}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
              {YT_CATEGORIES.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}
            </select>
          </Field>
          <Field label={t("upload.ageRestriction")}>
            <select value={d.age_restriction ?? "none"}
              onChange={(e) => setD({ ...d, age_restriction: e.target.value as UploadDefaults["age_restriction"] })}
              className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm">
              <option value="none">{t("common.none")}</option>
              <option value="18plus">18+</option>
            </select>
          </Field>
        </div>
        <div className="flex items-center gap-4">
          <label className="flex items-center gap-2 text-xs">
            <input type="checkbox" checked={!!d.made_for_kids}
              onChange={(e) => setD({ ...d, made_for_kids: e.target.checked })} />
            {t("upload.madeForKids")}
          </label>
          <label className="flex items-center gap-2 text-xs">
            <input type="checkbox" checked={d.comments_enabled !== false}
              onChange={(e) => setD({ ...d, comments_enabled: e.target.checked })} />
            {t("upload.commentsEnabled")}
          </label>
        </div>
        <Field label={t("upload.titleTemplate")}>
          <input value={d.title ?? ""} onChange={(e) => setD({ ...d, title: e.target.value || undefined })}
            placeholder={t("upload.titlePlaceholder")}
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
        <Field label={t("upload.descriptionTemplate")}>
          <Textarea rows={3} value={d.description ?? ""}
            onChange={(e) => setD({ ...d, description: e.target.value || undefined })}
            placeholder={t("upload.descriptionPlaceholder")} />
        </Field>
        <Field label={t("upload.tagsField")}>
          <input value={tagsRaw} onChange={(e) => setTagsRaw(e.target.value)}
            placeholder="webcomics, ai, shorts"
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
        <Field label={t("upload.playlistsField")}>
          <input value={playlistsRaw} onChange={(e) => setPlaylistsRaw(e.target.value)}
            placeholder="My channel"
            className="h-8 w-full rounded border border-border bg-secondary/30 px-2 text-sm" />
        </Field>
        <div className="flex justify-end">
          <Button className="h-8 px-3 text-xs" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? t("projectDefaults.saving") : t("upload.saveDefaults")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function UploadHistoryCard({ projectId, accounts }:
  { projectId: string; accounts: SocialAccountView[] }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const recs = useQuery<UploadRecordView[]>({
    queryKey: ["project-uploads", projectId],
    queryFn: () => api.listProjectUploadRecords(projectId, { limit: 50 }),
    refetchInterval: 8000,
  });
  const stats = useQuery<AccountUploadStats[]>({
    queryKey: ["project-account-stats", projectId],
    queryFn: () => api.getAccountUploadStats(projectId),
    refetchInterval: 8000,
  });
  const publish = useMutation({
    mutationFn: (id: string) => api.publishUploadRecord(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project-uploads", projectId] }),
  });
  const accountLabel = (id?: string) => accounts.find((a) => a.id === id)?.label || id || "—";
  const accountStats = (id: string) => stats.data?.find((s) => s.social_account_id === id);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("upload.history")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {accounts.length > 0 ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
            {accounts.map((a) => {
              const s = accountStats(a.id);
              return (
                <div key={a.id} className="rounded border border-border p-2 text-xs">
                  <div className="font-medium truncate" title={a.label || a.platform}>{a.label || a.platform}</div>
                  <div className="flex items-center gap-2 text-[11px] text-muted-foreground mt-0.5">
                    <span title={t("upload.total")}>{s?.total ?? 0} {t("upload.total")}</span>
                    <span title={t("upload.published")} className="text-green-400">{s?.published ?? 0} ✓</span>
                    <span title={t("upload.failed")} className="text-red-400">{s?.failed ?? 0} ✗</span>
                  </div>
                </div>
              );
            })}
          </div>
        ) : null}
        {(recs.data ?? []).length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("upload.noUploads")}</p>
        ) : (
          <ul className="divide-y divide-border">
            {(recs.data ?? []).map((r) => (
              <li key={r.id} className="py-2 flex items-center gap-2 text-sm">
                <Badge variant={r.status === "published" ? "success" : r.status === "uploaded" ? "info" : r.status === "failed" ? "danger" : "default"}>{r.status}</Badge>
                <span className="text-xs text-muted-foreground w-32 truncate">{accountLabel(r.social_account_id)}</span>
                <span className="flex-1 truncate" title={r.title}>{r.title || <span className="italic text-muted-foreground">{t("upload.noTitle")}</span>}</span>
                <span className="text-[10px] text-muted-foreground">{new Date(r.created_at).toLocaleDateString(i18n.resolvedLanguage)}</span>
                {r.external_ref ? (
                  <a href={r.external_ref} target="_blank" rel="noreferrer" className="text-xs underline">{t("common.open")}</a>
                ) : null}
                {r.status === "uploaded" ? (
                  <Button className="h-6 px-2 text-[10px]" onClick={() => publish.mutate(r.id)}>{t("upload.publish")}</Button>
                ) : null}
                <Link to={`/runs/${r.run_id}`} className="text-xs underline text-muted-foreground">{t("upload.runLink")}</Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function TabBtn({ active, children, onClick }:
  { active: boolean; children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={
        "px-4 py-2 text-sm border-b-2 -mb-px transition-colors " +
        (active ? "border-foreground text-foreground" : "border-transparent text-muted-foreground hover:text-foreground")
      }
    >
      {children}
    </button>
  );
}
