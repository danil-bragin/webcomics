"""Tests for MusicHandler — picker over the bundled manifest.

Covers:
- Empty manifest → graceful skip (publishes completed w/ empty object_key).
- Missing manifest object → same graceful skip.
- preferred_mood override bypasses LLM and matches manifest entry.
- LLM picks a real track id from the catalog.
- LLM returns an invalid track id → fallback to random.
"""
from __future__ import annotations

import json
from io import BytesIO

import pytest

from worker.handlers.music import MusicHandler


class _FakeMinioObj:
    def __init__(self, data: bytes) -> None:
        self._data = data

    def read(self) -> bytes:
        return self._data


class _FakeMinioClient:
    def __init__(self, manifest_payload: bytes | None) -> None:
        self.manifest = manifest_payload

    def get_object(self, bucket: str, key: str):
        if self.manifest is None:
            raise RuntimeError("not found")
        return _FakeMinioObj(self.manifest)


class _StubStore:
    bucket = "webcomics"

    def __init__(self, manifest: list[dict] | None) -> None:
        if manifest is None:
            self.client = _FakeMinioClient(None)
        else:
            self.client = _FakeMinioClient(json.dumps(manifest).encode())


class _StubLLMClient:
    def __init__(self, api_key: str = "k") -> None:
        self.api_key = api_key
        self.last_messages = None
        self.response_id = "carefree-folk-01"
        self.response_reasoning = "matches the mood"

    class _Chat:
        def __init__(self, parent):
            self.parent = parent
            self.completions = self

        async def create(self, **kw):
            self.parent.last_messages = kw["messages"]
            class _C:
                pass
            choice = _C()
            choice.message = _C()
            choice.message.content = json.dumps({
                "track_id": self.parent.response_id,
                "reasoning": self.parent.response_reasoning,
            })
            usage = _C()
            usage.total_tokens = 100
            usage.cost = 0.0001
            resp = _C()
            resp.choices = [choice]
            resp.usage = usage
            return resp

    @property
    def chat(self):
        return self._Chat(self)


class _StubLLM:
    def __init__(self, api_key: str = "k") -> None:
        self.default_model = "openai/gpt-4o-mini"
        self.client = _StubLLMClient(api_key)


MANIFEST = [
    {"id": "carefree-folk-01", "object_key": "library/music/carefree-folk-01.mp3",
     "mood": ["upbeat", "happy"], "genre": ["folk"], "tempo": "medium"},
    {"id": "sneaky-comedic-01", "object_key": "library/music/sneaky-comedic-01.mp3",
     "mood": ["playful", "sneaky"], "genre": ["comedy"], "tempo": "medium"},
    {"id": "chill-lofi-01", "object_key": "library/music/chill-lofi-01.mp3",
     "mood": ["chill", "moody"], "genre": ["lofi"], "tempo": "slow"},
]


@pytest.mark.asyncio
async def test_skips_gracefully_when_manifest_missing(bus, cancelled):
    store = _StubStore(None)
    h = MusicHandler(bus, _StubLLM(), store, cancelled)
    await h.handle({"run_id": "r1", "step_index": 0})
    assert bus.published[0][0] == "pipeline.music.completed"
    assert bus.published[0][1]["object_key"] == ""


@pytest.mark.asyncio
async def test_skips_gracefully_on_empty_manifest(bus, cancelled):
    store = _StubStore([])
    h = MusicHandler(bus, _StubLLM(), store, cancelled)
    await h.handle({"run_id": "r2", "step_index": 0})
    assert bus.published[0][1]["object_key"] == ""


@pytest.mark.asyncio
async def test_preferred_mood_bypasses_llm(bus, cancelled):
    store = _StubStore(MANIFEST)
    llm = _StubLLM()
    h = MusicHandler(bus, llm, store, cancelled)
    await h.handle({
        "run_id": "rm", "step_index": 0,
        "params": {"preferred_mood": "sneaky"},
    })
    payload = bus.published[0][1]
    assert payload["track_id"] == "sneaky-comedic-01"
    assert payload["object_key"].endswith("sneaky-comedic-01.mp3")
    assert llm.client.last_messages is None  # never invoked


@pytest.mark.asyncio
async def test_llm_picks_track_from_catalog(bus, cancelled):
    store = _StubStore(MANIFEST)
    llm = _StubLLM()
    llm.client.response_id = "chill-lofi-01"
    h = MusicHandler(bus, llm, store, cancelled)
    await h.handle({
        "run_id": "rl", "step_index": 0,
        "panels": [{"caption": "moody late-night reflection"}],
    })
    payload = bus.published[0][1]
    assert payload["track_id"] == "chill-lofi-01"
    assert llm.client.last_messages is not None


@pytest.mark.asyncio
async def test_llm_invalid_id_falls_back_to_random(bus, cancelled):
    store = _StubStore(MANIFEST)
    llm = _StubLLM()
    llm.client.response_id = "no-such-track"
    h = MusicHandler(bus, llm, store, cancelled)
    await h.handle({"run_id": "rf", "step_index": 0,
                    "panels": [{"caption": "x"}]})
    payload = bus.published[0][1]
    assert payload["track_id"] in {t["id"] for t in MANIFEST}
    assert "fallback" in payload["reasoning"].lower() or "fell back" in payload["reasoning"].lower()
