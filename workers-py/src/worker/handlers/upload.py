"""pipeline.upload.requested handler.

Resolves effective upload metadata from message payload:
  caption_metadata (from caption step) ← merged with
  params.upload_defaults (from project/template) ← merged with
  params (top-level overrides from run)

Per-account Redis advisory lock prevents two workers from grabbing the same
Firefox profile (which would crash on profile lock collision).
"""
from __future__ import annotations

import asyncio
import os
import shutil
import tempfile
import time
import uuid
from typing import Any

import httpx
import structlog

from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore

log = structlog.get_logger().bind(service="workers-py", worker="upload")

REQUEST_STREAM = "pipeline.upload.requested"
COMPLETED_STREAM = "pipeline.upload.completed"
FAILED_STREAM = "pipeline.upload.failed"

# Selenium grabs Firefox profile lock — single-replica-per-account.
LOCK_TTL_SECONDS = 15 * 60  # selenium upload can be slow on first run


def _resolve_metadata(
    captions: dict[str, Any],
    params: dict[str, Any],
) -> dict[str, Any]:
    """Resolve the final upload metadata.

    Precedence (low → high):
      caption_metadata.youtube (LLM JSON output) →
      caption_metadata.generic →
      params.* (top-level upload override from run / template / project)
    """
    yt = captions.get("youtube") or captions.get("generic") or {}
    # caption step new format: {title, description, tags[], hashtags[]}
    base = {
        "title": (yt.get("title") or "").strip(),
        "description": (yt.get("description") or yt.get("caption") or "").strip(),
        "tags": list(yt.get("tags") or []),
        "hashtags": list(yt.get("hashtags") or []),
    }
    # Append hashtags into description if missing.
    if base["hashtags"]:
        ht_line = " ".join(f"#{h.lstrip('#')}" for h in base["hashtags"])
        if ht_line not in base["description"]:
            base["description"] = (base["description"] + "\n\n" + ht_line).strip()

    # Run/template/project overrides (anything in params wins).
    out = dict(base)
    for k in (
        "title", "description", "visibility", "made_for_kids", "age_restriction",
        "category_id", "category_label", "comments_enabled", "scheduled_at",
        "playlist_names", "thumbnail_path", "headless",
    ):
        if k in params and params[k] not in (None, "", []):
            out[k] = params[k]
    # Tag merge (additive, dedupe preserving order).
    extra_tags = params.get("tags") or []
    if extra_tags:
        seen = set(out["tags"])
        for t in extra_tags:
            if t not in seen:
                out["tags"].append(t)
                seen.add(t)
    return out


class UploadHandler:
    def __init__(self, bus: Bus, store: ObjectStore, cancelled: CancelledRuns) -> None:
        self.bus = bus
        self.store = store
        self.cancelled = cancelled

    async def handle(self, msg: dict[str, Any]) -> None:
        run_id = str(msg.get("run_id", ""))
        step_index = int(msg.get("step_index", 0))
        if self.cancelled.contains(run_id):
            log.info("skipping cancelled run", run_id=run_id)
            return
        provider = str(msg.get("provider") or "telegram")
        video_key = str(msg.get("video_key") or "")
        params = msg.get("params") or {}
        firefox_profile = msg.get("firefox_profile_path") or params.get("firefox_profile_path") or ""
        social_account_id = msg.get("social_account_id") or params.get("social_account_id") or ""
        captions = msg.get("captions") or msg.get("caption_metadata") or {}

        ctx = log.bind(run_id=run_id, step_index=step_index, step_type="upload", provider=provider)

        start = time.perf_counter()
        screenshot_asset_id = ""
        try:
            if provider.endswith("_selenium"):
                if not firefox_profile:
                    raise RuntimeError(f"firefox_profile_path required for {provider}")
                # Clone profile to a temp dir so two Firefox instances can run
                # concurrently. Firefox holds an OS-level lock on parent.lock —
                # without cloning, every upload after the first stalls.
                cloned_profile = _clone_profile(firefox_profile, run_id)
                try:
                    result = await self._upload_selenium(
                        provider=provider,
                        run_id=run_id,
                        video_key=video_key,
                        firefox_profile=cloned_profile,
                        captions=captions,
                        params=params,
                    )
                    external_ref = result["video_url"]
                    video_id = result["video_id"]
                    final_visibility = result["final_visibility"]
                finally:
                    shutil.rmtree(cloned_profile, ignore_errors=True)
            else:
                data = await self._fetch_video(video_key)
                if provider == "telegram":
                    external_ref = await self._upload_telegram(data, params)
                    video_id = ""
                    final_visibility = ""
                else:
                    raise ValueError(f"unknown upload provider: {provider}")
        except Exception as e:
            ctx.exception("upload failed")
            screenshot_path = getattr(e, "screenshot_path", "") or ""
            fallback_url = getattr(e, "fallback_video_url", "") or ""
            fallback_id = getattr(e, "fallback_video_id", "") or ""
            if screenshot_path and os.path.exists(screenshot_path):
                screenshot_asset_id = await self._upload_screenshot(
                    run_id, step_index, screenshot_path,
                )
            trail = await self._upload_screenshot_trail(
                run_id, step_index, f"/tmp/upload-screenshots/{run_id}",
            )
            await self.bus.publish(FAILED_STREAM, {
                "run_id": run_id,
                "step_index": step_index,
                "social_account_id": social_account_id,
                "error": f"{type(e).__name__}: {str(e)[:500]}",
                "error_screenshot_asset_id": screenshot_asset_id,
                "screenshot_trail": trail,
                "fallback_video_url": fallback_url,
                "fallback_video_id": fallback_id,
            })
            return

        duration_ms = int((time.perf_counter() - start) * 1000)
        screenshot_trail = await self._upload_screenshot_trail(
            run_id, step_index, f"/tmp/upload-screenshots/{run_id}",
        )
        await self.bus.publish(COMPLETED_STREAM, {
            "run_id": run_id,
            "step_index": step_index,
            "external_ref": external_ref,
            "social_account_id": social_account_id,
            "video_id": video_id,
            "final_visibility": final_visibility,
            "screenshot_trail": screenshot_trail,
            "cost": {
                "provider": provider,
                "model": "",
                "units": 1.0,
                "unit_label": "uploads",
                "unit_cost_usd": 0.0,
                "total_cost_usd": 0.0,
            },
            "duration_ms": duration_ms,
        })
        ctx.info("upload done", external_ref=external_ref, duration_ms=duration_ms,
                 final_visibility=final_visibility)

    async def _upload_selenium(
        self,
        provider: str,
        run_id: str,
        video_key: str,
        firefox_profile: str,
        captions: dict,
        params: dict,
    ) -> dict[str, Any]:
        if not firefox_profile:
            raise RuntimeError(f"firefox_profile_path required for {provider}")
        # Per-platform caption block, falling back to YT if the caption worker
        # hasn't fanned out yet. Keeps backward compat with the existing flow.
        platform_key = provider.replace("_selenium", "")
        platform_captions = captions.get(platform_key) or captions.get("youtube") or {}
        meta = _resolve_metadata({platform_key: platform_captions, "youtube": platform_captions}, params)
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            None,
            lambda: _do_selenium_upload(
                provider=provider,
                video_key=video_key,
                store=self.store,
                firefox_profile=firefox_profile,
                meta=meta,
                run_id=run_id,
            ),
        )

    async def _fetch_video(self, key: str) -> bytes:
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            None,
            lambda: self.store.client.get_object(self.store.bucket, key).read(),
        )

    async def _upload_telegram(self, video: bytes, params: dict[str, Any]) -> str:
        token = os.getenv("TELEGRAM_BOT_TOKEN", "")
        if not token:
            raise RuntimeError("TELEGRAM_BOT_TOKEN not set")
        chat_id = params.get("chat_id") or os.getenv("TELEGRAM_CHAT_ID", "")
        if not chat_id:
            raise RuntimeError("chat_id missing (param or TELEGRAM_CHAT_ID)")
        caption = params.get("caption") or ""
        url = f"https://api.telegram.org/bot{token}/sendVideo"
        async with httpx.AsyncClient(timeout=120.0) as client:
            r = await client.post(
                url,
                data={"chat_id": str(chat_id), "caption": caption},
                files={"video": ("comic.mp4", video, "video/mp4")},
            )
            r.raise_for_status()
            body = r.json()
            mid = body.get("result", {}).get("message_id")
            return f"tg://{chat_id}/{mid}" if mid else "tg://unknown"

    async def _upload_screenshot(
        self,
        run_id: str,
        step_index: int,
        local_path: str,
    ) -> str:
        """Push the local screenshot into MinIO and return the asset key.

        Returns the object key the API can presign for UI display.
        """
        loop = asyncio.get_running_loop()
        key = f"runs/{run_id}/upload/{step_index}/error-{int(time.time())}.png"
        try:
            await loop.run_in_executor(
                None,
                lambda: self.store.client.fput_object(
                    self.store.bucket, key, local_path,
                    content_type="image/png",
                ),
            )
            return key
        except Exception as e:
            log.warning("screenshot upload failed", err=str(e))
            return ""

    async def _upload_screenshot_trail(
        self,
        run_id: str,
        step_index: int,
        local_dir: str,
    ) -> list[dict[str, str]]:
        """Upload every screenshot in local_dir to MinIO in lexical (step) order.

        Returns [{stage, object_key}] entries suitable for the upload completion
        payload — the consumer attaches them to the UploadRecord row so the UI
        can render a scrubbable thumbnail strip per upload.
        """
        if not local_dir or not os.path.isdir(local_dir):
            return []
        loop = asyncio.get_running_loop()
        out: list[dict[str, str]] = []
        for name in sorted(os.listdir(local_dir)):
            if not name.endswith(".png"):
                continue
            local = os.path.join(local_dir, name)
            # Lexical prefix sorts by step (01, 02, ...); strip it for the stage label.
            stage = name.removesuffix(".png")
            key = f"runs/{run_id}/upload/{step_index}/{name}"
            try:
                await loop.run_in_executor(
                    None,
                    lambda l=local, k=key: self.store.client.fput_object(
                        self.store.bucket, k, l, content_type="image/png",
                    ),
                )
                out.append({"stage": stage, "object_key": key})
            except Exception as e:
                log.warning("trail screenshot upload failed", name=name, err=str(e))
        return out


def _clone_profile(src: str, run_id: str) -> str:
    """Copy the Firefox profile dir to /tmp/yt-profile-<id>/ so several Firefox
    instances can run in parallel. The original profile is read-only-ish (we
    don't want concurrent writes); the clone is disposable."""
    dst = tempfile.mkdtemp(prefix=f"yt-profile-{run_id[:8]}-")
    # copy_function=shutil.copy2 preserves cookies.sqlite mtime; ignore lock files.
    def _ignore(_dir, names):
        return {n for n in names if n in ("parent.lock", "lock", ".parentlock")}
    shutil.copytree(src, dst, ignore=_ignore, dirs_exist_ok=True)
    return dst


def _do_selenium_upload(
    provider: str,
    video_key: str,
    store: ObjectStore,
    firefox_profile: str,
    meta: dict[str, Any],
    run_id: str,
) -> dict[str, Any]:
    """Blocking helper executed in worker thread. Dispatches by provider."""
    if provider == "youtube_selenium":
        from worker.providers.selenium_youtube import (
            YouTubeUploadConfig,
            download_video_to_tempfile,
            upload_to_youtube,
        )
        local_path = download_video_to_tempfile(store, video_key)
        screenshot_dir = f"/tmp/upload-screenshots/{run_id}"
        try:
            cfg = YouTubeUploadConfig(
                firefox_profile_path=firefox_profile,
                video_path=local_path,
                title=meta.get("title") or "Generated comic",
                description=meta.get("description") or "",
                tags=list(meta.get("tags") or []),
                made_for_kids=bool(meta.get("made_for_kids", False)),
                age_restriction=str(meta.get("age_restriction") or "none"),
                category_id=str(meta.get("category_id") or "22"),
                category_label=str(meta.get("category_label") or "People & Blogs"),
                comments_enabled=bool(meta.get("comments_enabled", True)),
                visibility=str(meta.get("visibility") or "unlisted"),
                scheduled_at=str(meta.get("scheduled_at") or ""),
                playlist_names=list(meta.get("playlist_names") or []),
                thumbnail_path=str(meta.get("thumbnail_path") or ""),
                headless=bool(meta.get("headless", True)),
                screenshot_dir=screenshot_dir,
            )
            result = upload_to_youtube(cfg)
            return {
                "video_url": result.video_url,
                "video_id": result.video_id,
                "final_visibility": result.final_visibility,
            }
        finally:
            try:
                os.unlink(local_path)
            except OSError:
                pass

    # Other selenium providers share a thin contract: download the video,
    # hand a config dict to the provider module, get back {video_url, video_id,
    # final_visibility}.
    from worker.providers import selenium_instagram, selenium_tiktok, selenium_facebook
    from worker.providers.selenium_youtube import download_video_to_tempfile
    impl = {
        "instagram_selenium": selenium_instagram.upload,
        "tiktok_selenium":    selenium_tiktok.upload,
        "facebook_selenium":  selenium_facebook.upload,
    }.get(provider)
    if impl is None:
        raise ValueError(f"unknown selenium provider: {provider}")
    local_path = download_video_to_tempfile(store, video_key)
    screenshot_dir = f"/tmp/upload-screenshots/{run_id}"
    try:
        return impl(
            firefox_profile=firefox_profile,
            video_path=local_path,
            meta=meta,
            screenshot_dir=screenshot_dir,
        )
    finally:
        try:
            os.unlink(local_path)
        except OSError:
            pass
