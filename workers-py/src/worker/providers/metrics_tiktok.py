"""TikTok video metrics scraper.

TT puts counts in the SIGI_STATE JSON island on the video page. We pull
the embedded JSON and read counters out of it. Falls back to DOM text
parsing if the JSON shape changes.
"""
from __future__ import annotations

import json
import re
import time
from typing import Any

import structlog

from ._selenium_common import firefox_driver

log = structlog.get_logger().bind(provider="metrics_tiktok")


def fetch(*, external_ref: str, profile_path: str) -> dict[str, Any]:
    if not profile_path:
        raise RuntimeError("profile_path required for tiktok metrics scrape")
    with firefox_driver(profile_path, headless=True) as driver:
        driver.get(external_ref)
        time.sleep(4)
        html = driver.page_source
        stats = _parse_sigi_state(html) or _parse_dom(html)
        return {
            "views":    int(stats.get("playCount") or stats.get("views") or 0),
            "likes":    int(stats.get("diggCount") or stats.get("likes") or 0),
            "comments": int(stats.get("commentCount") or stats.get("comments") or 0),
            "shares":   int(stats.get("shareCount") or stats.get("shares") or 0),
            "raw": {"page_source_len": len(html)},
        }


def _parse_sigi_state(html: str) -> dict[str, Any]:
    m = re.search(r'<script id="SIGI_STATE" type="application/json">(.+?)</script>', html, re.DOTALL)
    if not m:
        return {}
    try:
        state = json.loads(m.group(1))
        items = state.get("ItemModule") or {}
        for _id, item in items.items():
            stats = item.get("stats")
            if stats:
                return stats
    except Exception:
        return {}
    return {}


def _parse_dom(html: str) -> dict[str, Any]:
    out: dict[str, Any] = {}
    for key, pattern in (
        ("playCount",    r'"playCount":(\d+)'),
        ("diggCount",    r'"diggCount":(\d+)'),
        ("commentCount", r'"commentCount":(\d+)'),
        ("shareCount",   r'"shareCount":(\d+)'),
    ):
        m = re.search(pattern, html)
        if m:
            out[key] = int(m.group(1))
    return out
