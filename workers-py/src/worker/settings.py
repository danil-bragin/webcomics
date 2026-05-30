"""Worker settings loaded from environment.

Validated by pydantic-settings — fail fast on misconfiguration.
"""
from __future__ import annotations

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    worker_type: str = Field(default="script", alias="WORKER_TYPE")
    redis_url: str = Field(default="redis://localhost:6380/0", alias="REDIS_URL")
    consumer_group_prefix: str = Field(default="pipeline-py", alias="CONSUMER_GROUP_PREFIX")
    consumer_name: str = Field(default="worker-1", alias="CONSUMER_NAME")

    # MinIO / S3
    minio_endpoint: str = Field(default="localhost:9000", alias="MINIO_ENDPOINT")
    minio_access_key: str = Field(default="minioadmin", alias="MINIO_ACCESS_KEY")
    minio_secret_key: str = Field(default="minioadmin", alias="MINIO_SECRET_KEY")
    minio_bucket: str = Field(default="webcomics", alias="MINIO_BUCKET")
    minio_use_ssl: bool = Field(default=False, alias="MINIO_USE_SSL")

    # OpenRouter
    openrouter_api_key: str = Field(default="", alias="OPENROUTER_API_KEY")
    openrouter_base_url: str = Field(
        default="https://openrouter.ai/api/v1", alias="OPENROUTER_BASE_URL"
    )
    script_default_model: str = Field(
        default="openai/gpt-4o-mini", alias="SCRIPT_DEFAULT_MODEL"
    )

    # fal.ai
    fal_key: str = Field(default="", alias="FAL_KEY")
    image_default_model: str = Field(
        default="fal-ai/flux/schnell", alias="IMAGE_DEFAULT_MODEL"
    )
    image_price_usd: float = Field(default=0.003, alias="IMAGE_PRICE_USD")

    health_port: int = Field(default=8081, alias="HEALTH_PORT")

    # ElevenLabs (audio step)
    elevenlabs_api_key: str = Field(default="", alias="ELEVENLABS_API_KEY")
    elevenlabs_voice_id: str = Field(default="EXAVITQu4vr4xnSDxMaL", alias="ELEVENLABS_VOICE_ID")
    elevenlabs_model: str = Field(default="eleven_flash_v2_5", alias="ELEVENLABS_MODEL")
    elevenlabs_price_per_1k: float = Field(default=0.30, alias="ELEVENLABS_PRICE_PER_1K_CHARS")


def load() -> Settings:
    return Settings()
