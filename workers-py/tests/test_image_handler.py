"""Tests for ImageHandler — mocks fal.ai, verifies upload + publish."""
from __future__ import annotations

import pytest

from worker.handlers.image import ImageHandler


def _make_pseudo_png(size=(256, 256)) -> bytes:
    """Build a real PNG with enough variance to pass the blank-image guard."""
    from io import BytesIO
    import os
    from PIL import Image
    img = Image.frombytes("RGB", size, os.urandom(size[0] * size[1] * 3))
    buf = BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()


_DEFAULT_PNG = _make_pseudo_png()


class FakeFal:
    def __init__(self, data: bytes | None = None, content_type: str = "image/png") -> None:
        self.data = data if data is not None else _DEFAULT_PNG
        self.content_type = content_type

    async def generate(self, prompt, model, params, refs=None):
        return (
            self.data,
            self.content_type,
            {
                "provider": "fal", "model": "flux", "units": 1.0, "unit_label": "images",
                "unit_cost_usd": 0.003, "total_cost_usd": 0.003,
            },
        )


@pytest.mark.asyncio
async def test_publishes_image_completed(bus, store, cancelled):
    h = ImageHandler(bus, FakeFal(), store, cancelled)
    await h.handle({
        "run_id": "r1", "step_index": 1, "panel_index": 2,
        "prompt": "p", "output_key": "runs/r1/1/panel-2.png",
    })
    assert len(bus.published) == 1
    stream, payload = bus.published[0]
    assert stream == "pipeline.image.completed"
    assert payload["run_id"] == "r1"
    assert payload["panel_index"] == 2
    assert payload["object_key"] == "runs/r1/1/panel-2.png"
    assert payload["cost"]["total_cost_usd"] == 0.003
    assert "runs/r1/1/panel-2.png" in store.objects


@pytest.mark.asyncio
async def test_skips_cancelled_runs(bus, store, cancelled):
    cancelled.add("rc")
    h = ImageHandler(bus, FakeFal(), store, cancelled)
    await h.handle({"run_id": "rc", "step_index": 1, "panel_index": 0, "prompt": "p"})
    assert bus.published == []


@pytest.mark.asyncio
async def test_publishes_failure_when_fal_raises(bus, store, cancelled):
    class _Boom:
        async def generate(self, *a, **kw):
            raise RuntimeError("fal down")

    h = ImageHandler(bus, _Boom(), store, cancelled)
    await h.handle({"run_id": "r5", "step_index": 1, "panel_index": 0, "prompt": "p"})
    assert bus.published[0][0] == "pipeline.image.failed"


# --- Blank-image (silent fal moderation) detection --------------------------

from io import BytesIO

from PIL import Image

from worker.handlers.image import _BlankImageError, _validate_image_payload


def _png_bytes(color, size=(1024, 1024)) -> bytes:
    img = Image.new("RGB", size, color=color)
    buf = BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()


def test_validator_rejects_tiny_payload():
    with pytest.raises(_BlankImageError, match="too small"):
        _validate_image_payload(b"x" * 100, "image/png")


def test_validator_rejects_pure_black_payload():
    data = _png_bytes((0, 0, 0))
    with pytest.raises(_BlankImageError, match="black|too small"):
        _validate_image_payload(data, "image/png")


def test_validator_rejects_uniform_white_payload():
    data = _png_bytes((255, 255, 255))
    with pytest.raises(_BlankImageError, match="uniform|too small"):
        _validate_image_payload(data, "image/png")


def test_validator_accepts_real_image():
    _validate_image_payload(_make_pseudo_png(), "image/png")  # no raise


@pytest.mark.asyncio
async def test_retries_transient_fal_errors(bus, store, cancelled, monkeypatch):
    """Transient errors (timeout / 5xx) must trigger up to N retries."""
    import worker.handlers.image as image_mod

    async def _no_sleep(_):
        return
    monkeypatch.setattr(image_mod.asyncio, "sleep", _no_sleep)

    attempts = {"n": 0}

    class _FlakyFal:
        async def generate(self, *a, **kw):
            attempts["n"] += 1
            if attempts["n"] < 3:
                raise RuntimeError("503 Service temporarily unavailable")
            return (_DEFAULT_PNG, "image/png",
                    {"provider": "fal", "model": "x", "units": 1.0,
                     "unit_label": "img", "unit_cost_usd": 0.003, "total_cost_usd": 0.003})

    h = ImageHandler(bus, _FlakyFal(), store, cancelled)
    await h.handle({"run_id": "rT", "step_index": 1, "panel_index": 0,
                    "prompt": "p", "output_key": "runs/rT/1/p.png"})
    assert attempts["n"] == 3
    assert bus.published[0][0] == "pipeline.image.completed"


@pytest.mark.asyncio
async def test_does_not_retry_moderation_errors(bus, store, cancelled):
    """Content-moderation rejections are NOT transient — fail fast, no retries."""
    attempts = {"n": 0}

    class _Moderated:
        async def generate(self, *a, **kw):
            attempts["n"] += 1
            raise RuntimeError("[{'loc':['body','image_urls'],'msg':'content_policy flagged'}]")

    h = ImageHandler(bus, _Moderated(), store, cancelled)
    await h.handle({"run_id": "rM", "step_index": 1, "panel_index": 0,
                    "prompt": "x", "output_key": "runs/rM/1/p.png"})
    assert attempts["n"] == 1  # exactly one call, no retry
    assert bus.published[0][0] == "pipeline.image.failed"


@pytest.mark.asyncio
async def test_handler_marks_panel_failed_when_image_is_blank(bus, store, cancelled):
    """Regression: fal silent moderation returned 3 KB pitch-black PNG; pipeline
    must surface as `pipeline.image.failed`, not `completed`."""
    blank = _png_bytes((0, 0, 0))
    h = ImageHandler(bus, FakeFal(data=blank), store, cancelled)
    await h.handle({
        "run_id": "rb", "step_index": 1, "panel_index": 0,
        "prompt": "p", "output_key": "runs/rb/1/panel-0.png",
    })
    assert bus.published[0][0] == "pipeline.image.failed"
    assert "moderation" in bus.published[0][1]["error"].lower()
    assert "runs/rb/1/panel-0.png" not in store.objects  # never persisted
