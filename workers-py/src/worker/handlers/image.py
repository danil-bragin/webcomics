"""pipeline.image.requested handler — one panel per message."""
from __future__ import annotations

import asyncio
import base64
import time
from typing import Any

import structlog

from worker.providers.fal_images import FalImageClient
from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore

log = structlog.get_logger().bind(service="workers-py", worker="image")

REQUEST_STREAM = "pipeline.image.requested"
COMPLETED_STREAM = "pipeline.image.completed"
FAILED_STREAM = "pipeline.image.failed"

# Retry delays between fal.generate() attempts when fal returns a 5xx, network
# error, or explicit rate-limit hint. List length = max attempts.
_RETRY_BACKOFF = (0, 1.0, 3.0)


def _is_transient_fal_error(err: BaseException) -> bool:
    """Return True only when the error pattern indicates a retry might help.
    Content-moderation rejections, 4xx, and auth failures are NEVER transient."""
    msg = (str(err) or "").lower()
    transient_markers = (
        "timeout", "timed out", "connection", "temporarily unavailable",
        "internal server error", "502 ", "503 ", "504 ", "rate limit",
        "too many requests", "queue full",
    )
    non_transient_markers = (
        "content_policy", "moderation", "flagged", "no credentials", "unauthorized",
        "forbidden", "not found", "invalid",
    )
    if any(m in msg for m in non_transient_markers):
        return False
    return any(m in msg for m in transient_markers)


class _BlankImageError(Exception):
    """Raised when the generated image is almost entirely a single colour —
    fal's silent soft-moderation fallback. The 3 KB pitch-black 1024×1024 PNGs
    we observed get caught here so the run won't ship a blank panel."""


def _validate_image_payload(data: bytes, _content_type: str) -> None:
    # Sub-10 KB for a 1024×1024 PNG is almost certainly a near-uniform fill.
    if len(data) < 10 * 1024:
        raise _BlankImageError(f"payload too small ({len(data)} bytes)")
    try:
        from io import BytesIO
        from PIL import Image
        img = Image.open(BytesIO(data)).convert("RGB")
        thumb = img.resize((32, 32))
        flat = [v for px in thumb.getdata() for v in px]
        mean = sum(flat) / len(flat)
        if mean < 8:
            raise _BlankImageError(f"mean luminance {mean:.1f} — frame is black")
        var = sum((v - mean) ** 2 for v in flat) / len(flat)
        if var < 16:
            raise _BlankImageError(f"variance {var:.1f} — frame is uniform")
    except _BlankImageError:
        raise
    except Exception:
        # PIL decode error — don't false-positive on broken-but-non-blank bytes.
        return


class ImageHandler:
    def __init__(self, bus: Bus, fal: FalImageClient, store: ObjectStore, cancelled: CancelledRuns) -> None:
        self.bus = bus
        self.fal = fal
        self.store = store
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        run_id = msg.get("run_id", "")
        step_index = int(msg.get("step_index", 0))
        panel_index = int(msg.get("panel_index", 0))
        if self.cancelled.contains(run_id):
            log.info("skipping cancelled run", run_id=run_id, step_index=step_index, panel_index=panel_index)
            return
        prompt = msg.get("prompt", "")
        model = msg.get("model") or None
        params = msg.get("params") or {}
        output_key = msg.get("output_key") or f"runs/{run_id}/{step_index}/panel-{panel_index}.png"
        ref_keys = list(msg.get("ref_object_keys") or [])

        ctx = log.bind(run_id=run_id, step_index=step_index, step_type="image",
                       panel_index=panel_index, refs=len(ref_keys))
        start = time.perf_counter()
        # Auto-retry transient fal errors (network blips, 5xx, rate limits). Up
        # to 2 retries with 1s + 3s backoff. Non-transient errors (moderation,
        # auth, malformed prompt) break out immediately and surface as failed.
        try:
            data, content_type, cost = None, None, None
            for attempt, delay in enumerate(_RETRY_BACKOFF, start=1):
                try:
                    refs = await self._refs_to_data_uris(ref_keys)
                    data, content_type, cost = await self.fal.generate(prompt, model, params, refs)
                    break
                except Exception as e:
                    if not _is_transient_fal_error(e) or attempt == len(_RETRY_BACKOFF):
                        raise
                    ctx.warning("transient fal error — retrying",
                                attempt=attempt, delay=delay, err=str(e)[:200])
                    await asyncio.sleep(delay)
            assert data is not None  # invariant: break ran or we re-raised
            _validate_image_payload(data, content_type)
        except _BlankImageError as e:
            # Fal silently returns a tiny black placeholder when its content
            # moderation soft-blocks a prompt. Surface as a real failure so the
            # run won't ship a black panel into assemble.
            ctx.warning("blank/moderated image — failing panel", reason=str(e),
                        bytes=len(data) if 'data' in dir() else 0)
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index,
                "panel_index": panel_index,
                "error": f"image moderation rejected: {e}",
            })
            return
        except Exception as e:
            ctx.exception("image generate failed")
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id, "step_index": step_index,
                "panel_index": panel_index, "error": str(e),
            })
            return

        self.store.put_bytes(output_key, data, content_type)
        duration_ms = int((time.perf_counter() - start) * 1000)
        await self.bus.publish(COMPLETED_STREAM, {
            "run_id": run_id,
            "step_index": step_index,
            "panel_index": panel_index,
            "object_key": output_key,
            "bucket": self.store.bucket,
            "cost": cost,
            "duration_ms": duration_ms,
        })
        ctx.info(
            "image done",
            duration_ms=duration_ms,
            cost_usd=cost["total_cost_usd"],
            model=cost.get("model"),
        )

    async def _refs_to_data_uris(self, keys: list[str]) -> list[str]:
        if not keys:
            return []
        loop = asyncio.get_running_loop()
        out: list[str] = []
        for k in keys:
            buf = await loop.run_in_executor(
                None,
                lambda key=k: self.store.client.get_object(self.store.bucket, key).read(),
            )
            mime = "image/png" if k.lower().endswith(".png") else "image/jpeg"
            out.append(f"data:{mime};base64,{base64.b64encode(buf).decode()}")
        return out
