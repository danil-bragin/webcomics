# YouTube Data API upload — Google Cloud setup

This enables uploading to YouTube via the **official API** (instead of Selenium).
You do these steps once in the Google Cloud Console, then paste two values into
`web-api/.env`. Use the Google account that **owns the YouTube channel**.

## 1. Create a project
1. Go to https://console.cloud.google.com/
2. Top bar → project dropdown → **New Project** → name it (e.g. `webcomics-yt`) → **Create**.
3. Make sure the new project is selected.

## 2. Enable the API
1. https://console.cloud.google.com/apis/library/youtube.googleapis.com
2. Click **Enable**.

## 3. OAuth consent screen
1. https://console.cloud.google.com/apis/credentials/consent
2. User type: **External** → **Create**.
3. App name (anything), your email for support + developer contact → **Save and Continue**.
4. **Scopes** → **Add or remove scopes** → filter for `youtube.upload` → check
   `.../auth/youtube.upload` (and optionally `.../auth/youtube.readonly`) → **Update** → **Save and Continue**.
5. **Test users** → **Add users** → add the **Google account that owns the channel** → **Save and Continue**.
   (You stay in "Testing" mode — that's fine for personal use. The consent screen
   will show an "unverified app" warning you can click through; no Google review needed.)

## 4. Create the OAuth client
1. https://console.cloud.google.com/apis/credentials
2. **Create credentials** → **OAuth client ID**.
3. Application type: **Web application**.
4. **Authorized redirect URIs** → **Add URI** →
   `http://localhost:8080/api/youtube-oauth/callback`
5. **Create**. Copy the **Client ID** and **Client secret**.

## 5. Put creds in the app
Add to `web-api/.env`:
```
GOOGLE_OAUTH_CLIENT_ID=<your client id>
GOOGLE_OAUTH_CLIENT_SECRET=<your client secret>
GOOGLE_OAUTH_REDIRECT_URL=http://localhost:8080/api/youtube-oauth/callback
```
Restart the api. Then on the **Social → YouTube** account card, click
**Connect via API**, approve in the Google popup, and the channel is linked for
API uploads.

## Quota (important)
- `videos.insert` costs **1600 units**; the default daily quota is **10 000 units**
  ⇒ about **6 uploads/day** via API.
- The app counts API uploads and **falls back to Selenium** when the daily API
  budget is used up.
- To upload more per day, request a quota increase:
  **IAM & Admin → Quotas** → filter "YouTube Data API v3" → select the queries quota → **Edit**.

## Notes
- The `youtube.upload` scope is "sensitive". For personal use you do **not** need
  Google's app verification — keep the app in Testing and add yourself as a Test user.
- The refresh token is stored per channel and used to mint short-lived access
  tokens. It is a secret; the app never logs it or returns it in any API response.
