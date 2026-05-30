"""Tests for CaptionHandler — mocks OpenRouter, verifies per-platform JSON."""
from __future__ import annotations

import pytest

from worker.handlers.caption import CaptionHandler


class FakeLLM:
    def __init__(self, captions: dict, raises: bool = False) -> None:
        self.captions = captions
        self.raises = raises
        self.last_call: dict | None = None

    async def generate_caption(self, panels, platforms, plot_premise, plot_beats, characters, model):
        self.last_call = {
            "panels": panels, "platforms": platforms,
            "plot_premise": plot_premise, "plot_beats": plot_beats,
            "characters": characters, "model": model,
        }
        if self.raises:
            raise RuntimeError("boom")
        return self.captions, {"model": model or "x", "total_tokens": 80, "cost_usd": 0.0008}


@pytest.mark.asyncio
async def test_caption_override_bypasses_llm(bus, store, cancelled):
    llm = FakeLLM({})  # would explode if called
    h = CaptionHandler(bus, llm, store, cancelled)
    await h.handle({
        "run_id": "r1", "step_index": 3,
        "params": {"platforms": ["youtube", "twitter"], "caption_override": "Manual line"},
    })
    assert llm.last_call is None
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.caption.completed"
    assert payload["captions"]["youtube"]["caption"] == "Manual line"
    assert payload["captions"]["twitter"]["caption"] == "Manual line"
    assert payload["cost"]["total_cost_usd"] == 0
    assert payload["cost"]["provider"] == "manual"


@pytest.mark.asyncio
async def test_caption_llm_path_publishes_per_platform_map(bus, store, cancelled):
    llm = FakeLLM({
        "youtube": {"title": "Hot take", "caption": "long desc", "hashtags": ["ai", "comics"]},
        "twitter": {"title": "", "caption": "short tweet", "hashtags": ["#ai"]},
    })
    h = CaptionHandler(bus, llm, store, cancelled)
    await h.handle({
        "run_id": "r2", "step_index": 3, "model": "openai/gpt-4o-mini",
        "params": {"platforms": ["youtube", "twitter"]},
        "plot": {"premise": "epic tale", "beats": [{"name": "start", "description": "begin"}]},
        "characters": [{"name": "Hero"}],
        "panels": [{"prompt": "p1", "caption": "c1"}],
    })
    assert llm.last_call is not None
    assert llm.last_call["platforms"] == ["youtube", "twitter"]
    assert llm.last_call["plot_premise"] == "epic tale"
    assert llm.last_call["characters"][0]["name"] == "Hero"
    stream, payload = bus.published[0]
    assert stream == "pipeline.caption.completed"
    assert payload["captions"]["youtube"]["title"] == "Hot take"
    assert payload["captions"]["twitter"]["caption"] == "short tweet"
    assert payload["cost"]["provider"] == "openrouter"


@pytest.mark.asyncio
async def test_caption_skips_cancelled_run(bus, store, cancelled):
    cancelled.add("r3")
    llm = FakeLLM({})
    h = CaptionHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "r3", "step_index": 3, "params": {"platforms": ["youtube"]}})
    assert bus.published == []
    assert llm.last_call is None


@pytest.mark.asyncio
async def test_caption_llm_failure_publishes_failed(bus, store, cancelled):
    llm = FakeLLM({}, raises=True)
    h = CaptionHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "r4", "step_index": 3, "params": {"platforms": ["youtube"]}})
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.caption.failed"
    assert "boom" in payload["error"]


@pytest.mark.asyncio
async def test_caption_defaults_platforms_when_missing(bus, store, cancelled):
    llm = FakeLLM({"youtube": {"caption": "x"}, "twitter": {"caption": "y"}})
    h = CaptionHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "r5", "step_index": 3, "params": {}})
    # Default ["youtube", "twitter"] is applied.
    assert llm.last_call["platforms"] == ["youtube", "twitter"]
