"""pipeline.script.requested handler.

Calls OpenRouter, uploads the script.json to MinIO, publishes
pipeline.script.completed.
"""
from __future__ import annotations

import json
import time
from typing import Any

import structlog

from worker.providers.openrouter import OpenRouterClient
from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore

log = structlog.get_logger().bind(service="workers-py", worker="script")

REQUEST_STREAM = "pipeline.script.requested"
COMPLETED_STREAM = "pipeline.script.completed"
FAILED_STREAM = "pipeline.script.failed"


def _compose_system_prompt(
    base: str | None,
    characters: list[dict[str, Any]],
    environments: list[dict[str, Any]],
    plot: dict[str, Any] | None,
) -> str | None:
    """Prepend a structured brief (characters / environments / plot) to the
    user's system_prompt. Returns base unchanged when there is nothing to add.

    Keeps every section short so we don't blow the LLM's attention budget.
    """
    sections: list[str] = []

    if characters:
        lines = []
        for c in characters:
            name = c.get("name") or "?"
            desc = c.get("description") or ""
            traits = c.get("traits") or {}
            trait_str = ""
            if isinstance(traits, dict) and traits:
                trait_str = "; ".join(f"{k}: {v}" for k, v in traits.items() if v)
            line = f"- {name}"
            if desc:
                line += f" - {desc}"
            if trait_str:
                line += f" ({trait_str})"
            lines.append(line)
        sections.append("RECURRING CHARACTERS (keep visually consistent across panels):\n" + "\n".join(lines))

    if environments:
        lines = []
        for e in environments:
            name = e.get("name") or "?"
            desc = e.get("description") or ""
            traits = e.get("traits") or {}
            trait_str = ""
            if isinstance(traits, dict) and traits:
                trait_str = "; ".join(f"{k}: {v}" for k, v in traits.items() if v)
            line = f"- {name}"
            if desc:
                line += f" - {desc}"
            if trait_str:
                line += f" ({trait_str})"
            lines.append(line)
        sections.append("SETTINGS (use as backdrop):\n" + "\n".join(lines))

    if plot:
        bits = []
        if plot.get("premise"):
            bits.append(f"Premise: {plot['premise']}")
        beats = plot.get("beats") or []
        if isinstance(beats, list) and beats:
            beat_lines = []
            for b in beats:
                if not isinstance(b, dict):
                    continue
                bn = b.get("name") or ""
                bd = b.get("description") or ""
                if bn or bd:
                    beat_lines.append(f"- {bn}: {bd}" if bn else f"- {bd}")
            if beat_lines:
                bits.append("Key beats:\n" + "\n".join(beat_lines))
        if bits:
            sections.append("STORY ARC:\n" + "\n".join(bits))

    if not sections:
        return base
    prefix = "\n\n".join(sections)
    if base:
        return prefix + "\n\n---\n\n" + base
    return prefix + "\n\nWrite a panel-by-panel script in JSON {panels:[{prompt,caption}]} consistent with the above."


_LANG_NAMES = {"en": "English", "ru": "Russian", "fr": "French"}


def _apply_language(base: str | None, language: str) -> str | None:
    """Force caption language. Image prompts must stay English so diffusion
    models retain quality; we tell the LLM both rules explicitly so it doesn't
    translate the whole output. Always preserves a JSON-output hint so
    OpenAI's response_format=json_object guard doesn't reject the prompt."""
    if language == "en":
        return base
    name = _LANG_NAMES.get(language, "English")
    rule = (
        f"LANGUAGE: Write each panel's `caption` field in {name}. "
        f"Keep each panel's `prompt` field in English (image models train on English captions). "
        f"Never translate the image prompt. "
        f"Reply with STRICT JSON only — the response_format requires a json_object."
    )
    if base:
        return rule + "\n\n" + base
    return rule


class ScriptHandler:
    def __init__(self, bus: Bus, llm: OpenRouterClient, store: ObjectStore, cancelled: CancelledRuns) -> None:
        self.bus = bus
        self.llm = llm
        self.store = store
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        run_id = msg.get("run_id", "")
        step_index = int(msg.get("step_index", 0))
        if self.cancelled.contains(run_id):
            log.info("skipping cancelled run", run_id=run_id, step_index=step_index)
            return
        prompt = msg.get("prompt", "")
        system_prompt = msg.get("system_prompt") or None
        model = msg.get("model") or None
        language = (msg.get("language") or "en").lower()
        if language not in ("en", "ru", "fr"):
            language = "en"
        params = msg.get("params") or {}
        panel_count = 0
        pc = params.get("panel_count")
        if isinstance(pc, (int, float)):
            panel_count = int(pc)

        # Project-linked context (characters, environments, plot) is prepended
        # to system_prompt as a structured visual brief so the LLM stays on
        # the established vector across runs.
        characters = msg.get("characters") or []
        environments = msg.get("environments") or []
        plot = msg.get("plot") or None
        system_prompt = _compose_system_prompt(system_prompt, characters, environments, plot)
        system_prompt = _apply_language(system_prompt, language)

        ctx = log.bind(run_id=run_id, step_index=step_index, step_type="script", language=language)
        start = time.perf_counter()
        try:
            parsed, usage = await self.llm.generate_script(prompt, system_prompt, model, panel_count)
        except Exception as e:
            ctx.exception("script generate failed")
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index, "error": str(e),
            })
            return

        panels_raw = parsed.get("panels") or []
        panels: list[dict[str, Any]] = []
        for i, p in enumerate(panels_raw):
            if not isinstance(p, dict):
                continue
            # Force 0-based contiguous indices — the aggregate uses
            # panel_index < panels_expected, so we can't trust an LLM that
            # numbers panels 1..N or skips values.
            panels.append({
                "index": i,
                "prompt": str(p.get("prompt", "")),
                "caption": str(p.get("caption") or ""),
            })
        if not panels:
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index, "error": "no panels in script output",
            })
            return

        script_key = f"runs/{run_id}/{step_index}/script.json"
        self.store.put_bytes(
            script_key,
            json.dumps({"panels": panels, "raw": parsed}).encode(),
            "application/json",
        )

        cost_total = usage.get("cost_usd")
        total_tokens = usage.get("total_tokens", 0)
        if cost_total is None:
            # Best-effort fallback when OpenRouter doesn't surface usage.cost:
            # 4o-mini blended price ≈ $0.4/1M tokens.
            cost_total = round(total_tokens * 0.0000004, 6)
        unit_cost = round(cost_total / max(total_tokens, 1), 8) if total_tokens else 0
        cost_info = {
            "provider": "openrouter",
            "model": usage.get("model", ""),
            "units": float(total_tokens),
            "unit_label": "tokens",
            "unit_cost_usd": float(unit_cost),
            "total_cost_usd": float(cost_total),
        }

        duration_ms = int((time.perf_counter() - start) * 1000)
        await self.bus.publish(COMPLETED_STREAM, {
            "run_id": run_id,
            "step_index": step_index,
            "script_key": script_key,
            "bucket": self.store.bucket,
            "panels": panels,
            "cost": cost_info,
            "duration_ms": duration_ms,
        })
        ctx.info(
            "script done",
            panels=len(panels),
            cost_usd=cost_total,
            duration_ms=duration_ms,
        )
