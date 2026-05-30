// Shared option catalogs reused across Studio / TimelineEditor / ProjectDetail.

export type ResolutionPreset = {
  /** stable id used in form state */
  id: string;
  label: string;
  w: number;
  h: number;
};

export const RESOLUTION_PRESETS: ResolutionPreset[] = [
  { id: "square_1080",    label: "Square 1080×1080",     w: 1080, h: 1080 },
  { id: "portrait_1080",  label: "Portrait 1080×1920",   w: 1080, h: 1920 },
  { id: "landscape_1080", label: "Landscape 1920×1080",  w: 1920, h: 1080 },
  { id: "square_4k",      label: "Square 4K 2160×2160",  w: 2160, h: 2160 },
];

/** Map back to a preset given concrete width/height. Returns the first preset on no match. */
export function resolutionByDims(width: number, height: number): ResolutionPreset {
  return (
    RESOLUTION_PRESETS.find((r) => r.w === width && r.h === height) ?? RESOLUTION_PRESETS[0]
  );
}

export const FPS_PRESETS = [24, 30, 60] as const;
export const CODEC_PRESETS = ["h264", "h265"] as const;
export type Codec = (typeof CODEC_PRESETS)[number];

export const SCRIPT_MODELS = [
  "openai/gpt-4o-mini",
  "openai/gpt-4o",
  "anthropic/claude-3.5-sonnet",
  "google/gemini-2.0-flash-exp:free",
];

export const STYLE_REFS = [
  { value: "none",     label: "None — parallel, each panel independent" },
  { value: "anchor",   label: "Anchor — panel 0 referenced by all others" },
  { value: "previous", label: "Previous — cumulative (panel N gets refs 0..N-1)" },
] as const;

export type ImageModelOption = { slug: string; label: string; refs: boolean };

export const IMAGE_MODELS: ImageModelOption[] = [
  { slug: "fal-ai/flux/schnell",                       label: "Flux Schnell — $0.003 (text only)",       refs: false },
  { slug: "fal-ai/flux/dev",                           label: "Flux Dev — $0.025 (text only)",           refs: false },
  { slug: "fal-ai/flux-2/edit",                        label: "Flux 2 Edit dev — $0.013 (up to 10 refs)", refs: true },
  { slug: "fal-ai/flux-2-pro/edit",                    label: "Flux 2 Edit pro — $0.032 (10 refs)",      refs: true },
  { slug: "fal-ai/flux-pro/kontext",                   label: "Flux Kontext — $0.04 (1 ref)",            refs: true },
  { slug: "fal-ai/flux-pro/kontext/multi",             label: "Flux Kontext multi — $0.04 (4 refs)",     refs: true },
  { slug: "fal-ai/gpt-image-1.5/edit",                 label: "GPT Image 1.5 edit (low) — $0.013",       refs: true },
  { slug: "fal-ai/bytedance/seedream/v4/text-to-image", label: "Seedream v4 t2i — $0.03",                refs: false },
  { slug: "fal-ai/bytedance/seedream/v4/edit",         label: "Seedream v4 edit — $0.03 (10 refs)",      refs: true },
  { slug: "fal-ai/nano-banana-2",                      label: "Nano Banana 2 — $0.08",                   refs: true },
  { slug: "fal-ai/nano-banana-pro/edit",               label: "Nano Banana Pro edit — $0.15 (top)",      refs: true },
];
