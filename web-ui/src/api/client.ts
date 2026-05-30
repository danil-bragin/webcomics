// Typed API client. Types come from openapi-typescript output (schema.gen.ts).
// Run `npm run gen` after editing api/openapi/openapi.yaml.
import type { components } from "./schema.gen";

export type StepConfig = components["schemas"]["StepConfig"];
export type TemplateView = components["schemas"]["TemplateView"];
export type TemplateBody = components["schemas"]["TemplateBody"];
export type RunSummary = components["schemas"]["RunSummary"];
export type RunView = components["schemas"]["RunView"] & {
  project_id?: string;
  project_name?: string;
};
export type StepView = components["schemas"]["StepView"];
export type AttemptView = components["schemas"]["AttemptView"];
export type AssetView = components["schemas"]["AssetView"];
export type CostEntryView = components["schemas"]["CostEntryView"];
export type CreateRunRequest = components["schemas"]["CreateRunRequest"];
export type PresignedAssetURL = components["schemas"]["PresignedAssetURL"];
export type IDResponse = components["schemas"]["IDResponse"];
export type StatsView = components["schemas"]["StatsView"];
export type ProviderCost = components["schemas"]["ProviderCost"];
export type DayCost = components["schemas"]["DayCost"];
export type BalancesView = components["schemas"]["BalancesView"];
export type ProviderBalance = components["schemas"]["Provider"];
export type RunOverrides = components["schemas"]["RunOverrides"];
export type RegenerateStepRequest = components["schemas"]["RegenerateStepRequest"];
export type RegenerateStepResponse = components["schemas"]["RegenerateStepResponse"];
export type RequestAssembleRequest = components["schemas"]["RequestAssembleRequest"];
export type RequestAssembleResponse = components["schemas"]["RequestAssembleResponse"];
export type ProjectView = components["schemas"]["ProjectView"] & {
  runs_count?: number;
  uploaded_count?: number;
};
export type ProjectBody = components["schemas"]["ProjectBody"];
export type ProjectDetailView = components["schemas"]["ProjectDetailView"];
export type CharacterView = components["schemas"]["CharacterView"];
export type CharacterBody = components["schemas"]["CharacterBody"];
export type EnvironmentView = components["schemas"]["EnvironmentView"];
export type EnvironmentBody = components["schemas"]["EnvironmentBody"];
export type PlotView = components["schemas"]["PlotView"];
export type PlotBody = components["schemas"]["PlotBody"];
export type PlotBeatView = components["schemas"]["PlotBeatView"];
export type FormatView = components["schemas"]["FormatView"];
export type SocialAccountView = components["schemas"]["SocialAccountView"];
export type SocialAccountBody = components["schemas"]["SocialAccountBody"];

async function request<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const r = await fetch(input, {
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init,
  });
  if (r.status === 204) return undefined as T;
  if (!r.ok) {
    let msg = `HTTP ${r.status}`;
    try {
      const j = await r.json();
      msg = j?.error ?? msg;
    } catch {}
    throw new Error(msg);
  }
  return (await r.json()) as T;
}

export const api = {
  listTemplates: () => request<TemplateView[]>("/api/templates"),
  getTemplate: (id: string) => request<TemplateView>(`/api/templates/${id}`),
  createTemplate: (b: TemplateBody) =>
    request<IDResponse>("/api/templates", { method: "POST", body: JSON.stringify(b) }),
  updateTemplate: (id: string, b: TemplateBody) =>
    request<void>(`/api/templates/${id}`, { method: "PUT", body: JSON.stringify(b) }),

  // Presets — same data as templates but with rich marketplace metadata.
  listPresets: (opts: { category?: string; include_test?: boolean } = {}) => {
    const p = new URLSearchParams();
    if (opts.category) p.set("category", opts.category);
    if (opts.include_test) p.set("include_test", "true");
    const qs = p.toString();
    return request<PresetView[]>(`/api/presets${qs ? `?${qs}` : ""}`);
  },
  getPreset: (id: string) => request<PresetView>(`/api/presets/${id}`),
  createPreset: (b: PresetBody) =>
    request<{ id: string }>("/api/presets", { method: "POST", body: JSON.stringify(b) }),
  updatePreset: (id: string, b: PresetBody) =>
    request<void>(`/api/presets/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deletePreset: (id: string) =>
    request<void>(`/api/presets/${id}`, { method: "DELETE" }),

  listRuns: (opts?: { status?: string[]; q?: string; limit?: number; offset?: number; project_id?: string }) => {
    const params = new URLSearchParams();
    if (opts?.status && opts.status.length > 0) params.set("status", opts.status.join(","));
    if (opts?.q && opts.q.trim() !== "") params.set("q", opts.q.trim());
    if (opts?.limit != null) params.set("limit", String(opts.limit));
    if (opts?.offset != null) params.set("offset", String(opts.offset));
    if (opts?.project_id) params.set("project_id", opts.project_id);
    const qs = params.toString();
    return request<RunSummary[]>(`/api/runs${qs ? `?${qs}` : ""}`);
  },
  getRun: (id: string) => request<RunView>(`/api/runs/${id}`),
  createRun: (b: CreateRunRequest) =>
    request<IDResponse>("/api/runs", { method: "POST", body: JSON.stringify(b) }),
  cancelRun: (id: string) => request<void>(`/api/runs/${id}/cancel`, { method: "POST" }),
  retryRun: (id: string) => request<IDResponse>(`/api/runs/${id}/retry`, { method: "POST" }),
  regenerateStep: (id: string, idx: number, b: RegenerateStepRequest) =>
    request<RegenerateStepResponse>(`/api/runs/${id}/steps/${idx}/regenerate`, {
      method: "POST",
      body: JSON.stringify(b),
    }),
  requestAssemble: (id: string, b: RequestAssembleRequest = {}) =>
    request<RequestAssembleResponse>(`/api/runs/${id}/assemble`, {
      method: "POST",
      body: JSON.stringify(b),
    }),

  getAssetUrl: (id: string) => request<PresignedAssetURL>(`/api/assets/${id}/url`),

  getStats: () => request<StatsView>("/api/stats"),
  getBalances: () => request<BalancesView>("/api/balances"),

  listVoices: () => request<ElevenLabsVoice[]>("/api/elevenlabs/voices"),
  listFormats: () => request<FormatView[]>("/api/formats"),

  listProjects: () => request<ProjectView[]>("/api/projects"),
  getProject: (id: string) => request<ProjectDetailView>(`/api/projects/${id}`),
  createProject: (b: ProjectBody) =>
    request<IDResponse>("/api/projects", { method: "POST", body: JSON.stringify(b) }),
  updateProject: (id: string, b: ProjectBody) =>
    request<void>(`/api/projects/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteProject: (id: string) =>
    request<void>(`/api/projects/${id}`, { method: "DELETE" }),

  createCharacter: (pid: string, b: CharacterBody) =>
    request<IDResponse>(`/api/projects/${pid}/characters`, { method: "POST", body: JSON.stringify(b) }),
  updateCharacter: (id: string, b: CharacterBody) =>
    request<void>(`/api/characters/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteCharacter: (id: string) =>
    request<void>(`/api/characters/${id}`, { method: "DELETE" }),

  createEnvironment: (pid: string, b: EnvironmentBody) =>
    request<IDResponse>(`/api/projects/${pid}/environments`, { method: "POST", body: JSON.stringify(b) }),
  updateEnvironment: (id: string, b: EnvironmentBody) =>
    request<void>(`/api/environments/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteEnvironment: (id: string) =>
    request<void>(`/api/environments/${id}`, { method: "DELETE" }),

  upsertPlot: (pid: string, b: PlotBody) =>
    request<IDResponse>(`/api/projects/${pid}/plot`, { method: "PUT", body: JSON.stringify(b) }),

  createSocialAccount: (pid: string, b: SocialAccountBody) =>
    request<IDResponse>(`/api/projects/${pid}/social-accounts`, { method: "POST", body: JSON.stringify(b) }),
  updateSocialAccount: (id: string, b: SocialAccountBody) =>
    request<void>(`/api/social-accounts/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteSocialAccount: (id: string) =>
    request<void>(`/api/social-accounts/${id}`, { method: "DELETE" }),

  // Upload records.
  listRunUploadRecords: (runId: string) =>
    request<UploadRecordView[]>(`/api/runs/${runId}/upload-records`),
  listProjectUploadRecords: (projectId: string, q: { limit?: number; offset?: number } = {}) => {
    const sp = new URLSearchParams();
    if (q.limit) sp.set("limit", String(q.limit));
    if (q.offset) sp.set("offset", String(q.offset));
    const s = sp.toString();
    return request<UploadRecordView[]>(`/api/projects/${projectId}/upload-records${s ? "?" + s : ""}`);
  },
  getAccountUploadStats: (projectId: string) =>
    request<AccountUploadStats[]>(`/api/projects/${projectId}/account-upload-stats`),
  publishUploadRecord: (id: string) =>
    request<{ id: string }>(`/api/upload-records/${id}/publish`, { method: "POST" }),
  retryUploadRecord: (id: string) =>
    request<unknown>(`/api/upload-records/${id}/retry`, { method: "POST", body: "{}" }),
  approveUploadRecord: (id: string) =>
    request<{ id: string }>(`/api/upload-records/${id}/approve`, { method: "POST" }),
  rejectUploadRecord: (id: string) =>
    request<{ id: string }>(`/api/upload-records/${id}/reject`, { method: "POST" }),
  editUploadMetadata: (id: string, body: EditUploadMetadataBody) =>
    request<{ id: string }>(`/api/upload-records/${id}/metadata`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  getScreenshotUrl: (objectKey: string) =>
    request<{ url: string }>(`/api/upload-records/screenshot-url?key=${encodeURIComponent(objectKey)}`),
  listMusicLibrary: () => request<MusicTrack[]>("/api/music-library"),

  // Audio library (music / sfx / ambient / voice).
  listAudioTracks: (opts: { kind?: string; scope?: string; project_id?: string; mood?: string; q?: string } = {}) => {
    const p = new URLSearchParams();
    if (opts.kind) p.set("kind", opts.kind);
    if (opts.scope) p.set("scope", opts.scope);
    if (opts.project_id) p.set("project_id", opts.project_id);
    if (opts.mood) p.set("mood", opts.mood);
    if (opts.q) p.set("q", opts.q);
    const qs = p.toString();
    return request<AudioTrack[]>(`/api/audio/tracks${qs ? `?${qs}` : ""}`);
  },
  uploadAudioTrack: (form: FormData) =>
    fetch("/api/audio/tracks", { method: "POST", body: form }).then(async (r) => {
      if (!r.ok) throw new Error(await r.text());
      return r.json() as Promise<{ id: string; object_key: string }>;
    }),
  importAudioFromURL: (b: {
    kind: string; url: string; title?: string; tags?: string[]; mood?: string;
    scope?: string; project_id?: string; attribution?: string;
  }) =>
    request<{ id: string; object_key: string }>("/api/audio/tracks/import-url", {
      method: "POST", body: JSON.stringify(b),
    }),
  importAudioFromPixabay: (b: {
    kind: string; result: PixabayResult; mood?: string; scope?: string; project_id?: string;
  }) =>
    request<{ id: string; object_key: string }>("/api/audio/tracks/import-pixabay", {
      method: "POST", body: JSON.stringify(b),
    }),
  retagAudioTrack: (id: string, b: { tags?: string[]; mood?: string }) =>
    request<void>(`/api/audio/tracks/${id}`, { method: "PATCH", body: JSON.stringify(b) }),
  deleteAudioTrack: (id: string) =>
    request<void>(`/api/audio/tracks/${id}`, { method: "DELETE" }),
  audioTrackPreviewURL: (id: string) =>
    request<{ url: string }>(`/api/audio/tracks/${id}/preview-url`),
  pixabaySearch: (opts: { kind: string; q?: string; limit?: number }) => {
    const p = new URLSearchParams({ kind: opts.kind });
    if (opts.q) p.set("q", opts.q);
    if (opts.limit) p.set("limit", String(opts.limit));
    return request<PixabayResult[]>(`/api/audio/pixabay/search?${p.toString()}`);
  },

  // Firefox-login orchestration (embedded social account auth).
  fxStatus: () => request<{ enabled: boolean }>("/api/firefox-login/status"),
  fxStart: (b: { project_id: string; platform: string; label?: string }) =>
    request<FxSession>("/api/firefox-login/sessions", { method: "POST", body: JSON.stringify(b) }),
  fxGet: (id: string) => request<FxSession>(`/api/firefox-login/sessions/${id}`),
  fxFinish: (id: string, b: { label?: string } = {}) =>
    request<{ session: FxSession; social_account_id: string; firefox_profile_path: string }>(
      `/api/firefox-login/sessions/${id}/finish`,
      { method: "POST", body: JSON.stringify(b) },
    ),
  fxCancel: (id: string) =>
    request<void>(`/api/firefox-login/sessions/${id}`, { method: "DELETE" }),

  presignUpload: (b: { kind: string; filename: string; content_type: string }) =>
    request<{ asset_id: string; url: string; object_key: string; bucket: string; mime: string }>(
      "/api/uploads/presign",
      { method: "POST", body: JSON.stringify(b) },
    ),
};

export type EditUploadMetadataBody = {
  title: string;
  description: string;
  tags: string[];
  hashtags: string[];
  visibility: "public" | "unlisted" | "private";
  made_for_kids: boolean;
  age_restriction: "none" | "18plus";
  category_id: string;
  category_label: string;
  comments_enabled: boolean;
  playlist_names: string[];
};

export type UploadRecordView = {
  id: string;
  run_id: string;
  project_id?: string;
  social_account_id?: string;
  step_index: number;
  status: "pending" | "metadata_ready" | "pending_review" | "approved" | "rejected" | "uploading" | "uploaded" | "published" | "failed";
  provider: string;
  platform_target?: string;
  title: string;
  description: string;
  tags: string[];
  hashtags: string[];
  visibility: "public" | "unlisted" | "private";
  made_for_kids: boolean;
  age_restriction: "none" | "18plus";
  category_id: string;
  category_label: string;
  comments_enabled: boolean;
  playlist_names: string[];
  scheduled_at?: string;
  external_ref?: string;
  external_id?: string;
  thumbnail_asset_id?: string;
  attempts: number;
  error?: string;
  error_screenshot_asset_id?: string;
  metadata_overridden?: boolean;
  audience_confidence?: number;
  audience_reasoning?: string;
  hook?: string;
  screenshot_trail?: { stage: string; object_key: string }[];
  started_at?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
};

export type AccountUploadStats = {
  social_account_id: string;
  total: number;
  uploaded: number;
  published: number;
  failed: number;
  last_upload_at?: string;
};

export type FxSession = {
  id: string;
  port: number;
  container: string;
  host_dir: string;
  vnc_url: string;
  status: "starting" | "ready" | "finished" | "error";
  project_id: string;
  platform: string;
  label: string;
  profile_inner?: string;
  created_at: string;
  error?: string;
};

export type MusicTrack = {
  id: string;
  title: string;
  artist: string;
  object_key: string;
  duration_s: number;
  mood: string[];
  genre: string[];
  tempo: string;
  license: string;
  attribution?: string;
};

export type PresetCategory = "meme" | "shorts" | "story" | "demo" | "custom";

export type PresetView = {
  id: string;
  name: string;
  description?: string;
  category?: PresetCategory;
  icon?: string;
  steps: StepConfig[];
  sample_prompts?: string[];
  format_id?: string;
  defaults?: Record<string, unknown>;
  max_cost_usd: number;
  is_test?: boolean;
  created_at: string;
  updated_at: string;
};

export type PresetBody = {
  name: string;
  description?: string;
  category?: PresetCategory;
  icon?: string;
  steps: StepConfig[];
  sample_prompts?: string[];
  format_id?: string;
  defaults?: Record<string, unknown>;
  max_cost_usd?: number;
};

export type AudioTrack = {
  id: string;
  kind: "music" | "sfx" | "ambient" | "voice";
  title: string;
  tags: string[];
  mood: string;
  duration_ms: number;
  object_key: string;
  bucket: string;
  source: "manual" | "url" | "pixabay";
  source_ref: string;
  attribution: string;
  scope: "global" | "project";
  project_id?: string;
  bytes: number;
  created_at: string;
};

export type PixabayResult = {
  id: string;
  title: string;
  tags: string[];
  duration_ms: number;
  preview_url: string;
  download_url: string;
  page_url: string;
  author: string;
  attribution: string;
  mime_type: string;
};

export type ElevenLabsVoice = {
  voice_id: string;
  name: string;
  category?: string;
  description?: string;
  preview_url?: string;
  labels?: Record<string, string>;
};
