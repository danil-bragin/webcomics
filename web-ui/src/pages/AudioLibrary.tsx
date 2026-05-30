import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, type AudioTrack, type PixabayResult } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

type Kind = "music" | "sfx" | "ambient" | "voice";
const KINDS: Kind[] = ["music", "sfx", "ambient", "voice"];

const MOODS = ["", "carefree", "epic", "chill", "sneaky", "playful", "energetic", "smooth", "tense", "dramatic"];

export function AudioLibrary() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const projectId = params.get("project_id") ?? "";
  const initialKind = (params.get("kind") as Kind | null) ?? "music";
  const [kind, setKind] = useState<Kind>(initialKind);
  const [search, setSearch] = useState("");
  const [mood, setMood] = useState("");
  const qc = useQueryClient();

  const tracksQuery = useQuery<AudioTrack[]>({
    queryKey: ["audio-tracks", kind, projectId, search, mood],
    queryFn: () =>
      api.listAudioTracks({
        kind,
        project_id: projectId || undefined,
        q: search || undefined,
        mood: mood || undefined,
      }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteAudioTrack(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["audio-tracks"] }),
  });

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">{t("library.audio.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {projectId
              ? t("library.audio.subtitle_project", { project: projectId.slice(0, 8) })
              : t("library.audio.subtitle_global")}
          </p>
        </div>
        <div className="text-xs text-muted-foreground">{t("library.audio.hint")}</div>
      </div>

      {/* Kind tabs */}
      <div className="flex gap-2 border-b border-border">
        {KINDS.map((k) => (
          <button
            key={k}
            onClick={() => {
              setKind(k);
              const next = new URLSearchParams(params);
              next.set("kind", k);
              setParams(next, { replace: true });
            }}
            className={`px-3 py-2 text-sm border-b-2 -mb-px ${
              kind === k
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            {t(`library.audio.kinds.${k}`)}
          </button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground -mt-4">{t(`library.audio.kindHelp.${kind}`)}</p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <UploadCard kind={kind} projectId={projectId} onDone={() => qc.invalidateQueries({ queryKey: ["audio-tracks"] })} />
        <ImportURLCard kind={kind} projectId={projectId} onDone={() => qc.invalidateQueries({ queryKey: ["audio-tracks"] })} />
        <PixabayCard kind={kind} projectId={projectId} onDone={() => qc.invalidateQueries({ queryKey: ["audio-tracks"] })} />
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>{t("library.audio.tracksCount", { count: tracksQuery.data?.length ?? 0 })}</CardTitle>
          <div className="flex gap-2">
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("library.audio.searchTracks")}
              className="h-9 w-64 rounded-md border border-border bg-secondary/30 px-3 text-sm"
            />
            <select
              value={mood}
              onChange={(e) => setMood(e.target.value)}
              className="h-9 rounded-md border border-border bg-secondary/30 px-2 text-sm"
            >
              {MOODS.map((m) => (
                <option key={m || "any"} value={m}>
                  {m || t("common.any")}
                </option>
              ))}
            </select>
          </div>
        </CardHeader>
        <CardContent className="space-y-1">
          {tracksQuery.isLoading && <div className="text-sm text-muted-foreground">{t("common.loading")}</div>}
          {tracksQuery.isError && <div className="text-sm text-red-400">{t("common.error")}: {String(tracksQuery.error)}</div>}
          {tracksQuery.data?.length === 0 && (
            <div className="text-sm text-muted-foreground py-6 text-center">
              {t("library.audio.emptyKind", { kind: t(`library.audio.kinds.${kind}`) })}
            </div>
          )}
          {tracksQuery.data?.map((t) => (
            <TrackRow key={t.id} track={t} onDelete={() => deleteMut.mutate(t.id)} />
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

function TrackRow({ track, onDelete }: { track: AudioTrack; onDelete: () => void }) {
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [duration, setDuration] = useState<number>(track.duration_ms / 1000);

  const loadPreview = async () => {
    if (previewUrl || loading) return;
    setLoading(true);
    try {
      const { url } = await api.audioTrackPreviewURL(track.id);
      setPreviewUrl(url);
    } finally {
      setLoading(false);
    }
  };

  const fmt = (s: number) => {
    if (!s || !isFinite(s)) return "—";
    const m = Math.floor(s / 60);
    const r = Math.floor(s % 60);
    return `${m}:${r.toString().padStart(2, "0")}`;
  };

  return (
    <div className="py-2 border-b border-border/40 last:border-b-0 space-y-1.5">
      <div className="flex items-center gap-3">
        <div className="flex-1 min-w-0">
          <div className="font-medium text-sm truncate">{track.title}</div>
          <div className="text-xs text-muted-foreground flex gap-2 flex-wrap mt-0.5">
            {track.mood && <Badge variant="info" className="text-[10px]">{track.mood}</Badge>}
            {track.tags.slice(0, 5).map((tag) => (
              <Badge key={tag} variant="default" className="text-[10px]">{tag}</Badge>
            ))}
          </div>
        </div>
        <div className="text-xs text-muted-foreground tabular-nums">{fmt(duration)}</div>
        <Badge variant={track.scope === "global" ? "info" : "default"} className="text-[10px]">
          {track.scope}
        </Badge>
        <Badge variant="default" className="text-[10px]">{track.source}</Badge>
        <Button variant="ghost" onClick={onDelete}>✕</Button>
      </div>
      <div className="pl-0">
        {previewUrl ? (
          <audio
            src={previewUrl}
            controls
            preload="metadata"
            className="h-8 w-full"
            onLoadedMetadata={(e) => setDuration(e.currentTarget.duration)}
          />
        ) : (
          <button
            onClick={loadPreview}
            disabled={loading}
            className="text-xs px-2 py-1 rounded border border-border bg-secondary/30 hover:bg-secondary/60 w-full text-left"
          >
            {loading ? "…" : "▶"}
          </button>
        )}
      </div>
    </div>
  );
}

function UploadCard({ kind, projectId, onDone }: { kind: Kind; projectId: string; onDone: () => void }) {
  const { t } = useTranslation();
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [mood, setMood] = useState("");
  const [tags, setTags] = useState("");
  const [scope, setScope] = useState<"global" | "project">(projectId ? "project" : "global");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!file) {
      setErr("pick a file first");
      return;
    }
    setErr(null);
    setBusy(true);
    try {
      const fd = new FormData();
      fd.set("file", file);
      fd.set("kind", kind);
      fd.set("title", title || file.name.replace(/\.[^.]+$/, ""));
      fd.set("mood", mood);
      fd.set("tags", tags);
      fd.set("scope", scope);
      if (scope === "project") fd.set("project_id", projectId);
      await api.uploadAudioTrack(fd);
      setFile(null);
      setTitle("");
      setTags("");
      setMood("");
      onDone();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader><CardTitle>{t("library.audio.uploadCard")}</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        <input
          type="file"
          accept="audio/*"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          className="text-sm w-full"
        />
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t("library.audio.titlePlaceholder")}
          className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
        />
        <div className="flex gap-2">
          <select
            value={mood}
            onChange={(e) => setMood(e.target.value)}
            className="h-9 flex-1 rounded-md border border-border bg-secondary/30 px-2 text-sm"
          >
            {MOODS.map((m) => (
              <option key={m || "any"} value={m}>{m || t("studio.mood")}</option>
            ))}
          </select>
          <select
            value={scope}
            onChange={(e) => setScope(e.target.value as "global" | "project")}
            className="h-9 rounded-md border border-border bg-secondary/30 px-2 text-sm"
            disabled={!projectId}
          >
            <option value="global">{t("library.audio.scope.global")}</option>
            {projectId && <option value="project">{t("library.audio.scope.project")}</option>}
          </select>
        </div>
        <input
          value={tags}
          onChange={(e) => setTags(e.target.value)}
          placeholder={t("library.audio.tagsPlaceholder")}
          className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
        />
        <Button disabled={busy || !file} onClick={submit} className="w-full">
          {busy ? t("library.audio.uploading") : t("common.upload")}
        </Button>
        {err && <div className="text-xs text-red-400">{err}</div>}
      </CardContent>
    </Card>
  );
}

function ImportURLCard({ kind, projectId, onDone }: { kind: Kind; projectId: string; onDone: () => void }) {
  const { t } = useTranslation();
  const [url, setUrl] = useState("");
  const [title, setTitle] = useState("");
  const [mood, setMood] = useState("");
  const [scope, setScope] = useState<"global" | "project">(projectId ? "project" : "global");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!url) return;
    setErr(null);
    setBusy(true);
    try {
      await api.importAudioFromURL({
        kind,
        url,
        title: title || undefined,
        mood: mood || undefined,
        scope,
        project_id: scope === "project" ? projectId : undefined,
      });
      setUrl("");
      setTitle("");
      setMood("");
      onDone();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader><CardTitle>{t("library.audio.importUrlCard")}</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder={t("library.audio.urlPlaceholder")}
          className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
        />
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t("library.audio.titleOptionalPlaceholder")}
          className="h-9 w-full rounded-md border border-border bg-secondary/30 px-3 text-sm"
        />
        <div className="flex gap-2">
          <select
            value={mood}
            onChange={(e) => setMood(e.target.value)}
            className="h-9 flex-1 rounded-md border border-border bg-secondary/30 px-2 text-sm"
          >
            {MOODS.map((m) => (
              <option key={m || "any"} value={m}>{m || t("studio.mood")}</option>
            ))}
          </select>
          <select
            value={scope}
            onChange={(e) => setScope(e.target.value as "global" | "project")}
            className="h-9 rounded-md border border-border bg-secondary/30 px-2 text-sm"
            disabled={!projectId}
          >
            <option value="global">{t("library.audio.scope.global")}</option>
            {projectId && <option value="project">{t("library.audio.scope.project")}</option>}
          </select>
        </div>
        <Button disabled={busy || !url} onClick={submit} className="w-full">
          {busy ? t("library.audio.importing") : t("common.import")}
        </Button>
        {err && <div className="text-xs text-red-400">{err}</div>}
      </CardContent>
    </Card>
  );
}

function PixabayCard({ kind, projectId, onDone }: { kind: Kind; projectId: string; onDone: () => void }) {
  const { t } = useTranslation();
  const [q, setQ] = useState("");
  const [results, setResults] = useState<PixabayResult[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const search = async () => {
    setErr(null);
    setBusy(true);
    try {
      const r = await api.pixabaySearch({ kind, q, limit: 10 });
      setResults(r);
      if (r.length === 0) {
        setErr(t("library.audio.noResults"));
      }
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const importOne = async (result: PixabayResult) => {
    setBusy(true);
    try {
      await api.importAudioFromPixabay({
        kind,
        result,
        scope: projectId ? "project" : "global",
        project_id: projectId || undefined,
      });
      onDone();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader><CardTitle>{t("library.audio.pixabayCard")}</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        <div className="flex gap-2">
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && search()}
            placeholder={t("library.audio.searchPixabay")}
            className="h-9 flex-1 rounded-md border border-border bg-secondary/30 px-3 text-sm"
          />
          <Button disabled={busy} onClick={search}>{t("common.search")}</Button>
        </div>
        {err && <div className="text-xs text-amber-400">{err}</div>}
        <div className="max-h-56 overflow-y-auto space-y-1">
          {results.map((r) => (
            <div key={r.id} className="flex items-center gap-2 text-xs">
              <span className="flex-1 truncate">{r.title}</span>
              {r.preview_url && (
                <audio src={r.preview_url} controls className="h-6" />
              )}
              <Button variant="default" onClick={() => importOne(r)} disabled={busy}>
                +
              </Button>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
