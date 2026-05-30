// Bundles Root.tsx and renders the Comic composition to MP4.
import { bundle } from "@remotion/bundler";
import { renderMedia, selectComposition } from "@remotion/renderer";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { writeFile, mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import { ObjectStore } from "./minio.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

type TransitionSpec = Record<string, unknown>;
type Effect = Record<string, unknown>;
type CaptionSpec = { text: string; position?: string; style?: Record<string, unknown> };

export type AssembleInput = {
  runId: string;
  stepIndex: number;
  panels: {
    index: number;
    objectKey: string;
    durationMs: number;
    transition: string;
    transition_in?: TransitionSpec;
    effects?: Effect[];
    caption?: CaptionSpec;
  }[];
  outputKey: string;
  width: number;
  height: number;
  fps: number;
  audioKey?: string;
  musicKey?: string;
  ambientKey?: string;
  sfxKeys?: Record<string, string>; // panel_index → object_key (JSON keys are strings)
};

let bundleCache: string | null = null;

async function ensureBundle(): Promise<string> {
  if (bundleCache) return bundleCache;
  bundleCache = await bundle({
    entryPoint: path.join(__dirname, "Root.tsx"),
    webpackOverride: (c) => c,
  });
  return bundleCache;
}

export async function assemble(
  input: AssembleInput,
  store: ObjectStore,
): Promise<{ bytes: number; outputKey: string }> {
  const tmp = await mkdtemp(path.join(os.tmpdir(), "comic-"));
  try {
    // Encode panels as data URIs — Chromium blocks file:// URLs for assets.
    const panelProps = [] as Array<{
      index: number;
      src: string;
      durationMs: number;
      transition: string;
      transition_in?: TransitionSpec;
      effects?: Effect[];
      caption?: CaptionSpec;
    }>;
    for (const p of input.panels) {
      const buf = await store.getBuffer(p.objectKey);
      const mime = p.objectKey.endsWith(".png") ? "image/png" : "image/jpeg";
      panelProps.push({
        index: p.index,
        src: `data:${mime};base64,${buf.toString("base64")}`,
        durationMs: p.durationMs,
        transition: p.transition,
        transition_in: p.transition_in,
        effects: p.effects,
        caption: p.caption,
      });
    }
    let audioSrc = "";
    if (input.audioKey) {
      const buf = await store.getBuffer(input.audioKey);
      audioSrc = `data:audio/mpeg;base64,${buf.toString("base64")}`;
    }
    let musicSrc = "";
    if (input.musicKey) {
      const buf = await store.getBuffer(input.musicKey);
      musicSrc = `data:audio/mpeg;base64,${buf.toString("base64")}`;
    }
    let ambientSrc = "";
    if (input.ambientKey) {
      const buf = await store.getBuffer(input.ambientKey);
      ambientSrc = `data:audio/mpeg;base64,${buf.toString("base64")}`;
    }
    // sfxByPanel[index] = data URI; downloaded once per unique object_key.
    const sfxByPanel: Record<number, string> = {};
    if (input.sfxKeys) {
      const cache = new Map<string, string>();
      for (const [idxStr, key] of Object.entries(input.sfxKeys)) {
        if (!key) continue;
        let uri = cache.get(key);
        if (!uri) {
          const buf = await store.getBuffer(key);
          uri = `data:audio/mpeg;base64,${buf.toString("base64")}`;
          cache.set(key, uri);
        }
        sfxByPanel[Number(idxStr)] = uri;
      }
    }
    const serveUrl = await ensureBundle();
    const inputProps = {
      panels: panelProps,
      width: input.width,
      height: input.height,
      fps: input.fps,
      audioSrc,
      musicSrc,
      ambientSrc,
      sfxByPanel,
    };
    // Chromium bring-up under concurrent load occasionally hangs the
    // webpack server before serveUrl is reachable. Retry the whole render up
    // to 3 times with growing backoff before surfacing as failed.
    const outFile = path.join(tmp, "video.mp4");
    let lastErr: unknown = undefined;
    for (let attempt = 1; attempt <= 3; attempt++) {
      try {
        const composition = await selectComposition({
          serveUrl,
          id: "Comic",
          inputProps,
          timeoutInMilliseconds: 60_000,
        });
        await renderMedia({
          composition,
          serveUrl,
          codec: "h264",
          outputLocation: outFile,
          inputProps,
          timeoutInMilliseconds: 120_000,
        });
        lastErr = undefined;
        break;
      } catch (e) {
        lastErr = e;
        if (attempt < 3) {
          await new Promise((r) => setTimeout(r, 1500 * attempt));
        }
      }
    }
    if (lastErr) throw lastErr;
    const { readFile, stat } = await import("node:fs/promises");
    const data = await readFile(outFile);
    const st = await stat(outFile);
    await store.putBuffer(input.outputKey, data, "video/mp4");
    return { bytes: st.size, outputKey: input.outputKey };
  } finally {
    await rm(tmp, { recursive: true, force: true });
  }
}
