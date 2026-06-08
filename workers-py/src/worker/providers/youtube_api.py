"""YouTube Data API v3 upload provider (videos.insert).

The legit alternative to the Selenium browser flow. Uses a per-channel OAuth
refresh token (stored on the social account) to mint a short-lived access token,
then does a single multipart/related upload. Our videos are small (~15-25 MB),
so a simple (non-resumable) upload is enough.
"""
from __future__ import annotations

import json
from typing import Any

import httpx

TOKEN_URL = "https://oauth2.googleapis.com/token"
UPLOAD_URL = (
    "https://www.googleapis.com/upload/youtube/v3/videos"
    "?part=snippet,status&uploadType=multipart&notifySubscribers=false"
)


class YouTubeAPIError(Exception):
    """Raised on any non-2xx from the token or upload endpoints."""


def _access_token(client_id: str, client_secret: str, refresh_token: str) -> str:
    if not (client_id and client_secret and refresh_token):
        raise YouTubeAPIError("missing OAuth client_id/secret/refresh_token")
    with httpx.Client(timeout=30.0) as c:
        r = c.post(TOKEN_URL, data={
            "client_id": client_id,
            "client_secret": client_secret,
            "refresh_token": refresh_token,
            "grant_type": "refresh_token",
        })
    if r.status_code != 200:
        raise YouTubeAPIError(f"token refresh {r.status_code}: {r.text[:300]}")
    tok = r.json().get("access_token")
    if not tok:
        raise YouTubeAPIError("token refresh returned no access_token")
    return tok


def upload(
    store,
    video_key: str,
    refresh_token: str,
    client_id: str,
    client_secret: str,
    meta: dict[str, Any],
) -> dict[str, Any]:
    """Upload the run's video via videos.insert. Returns
    {video_url, video_id, final_visibility}."""
    token = _access_token(client_id, client_secret, refresh_token)

    # Pull the rendered video bytes from MinIO.
    data = store.client.get_object(store.bucket, video_key).read()

    visibility = (meta.get("visibility") or "unlisted").lower()
    if visibility not in ("public", "unlisted", "private"):
        visibility = "unlisted"
    body = {
        "snippet": {
            "title": (meta.get("title") or "Generated video")[:100],
            "description": meta.get("description") or "",
            "tags": list(meta.get("tags") or []),
            "categoryId": str(meta.get("category_id") or "22"),
        },
        "status": {
            "privacyStatus": visibility,
            "selfDeclaredMadeForKids": bool(meta.get("made_for_kids", False)),
        },
    }

    # Hand-build a multipart/related body (RFC 2387) — what the YouTube
    # uploadType=multipart endpoint expects: JSON metadata part, then media.
    boundary = "wcm_yt_boundary_7f3a9c"
    crlf = b"\r\n"
    meta_part = json.dumps(body).encode("utf-8")
    parts = (
        b"--" + boundary.encode() + crlf
        + b"Content-Type: application/json; charset=UTF-8" + crlf + crlf
        + meta_part + crlf
        + b"--" + boundary.encode() + crlf
        + b"Content-Type: video/mp4" + crlf + crlf
        + data + crlf
        + b"--" + boundary.encode() + b"--" + crlf
    )
    with httpx.Client(timeout=300.0) as c:
        r = c.post(
            UPLOAD_URL,
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": f"multipart/related; boundary={boundary}",
            },
            content=parts,
        )
    if r.status_code not in (200, 201):
        raise YouTubeAPIError(f"videos.insert {r.status_code}: {r.text[:400]}")
    vid = r.json().get("id")
    if not vid:
        raise YouTubeAPIError(f"videos.insert returned no id: {r.text[:200]}")
    return {
        "video_url": f"https://youtu.be/{vid}",
        "video_id": vid,
        "final_visibility": visibility,
    }
