"""Facebook reel metrics scraper.

FB inlines counts as text near the reel ("12K просмотров", "234 reactions").
We regex aggressively over the rendered DOM. Numbers come back as ints
(K/M expanded). Best-effort — FB ships UI changes weekly.
"""
from __future__ import annotations

import re
import time
from typing import Any

import structlog

from ._selenium_common import firefox_driver

log = structlog.get_logger().bind(provider="metrics_facebook")


def fetch(*, external_ref: str, profile_path: str) -> dict[str, Any]:
    if not profile_path:
        raise RuntimeError("profile_path required for facebook metrics scrape")
    with firefox_driver(profile_path, headless=True) as driver:
        driver.get(external_ref)
        time.sleep(5)
        html = driver.page_source
        return {
            "views":    _extract(html, r'([\d.,]+\s*[KMM]?)\s*(?:views|просмотр)'),
            "likes":    _extract(html, r'([\d.,]+\s*[KMM]?)\s*(?:reactions|реакц|like|лайк)'),
            "comments": _extract(html, r'([\d.,]+\s*[KMM]?)\s*(?:comments|коммент)'),
            "shares":   _extract(html, r'([\d.,]+\s*[KMM]?)\s*(?:shares|поделил)'),
            "raw": {"page_source_len": len(html)},
        }


def _extract(html: str, pattern: str) -> int:
    m = re.search(pattern, html, re.IGNORECASE)
    if not m:
        return 0
    return _parse_count(m.group(1))


def _parse_count(s: str) -> int:
    s = s.strip().replace(",", "").replace(" ", "")
    mult = 1
    if s.endswith("K") or s.endswith("k"):
        mult = 1_000
        s = s[:-1]
    elif s.endswith("M") or s.endswith("m"):
        mult = 1_000_000
        s = s[:-1]
    try:
        return int(float(s) * mult)
    except ValueError:
        return 0
