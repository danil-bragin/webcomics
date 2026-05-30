"""pipeline.audio.requested handler — TTS the panel captions.

Per-panel synthesis + silence padding so the final voiceover spans the full
assemble duration. Without padding, a 12s video with 5s of dialog would have
seven seconds of dead air at the end.
"""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import tempfile
import time
from typing import Any

import structlog

from worker.providers.elevenlabs import ElevenLabsClient
from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore

log = structlog.get_logger().bind(service="workers-py", worker="audio")

REQUEST_STREAM = "pipeline.audio.requested"
COMPLETED_STREAM = "pipeline.audio.completed"
FAILED_STREAM = "pipeline.audio.failed"

# Head-of-panel breathing room before the line is read.
_PANEL_INTRO_MS = 200


class AudioHandler:
    def __init__(self, bus: Bus, el: ElevenLabsClient, store: ObjectStore, cancelled: CancelledRuns) -> None:
        self.bus = bus
        self.el = el
        self.store = store
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        run_id = str(msg.get("run_id", ""))
        step_index = int(msg.get("step_index", 0))
        if self.cancelled.contains(run_id):
            log.info("skipping cancelled run", run_id=run_id, step_index=step_index)
            return
        captions = list(msg.get("captions") or [])
        prompt = str(msg.get("prompt") or "")
        # Per-panel timing: when set we synthesise each line separately and pad
        # to its panel slot; otherwise fall back to the old single-shot TTS.
        panel_duration_ms = int(msg.get("panel_duration_ms") or 0)
        panel_count = int(msg.get("panel_count") or len(captions))
        language = (msg.get("language") or "en").lower()
        if language not in ("en", "ru", "fr"):
            language = "en"
        text_fallback = ". ".join(c for c in captions if c) or prompt
        if not (captions or text_fallback.strip()):
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index, "error": "no text to synthesize",
            })
            return

        output_key = msg.get("output_key") or f"runs/{run_id}/{step_index}/audio.mp3"
        params = dict(msg.get("params") or {})
        voice_id = params.get("voice_id") or None
        model_id = msg.get("model") or params.get("model") or None
        speed = params.get("speed")

        ctx = log.bind(run_id=run_id, step_index=step_index, step_type="audio",
                       panel_count=panel_count, panel_duration_ms=panel_duration_ms,
                       language=language)
        start = time.perf_counter()
        try:
            if panel_duration_ms > 0 and captions:
                data, cost = await self._synthesize_per_panel(
                    captions, panel_duration_ms,
                    voice_id=voice_id, model_id=model_id, speed=speed,
                )
            else:
                data, cost = await self.el.synthesize(
                    text_fallback,
                    voice_id=voice_id, model_id=model_id,
                    speed=float(speed) if speed is not None else None,
                )
        except Exception as e:
            ctx.exception("audio synth failed")
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index, "error": str(e),
            })
            return

        self.store.put_bytes(output_key, data, "audio/mpeg")
        duration_ms = int((time.perf_counter() - start) * 1000)
        await self.bus.publish(COMPLETED_STREAM, {
            "run_id": run_id,
            "step_index": step_index,
            "object_key": output_key,
            "bucket": self.store.bucket,
            "cost": cost,
            "duration_ms": duration_ms,
        })
        ctx.info("audio done", chars=sum(len(c) for c in captions),
                 duration_ms=duration_ms, cost_usd=cost["total_cost_usd"])

    async def _synthesize_per_panel(
        self,
        captions: list[str],
        panel_duration_ms: int,
        voice_id: str | None,
        model_id: str | None,
        speed: Any,
    ) -> tuple[bytes, dict]:
        """One TTS call per caption, then ffmpeg stitches them with silence
        padding so segment i lands in slot i of the assemble timeline."""
        speed_val = float(speed) if speed is not None else None
        per_panel_mp3s: list[bytes] = []
        total_units = 0.0
        total_cost = 0.0
        model_label = ""
        for cap in captions:
            text = (cap or "").strip()
            if not text:
                per_panel_mp3s.append(b"")
                continue
            data, cost = await self.el.synthesize(
                text, voice_id=voice_id, model_id=model_id, speed=speed_val,
            )
            per_panel_mp3s.append(data)
            total_units += float(cost.get("units", 0))
            total_cost += float(cost.get("total_cost_usd", 0))
            model_label = cost.get("model") or model_label

        merged = _stitch_to_panels(per_panel_mp3s, panel_duration_ms)
        cost = {
            "provider": "elevenlabs",
            "model": model_label or "eleven_flash_v2_5",
            "units": total_units,
            "unit_label": "characters",
            "unit_cost_usd": (total_cost / total_units) if total_units else 0.0,
            "total_cost_usd": total_cost,
        }
        return merged, cost


def _stitch_to_panels(panel_mp3s: list[bytes], panel_duration_ms: int) -> bytes:
    """Render each panel as exactly panel_duration_ms of audio: TTS bytes
    followed by enough silence to fill the slot. Pre-renders one shared silence
    track of the panel length so each segment is reliably the same size."""
    ffmpeg = shutil.which("ffmpeg")
    if not ffmpeg:
        return b"".join(p for p in panel_mp3s if p)

    work = tempfile.mkdtemp(prefix="audio-stitch-")
    try:
        target_s = panel_duration_ms / 1000.0
        head_delay_s = _PANEL_INTRO_MS / 1000.0

        # Generate a silence track exactly one panel long. Reused for empty
        # captions + tail padding.
        silence_path = os.path.join(work, "silence.mp3")
        subprocess.run(
            [ffmpeg, "-y", "-f", "lavfi", "-i",
             "anullsrc=channel_layout=mono:sample_rate=44100",
             "-t", f"{target_s}", "-c:a", "libmp3lame", "-b:a", "128k",
             silence_path],
            check=False, capture_output=True,
        )

        padded_paths: list[str] = []
        for i, data in enumerate(panel_mp3s):
            dst_path = os.path.join(work, f"pad-{i}.mp3")
            if not data:
                # Empty caption — entire panel is silence.
                subprocess.run(["cp", silence_path, dst_path], check=False)
                padded_paths.append(dst_path)
                continue

            src_path = os.path.join(work, f"src-{i}.mp3")
            with open(src_path, "wb") as f:
                f.write(data)

            # Stretch the TTS to exactly target_s by mixing it on top of the
            # silence track, then trimming to length. amix gracefully handles
            # a TTS shorter than the silence (silence carries the tail).
            # adelay pushes the voiceover to head_delay_s so the panel can
            # establish before the line starts.
            head_ms = int(head_delay_s * 1000)
            filter_complex = (
                f"[0:a]adelay={head_ms}:all=1[v];"
                f"[v][1:a]amix=inputs=2:duration=longest:weights=1 0.001[mix]"
            )
            r = subprocess.run(
                [ffmpeg, "-y",
                 "-i", src_path,
                 "-i", silence_path,
                 "-filter_complex", filter_complex,
                 "-map", "[mix]",
                 "-t", f"{target_s}",
                 "-c:a", "libmp3lame", "-b:a", "128k",
                 dst_path],
                check=False, capture_output=True, text=True,
            )
            if r.returncode != 0 or not os.path.exists(dst_path):
                # Fallback: ignore stretching, copy raw TTS only.
                subprocess.run(["cp", src_path, dst_path], check=False)
            padded_paths.append(dst_path)

        concat_list = os.path.join(work, "list.txt")
        with open(concat_list, "w") as f:
            for p in padded_paths:
                f.write(f"file '{p}'\n")
        out_path = os.path.join(work, "out.mp3")
        subprocess.run(
            [ffmpeg, "-y", "-f", "concat", "-safe", "0", "-i", concat_list,
             "-c:a", "libmp3lame", "-b:a", "128k", out_path],
            check=False, capture_output=True,
        )
        with open(out_path, "rb") as f:
            return f.read()
    finally:
        shutil.rmtree(work, ignore_errors=True)


def _parse_duration_seconds(ffmpeg_stderr: str) -> float:
    m = re.search(r"Duration: (\d+):(\d+):(\d+\.\d+)", ffmpeg_stderr or "")
    if not m:
        return 0.0
    h, m_, s = m.groups()
    return int(h) * 3600 + int(m_) * 60 + float(s)
