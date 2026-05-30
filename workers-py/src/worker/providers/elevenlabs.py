"""ElevenLabs TTS provider."""
from __future__ import annotations

import httpx


# Default Rachel voice. Override with ELEVENLABS_VOICE_ID.
DEFAULT_VOICE = "EXAVITQu4vr4xnSDxMaL"

# Approximate price: $0.30 / 1k characters for the Starter tier.
# Override with ELEVENLABS_PRICE_PER_1K_CHARS.
DEFAULT_PRICE_PER_1K = 0.30


class ElevenLabsClient:
    def __init__(self, api_key: str, voice_id: str, model_id: str, price_per_1k_chars: float) -> None:
        self.api_key = api_key
        self.voice_id = voice_id or DEFAULT_VOICE
        self.model_id = model_id or "eleven_flash_v2_5"
        self.price_per_1k_chars = price_per_1k_chars or DEFAULT_PRICE_PER_1K

    async def synthesize(
        self,
        text: str,
        voice_id: str | None = None,
        model_id: str | None = None,
        speed: float | None = None,
    ) -> tuple[bytes, dict]:
        """Return (mp3_bytes, cost_info). voice_id/model_id/speed override the
        client defaults for a single call (per-run params from the orchestrator).
        """
        if not self.api_key:
            raise RuntimeError("ELEVENLABS_API_KEY not set")
        if not text.strip():
            raise RuntimeError("empty text")
        voice = voice_id or self.voice_id
        model = model_id or self.model_id
        url = f"https://api.elevenlabs.io/v1/text-to-speech/{voice}"
        headers = {
            "xi-api-key": self.api_key,
            "accept": "audio/mpeg",
            "content-type": "application/json",
        }
        voice_settings: dict = {"stability": 0.5, "similarity_boost": 0.75}
        if speed is not None:
            # Clamp to ElevenLabs' accepted range.
            voice_settings["speed"] = max(0.7, min(1.2, float(speed)))
        body = {
            "text": text,
            "model_id": model,
            "voice_settings": voice_settings,
        }
        async with httpx.AsyncClient(timeout=120.0) as client:
            r = await client.post(url, json=body, headers=headers)
            r.raise_for_status()
            data = r.content
        chars = len(text)
        total = round(chars * self.price_per_1k_chars / 1000.0, 6)
        unit_cost = round(self.price_per_1k_chars / 1000.0, 8)
        return data, {
            "provider": "elevenlabs",
            "model": model,
            "units": float(chars),
            "unit_label": "characters",
            "unit_cost_usd": unit_cost,
            "total_cost_usd": total,
        }
