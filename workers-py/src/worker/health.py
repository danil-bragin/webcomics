"""Tiny stdlib HTTP server exposing /health and /ready.

Health = process alive. Ready = Redis reachable. Used by docker compose
healthchecks so a wedged worker container gets rolled.
"""
from __future__ import annotations

import asyncio
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from threading import Thread

import redis.asyncio as aioredis


class _State:
    def __init__(self) -> None:
        self.redis_url: str = ""
        self.ready: bool = False


_state = _State()


class _Handler(BaseHTTPRequestHandler):
    def log_message(self, *_a, **_k) -> None:
        # Silence default access log; we run our own structlog elsewhere.
        return

    def do_GET(self) -> None:
        if self.path == "/health":
            self._json(200, {"status": "ok"})
        elif self.path == "/ready":
            self._json(200 if _state.ready else 503, {"ready": _state.ready})
        else:
            self._json(404, {"error": "not found"})

    def _json(self, status: int, body: dict) -> None:
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def start(port: int, redis_url: str) -> None:
    _state.redis_url = redis_url
    srv = HTTPServer(("0.0.0.0", port), _Handler)
    Thread(target=srv.serve_forever, daemon=True).start()
    Thread(target=_run_readiness_probe, args=(redis_url,), daemon=True).start()


def _run_readiness_probe(redis_url: str) -> None:
    asyncio.run(_probe(redis_url))


async def _probe(redis_url: str) -> None:
    client = aioredis.from_url(redis_url, decode_responses=False)
    while True:
        try:
            await client.ping()
            _state.ready = True
        except Exception:
            _state.ready = False
        await asyncio.sleep(5)
