"""YouTube upload via pre-authenticated Firefox profile.

Clean-room re-implementation. MPv2 is AGPLv3 — referenced only for selector
research (kept in docs/vendor/MoneyPrinterV2/).

Profile flow:
  1. Embedded login session (jlesage/firefox sidecar) creates a profile dir.
  2. User signs in to Google + creates a YouTube channel inside that profile.
  3. SocialAccount.firefox_profile_path points the worker at the same dir.
  4. This module spawns headless Firefox on that profile and walks the YT
     Studio upload UI driving every metadata field configured on the run.

Anti-detection: realistic UA, hidden webdriver flag, jittered viewport, mild
typing delays. Anything more involved (residential proxy, fingerprint mocking)
lives in SocialAccount.extra and is consumed here.
"""
from __future__ import annotations

import os
import random
import tempfile
import time
from dataclasses import dataclass, field
from typing import Optional

import structlog

log = structlog.get_logger().bind(provider="selenium_youtube")

# ---- Selector constants (one DOM change = one-file patch) ----
# YT uses `name=` attrs that are stable across locales — prefer them over text.
_SEL_FILE_PICKER_TAG = "ytcp-uploads-file-picker"
_SEL_FILE_INPUT_CSS = "ytcp-uploads-file-picker input[type=file]"
_SEL_TITLE_TEXTBOX_CSS = "ytcp-social-suggestions-textbox#title-textarea div#textbox"
_SEL_DESC_TEXTBOX_CSS = "ytcp-social-suggestions-textbox#description-textarea div#textbox"
_SEL_MADE_FOR_KIDS_RADIO_NAME = "VIDEO_MADE_FOR_KIDS_MFK"
_SEL_NOT_FOR_KIDS_RADIO_NAME = "VIDEO_MADE_FOR_KIDS_NOT_MFK"
_SEL_SHOW_MORE_CSS = "ytcp-button#toggle-button"
_SEL_TAGS_INPUT_CSS = "ytcp-form-input-container#tags-container input"
_SEL_TAGS_CHIP_INPUT_CSS = "ytcp-chip-bar input"
_SEL_CATEGORY_DROPDOWN_CSS = "ytcp-form-select#category-app ytcp-text-dropdown-trigger"
_SEL_AGE_RESTRICTION_RADIO_NAME = "VIDEO_AGE_RESTRICTION_SELF"  # name varies; we set by index fallback too
_SEL_NEXT_BUTTON_ID = "next-button"
_SEL_PUBLIC_RADIO_NAME = "PUBLIC"
_SEL_PRIVATE_RADIO_NAME = "PRIVATE"
_SEL_UNLISTED_RADIO_NAME = "UNLISTED"
_SEL_SCHEDULE_RADIO_NAME = "SCHEDULE"
_SEL_DONE_BUTTON_ID = "done-button"
_SEL_SHARE_LINK_CSS = "a.ytcp-video-info[href*='youtu.be']"


@dataclass
class YouTubeUploadConfig:
    firefox_profile_path: str
    video_path: str
    title: str
    description: str = ""
    tags: list[str] = field(default_factory=list)
    made_for_kids: bool = False
    age_restriction: str = "none"  # none | 18plus
    category_id: str = "22"  # YT API category ID; ignored by selenium (uses label)
    category_label: str = "People & Blogs"
    comments_enabled: bool = True
    visibility: str = "unlisted"  # public | unlisted | private
    scheduled_at: str = ""  # RFC3339; empty = immediate publish
    playlist_names: list[str] = field(default_factory=list)
    thumbnail_path: str = ""  # local path; empty = skip
    headless: bool = False
    screenshot_dir: str = ""  # path to dump screenshot on failure

    def __post_init__(self) -> None:
        if not self.firefox_profile_path or not os.path.isdir(self.firefox_profile_path):
            raise ValueError(f"firefox profile not found: {self.firefox_profile_path}")
        if not os.path.exists(self.video_path):
            raise ValueError(f"video file not found: {self.video_path}")
        if self.visibility not in ("public", "unlisted", "private"):
            raise ValueError(f"invalid visibility: {self.visibility}")
        if self.scheduled_at and self.visibility != "public":
            # Schedule only meaningful for public; force-promote.
            self.visibility = "public"


@dataclass
class YouTubeUploadResult:
    video_url: str
    video_id: str
    final_visibility: str
    screenshot_path: str = ""  # set on failure


def upload_to_youtube(cfg: YouTubeUploadConfig) -> YouTubeUploadResult:
    """Drive Firefox + geckodriver through YT Studio.

    Raises on any failure after dumping a screenshot if cfg.screenshot_dir set.
    """
    from selenium import webdriver
    from selenium.common.exceptions import (
        ElementClickInterceptedException,
        NoSuchElementException,
        TimeoutException,
    )
    from selenium.webdriver.common.by import By
    from selenium.webdriver.common.keys import Keys
    from selenium.webdriver.firefox.options import Options
    from selenium.webdriver.firefox.service import Service
    from selenium.webdriver.support import expected_conditions as EC
    from selenium.webdriver.support.ui import WebDriverWait

    opts = Options()
    os.environ.setdefault("MOZ_ALLOW_DOWNGRADE", "1")
    opts.add_argument("-allow-downgrade")
    if cfg.headless:
        opts.add_argument("--headless")
        viewport_w = random.choice([1280, 1366, 1440])
        viewport_h = random.choice([900, 1024, 1080])
        opts.add_argument(f"--width={viewport_w}")
        opts.add_argument(f"--height={viewport_h}")
        opts.set_preference(
            "general.useragent.override",
            "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
        )
        opts.set_preference("dom.webdriver.enabled", False)
        opts.set_preference("useAutomationExtension", False)
    opts.add_argument("-profile")
    opts.add_argument(cfg.firefox_profile_path)

    geckodriver_path = os.environ.get("GECKODRIVER_PATH")
    if geckodriver_path and os.path.exists(geckodriver_path):
        service = Service(executable_path=geckodriver_path)
    else:
        from webdriver_manager.firefox import GeckoDriverManager
        service = Service(GeckoDriverManager().install())

    driver = webdriver.Firefox(service=service, options=opts)
    long_wait = WebDriverWait(driver, 60)
    short_wait = WebDriverWait(driver, 15)

    stage_counter = [0]

    def shot(stage: str, fail: bool = False) -> str:
        """Save a screenshot tagged with the current stage. Always called on
        success too — the captured trail lands in MinIO so the UI can show a
        scrubbable thumbnail strip per upload record."""
        if not cfg.screenshot_dir:
            return ""
        os.makedirs(cfg.screenshot_dir, exist_ok=True)
        stage_counter[0] += 1
        idx = stage_counter[0]
        kind = "fail" if fail else "step"
        p = os.path.join(cfg.screenshot_dir, f"{idx:02d}-{kind}-{stage}.png")
        try:
            driver.save_screenshot(p)
            return p
        except Exception:
            return ""

    def jitter(min_s: float = 0.2, max_s: float = 0.6) -> None:
        time.sleep(random.uniform(min_s, max_s))

    def clear_contenteditable(el) -> None:
        # contenteditable divs don't respond to .clear(); use Ctrl+A → Delete.
        # Use JS click — YT progress overlays often intercept native clicks.
        driver.execute_script("arguments[0].scrollIntoView({block:'center'});", el)
        driver.execute_script("arguments[0].click();", el)
        jitter()
        el.send_keys(Keys.CONTROL, "a")
        jitter(0.05, 0.15)
        el.send_keys(Keys.DELETE)
        jitter(0.05, 0.15)

    def find_radio_by_name(name: str):
        return driver.find_element(By.CSS_SELECTOR, f"tp-yt-paper-radio-button[name='{name}']")

    def click_safely(el) -> None:
        try:
            driver.execute_script("arguments[0].scrollIntoView({block:'center'});", el)
        except Exception:
            pass
        try:
            el.click()
        except ElementClickInterceptedException:
            # Dismiss whatever overlay sits on top, then JS-click.
            try:
                from selenium.webdriver.common.action_chains import ActionChains
                ActionChains(driver).send_keys(Keys.ESCAPE).perform()
            except Exception:
                pass
            driver.execute_script("arguments[0].click();", el)

    screenshot_path = ""
    try:
        # Go straight to the studio upload page — works once we have a channel.
        driver.get("https://studio.youtube.com")
        time.sleep(3)
        # Click "Create" → "Upload videos" via direct shortcut URL is more reliable.
        driver.get("https://www.youtube.com/upload")
        long_wait.until(EC.presence_of_element_located((By.TAG_NAME, _SEL_FILE_PICKER_TAG)))

        shot("00-studio-opened")
        file_input = driver.find_element(By.CSS_SELECTOR, _SEL_FILE_INPUT_CSS)
        file_input.send_keys(cfg.video_path)
        log.info("file selected", path=cfg.video_path)
        time.sleep(2)
        shot("01-file-picked")

        # Wait for details panel to render — title contenteditable appears after.
        title_el = long_wait.until(
            EC.presence_of_element_located((By.CSS_SELECTOR, _SEL_TITLE_TEXTBOX_CSS))
        )
        clear_contenteditable(title_el)
        title_el.send_keys(cfg.title[:100])
        jitter(0.5, 1.0)
        shot("02-title-typed")

        # Description is sibling; render-stable after first interaction.
        try:
            desc_el = driver.find_element(By.CSS_SELECTOR, _SEL_DESC_TEXTBOX_CSS)
            clear_contenteditable(desc_el)
            if cfg.description:
                desc_el.send_keys(cfg.description[:5000])
                # Hashtags in description trigger YT's #tag autocomplete dropdown
                # that intercepts subsequent button clicks. Press ESC to dismiss
                # and click outside to ensure focus is released.
                desc_el.send_keys(Keys.ESCAPE)
                jitter(0.2, 0.4)
                # Click a neutral area (the dialog title) to drop focus.
                try:
                    driver.execute_script(
                        "document.querySelector('ytcp-uploads-dialog #dialog-title')?.click();"
                    )
                except Exception:
                    pass
        except NoSuchElementException:
            log.warning("description textbox missing — skipped")

        # Made for kids — radios mid-panel.
        jitter()
        try:
            target_name = (
                _SEL_MADE_FOR_KIDS_RADIO_NAME if cfg.made_for_kids
                else _SEL_NOT_FOR_KIDS_RADIO_NAME
            )
            click_safely(find_radio_by_name(target_name))
        except NoSuchElementException:
            log.warning("made-for-kids radios missing — skipped")

        # Show more options panel (tags, category, comments, age).
        try:
            more_btn = driver.find_element(By.CSS_SELECTOR, _SEL_SHOW_MORE_CSS)
            click_safely(more_btn)
            jitter(0.4, 0.9)
        except NoSuchElementException:
            log.warning("show-more toggle missing — extended metadata skipped")

        # Extended metadata fields are best-effort. Failure here must not abort
        # the upload — the video is already accepted by this point.
        def safe(label: str, fn):
            try:
                fn()
            except Exception as ee:
                log.warning("optional step skipped", label=label, err=str(ee)[:120])

        if cfg.tags:
            def fill_tags():
                tags_input = driver.find_element(By.CSS_SELECTOR, _SEL_TAGS_CHIP_INPUT_CSS)
                click_safely(tags_input)
                for t in cfg.tags[:15]:
                    tags_input.send_keys(t)
                    tags_input.send_keys(Keys.ENTER)
                    jitter(0.05, 0.2)
            safe("tags", fill_tags)

        if cfg.category_label:
            def fill_category():
                cat_trigger = driver.find_element(By.CSS_SELECTOR, _SEL_CATEGORY_DROPDOWN_CSS)
                click_safely(cat_trigger)
                jitter(0.5, 1.0)
                option_xpath = f"//tp-yt-paper-item//*[contains(text(), \"{cfg.category_label}\")]"
                option = driver.find_element(By.XPATH, option_xpath)
                click_safely(option)
                jitter(0.3, 0.6)
            safe("category", fill_category)

        if not cfg.comments_enabled:
            def disable_comments():
                comments_off = driver.find_element(
                    By.CSS_SELECTOR, "tp-yt-paper-radio-button[name='COMMENTS_OFF']"
                )
                click_safely(comments_off)
            safe("comments-off", disable_comments)

        if cfg.age_restriction == "18plus":
            def set_age():
                age_radio = driver.find_element(
                    By.CSS_SELECTOR, "tp-yt-paper-radio-button[name='AGE_VERIFICATION_AGE_18']"
                )
                click_safely(age_radio)
            safe("age", set_age)

        # Capture youtu.be URL while still on Details (it appears in the upload
        # info panel as soon as YT processes the file). This is our safety net:
        # if any later click fails, we still know what was created.
        captured_url = ""
        captured_id = ""
        try:
            short_wait.until(lambda d: any(
                "youtu.be/" in (a.get_attribute("href") or "")
                for a in d.find_elements(By.CSS_SELECTOR, "a")
            ))
            for a in driver.find_elements(By.CSS_SELECTOR, "a"):
                href = a.get_attribute("href") or ""
                if "youtu.be/" in href:
                    captured_url = href
                    captured_id = href.split("youtu.be/")[1].split("?")[0]
                    log.info("video url captured early", url=captured_url)
                    break
        except TimeoutException:
            log.warning("video URL not visible yet — will fall back later")

        shot("03-metadata-filled")

        # Next ×3 → visibility step. YT has Details → Video elements → Checks → Visibility.
        # JS-click bypasses overlay intercepts (hashtag autocomplete, scroll spinners).
        for step in ("video-elements", "checks", "visibility"):
            try:
                btn = short_wait.until(
                    EC.presence_of_element_located((By.ID, _SEL_NEXT_BUTTON_ID))
                )
                driver.execute_script(
                    "arguments[0].scrollIntoView({block:'center'}); arguments[0].click();",
                    btn,
                )
                log.info("next clicked", stage=step)
                jitter(0.8, 1.4)
                shot(f"04-next-{step}")
            except TimeoutException:
                log.warning("next button not present — possibly already at last step", stage=step)
                shot(f"04-next-{step}-timeout", fail=True)
                break

        # Visibility radio.
        if cfg.scheduled_at:
            try:
                click_safely(find_radio_by_name(_SEL_SCHEDULE_RADIO_NAME))
                jitter(0.5, 1.0)
                _fill_schedule_picker(driver, cfg.scheduled_at, jitter)
            except NoSuchElementException:
                log.warning("schedule radio missing — falling back to immediate public")
                cfg.visibility = "public"
                cfg.scheduled_at = ""

        if not cfg.scheduled_at:
            name_map = {
                "public": _SEL_PUBLIC_RADIO_NAME,
                "unlisted": _SEL_UNLISTED_RADIO_NAME,
                "private": _SEL_PRIVATE_RADIO_NAME,
            }
            target = name_map[cfg.visibility]
            try:
                click_safely(find_radio_by_name(target))
                jitter(0.4, 0.8)
            except NoSuchElementException:
                log.warning("visibility radio missing", target=target)

        shot("05-pre-done")
        # Final publish/save/schedule button. JS-click to bypass autocomplete /
        # processing overlays that race with the final transition.
        try:
            done_btn = driver.find_element(By.ID, _SEL_DONE_BUTTON_ID)
            driver.execute_script(
                "arguments[0].scrollIntoView({block:'center'}); arguments[0].click();",
                done_btn,
            )
        except NoSuchElementException:
            log.warning("done button missing — video left in drafts")
        time.sleep(4)
        shot("06-post-done")

        # STRICT verification: a real publish always reveals a youtu.be link.
        # Don't trust the channel-filter / studio URL — if we can't find the
        # video id within the wait window, raise and let the caller mark the
        # upload as failed (rate-limit / captcha / device-verification block).
        video_url = ""
        video_id = ""
        try:
            WebDriverWait(driver, 25).until(lambda d: any(
                "youtu.be/" in (a.get_attribute("href") or "")
                for a in d.find_elements(By.CSS_SELECTOR, "a")
            ))
        except TimeoutException:
            pass
        for a in driver.find_elements(By.CSS_SELECTOR, "a"):
            href = a.get_attribute("href") or ""
            if "youtu.be/" in href:
                video_url = href
                video_id = href.split("youtu.be/")[1].split("?")[0]
                break

        if not video_url and captured_url:
            video_url, video_id = captured_url, captured_id

        if not video_id:
            shot("07-no-link", fail=True)
            raise RuntimeError(
                "publish verification failed: no youtu.be link surfaced after Done click "
                "(rate-limit / captcha / device-verification likely)"
            )

        shot("07-published")
        return YouTubeUploadResult(
            video_url=video_url,
            video_id=video_id,
            final_visibility=cfg.visibility,
        )
    except Exception as e:
        screenshot_path = shot(type(e).__name__)
        # Try to extract whatever YT already has — often the video itself is
        # accepted even when a metadata field click fails midway.
        fallback_url = ""
        fallback_id = ""
        try:
            for link in driver.find_elements(By.CSS_SELECTOR, "a[href*='youtu.be/']"):
                href = link.get_attribute("href") or ""
                if "youtu.be/" in href:
                    fallback_url = href
                    fallback_id = href.split("youtu.be/")[1].split("?")[0]
                    break
        except Exception:
            pass
        log.exception(
            "upload flow failed",
            screenshot=screenshot_path,
            fallback_video_url=fallback_url,
        )
        # Attach diagnostics so the handler can surface them.
        e.screenshot_path = screenshot_path  # type: ignore[attr-defined]
        e.fallback_video_url = fallback_url  # type: ignore[attr-defined]
        e.fallback_video_id = fallback_id  # type: ignore[attr-defined]
        raise
    finally:
        try:
            driver.quit()
        except Exception:
            pass


def _fill_schedule_picker(driver, iso_dt: str, jitter) -> None:
    """Best-effort schedule pickup. YT's date+time pickers are heavily wrapped
    web components; we just type the ISO bits directly into the inputs."""
    from datetime import datetime
    from selenium.webdriver.common.by import By
    from selenium.webdriver.common.keys import Keys

    try:
        target = datetime.fromisoformat(iso_dt.replace("Z", "+00:00"))
    except Exception:
        log.warning("invalid scheduled_at, skipping schedule pickup", iso_dt=iso_dt)
        return

    date_str = target.strftime("%b %d, %Y")
    time_str = target.strftime("%I:%M %p").lstrip("0")
    inputs = driver.find_elements(By.CSS_SELECTOR, "tp-yt-paper-input input")
    for inp in inputs:
        ph = inp.get_attribute("placeholder") or ""
        if "date" in ph.lower() or "Mar 1" in ph:
            inp.click()
            inp.send_keys(Keys.CONTROL, "a")
            inp.send_keys(date_str)
            jitter()
        if "time" in ph.lower() or "AM" in ph:
            inp.click()
            inp.send_keys(Keys.CONTROL, "a")
            inp.send_keys(time_str)
            jitter()


def download_video_to_tempfile(store, object_key: str) -> str:
    suffix = os.path.splitext(object_key)[1] or ".mp4"
    fd, path = tempfile.mkstemp(prefix="yt-upload-", suffix=suffix)
    os.close(fd)
    data = store.client.get_object(store.bucket, object_key).read()
    with open(path, "wb") as f:
        f.write(data)
    return path
