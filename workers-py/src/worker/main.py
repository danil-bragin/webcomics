"""Worker process entrypoint.

`WORKER_TYPE=script|image` selects which handler is bound to which Redis
stream. Each replica only handles one type; horizontal scaling = more replicas.
"""
from __future__ import annotations

import asyncio
import logging
import os
import signal
import sys

import structlog

from worker import settings, health
from worker.handlers.script import ScriptHandler
from worker.providers.openrouter import OpenRouterClient
from worker.redis_bus import Bus, CancelledRuns
from worker.storage.minio_client import ObjectStore


def configure_logging() -> None:
    logging.basicConfig(
        format="%(message)s",
        stream=sys.stdout,
        level=os.getenv("LOG_LEVEL", "INFO").upper(),
    )
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.JSONRenderer(),
        ],
    )


async def run_script(cfg: settings.Settings) -> None:
    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-script",
        consumer_name=cfg.consumer_name,
    )
    store = ObjectStore(
        cfg.minio_endpoint, cfg.minio_access_key, cfg.minio_secret_key,
        cfg.minio_bucket, cfg.minio_use_ssl,
    )
    llm = OpenRouterClient(cfg.openrouter_api_key, cfg.openrouter_base_url, cfg.script_default_model)
    cancelled = CancelledRuns()
    handler = ScriptHandler(bus, llm, store, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.script.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def run_image(cfg: settings.Settings) -> None:
    from worker.handlers.image import ImageHandler
    from worker.providers.fal_images import FalImageClient

    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-image",
        consumer_name=cfg.consumer_name,
    )
    store = ObjectStore(
        cfg.minio_endpoint, cfg.minio_access_key, cfg.minio_secret_key,
        cfg.minio_bucket, cfg.minio_use_ssl,
    )
    fal = FalImageClient(cfg.fal_key, cfg.image_default_model, cfg.image_price_usd)
    cancelled = CancelledRuns()
    handler = ImageHandler(bus, fal, store, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.image.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def run_audio(cfg: settings.Settings) -> None:
    from worker.handlers.audio import AudioHandler
    from worker.providers.elevenlabs import ElevenLabsClient

    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-audio",
        consumer_name=cfg.consumer_name,
    )
    store = ObjectStore(
        cfg.minio_endpoint, cfg.minio_access_key, cfg.minio_secret_key,
        cfg.minio_bucket, cfg.minio_use_ssl,
    )
    el = ElevenLabsClient(
        cfg.elevenlabs_api_key, cfg.elevenlabs_voice_id,
        cfg.elevenlabs_model, cfg.elevenlabs_price_per_1k,
    )
    cancelled = CancelledRuns()
    handler = AudioHandler(bus, el, store, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.audio.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def run_caption(cfg: settings.Settings) -> None:
    from worker.handlers.caption import CaptionHandler

    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-caption",
        consumer_name=cfg.consumer_name,
    )
    store = ObjectStore(
        cfg.minio_endpoint, cfg.minio_access_key, cfg.minio_secret_key,
        cfg.minio_bucket, cfg.minio_use_ssl,
    )
    llm = OpenRouterClient(cfg.openrouter_api_key, cfg.openrouter_base_url, cfg.script_default_model)
    cancelled = CancelledRuns()
    handler = CaptionHandler(bus, llm, store, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.caption.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def run_music(cfg: settings.Settings) -> None:
    from worker.handlers.music import MusicHandler

    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-music",
        consumer_name=cfg.consumer_name,
    )
    store = ObjectStore(
        cfg.minio_endpoint, cfg.minio_access_key, cfg.minio_secret_key,
        cfg.minio_bucket, cfg.minio_use_ssl,
    )
    llm = OpenRouterClient(cfg.openrouter_api_key, cfg.openrouter_base_url, cfg.script_default_model)
    cancelled = CancelledRuns()
    handler = MusicHandler(bus, llm, store, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.music.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def run_metrics(cfg: settings.Settings) -> None:
    from worker.handlers.metrics import MetricsHandler

    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-metrics",
        consumer_name=cfg.consumer_name,
    )
    cancelled = CancelledRuns()
    handler = MetricsHandler(bus, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.metrics.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def run_upload(cfg: settings.Settings) -> None:
    from worker.handlers.upload import UploadHandler

    bus = Bus(
        cfg.redis_url,
        consumer_group=f"{cfg.consumer_group_prefix}-upload",
        consumer_name=cfg.consumer_name,
    )
    store = ObjectStore(
        cfg.minio_endpoint, cfg.minio_access_key, cfg.minio_secret_key,
        cfg.minio_bucket, cfg.minio_use_ssl,
    )
    cancelled = CancelledRuns()
    handler = UploadHandler(bus, store, cancelled)
    try:
        await asyncio.gather(
            bus.consume("pipeline.upload.requested", handler.handle),
            bus.watch_cancellations(cancelled),
        )
    finally:
        await bus.close()


async def amain() -> int:
    configure_logging()
    cfg = settings.load()
    log = structlog.get_logger().bind(worker_type=cfg.worker_type)
    log.info("starting")

    # Background health server. /health = liveness, /ready = Redis up.
    health.start(cfg.health_port, cfg.redis_url)

    loop = asyncio.get_event_loop()
    stop_event = asyncio.Event()

    def stop(_sig: int = 0, _frame: object = None) -> None:
        stop_event.set()

    for s in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(s, stop)

    if cfg.worker_type == "script":
        task = asyncio.create_task(run_script(cfg))
    elif cfg.worker_type == "image":
        task = asyncio.create_task(run_image(cfg))
    elif cfg.worker_type == "audio":
        task = asyncio.create_task(run_audio(cfg))
    elif cfg.worker_type == "caption":
        task = asyncio.create_task(run_caption(cfg))
    elif cfg.worker_type == "music":
        task = asyncio.create_task(run_music(cfg))
    elif cfg.worker_type == "upload":
        task = asyncio.create_task(run_upload(cfg))
    elif cfg.worker_type == "metrics":
        task = asyncio.create_task(run_metrics(cfg))
    else:
        log.error("unknown worker_type", worker_type=cfg.worker_type)
        return 2

    done, pending = await asyncio.wait(
        [task, asyncio.create_task(stop_event.wait())],
        return_when=asyncio.FIRST_COMPLETED,
    )
    for p in pending:
        p.cancel()
    for d in done:
        exc = d.exception()
        if exc and not isinstance(exc, asyncio.CancelledError):
            import traceback
            tb = "".join(traceback.format_exception(type(exc), exc, exc.__traceback__))
            log.error("worker crashed", traceback=tb)
            return 1
    return 0


def run() -> None:
    sys.exit(asyncio.run(amain()))


if __name__ == "__main__":
    run()
