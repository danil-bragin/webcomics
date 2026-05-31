"""Shared selenium helpers for the IG/TT/FB providers. Keeps the YT module
unchanged while the new providers reuse driver bootstrap + screenshot trail.

The Firefox + geckodriver dance, profile cloning, anti-detection flags, and
trail screenshotting are identical across platforms — only the per-step
selectors and URLs differ.
"""
from __future__ import annotations

import os
import random
import time
from contextlib import contextmanager
from typing import Optional

import structlog

log = structlog.get_logger().bind(component="selenium_common")


def jitter() -> float:
    """Small randomised sleep so consecutive actions don't look scripted."""
    t = random.uniform(0.4, 1.2)
    time.sleep(t)
    return t


def take_screenshot(driver, screenshot_dir: str, stage: str) -> str:
    """Save a screenshot under screenshot_dir with a step-numbered prefix so
    sorted listdir yields the chronological trail (used by the API on
    upload completion)."""
    if not screenshot_dir:
        return ""
    os.makedirs(screenshot_dir, exist_ok=True)
    existing = [n for n in os.listdir(screenshot_dir) if n.endswith(".png")]
    idx = len(existing) + 1
    name = f"{idx:02d}-step-{stage}.png"
    path = os.path.join(screenshot_dir, name)
    try:
        driver.save_screenshot(path)
    except Exception as e:
        log.warning("screenshot failed", stage=stage, err=str(e))
        return ""
    return path


@contextmanager
def firefox_driver(profile_path: str, headless: bool = True):
    """Yield a webdriver.Firefox bound to a cloned profile dir."""
    from selenium import webdriver
    from selenium.webdriver.firefox.options import Options
    from selenium.webdriver.firefox.service import Service

    opts = Options()
    if headless:
        opts.add_argument("-headless")
    opts.set_preference("dom.webdriver.enabled", False)
    opts.set_preference("useAutomationExtension", False)
    # Use the cloned profile so we don't trip Firefox's parent.lock.
    opts.add_argument("-profile")
    opts.add_argument(profile_path)
    # Realistic viewport.
    opts.add_argument("--width=1280")
    opts.add_argument("--height=2200")  # tall so reels editor fits
    service = Service(log_output=os.devnull)
    driver = webdriver.Firefox(options=opts, service=service)
    driver.set_window_size(1280, 2200)
    driver.implicitly_wait(2)
    try:
        yield driver
    finally:
        try:
            driver.quit()
        except Exception:
            pass


class UploadError(RuntimeError):
    """Wraps a selenium exception with the screenshot path the API displays."""

    def __init__(self, message: str, screenshot_path: str = "", fallback_video_url: str = "", fallback_video_id: str = ""):
        super().__init__(message)
        self.screenshot_path = screenshot_path
        self.fallback_video_url = fallback_video_url
        self.fallback_video_id = fallback_video_id


def safe_step(driver, screenshot_dir: str, stage: str):
    """Decorator-like helper: call this when each major step finishes so we
    have a trail screenshot. Returns the screenshot path or empty string."""
    return take_screenshot(driver, screenshot_dir, stage)


__all__ = [
    "jitter",
    "take_screenshot",
    "firefox_driver",
    "UploadError",
    "safe_step",
    "Optional",
]
