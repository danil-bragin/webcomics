"""Redis Streams XREADGROUP consumer + publisher.

Watermill's redis-stream publisher writes each message as a single field
named "payload" containing the JSON bytes. We mirror that on the producer
side so the Go consumer can decode our completion events.

Also exposes `CancelledRuns`: a shared set of run_ids that received a
`pipeline.run.cancelled` event. Handlers consult it at the top of each
message to short-circuit cancelled work.
"""
from __future__ import annotations

import asyncio
import json
import logging
from collections.abc import Awaitable, Callable
from typing import Any

import redis.asyncio as aioredis
import redis.exceptions as aioredis_exc

log = logging.getLogger(__name__)

# Watermill's redis-stream publisher uses key "payload" for the body.
PAYLOAD_FIELD = "payload"


class CancelledRuns:
    """In-process set of run_ids that have been cancelled. Fed by
    Bus.watch_cancellations(); read by handlers via .contains()."""

    def __init__(self) -> None:
        self._set: set[str] = set()

    def add(self, run_id: str) -> None:
        if run_id:
            self._set.add(run_id)

    def contains(self, run_id: str) -> bool:
        return run_id in self._set


class Bus:
    """Thin async wrapper over redis-py for stream consume + publish."""

    def __init__(self, url: str, consumer_group: str, consumer_name: str) -> None:
        self.redis = aioredis.from_url(url, decode_responses=False)
        self.group = consumer_group
        self.consumer = consumer_name

    async def ensure_group(self, stream: str) -> None:
        try:
            await self.redis.xgroup_create(stream, self.group, id="0", mkstream=True)
        except aioredis.ResponseError as e:
            if "BUSYGROUP" not in str(e):
                raise

    async def consume(
        self,
        stream: str,
        handler: Callable[[dict[str, Any]], Awaitable[None]],
    ) -> None:
        """Consume messages from a stream in a loop. XACK on success.

        Also reclaims messages stranded in the PEL of dead consumers (worker
        crash, container restart) via XAUTOCLAIM every loop iteration. Without
        this, story-style runs that emit >1 message per panel get stuck
        forever after a single worker bounce — the new consumer name doesn't
        see the old PEL entries and XREADGROUP only returns ">" (new only)."""
        await self.ensure_group(stream)
        log.info("listening", extra={"stream": stream, "group": self.group})
        autoclaim_cursor = "0-0"
        while True:
            # Reclaim any pending entries idle > 60s before pulling new ones.
            # XAUTOCLAIM returns (next_cursor, claimed_entries, deleted_ids).
            try:
                next_cursor, claimed, _ = await self.redis.xautoclaim(
                    stream, self.group, self.consumer,
                    min_idle_time=60_000, start_id=autoclaim_cursor, count=8,
                )
                autoclaim_cursor = next_cursor if next_cursor else "0-0"
                if claimed:
                    log.info("autoclaimed pending", extra={"stream": stream, "n": len(claimed)})
                    await self._process(stream, claimed, handler)
            except aioredis_exc.RedisError as e:
                log.warning("xautoclaim err", extra={"err": str(e)})

            try:
                resp = await self.redis.xreadgroup(
                    self.group,
                    self.consumer,
                    {stream: ">"},
                    count=8,
                    block=5000,
                )
            except aioredis_exc.TimeoutError:
                # XREADGROUP BLOCK expired with no messages — keep polling.
                continue
            except aioredis_exc.RedisError as e:
                log.warning("redis err, retrying", extra={"err": str(e)})
                await asyncio.sleep(1)
                continue
            if not resp:
                continue
            for _stream, entries in resp:
                await self._process(stream, entries, handler)

    async def _process(
        self,
        stream: str,
        entries: list,
        handler: Callable[[dict[str, Any]], Awaitable[None]],
    ) -> None:
        """Decode + dispatch a batch of XREADGROUP/XAUTOCLAIM entries."""
        for msg_id, fields in entries:
            raw = fields.get(PAYLOAD_FIELD.encode()) or fields.get(PAYLOAD_FIELD)
            if raw is None:
                log.warning("no payload field", extra={"id": msg_id})
                await self.redis.xack(stream, self.group, msg_id)
                continue
            try:
                payload = json.loads(raw)
            except json.JSONDecodeError as e:
                log.error("bad json", extra={"err": str(e)})
                await self.redis.xack(stream, self.group, msg_id)
                continue
            try:
                await handler(payload)
            except Exception:
                log.exception("handler crashed", extra={"id": msg_id})
                # Don't ACK — let autoclaim retry after the deadletter window.
                continue
            await self.redis.xack(stream, self.group, msg_id)

    async def publish(self, stream: str, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode()
        await self.redis.xadd(stream, {PAYLOAD_FIELD: body})

    async def watch_cancellations(self, cancelled: CancelledRuns) -> None:
        """Subscribe to pipeline.run.cancelled and populate `cancelled`.
        Run alongside the main consume() in a separate asyncio.Task. Uses a
        consumer group keyed per process so every worker replica sees every
        cancellation event."""
        stream = "pipeline.run.cancelled"
        # A consumer-group-per-process — every worker must see the event.
        import uuid as _uuid
        group = f"cancel-watch-{_uuid.uuid4().hex[:8]}"
        try:
            await self.redis.xgroup_create(stream, group, id="$", mkstream=True)
        except aioredis.ResponseError as e:
            if "BUSYGROUP" not in str(e):
                raise
        while True:
            try:
                resp = await self.redis.xreadgroup(
                    group, self.consumer, {stream: ">"}, count=16, block=5000,
                )
            except aioredis_exc.TimeoutError:
                continue
            except aioredis_exc.RedisError:
                await asyncio.sleep(1)
                continue
            if not resp:
                continue
            for _stream, entries in resp:
                for msg_id, fields in entries:
                    raw = fields.get(PAYLOAD_FIELD.encode()) or fields.get(PAYLOAD_FIELD)
                    if raw is None:
                        await self.redis.xack(stream, group, msg_id)
                        continue
                    try:
                        payload = json.loads(raw)
                        cancelled.add(str(payload.get("run_id", "")))
                    except Exception:
                        pass
                    await self.redis.xack(stream, group, msg_id)

    async def close(self) -> None:
        await self.redis.aclose()
