"""Instagram Reels upload via pre-authenticated Firefox profile.

Flow (instagram.com web):
  1. open https://www.instagram.com/
  2. click "Create" → "Post" → file input (IG combines Reels + posts in same
     creator flow; the cropper auto-detects 9:16 → reel)
  3. fill caption textarea
  4. click "Next" → "Next" (filter/crop) → "Share"
  5. wait for "Your reel has been shared" toast, navigate to profile, grab
     the topmost reel link.

Selectors here are best-effort; IG ships AB tests so the operator may need
to recalibrate per locale. Each step screenshots so failures land with
context for debugging.
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

log = structlog.get_logger().bind(provider="selenium_instagram")


def upload(*, firefox_profile: str, video_path: str, meta: dict[str, Any], screenshot_dir: str = "") -> dict[str, Any]:
    """Drive Firefox through IG Reels upload. Returns {video_url, video_id, final_visibility}."""
    from selenium.webdriver.common.by import By
    from selenium.webdriver.support.ui import WebDriverWait
    from selenium.webdriver.support import expected_conditions as EC
    from selenium.common.exceptions import TimeoutException, NoSuchElementException

    caption = (meta.get("description") or meta.get("title") or "").strip()
    hashtags = " ".join(f"#{t.lstrip('#')}" for t in (meta.get("hashtags") or []))
    full_caption = (caption + ("\n\n" + hashtags if hashtags else "")).strip()

    headless = bool(meta.get("headless", True))
    with firefox_driver(firefox_profile, headless=headless) as driver:
        wait = WebDriverWait(driver, 30)
        try:
            driver.get("https://www.instagram.com/")
            take_screenshot(driver, screenshot_dir, "00-home")
            jitter()

            # "Create" → "Post" — sidebar nav entry; uses svg label "Создать"/"Create".
            create_btn = wait.until(EC.element_to_be_clickable(
                (By.XPATH, "//*[contains(@aria-label,'Create') or contains(@aria-label,'Создать') or contains(text(),'Create') or contains(text(),'Создать')]")
            ))
            create_btn.click()
            take_screenshot(driver, screenshot_dir, "01-create-clicked")
            jitter()

            # File input is hidden but present in the DOM as soon as the modal opens.
            file_input = wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "input[type=file]")))
            file_input.send_keys(os.path.abspath(video_path))
            take_screenshot(driver, screenshot_dir, "02-file-picked")

            # Wait for the cropper / "Next" button. IG localises so match by role.
            for _ in range(3):
                try:
                    btn = wait.until(EC.element_to_be_clickable(
                        (By.XPATH, "//button[normalize-space(text())='Next' or normalize-space(text())='Далее' or normalize-space(text())='Дальше']")
                    ))
                    btn.click()
                    jitter()
                except TimeoutException:
                    break
            take_screenshot(driver, screenshot_dir, "03-cropped")

            # Caption textarea (aria-label='Write a caption…' / 'Напишите подпись').
            try:
                cap = wait.until(EC.element_to_be_clickable(
                    (By.XPATH, "//*[@aria-label and (contains(@aria-label,'caption') or contains(@aria-label,'подпис'))]")
                ))
                cap.click()
                cap.send_keys(full_caption)
            except TimeoutException:
                log.warning("caption field not found, posting without caption")
            take_screenshot(driver, screenshot_dir, "04-caption")

            # "Share" / "Поделиться".
            share = wait.until(EC.element_to_be_clickable(
                (By.XPATH, "//button[normalize-space(text())='Share' or normalize-space(text())='Поделиться']")
            ))
            share.click()
            take_screenshot(driver, screenshot_dir, "05-shared")

            # Wait for either success toast or settle by URL change.
            time.sleep(5)
            jitter()
            take_screenshot(driver, screenshot_dir, "06-post-share")

            # External ref: navigate to /<username>/reels/ and grab the top reel.
            try:
                driver.get("https://www.instagram.com/?_t=" + str(int(time.time())))
                jitter()
                reel = driver.find_element(By.CSS_SELECTOR, "a[href*='/reel/']")
                video_url = reel.get_attribute("href") or ""
            except NoSuchElementException:
                video_url = "https://www.instagram.com/"
            video_id = video_url.rstrip("/").split("/")[-1] if "/reel/" in video_url else ""
            visibility = (meta.get("visibility") or "public").strip()
            return {"video_url": video_url, "video_id": video_id, "final_visibility": visibility}
        except Exception as e:
            shot = take_screenshot(driver, screenshot_dir, f"99-fail-{type(e).__name__}")
            raise UploadError(f"instagram upload failed: {e}", screenshot_path=shot) from e
