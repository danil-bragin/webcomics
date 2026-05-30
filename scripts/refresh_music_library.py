#!/usr/bin/env python3
"""Download curated music tracks → MinIO → regenerate manifest.

Reads `ops/music-library/sources.json` (curated list of public-CDN URLs +
tags), downloads each MP3, uploads to MinIO under `library/music/<id>.mp3`,
and writes the consumed manifest back to `library/music/manifest.json` so
the music worker can pick from it.

Trending refresh: edit sources.json with new URLs (or use a future Pixabay
scraper) and re-run. Make target `make refresh-music`.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
from pathlib import Path
from urllib.request import urlopen, Request

from minio import Minio

ROOT = Path(__file__).resolve().parent.parent
SOURCES = ROOT / "ops" / "music-library" / "sources.json"

USER_AGENT = "webcomics-music-refresher/1.0"


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


def fetch(url: str) -> bytes:
    req = Request(url, headers={"User-Agent": USER_AGENT})
    with urlopen(req, timeout=60) as r:
        return r.read()


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--bucket", default=env("MINIO_BUCKET", "webcomics"))
    p.add_argument("--endpoint", default=env("MINIO_ENDPOINT", "localhost:9000"))
    p.add_argument("--access-key", default=env("MINIO_ACCESS_KEY", "minioadmin"))
    p.add_argument("--secret-key", default=env("MINIO_SECRET_KEY", "minioadmin"))
    p.add_argument("--secure", action="store_true", default=env("MINIO_USE_SSL") == "true")
    p.add_argument("--dry-run", action="store_true")
    args = p.parse_args()

    if not SOURCES.exists():
        print(f"sources file missing: {SOURCES}", file=sys.stderr)
        return 1

    with open(SOURCES) as f:
        sources = json.load(f)

    client = Minio(args.endpoint, access_key=args.access_key, secret_key=args.secret_key, secure=args.secure)
    if not client.bucket_exists(args.bucket):
        print(f"bucket missing: {args.bucket}", file=sys.stderr)
        return 1

    manifest = []
    for entry in sources:
        track_id = entry["id"]
        url = entry["url"]
        object_key = f"library/music/{track_id}.mp3"

        # Skip download if already uploaded — avoids hammering CDN on every refresh.
        if not args.dry_run:
            try:
                client.stat_object(args.bucket, object_key)
                print(f"  exists, skip: {object_key}")
                manifest.append(_to_manifest(entry, object_key))
                continue
            except Exception:
                pass

        print(f"  download {track_id} ← {url}")
        try:
            data = fetch(url)
        except Exception as e:
            print(f"    FAILED {e}", file=sys.stderr)
            continue

        if args.dry_run:
            print(f"    (dry-run) would upload {len(data)} bytes to {object_key}")
            manifest.append(_to_manifest(entry, object_key))
            continue

        with tempfile.NamedTemporaryFile(suffix=".mp3", delete=False) as tf:
            tf.write(data)
            tf.flush()
            client.fput_object(args.bucket, object_key, tf.name, content_type="audio/mpeg")
        os.unlink(tf.name)
        manifest.append(_to_manifest(entry, object_key))
        print(f"    uploaded {len(data)/1024:.1f} KB → {object_key}")

    # Upload manifest.
    manifest_key = "library/music/manifest.json"
    body = json.dumps(manifest, indent=2).encode()
    if args.dry_run:
        print(f"(dry-run) manifest would have {len(manifest)} tracks")
    else:
        from io import BytesIO
        client.put_object(args.bucket, manifest_key, BytesIO(body), length=len(body),
                          content_type="application/json")
        print(f"manifest pushed: {manifest_key} ({len(manifest)} tracks)")
    return 0


def _to_manifest(entry: dict, object_key: str) -> dict:
    return {
        "id": entry["id"],
        "title": entry.get("title", ""),
        "artist": entry.get("artist", ""),
        "object_key": object_key,
        "duration_s": entry.get("duration_s", 0),
        "mood": entry.get("mood", []),
        "genre": entry.get("genre", []),
        "tempo": entry.get("tempo", "medium"),
        "license": entry.get("license", "Pixabay Content License"),
        "source": entry.get("source", "pixabay"),
        "source_url": entry.get("url", ""),
    }


if __name__ == "__main__":
    sys.exit(main())
