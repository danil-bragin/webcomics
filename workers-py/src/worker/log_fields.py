"""Unified log field names — same vocabulary as Go (platform/logger) and Node
(renderer-node/src/log.ts). Grepping `run_id=<X>` across all three runtimes
yields a coherent timeline."""
from __future__ import annotations

import structlog

SERVICE = "service"
WORKER = "worker"
RUN_ID = "run_id"
STEP_INDEX = "step_index"
STEP_TYPE = "step_type"
PANEL_INDEX = "panel_index"


def bind(service: str, worker: str) -> structlog.stdlib.BoundLogger:
    return structlog.get_logger().bind(**{SERVICE: service, WORKER: worker})
