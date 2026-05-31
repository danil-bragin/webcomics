"""TikTok upload via pre-authenticated Firefox profile.

Flow (tiktok.com web):
  1. open https://www.tiktok.com/tiktokstudio/upload?from=upload
  2. wait for file input (hidden), .send_keys(video_path)
  3. wait for the caption editor (contenteditable div)
  4. type caption + hashtags
  5. click "Post" / "Опубликовать"
  6. wait for redirect to /tiktokstudio/content, capture the topmost
     video link.

TikTok studio UI changes more often than YT; selectors here are best-effort
and each step screenshots so failures surface with context.
"""
from __future__ import annotations

import os
import time
from typing import Any

import structlog

from ._selenium_common import (
    UploadError,
    firefox_driver,
    jitter,
    take_screenshot,
)

log = structlog.get_logger().bind(provider="selenium_tiktok")


def upload(*, firefox_profile: str, video_path: str, meta: dict[str, Any], screenshot_dir: str = "") -> dict[str, Any]:
    from selenium.webdriver.common.by import By
    from selenium.webdriver.common.keys import Keys
    from selenium.webdriver.support.ui import WebDriverWait
    from selenium.webdriver.support import expected_conditions as EC
    from selenium.common.exceptions import TimeoutException, NoSuchElementException

    caption = (meta.get("description") or meta.get("title") or "").strip()
    hashtags = " ".join(f"#{t.lstrip('#')}" for t in (meta.get("hashtags") or []))
    full_caption = (caption + (" " + hashtags if hashtags else "")).strip()[:2200]

    headless = bool(meta.get("headless", True))
    with firefox_driver(firefox_profile, headless=headless) as driver:
        wait = WebDriverWait(driver, 60)
        try:
            driver.get("https://www.tiktok.com/tiktokstudio/upload?from=upload")
            take_screenshot(driver, screenshot_dir, "00-studio")
            jitter()

            # Hidden file input — present from page load.
            file_input = wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "input[type=file]")))
            file_input.send_keys(os.path.abspath(video_path))
            take_screenshot(driver, screenshot_dir, "01-file-picked")

            # Wait for the upload progress + caption editor.
            # Caption editor is a contenteditable div with role=combobox in newer TT studios.
            cap_el = None
            for sel in (
                "div[role='combobox'][contenteditable='true']",
                "div[data-text='true']",
                "div[contenteditable='true']",
            ):
                try:
                    cap_el = wait.until(EC.element_to_be_clickable((By.CSS_SELECTOR, sel)))
                    break
                except TimeoutException:
                    continue
            if cap_el is None:
                raise TimeoutException("tiktok caption editor not found")
            cap_el.click()
            jitter()
            # Clear any pre-filled filename.
            cap_el.send_keys(Keys.CONTROL, "a")
            cap_el.send_keys(Keys.DELETE)
            cap_el.send_keys(full_caption)
            take_screenshot(driver, screenshot_dir, "02-caption")

            # Visibility — TT defaults to "Everyone" / public. We just press post.
            # Optional: set "Public/Friends/Only me" via radio.
            visibility = (meta.get("visibility") or "public").strip()

            # "Post" / "Опубликовать" button.
            post = wait.until(EC.element_to_be_clickable(
                (By.XPATH, "//button[normalize-space(text())='Post' or normalize-space(text())='Опубликовать']")
            ))
            post.click()
            take_screenshot(driver, screenshot_dir, "03-post-clicked")

            # Wait for redirect or success modal.
            try:
                wait.until(EC.url_contains("/tiktokstudio/content"))
            except TimeoutException:
                time.sleep(5)
            take_screenshot(driver, screenshot_dir, "04-post-redirect")

            # Pull external ref: topmost video link in the content table.
            try:
                row = driver.find_element(By.CSS_SELECTOR, "a[href*='/video/']")
                video_url = row.get_attribute("href") or ""
            except NoSuchElementException:
                video_url = "https://www.tiktok.com/"
            video_id = video_url.rstrip("/").split("/")[-1] if "/video/" in video_url else ""
            return {"video_url": video_url, "video_id": video_id, "final_visibility": visibility}
        except Exception as e:
            shot = take_screenshot(driver, screenshot_dir, f"99-fail-{type(e).__name__}")
            raise UploadError(f"tiktok upload failed: {e}", screenshot_path=shot) from e
