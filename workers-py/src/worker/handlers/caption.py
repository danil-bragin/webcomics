"""pipeline.caption.requested — compose social-post captions via LLM."""
from __future__ import annotations

import time
from typing import Any

import structlog

from worker.providers.openrouter import OpenRouterClient
from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore

log = structlog.get_logger().bind(service="workers-py", worker="caption")

REQUEST_STREAM = "pipeline.caption.requested"
COMPLETED_STREAM = "pipeline.caption.completed"
FAILED_STREAM = "pipeline.caption.failed"


class CaptionHandler:
    """Composes per-platform captions and forwards them to the upload step.

    Takes the script panels + plot + platforms from the event payload, calls
    OpenRouter, publishes a structured per-platform map. If a `caption_override`
    is set in params, skips the LLM entirely and uses it as the generic
    caption for every platform — gives the user a quick manual escape hatch.
    """

    def __init__(self, bus: Bus, llm: OpenRouterClient, store: ObjectStore, cancelled: CancelledRuns) -> None:
        self.bus = bus
        self.llm = llm
        self.store = store
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        run_id = str(msg.get("run_id", ""))
        step_index = int(msg.get("step_index", 0))
        if self.cancelled.contains(run_id):
            log.info("skipping cancelled run", run_id=run_id, step_index=step_index)
            return

        params = dict(msg.get("params") or {})
        # Default to all selenium-supported surfaces so a single run carries
        # metadata for whichever account it's later scheduled onto.
        platforms = params.get("platforms") or ["youtube", "instagram", "tiktok", "facebook"]
        if not isinstance(platforms, list):
            platforms = ["youtube"]
        caption_override = params.get("caption_override")

        plot = msg.get("plot") or {}
        plot_premise = ""
        plot_beats: list[dict] = []
        if isinstance(plot, dict):
            plot_premise = plot.get("premise", "") or ""
            beats_raw = plot.get("beats")
            if isinstance(beats_raw, list):
                plot_beats = beats_raw

        characters = msg.get("characters") or []
        panels = msg.get("panels") or []

        ctx = log.bind(run_id=run_id, step_index=step_index, step_type="caption")
        start = time.perf_counter()

        if caption_override:
            # Skip the LLM — copy the same string into every requested platform
            # so downstream upload step doesn't have to branch on it.
            captions = {p: {"caption": caption_override, "title": caption_override[:60], "hashtags": []} for p in platforms}
            cost = {
                "provider": "manual",
                "model": "user-supplied",
                "units": 0,
                "unit_label": "tokens",
                "unit_cost_usd": 0,
                "total_cost_usd": 0,
            }
            duration_ms = int((time.perf_counter() - start) * 1000)
            await self.bus.publish(COMPLETED_STREAM, {
                "run_id": run_id, "step_index": step_index,
                "captions": captions, "bucket": self.store.bucket,
                "cost": cost, "duration_ms": duration_ms,
            })
            ctx.info("caption from override", duration_ms=duration_ms)
            return

        model = msg.get("model") or None
        language = (msg.get("language") or "en").lower()
        if language not in ("en", "ru", "fr"):
            language = "en"
        try:
            captions, usage = await self.llm.generate_caption(
                panels=panels,
                platforms=platforms,
                plot_premise=plot_premise,
                plot_beats=plot_beats,
                characters=characters,
                model=model,
                language=language,
            )
        except Exception as e:
            ctx.exception("caption gen failed")
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index, "error": str(e),
            })
            return

        cost_total = usage.get("cost_usd")
        total_tokens = usage.get("total_tokens", 0)
        if cost_total is None:
            cost_total = round(total_tokens * 0.0000004, 6)
        unit_cost = round(cost_total / max(total_tokens, 1), 8) if total_tokens else 0
        cost = {
            "provider": "openrouter",
            "model": usage.get("model", ""),
            "units": float(total_tokens),
            "unit_label": "tokens",
            "unit_cost_usd": float(unit_cost),
            "total_cost_usd": float(cost_total),
        }
        duration_ms = int((time.perf_counter() - start) * 1000)

        # New LLM schema: {audience, hook, platforms{platform: {title,description,tags,hashtags,caption}}}.
        # Old downstream code reads captions[platform], so flatten both shapes
        # into the published payload — preserves backward compatibility while
        # also exposing the structured "metadata" block consumers can backfill
        # UploadRecord rows with.
        audience = (captions or {}).get("audience") or {}
        hook = (captions or {}).get("hook") or ""
        per_platform = (captions or {}).get("platforms") or {}
        if not per_platform:
            # Fallback: model returned old flat shape — treat the whole map as
            # per-platform.
            per_platform = {k: v for k, v in (captions or {}).items() if isinstance(v, dict)}
        flattened: dict[str, Any] = dict(per_platform)
        # Also drop the legacy key the upload handler uses.
        if "youtube_shorts" in per_platform and "youtube" not in flattened:
            flattened["youtube"] = per_platform["youtube_shorts"]

        await self.bus.publish(COMPLETED_STREAM, {
            "run_id": run_id, "step_index": step_index,
            "captions": flattened,
            "metadata": {
                "audience": audience,
                "hook": hook,
                "platforms": per_platform,
            },
            "bucket": self.store.bucket,
            "cost": cost, "duration_ms": duration_ms,
        })
        ctx.info("caption done",
                 platforms=list(per_platform.keys()),
                 duration_ms=duration_ms, cost_usd=cost_total,
                 made_for_kids=audience.get("made_for_kids"))
