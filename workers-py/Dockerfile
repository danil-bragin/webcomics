FROM python:3.12-slim

ENV PIP_NO_CACHE_DIR=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

WORKDIR /app
COPY pyproject.toml ./
COPY src ./src

RUN pip install --upgrade pip && pip install .

ENV PYTHONPATH=/app/src
ENTRYPOINT ["worker"]
