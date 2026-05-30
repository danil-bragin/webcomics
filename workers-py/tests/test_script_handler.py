"""Tests for ScriptHandler — mocks OpenRouter, verifies upload + publish."""
from __future__ import annotations

import json

import pytest

from worker.handlers.script import ScriptHandler


class FakeLLM:
    def __init__(self, panels: list[dict]) -> None:
        self.panels = panels

    async def generate_script(self, prompt, system_prompt, model, panel_count):
        return ({"panels": self.panels}, {"model": model or "gpt-4o-mini", "total_tokens": 50, "cost_usd": 0.001})


@pytest.mark.asyncio
async def test_publishes_script_completed(bus, store, cancelled):
    llm = FakeLLM([{"index": 0, "prompt": "a", "caption": "x"}])
    h = ScriptHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "r1", "step_index": 0, "prompt": "hi", "params": {"panel_count": 1}})
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.script.completed"
    assert payload["run_id"] == "r1"
    assert payload["step_index"] == 0
    assert payload["script_key"] == "runs/r1/0/script.json"
    assert payload["panels"][0]["prompt"] == "a"
    assert payload["cost"]["provider"] == "openrouter"
    assert payload["cost"]["total_cost_usd"] == 0.001
    # Uploaded the script.json to the store under the same key.
    assert "runs/r1/0/script.json" in store.objects
    body = json.loads(store.objects["runs/r1/0/script.json"][0])
    assert body["panels"][0]["prompt"] == "a"


@pytest.mark.asyncio
async def test_publishes_failure_when_no_panels(bus, store, cancelled):
    llm = FakeLLM([])
    h = ScriptHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "r2", "step_index": 0, "prompt": "x"})
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.script.failed"
    assert "no panels" in payload["error"]


@pytest.mark.asyncio
async def test_skips_cancelled_runs(bus, store, cancelled):
    cancelled.add("r3")
    llm = FakeLLM([{"index": 0, "prompt": "a"}])
    h = ScriptHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "r3", "step_index": 0, "prompt": "x"})
    assert bus.published == []


@pytest.mark.asyncio
async def test_publishes_failure_when_llm_raises(bus, store, cancelled):
    class _Boom:
        async def generate_script(self, *args, **kwargs):
            raise RuntimeError("rate limited")

    h = ScriptHandler(bus, _Boom(), store, cancelled)
    await h.handle({"run_id": "r4", "step_index": 0, "prompt": "x"})
    assert bus.published[0][0] == "pipeline.script.failed"
    assert "rate limited" in bus.published[0][1]["error"]
