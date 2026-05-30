FROM python:3.12-slim

ENV PIP_NO_CACHE_DIR=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

WORKDIR /app
# ffmpeg required by audio handler to stitch per-panel TTS to fixed slot
# lengths. Without it the worker falls back to raw concat → audio is way
# shorter than the timeline, causing A/V drift in assemble.
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg \
 && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml ./
COPY src ./src

RUN pip install --upgrade pip && pip install .

ENV PYTHONPATH=/app/src
ENTRYPOINT ["worker"]
