import React from "react";
import {
  AbsoluteFill,
  Audio,
  Img,
  Sequence,
  Easing,
  interpolate,
  useCurrentFrame,
} from "remotion";

// Backend wire format. Two shapes are accepted:
//
//   legacy:  panels=[{index, src, durationMs, transition: "crossfade"|"slide"|"none"}]
//   timeline: panels=[{index, src, durationMs, transition_in: {type, duration_ms, easing, direction}, effects: [...]}]
//
// Backward compat lets the old simple assemble step keep working while the
// timeline editor builds the richer shape.

export type EasingName = "linear" | "ease-in" | "ease-out" | "ease-in-out" | "cubic";
export type Direction = "left" | "right" | "up" | "down";
export type TransitionType =
  | "none"
  | "fade"
  | "crossfade"
  | "slide"
  | "push"
  | "zoom"
  | "wipe";

export type TransitionSpec = {
  type: TransitionType;
  duration_ms?: number;
  easing?: EasingName;
  direction?: Direction;
};

export type KenBurnsEffect = {
  type: "ken_burns";
  zoom?: [number, number];
  pan?: [[number, number], [number, number]]; // [[x0,y0], [x1,y1]] in pixels
};

export type ShakeEffect = {
  type: "shake";
  intensity?: number;
  at_ms?: number;
  duration_ms?: number;
};

export type Effect = KenBurnsEffect | ShakeEffect;

export type SubtitlePreset = "bottom_karaoke" | "impact_meme" | "word_pop";

export type CaptionSpec = {
  text: string;
  position?: "top" | "bottom" | "center";
  style?: React.CSSProperties;
  style_preset?: SubtitlePreset;
};

export type Panel = {
  index: number;
  src: string;
  durationMs: number;
  // legacy
  transition?: string;
  // new
  transition_in?: TransitionSpec;
  effects?: Effect[];
  caption?: CaptionSpec;
};

export type ComicProps = {
  panels: Panel[];
  width: number;
  height: number;
  fps: number;
  audioSrc?: string;
  musicSrc?: string;
  ambientSrc?: string;
  sfxByPanel?: Record<number, string>;
};

const easingFn = (name: EasingName | undefined): ((n: number) => number) => {
  switch (name) {
    case "ease-in": return Easing.in(Easing.cubic);
    case "ease-out": return Easing.out(Easing.cubic);
    case "ease-in-out": return Easing.inOut(Easing.cubic);
    case "cubic": return Easing.cubic;
    case "linear":
    default:
      return Easing.linear;
  }
};

const resolveTransition = (panel: Panel): TransitionSpec => {
  if (panel.transition_in) return panel.transition_in;
  // Legacy string mapping.
  switch (panel.transition) {
    case "crossfade": return { type: "crossfade", duration_ms: 260, easing: "ease-in-out" };
    case "slide": return { type: "slide", duration_ms: 320, easing: "ease-out", direction: "left" };
    case "fade": return { type: "fade", duration_ms: 260, easing: "ease-in-out" };
    case "none":
    default:
      return { type: "none" };
  }
};

const KenBurnsLayer: React.FC<{ effect?: KenBurnsEffect; durationFrames: number; children: React.ReactNode }> =
  ({ effect, durationFrames, children }) => {
    const frame = useCurrentFrame();
    const zoomStart = effect?.zoom?.[0] ?? 1.0;
    const zoomEnd = effect?.zoom?.[1] ?? 1.08;
    const panStartX = effect?.pan?.[0]?.[0] ?? 0;
    const panStartY = effect?.pan?.[0]?.[1] ?? 0;
    const panEndX = effect?.pan?.[1]?.[0] ?? 0;
    const panEndY = effect?.pan?.[1]?.[1] ?? -10;
    const scale = interpolate(frame, [0, durationFrames], [zoomStart, zoomEnd], { extrapolateRight: "clamp" });
    const tx = interpolate(frame, [0, durationFrames], [panStartX, panEndX], { extrapolateRight: "clamp" });
    const ty = interpolate(frame, [0, durationFrames], [panStartY, panEndY], { extrapolateRight: "clamp" });
    return (
      <div style={{ width: "100%", height: "100%", transform: `scale(${scale}) translate(${tx}px, ${ty}px)`, transformOrigin: "center" }}>
        {children}
      </div>
    );
  };

const Shake: React.FC<{ effect: ShakeEffect; fps: number; children: React.ReactNode }> = ({ effect, fps, children }) => {
  const frame = useCurrentFrame();
  const startFrame = Math.round(((effect.at_ms ?? 0) / 1000) * fps);
  const durationFrames = Math.max(1, Math.round(((effect.duration_ms ?? 300) / 1000) * fps));
  const intensity = effect.intensity ?? 0.4;
  if (frame < startFrame || frame > startFrame + durationFrames) {
    return <>{children}</>;
  }
  const t = (frame - startFrame) / durationFrames;
  const decay = 1 - t;
  const tx = Math.sin(frame * 1.7) * intensity * 12 * decay;
  const ty = Math.cos(frame * 2.1) * intensity * 12 * decay;
  return <div style={{ transform: `translate(${tx}px, ${ty}px)` }}>{children}</div>;
};

// Returns the (offsetX, offsetY, opacity) for the in-transition.
const transitionStyle = (
  tr: TransitionSpec,
  panelFrame: number,
  fps: number,
): React.CSSProperties => {
  const durFrames = Math.max(1, Math.round(((tr.duration_ms ?? 280) / 1000) * fps));
  if (panelFrame >= durFrames) return {};
  const ease = easingFn(tr.easing);
  const progress = ease(Math.max(0, Math.min(1, panelFrame / durFrames)));
  switch (tr.type) {
    case "fade":
    case "crossfade":
      return { opacity: progress };
    case "slide": {
      const sign = tr.direction === "right" ? -1 : tr.direction === "down" ? -1 : 1;
      const axis = tr.direction === "up" || tr.direction === "down" ? "Y" : "X";
      const offset = sign * (1 - progress) * 100;
      return { transform: `translate${axis}(${offset}%)` };
    }
    case "push": {
      const sign = tr.direction === "right" ? -1 : tr.direction === "down" ? -1 : 1;
      const axis = tr.direction === "up" || tr.direction === "down" ? "Y" : "X";
      const offset = sign * (1 - progress) * 100;
      return { transform: `translate${axis}(${offset}%)`, opacity: progress };
    }
    case "zoom": {
      const scale = 0.6 + 0.4 * progress;
      return { transform: `scale(${scale})`, opacity: progress };
    }
    case "wipe": {
      // Reveal via clip-path. Direction picks the side.
      const pct = (1 - progress) * 100;
      switch (tr.direction) {
        case "right": return { clipPath: `inset(0 0 0 ${pct}%)` };
        case "up":    return { clipPath: `inset(${pct}% 0 0 0)` };
        case "down":  return { clipPath: `inset(0 0 ${pct}% 0)` };
        case "left":
        default:      return { clipPath: `inset(0 ${pct}% 0 0)` };
      }
    }
    case "none":
    default:
      return {};
  }
};

const PRESET_STYLES: Record<SubtitlePreset, React.CSSProperties> = {
  // TikTok / Shorts standard: white body, thick black outline, bold sans-serif,
  // safe-zone bottom (mobile UI overlays bottom 30%).
  bottom_karaoke: {
    fontFamily: "Inter, Helvetica, Arial, sans-serif",
    fontWeight: 800,
    fontSize: 56,
    lineHeight: 1.15,
    color: "#FFFFFF",
    WebkitTextStroke: "3px #000000",
    textShadow: "0 0 12px rgba(0,0,0,0.85)",
    letterSpacing: 0,
  },
  // Classic meme: Impact, all-caps, white + heavy stroke.
  impact_meme: {
    fontFamily: "Impact, Haettenschweiler, Anton, sans-serif",
    fontWeight: 700,
    fontSize: 72,
    lineHeight: 1.0,
    color: "#FFFFFF",
    WebkitTextStroke: "4px #000000",
    textShadow: "0 4px 12px rgba(0,0,0,0.9)",
    textTransform: "uppercase",
    letterSpacing: 2,
  },
  // Word-pop reuses bottom_karaoke base; per-word animation is layered above.
  word_pop: {
    fontFamily: "Inter, Helvetica, Arial, sans-serif",
    fontWeight: 800,
    fontSize: 60,
    lineHeight: 1.1,
    color: "#FFFFFF",
    WebkitTextStroke: "3px #000000",
    textShadow: "0 0 14px rgba(0,0,0,0.9)",
  },
};

const POSITION_STYLES = (position: CaptionSpec["position"]): React.CSSProperties => {
  // YT Shorts UI overlays bottom ~30% with the share/like/scroll affordances —
  // place captions at ~22% from the bottom to stay readable.
  switch (position) {
    case "top": return { top: "10%" };
    case "center": return { top: "50%", transform: "translateY(-50%)" };
    default: return { bottom: "22%" };
  }
};

const WordPopText: React.FC<{ text: string; durationFrames: number }> = ({ text, durationFrames }) => {
  const frame = useCurrentFrame();
  const words = text.trim().split(/\s+/);
  const perWordFrames = Math.max(1, Math.floor(durationFrames / Math.max(1, words.length)));
  return (
    <>
      {words.map((w, i) => {
        const start = i * perWordFrames;
        const t = Math.max(0, Math.min(1, (frame - start) / 10));
        const scale = 0.6 + 0.4 * t;
        const opacity = t;
        return (
          <span key={i} style={{ display: "inline-block", margin: "0 0.25em", transform: `scale(${scale})`, opacity }}>
            {w}
          </span>
        );
      })}
    </>
  );
};

const Caption: React.FC<{ spec: CaptionSpec; durationFrames: number }> = ({ spec, durationFrames }) => {
  const preset = spec.style_preset ?? "bottom_karaoke";
  const baseStyle: React.CSSProperties = {
    position: "absolute",
    left: 0,
    right: 0,
    textAlign: "center",
    padding: "0 6%",
    pointerEvents: "none",
    zIndex: 10,
    ...PRESET_STYLES[preset],
  };
  const pos = POSITION_STYLES(spec.position);
  return (
    <div style={{ ...baseStyle, ...pos, ...(spec.style ?? {}) }}>
      {preset === "word_pop"
        ? <WordPopText text={spec.text} durationFrames={durationFrames} />
        : spec.text}
    </div>
  );
};

const PanelClip: React.FC<{ panel: Panel; durationFrames: number; fps: number }> = ({ panel, durationFrames, fps }) => {
  const frame = useCurrentFrame();
  const tr = resolveTransition(panel);
  const style = transitionStyle(tr, frame, fps);
  // If the user supplied an effects array (even empty), respect it as-is.
  // Only fall back to the subtle default Ken Burns when effects is omitted.
  const effectsExplicit = panel.effects !== undefined;
  const effects = panel.effects ?? [];
  const kb = effects.find((e): e is KenBurnsEffect => e.type === "ken_burns");
  const shakes = effects.filter((e): e is ShakeEffect => e.type === "shake");
  let inner: React.ReactNode = (
    <Img src={panel.src} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
  );
  if (kb || !effectsExplicit) {
    inner = <KenBurnsLayer effect={kb} durationFrames={durationFrames}>{inner}</KenBurnsLayer>;
  }
  for (const s of shakes) {
    inner = <Shake key={`shake-${s.at_ms ?? 0}`} effect={s} fps={fps}>{inner}</Shake>;
  }
  return (
    <AbsoluteFill style={{ backgroundColor: "#000", ...style }}>
      {inner}
      {panel.caption ? <Caption spec={panel.caption} durationFrames={durationFrames} /> : null}
    </AbsoluteFill>
  );
};

export const Comic: React.FC<ComicProps> = ({ panels, fps, audioSrc, musicSrc, ambientSrc, sfxByPanel }) => {
  let cursor = 0;
  const sfxSequences: React.ReactNode[] = [];
  return (
    <AbsoluteFill style={{ backgroundColor: "#000" }}>
      {panels.map((p) => {
        const durationFrames = Math.max(1, Math.round((p.durationMs / 1000) * fps));
        const node = (
          <Sequence key={p.index} from={cursor} durationInFrames={durationFrames}>
            <PanelClip panel={p} durationFrames={durationFrames} fps={fps} />
          </Sequence>
        );
        // Per-panel SFX: drop the sound at the start of the panel slot. Short
        // clips end on their own; long clips clip at the panel boundary.
        const sfxSrc = sfxByPanel?.[p.index];
        if (sfxSrc) {
          sfxSequences.push(
            <Sequence key={`sfx-${p.index}`} from={cursor} durationInFrames={durationFrames}>
              <Audio src={sfxSrc} volume={0.7} />
            </Sequence>
          );
        }
        cursor += durationFrames;
        return node;
      })}
      {ambientSrc ? <Audio src={ambientSrc} volume={0.18} loop /> : null}
      {musicSrc ? <Audio src={musicSrc} volume={0.3} loop /> : null}
      {audioSrc ? <Audio src={audioSrc} volume={1.0} /> : null}
      {sfxSequences}
    </AbsoluteFill>
  );
};

export const totalDurationFrames = (panels: Panel[], fps: number): number => {
  let total = 0;
  for (const p of panels) total += Math.max(1, Math.round((p.durationMs / 1000) * fps));
  return total;
};
