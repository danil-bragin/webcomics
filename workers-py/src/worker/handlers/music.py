"""pipeline.music.requested — pick a track from the curated CC0 library.

Reads `library/music/manifest.json` from MinIO, asks the LLM to pick a track id
matching the script's tone, and publishes the chosen object_key as the music
asset for the next assemble step.
"""
from __future__ import annotations

import json
import os
import random
import time
from typing import Any

import httpx
import structlog

from worker.providers.openrouter import OpenRouterClient
from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore

log = structlog.get_logger().bind(service="workers-py", worker="music")

REQUEST_STREAM = "pipeline.music.requested"
COMPLETED_STREAM = "pipeline.music.completed"
FAILED_STREAM = "pipeline.music.failed"

MANIFEST_KEY = "library/music/manifest.json"

# Worker fetches the DB-backed audio library when WEB_API_URL is set so that
# tracks uploaded via /library/audio show up here without a redeploy. Falls
# back to the static MinIO manifest when the API is unreachable or empty.
WEB_API_URL = os.environ.get("WEB_API_URL", "http://localhost:8080")
WEB_API_KEY = os.environ.get("API_KEY", "")


class MusicHandler:
    """Selects (not generates) a music track via an LLM mood-match prompt."""

    def __init__(self, bus: Bus, llm: OpenRouterClient, store: ObjectStore, cancelled: CancelledRuns) -> None:
        self.bus = bus
        self.llm = llm
        self.store = store
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        run_id = str(msg.get("run_id", ""))
        step_index = int(msg.get("step_index", 0))
        if self.cancelled.contains(run_id):
            log.info("skipping cancelled run", run_id=run_id)
            return

        params = dict(msg.get("params") or {})
        preferred_mood = params.get("preferred_mood") or ""
        track_id_override = params.get("track_id") or ""
        project_id = msg.get("project_id") or params.get("project_id") or ""
        panels = msg.get("panels") or []
        plot = msg.get("plot") or {}
        plot_premise = plot.get("premise", "") if isinstance(plot, dict) else ""

        ctx = log.bind(run_id=run_id, step_index=step_index, step_type="music")
        start = time.perf_counter()

        manifest = _load_db_catalog(project_id, ctx)
        if not manifest:
            try:
                manifest_bytes = self.store.client.get_object(self.store.bucket, MANIFEST_KEY).read()
                manifest = json.loads(manifest_bytes)
            except Exception as e:
                ctx.warning("music manifest + DB empty — fallback to no music", err=str(e))
                await self.bus.publish(COMPLETED_STREAM, {
                    "run_id": run_id, "step_index": step_index,
                    "object_key": "", "bucket": self.store.bucket,
                    "cost": _zero_cost(), "duration_ms": 0,
                })
                return

        if not isinstance(manifest, list) or not manifest:
            ctx.warning("manifest empty")
            await self.bus.publish(COMPLETED_STREAM, {
                "run_id": run_id, "step_index": step_index,
                "object_key": "", "bucket": self.store.bucket,
                "cost": _zero_cost(), "duration_ms": 0,
            })
            return

        # Explicit track_id override — short-circuit LLM + mood logic.
        if track_id_override:
            forced = next((t for t in manifest if t.get("id") == track_id_override), None)
            if forced is not None:
                ctx.info("music forced by track_id", track_id=track_id_override)
                await self.bus.publish(COMPLETED_STREAM, {
                    "run_id": run_id, "step_index": step_index,
                    "object_key": forced.get("object_key", ""),
                    "bucket": self.store.bucket,
                    "track_id": forced.get("id", ""),
                    "reasoning": f"forced track_id={track_id_override}",
                    "cost": _zero_cost(),
                    "duration_ms": int((time.perf_counter() - start) * 1000),
                })
                return

        chosen, reasoning, cost = await self._pick(
            preferred_mood=str(preferred_mood),
            manifest=manifest,
            panels=panels,
            plot_premise=str(plot_premise),
            params=params,
        )
        ctx.info("music picked", track_id=chosen.get("id"), reasoning=reasoning[:120])

        duration_ms = int((time.perf_counter() - start) * 1000)
        await self.bus.publish(COMPLETED_STREAM, {
            "run_id": run_id, "step_index": step_index,
            "object_key": chosen.get("object_key", ""),
            "bucket": self.store.bucket,
            "track_id": chosen.get("id", ""),
            "reasoning": reasoning,
            "cost": cost,
            "duration_ms": duration_ms,
        })

    async def _pick(
        self,
        preferred_mood: str,
        manifest: list[dict],
        panels: list[dict],
        plot_premise: str,
        params: dict,
    ) -> tuple[dict, str, dict]:
        # Project override: preferred_mood skips the LLM and picks first match.
        if preferred_mood:
            for t in manifest:
                if preferred_mood.lower() in [m.lower() for m in (t.get("mood") or [])]:
                    return t, f"matched preferred_mood='{preferred_mood}'", _zero_cost()

        # No OpenRouter key → random.
        if not self.llm.client.api_key:
            choice = random.choice(manifest)
            return choice, "random (LLM disabled)", _zero_cost()

        script_text = "\n".join(
            f"- {p.get('caption') or p.get('prompt', '')}"
            for p in panels[:6]
        )
        catalog = "\n".join(
            f"- {t['id']}: mood={t.get('mood')} genre={t.get('genre')} tempo={t.get('tempo')}"
            for t in manifest
        )
        system = (
            "You are a music supervisor for a short comic video. Pick ONE track id from the catalog "
            "that best matches the script's emotional tone and pacing. Reply STRICT JSON: "
            '{"track_id":"<id>","reasoning":"one sentence"}.'
        )
        user = (
            (f"Premise: {plot_premise}\n\n" if plot_premise else "")
            + (f"Panels:\n{script_text}\n\n" if script_text else "")
            + f"Catalog:\n{catalog}"
        )
        try:
            resp = await self.llm.client.chat.completions.create(
                model=self.llm.default_model,
                messages=[{"role": "system", "content": system}, {"role": "user", "content": user}],
                response_format={"type": "json_object"},
                temperature=0.4,
            )
            content = resp.choices[0].message.content or "{}"
            parsed = json.loads(content)
            picked_id = str(parsed.get("track_id", ""))
            reasoning = str(parsed.get("reasoning", ""))
            chosen = next((t for t in manifest if t["id"] == picked_id), None)
            if chosen is None:
                chosen = random.choice(manifest)
                reasoning = f"invalid id '{picked_id}'; fell back to random"
            cost = {
                "provider": "openrouter",
                "model": self.llm.default_model,
                "units": float(resp.usage.total_tokens) if resp.usage else 0.0,
                "unit_label": "tokens",
                "unit_cost_usd": 0.0,
                "total_cost_usd": float(getattr(resp.usage, "cost", 0) or 0),
            }
            return chosen, reasoning, cost
        except Exception as e:
            log.warning("LLM pick failed — random fallback", err=str(e))
            return random.choice(manifest), "fallback (LLM error)", _zero_cost()


def _load_db_catalog(project_id: str, ctx) -> list[dict]:
    """Query the web-api audio library for music tracks. Returns the catalog in
    the manifest shape used downstream: list of dicts with id/object_key/mood.
    Empty list when the API is unreachable or empty — the caller falls back to
    the static manifest."""
    if not WEB_API_URL:
        return []
    params = {"kind": "music"}
    if project_id:
        params["project_id"] = project_id
    headers = {}
    if WEB_API_KEY:
        headers["X-API-Key"] = WEB_API_KEY
    try:
        with httpx.Client(timeout=5.0) as c:
            r = c.get(f"{WEB_API_URL}/api/audio/tracks", params=params, headers=headers)
        if r.status_code != 200:
            ctx.warning("audiolib api non-200", status=r.status_code)
            return []
        rows = r.json()
        out: list[dict] = []
        for row in rows:
            mood_csv = row.get("mood") or ""
            moods = [mood_csv] if mood_csv else []
            tags = row.get("tags") or []
            out.append({
                "id": row.get("id"),
                "title": row.get("title"),
                "object_key": row.get("object_key"),
                "duration_s": (row.get("duration_ms") or 0) // 1000,
                "mood": moods + [t for t in tags if t],
                "genre": [t for t in tags if t],
                "tempo": "medium",
                "license": row.get("attribution") or "",
                "attribution": row.get("attribution") or "",
            })
        return out
    except Exception as e:
        ctx.warning("audiolib api unreachable", err=str(e))
        return []


def _zero_cost() -> dict:
    return {
        "provider": "manual",
        "model": "",
        "units": 0,
        "unit_label": "tokens",
        "unit_cost_usd": 0.0,
        "total_cost_usd": 0.0,
    }
