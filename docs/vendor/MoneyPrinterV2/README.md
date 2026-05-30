# MoneyPrinterV2 — vendored reference (read-only)

Source: https://github.com/FujiwaraChoki/MoneyPrinterV2 (FujiwaraChoki).

## License

**AGPLv3** (see `LICENSE`). This is more viral than GPLv3 — AGPL forces any
network-accessible derivative to share its source under AGPL too.

Implication: **we cannot import / copy this code into the webcomics project**.
Any of our own code that runs this code path would become AGPL-bound.

## How to use this folder

- **Read** for design inspiration only.
- DOM selectors, flow ordering, error-handling patterns: treat as observations.
- Re-implement clean-room in our own files (`workers-py/src/worker/providers/`).
- Do not paste blocks unmodified.

## Files copied (snapshot at vendoring time)

| File | Why we look at it |
|---|---|
| `YouTube.py` | upload_video flow — file picker, title, description, kids checkbox, next/done sequence, selector ids |
| `Twitter.py` | compose tweet flow — textbox selectors, post button fallbacks |
| `PostBridge.py` | reference REST API client (not selected for v1 — kept for context) |
| `constants.py` | DOM selector constants for YouTube/Twitter |

## Our derived implementation

Lives in `workers-py/src/worker/providers/selenium_youtube.py` (and `_twitter.py`).
Re-derived selector constants live next to each provider as Python constants.
