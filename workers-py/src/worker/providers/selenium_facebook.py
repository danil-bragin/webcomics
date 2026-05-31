"""Facebook Reels upload via pre-authenticated Firefox profile.

Flow (facebook.com web):
  1. open https://www.facebook.com/reel/create
     (works for personal + Page profiles; FB resolves which surface)
  2. file input (hidden)
  3. caption / описание
  4. click "Next" → "Publish" / "Поделиться"
  5. capture redirect URL → /reel/<id>

FB rolls UI tweaks faster than YT but slower than IG. Selectors here lean on
aria-labels and role=button to survive locale + AB changes.
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

log = structlog.get_logger().bind(provider="selenium_facebook")


def upload(*, firefox_profile: str, video_path: str, meta: dict[str, Any], screenshot_dir: str = "") -> dict[str, Any]:
    from selenium.webdriver.common.by import By
    from selenium.webdriver.support.ui import WebDriverWait
    from selenium.webdriver.support import expected_conditions as EC
    from selenium.common.exceptions import TimeoutException, NoSuchElementException

    caption = (meta.get("description") or meta.get("title") or "").strip()
    hashtags = " ".join(f"#{t.lstrip('#')}" for t in (meta.get("hashtags") or []))
    full_caption = (caption + ("\n\n" + hashtags if hashtags else "")).strip()

    headless = bool(meta.get("headless", True))
    with firefox_driver(firefox_profile, headless=headless) as driver:
        wait = WebDriverWait(driver, 60)
        try:
            driver.get("https://www.facebook.com/reel/create")
            take_screenshot(driver, screenshot_dir, "00-reel-create")
            jitter()

            file_input = wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "input[type=file]")))
            file_input.send_keys(os.path.abspath(video_path))
            take_screenshot(driver, screenshot_dir, "01-file-picked")

            # Wait through processing screen until "Next" enables.
            for _ in range(3):
                try:
                    nxt = wait.until(EC.element_to_be_clickable(
                        (By.XPATH, "//*[@role='button' and (normalize-space(text())='Next' or normalize-space(text())='Далее')]")
                    ))
                    nxt.click()
                    jitter()
                except TimeoutException:
                    break
            take_screenshot(driver, screenshot_dir, "02-after-next")

            # Caption editor — contenteditable, aria-label 'Describe your reel' / 'Опишите'.
            try:
                cap = wait.until(EC.element_to_be_clickable(
                    (By.XPATH, "//*[@contenteditable='true' and (@aria-label='Describe your reel' or contains(@aria-label,'Опишите'))]")
                ))
                cap.click()
                cap.send_keys(full_caption)
            except TimeoutException:
                log.warning("caption field not found, publishing without caption")
            take_screenshot(driver, screenshot_dir, "03-caption")

            # "Publish" / "Поделиться" / "Опубликовать".
            publish = wait.until(EC.element_to_be_clickable(
                (By.XPATH, "//*[@role='button' and (normalize-space(text())='Publish' or normalize-space(text())='Поделиться' or normalize-space(text())='Опубликовать')]")
            ))
            publish.click()
            take_screenshot(driver, screenshot_dir, "04-published")
            time.sleep(5)

            # Capture URL — FB redirects to a manage page; grab the latest reel link.
            try:
                reel = driver.find_element(By.CSS_SELECTOR, "a[href*='/reel/']")
                video_url = reel.get_attribute("href") or driver.current_url
            except NoSuchElementException:
                video_url = driver.current_url
            video_id = video_url.rstrip("/").split("/")[-1] if "/reel/" in video_url else ""
            visibility = (meta.get("visibility") or "public").strip()
            return {"video_url": video_url, "video_id": video_id, "final_visibility": visibility}
        except Exception as e:
            shot = take_screenshot(driver, screenshot_dir, f"99-fail-{type(e).__name__}")
            raise UploadError(f"facebook upload failed: {e}", screenshot_path=shot) from e
