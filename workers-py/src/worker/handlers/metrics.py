"""pipeline.metrics.requested handler.

Consumes per-upload metrics-fetch requests for selenium-based platforms
(IG / TT / FB). YT is fetched in-process by the Go API; this worker handles
the platforms that need a logged-in Firefox profile to read counters.

Message shape:
  {
    "upload_record_id": "...",
    "external_ref":     "https://www.instagram.com/reel/<shortcode>/",
    "platform":         "instagram_selenium",
    "social_account_id": "...",
    "profile_path":     "/profiles/.../profile"
  }

On success publishes pipeline.metrics.completed with:
  {upload_record_id, views, likes, comments, shares, raw}
On failure publishes pipeline.metrics.failed with error.
"""
from __future__ import annotations

import asyncio
import time
from typing import Any

import structlog

from worker.providers import metrics_instagram, metrics_tiktok, metrics_facebook

log = structlog.get_logger().bind(component="metrics_handler")

REQUEST_STREAM = "pipeline.metrics.requested"
COMPLETED_STREAM = "pipeline.metrics.completed"
FAILED_STREAM = "pipeline.metrics.failed"

_FETCHERS = {
    "instagram_selenium": metrics_instagram.fetch,
    "tiktok_selenium":    metrics_tiktok.fetch,
    "facebook_selenium":  metrics_facebook.fetch,
}


class MetricsHandler:
    def __init__(self, bus, cancelled) -> None:
        self.bus = bus
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        upload_id = str(msg.get("upload_record_id") or "")
        external_ref = str(msg.get("external_ref") or "")
        platform = str(msg.get("platform") or "")
        profile = str(msg.get("profile_path") or "")
        ctx = log.bind(upload_record_id=upload_id, platform=platform)
        if not (upload_id and external_ref and platform):
            ctx.warning("missing fields, skipping")
            return
        fn = _FETCHERS.get(platform)
        if fn is None:
            ctx.warning("no fetcher for platform")
            return
        start = time.perf_counter()
        try:
            # Run sync selenium in a thread so the event loop isn't blocked.
            snap = await asyncio.get_running_loop().run_in_executor(
                None, lambda: fn(external_ref=external_ref, profile_path=profile),
            )
            await self.bus.publish(COMPLETED_STREAM, {
                "upload_record_id": upload_id,
                "views":    int(snap.get("views", 0)),
                "likes":    int(snap.get("likes", 0)),
                "comments": int(snap.get("comments", 0)),
                "shares":   int(snap.get("shares", 0)),
                "raw":      snap.get("raw", {}),
            })
            ctx.info("metrics fetched",
                     duration_ms=int((time.perf_counter() - start) * 1000),
                     views=snap.get("views", 0))
        except Exception as e:
            ctx.exception("metrics fetch failed")
            await self.bus.publish(FAILED_STREAM, {
                "upload_record_id": upload_id, "error": str(e)[:500],
            })
