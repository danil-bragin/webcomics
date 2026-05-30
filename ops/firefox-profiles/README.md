# Firefox profiles for selenium upload workers

Each subdirectory here is a persistent Firefox profile that the `worker-upload`
container reuses for selenium-based posting (YouTube Shorts, X, etc.). Mounting
the directory into the container preserves the YouTube session cookies between
restarts.

## One-time login (per social account)

```bash
# 1. Spin up the helper Firefox container with web-VNC (port 5800).
docker compose -f dev.compose.yml --profile setup up firefox-login

# 2. Open http://localhost:5800 in a real browser.
# 3. Inside that Firefox: go to https://accounts.google.com, sign in to the
#    target YouTube account. Complete any 2FA prompts.
# 4. Visit https://studio.youtube.com once to land in YouTube Studio (verifies
#    the account is creator-enabled).
# 5. Click "Exit" in the jlesage UI menu. Stop the compose service.

# 6. Find the actual profile path inside the volume:
ls ops/firefox-profiles/youtube-main/.mozilla/firefox/
# e.g. abc1234.default-release

# 7. Register a SocialAccount pointing at the *inner* profile dir (the worker
#    sees it as /profiles/youtube-main/.mozilla/firefox/<id>):
curl -X POST http://localhost:8080/api/projects/<PID>/social-accounts \
  -H 'content-type: application/json' \
  -d '{
    "platform": "youtube_selenium",
    "label": "main youtube",
    "firefox_profile_path": "/profiles/youtube-main/.mozilla/firefox/abc1234.default-release"
  }'
```

## Daily worker run

The `worker-upload` service mounts `./ops/firefox-profiles` to `/profiles`
read-write. Headless Firefox launches with `-profile <path>`, picks up the
saved cookies, and drives YouTube Studio. `GECKODRIVER_PATH=/usr/local/bin/geckodriver`
skips webdriver-manager's network fetch at boot.

## Maintenance

- YouTube DOM selectors live in `workers-py/src/worker/providers/selenium_youtube.py`.
- If YouTube forces a re-login (rare; usually 6+ months), repeat the one-time
  flow above. The profile dir is reused, so no SocialAccount changes needed.
- For a second account, create a new sibling dir (e.g. `youtube-secondary`)
  and a second `firefox-login-N` service or repeat with a different volume.
- **Don't** commit profile dirs to git — add to `.gitignore`.
