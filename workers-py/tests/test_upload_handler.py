"""Tests for UploadHandler — stubs selenium + telegram. No real network."""
from __future__ import annotations

from datetime import datetime, timedelta, timezone
from unittest.mock import patch

import pytest

from worker.handlers import upload as upload_mod
from worker.handlers.upload import UploadHandler


@pytest.fixture(autouse=True)
def _no_profile_clone(monkeypatch):
    """Skip real fs profile clone in every test that touches the YT path."""
    monkeypatch.setattr(upload_mod, "_clone_profile",
                        lambda src, run_id: f"/tmp/_test-{run_id}-profile")
    # shutil.rmtree on a path that doesn't exist is fine (ignore_errors=True), so
    # the handler's finally clause is safe even though the dir was never created.
    yield


def _make_msg(provider: str, **kw) -> dict:
    base = {
        "run_id": "r1",
        "step_index": 5,
        "provider": provider,
        "video_key": "runs/r1/4/video.mp4",
        "params": {},
        "captions": {},
        "firefox_profile_path": "/tmp/profile",
        "scheduled_at": "",
    }
    base.update(kw)
    return base


@pytest.mark.asyncio
async def test_youtube_selenium_invokes_helper_and_publishes_completed(bus, store, cancelled, monkeypatch):
    store.objects["runs/r1/4/video.mp4"] = (b"mp4-bytes", "video/mp4")
    calls = {}

    def fake_helper(video_key, store, firefox_profile, meta, run_id):
        calls["video_key"] = video_key
        calls["meta"] = meta
        return {
            "video_url": "https://youtu.be/abc123",
            "video_id": "abc123",
            "final_visibility": meta.get("visibility", "unlisted"),
        }

    monkeypatch.setattr(upload_mod, "_do_youtube_upload", fake_helper)

    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg(
        "youtube_selenium",
        captions={"youtube": {"title": "T1", "caption": "desc", "hashtags": ["a", "#b"]}},
        params={"visibility": "public"},
    ))
    assert calls["video_key"] == "runs/r1/4/video.mp4"
    assert calls["meta"]["title"] == "T1"
    assert "desc" in calls["meta"]["description"]
    assert "#a" in calls["meta"]["description"]
    assert "#b" in calls["meta"]["description"]
    assert calls["meta"]["visibility"] == "public"

    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.upload.completed"
    assert payload["external_ref"] == "https://youtu.be/abc123"
    assert payload["video_id"] == "abc123"
    assert payload["final_visibility"] == "public"


@pytest.mark.asyncio
async def test_youtube_selenium_requires_profile(bus, store, cancelled):
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg("youtube_selenium", firefox_profile_path=""))
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.upload.failed"
    assert "firefox_profile_path" in payload["error"]


@pytest.mark.asyncio
async def test_unknown_provider_fails(bus, store, cancelled):
    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg("nope_provider"))
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.upload.failed"
    assert "unknown upload provider" in payload["error"]


@pytest.mark.asyncio
async def test_cancelled_run_skipped(bus, store, cancelled):
    cancelled.add("r-cancel")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg("youtube_selenium", run_id="r-cancel"))
    assert bus.published == []


@pytest.mark.skip(reason="scheduled_at sleep moved into selenium provider; handler no longer blocks")
@pytest.mark.asyncio
async def test_scheduled_at_sleeps_until_target(bus, store, cancelled, monkeypatch):
    """scheduled_at in the future should trigger asyncio.sleep with positive seconds."""
    captured: dict = {}

    async def fake_sleep(seconds):
        captured["slept"] = seconds

    monkeypatch.setattr(upload_mod.asyncio, "sleep", fake_sleep)
    monkeypatch.setattr(upload_mod, "_do_youtube_upload",
                        lambda **kw: {"video_url": "https://youtu.be/x", "video_id": "x", "final_visibility": "unlisted"})

    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    future = (datetime.now(timezone.utc) + timedelta(seconds=30)).isoformat()
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg(
        "youtube_selenium",
        scheduled_at=future,
        captions={"youtube": {"title": "T", "caption": "d", "hashtags": []}},
    ))
    assert "slept" in captured
    # Should be roughly 30 seconds — allow generous range for clock skew.
    assert 25 <= captured["slept"] <= 31


@pytest.mark.asyncio
async def test_scheduled_at_in_past_does_not_sleep(bus, store, cancelled, monkeypatch):
    captured: dict = {}

    async def fake_sleep(seconds):
        captured["slept"] = seconds

    monkeypatch.setattr(upload_mod.asyncio, "sleep", fake_sleep)
    monkeypatch.setattr(upload_mod, "_do_youtube_upload",
                        lambda **kw: {"video_url": "https://youtu.be/x", "video_id": "x", "final_visibility": "unlisted"})

    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    past = (datetime.now(timezone.utc) - timedelta(seconds=60)).isoformat()
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg(
        "youtube_selenium",
        scheduled_at=past,
        captions={"youtube": {"title": "T", "caption": "d", "hashtags": []}},
    ))
    # Sleep should not have been invoked (or invoked with 0).
    assert captured.get("slept", 0) == 0


@pytest.mark.asyncio
async def test_scheduled_at_invalid_is_ignored(bus, store, cancelled, monkeypatch):
    monkeypatch.setattr(upload_mod, "_do_youtube_upload",
                        lambda **kw: {"video_url": "https://youtu.be/y", "video_id": "y", "final_visibility": "unlisted"})
    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg(
        "youtube_selenium",
        scheduled_at="totally not a date",
        captions={"youtube": {"caption": "c"}},
    ))
    assert len(bus.published) == 1
    stream, _ = bus.published[0]
    assert stream == "pipeline.upload.completed"


@pytest.mark.asyncio
async def test_youtube_uses_generic_caption_when_no_youtube_key(bus, store, cancelled, monkeypatch):
    seen = {}

    def fake_helper(**kw):
        seen.update(kw)
        return {"video_url": "https://youtu.be/x", "video_id": "x", "final_visibility": "unlisted"}

    monkeypatch.setattr(upload_mod, "_do_youtube_upload", fake_helper)
    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg(
        "youtube_selenium",
        captions={"generic": {"title": "Gtitle", "caption": "Gdesc", "hashtags": []}},
    ))
    assert seen["meta"]["title"] == "Gtitle"
    assert "Gdesc" in seen["meta"]["description"]


@pytest.mark.asyncio
async def test_publish_verification_failure_surfaces_as_failed(bus, store, cancelled, monkeypatch):
    """Regression: when selenium raises 'publish verification failed' (rate
    limit / captcha / device challenge), the upload MUST publish
    pipeline.upload.failed — not silently report success with a studio URL."""

    def boom(**_):
        raise RuntimeError("publish verification failed: no youtu.be link surfaced")

    monkeypatch.setattr(upload_mod, "_do_youtube_upload", boom)
    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg("youtube_selenium"))
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.upload.failed"
    assert "verification" in payload["error"]


@pytest.mark.asyncio
async def test_screenshot_trail_published_on_success(bus, store, cancelled, monkeypatch):
    """The handler must include `screenshot_trail` on the completed event so the
    UI can render the per-stage debug strip."""
    monkeypatch.setattr(
        upload_mod, "_do_youtube_upload",
        lambda **kw: {"video_url": "https://youtu.be/ok", "video_id": "ok", "final_visibility": "public"},
    )

    async def _stub_trail(self, run_id, step_index, local_dir):
        return [
            {"stage": "01-step-studio-opened", "object_key": f"runs/{run_id}/upload/{step_index}/01.png"},
            {"stage": "02-step-title-typed",  "object_key": f"runs/{run_id}/upload/{step_index}/02.png"},
        ]
    monkeypatch.setattr(UploadHandler, "_upload_screenshot_trail", _stub_trail)

    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg("youtube_selenium"))
    stream, payload = bus.published[0]
    assert stream == "pipeline.upload.completed"
    assert len(payload["screenshot_trail"]) == 2
    assert payload["screenshot_trail"][0]["stage"].endswith("studio-opened")


@pytest.mark.asyncio
async def test_screenshot_trail_published_on_failure(bus, store, cancelled, monkeypatch):
    """Even when the upload fails the trail must accompany the failure event so
    the operator can see exactly where things broke."""
    def boom(**_):
        raise RuntimeError("publish verification failed")
    monkeypatch.setattr(upload_mod, "_do_youtube_upload", boom)

    async def _stub_trail(self, run_id, step_index, local_dir):
        return [{"stage": "07-fail-no-link",
                 "object_key": f"runs/{run_id}/upload/{step_index}/fail.png"}]
    monkeypatch.setattr(UploadHandler, "_upload_screenshot_trail", _stub_trail)

    store.objects["runs/r1/4/video.mp4"] = (b"x", "video/mp4")
    h = UploadHandler(bus, store, cancelled)
    await h.handle(_make_msg("youtube_selenium"))
    stream, payload = bus.published[0]
    assert stream == "pipeline.upload.failed"
    assert len(payload["screenshot_trail"]) == 1
