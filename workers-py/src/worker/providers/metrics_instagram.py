"""Instagram reel metrics scraper.

Best-effort DOM scrape — IG hides counts behind login + AB tests them.
Returns {views, likes, comments, shares, raw}. 0 means "couldn't read",
not necessarily "no engagement"; the API surfaces the last fetched_at so
the UI shows "stale 6h" instead of treating 0 as truth.
"""
from __future__ import annotations

import re
import time
from typing import Any

import structlog

from ._selenium_common import firefox_driver

log = structlog.get_logger().bind(provider="metrics_instagram")


def fetch(*, external_ref: str, profile_path: str) -> dict[str, Any]:
    from selenium.webdriver.common.by import By
    if not profile_path:
        raise RuntimeError("profile_path required for instagram metrics scrape")
    with firefox_driver(profile_path, headless=True) as driver:
        driver.get(external_ref)
        time.sleep(4)
        html = driver.page_source
        views = _parse_count(html, r'"play_count":(\d+)') or _parse_count(html, r'"video_view_count":(\d+)')
        likes = _parse_count(html, r'"edge_media_preview_like":\{"count":(\d+)') or _parse_count(html, r'"like_count":(\d+)')
        comments = _parse_count(html, r'"edge_media_to_parent_comment":\{"count":(\d+)') or _parse_count(html, r'"comment_count":(\d+)')
        return {
            "views": views or 0,
            "likes": likes or 0,
            "comments": comments or 0,
            "shares": 0,
            "raw": {"page_source_len": len(html)},
        }


def _parse_count(html: str, pattern: str) -> int:
    m = re.search(pattern, html)
    if not m:
        return 0
    try:
        return int(m.group(1))
    except (ValueError, IndexError):
        return 0
