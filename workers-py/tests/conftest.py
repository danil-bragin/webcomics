"""Test fixtures + fakes for worker handlers."""
from __future__ import annotations

import asyncio
import json
from typing import Any

import pytest

from worker.redis_bus import CancelledRuns


class FakeBus:
    """In-memory Bus stand-in. Records every publish for assertions."""

    def __init__(self) -> None:
        self.published: list[tuple[str, dict[str, Any]]] = []

    async def publish(self, stream: str, payload: dict[str, Any]) -> None:
        # Round-trip through JSON to surface any non-serializable payloads.
        body = json.loads(json.dumps(payload))
        self.published.append((stream, body))


class FakeStore:
    """In-memory MinIO stand-in."""

    def __init__(self) -> None:
        self.objects: dict[str, tuple[bytes, str]] = {}
        # The handler reaches into self.client.get_object().read() for downloads.
        self.client = self
        self.bucket = "test"

    def put_bytes(self, key: str, data: bytes, content_type: str) -> None:
        self.objects[key] = (data, content_type)

    def get_object(self, bucket: str, key: str):
        class _Reader:
            def __init__(self, data: bytes) -> None:
                self._data = data

            def read(self) -> bytes:
                return self._data

        return _Reader(self.objects[key][0])


@pytest.fixture
def bus() -> FakeBus:
    return FakeBus()


@pytest.fixture
def store() -> FakeStore:
    return FakeStore()


@pytest.fixture
def cancelled() -> CancelledRuns:
    return CancelledRuns()
