"""fal.ai image provider with multi-model dispatch.

Supported model slugs (auto-detected):
  - fal-ai/flux/schnell, fal-ai/flux/dev — text-only, no refs
  - fal-ai/flux-2/edit, fal-ai/flux-2-pro/edit — text + multi-image refs
  - fal-ai/flux-pro/kontext, fal-ai/flux-pro/kontext/multi — text + 1..4 refs
  - fal-ai/gpt-image-1.5/edit — text + refs
  - fal-ai/bytedance/seedream/v4/text-to-image, /seedream-v4/edit — text + up to 10 refs
  - fal-ai/nano-banana-2, fal-ai/nano-banana-pro, fal-ai/nano-banana-pro/edit — text + refs

Ref images come in as data URIs (encoded in-process from MinIO bytes) so
they reach fal even when MinIO sits behind localhost.
"""
from __future__ import annotations

import logging
import os
from typing import Any

import fal_client
import httpx

log = logging.getLogger(__name__)

# Per-image price defaults (USD). Override via FAL_PRICE_<slug>.
# 1024x1024 ≈ 1.05 MP for MP-priced models.
DEFAULT_PRICES: dict[str, float] = {
    "fal-ai/flux/schnell":             0.003,
    "fal-ai/flux/dev":                 0.025,
    "fal-ai/flux-2/edit":              0.013,   # 1.05MP × $0.012
    "fal-ai/flux-2-pro/edit":          0.032,   # 1.05MP × $0.03
    "fal-ai/flux-pro/kontext":         0.04,
    "fal-ai/flux-pro/kontext/multi":   0.04,
    "fal-ai/flux-pro/kontext/max":     0.08,
    "fal-ai/gpt-image-1.5/edit":       0.013,   # low quality 1024²
    "fal-ai/gpt-image-1.5":            0.013,
    "fal-ai/bytedance/seedream/v4/text-to-image":    0.03,
    "fal-ai/bytedance/seedream/v4/edit": 0.03,
    "fal-ai/nano-banana-2":            0.08,
    "fal-ai/nano-banana-pro":          0.15,
    "fal-ai/nano-banana-pro/edit":     0.15,
}


def _price_for(model: str) -> float:
    env_key = "FAL_PRICE_" + model.replace("/", "_").replace("-", "_").upper()
    val = os.getenv(env_key)
    if val:
        try: return float(val)
        except ValueError: pass
    return DEFAULT_PRICES.get(model, 0.003)


def _accepts_refs(model: str) -> bool:
    return any(s in model for s in (
        "kontext", "flux-2/edit", "flux-2-pro/edit", "gpt-image",
        "seedream", "nano-banana",
    ))


def _build_arguments(model: str, prompt: str, params: dict[str, Any], refs: list[str]) -> dict[str, Any]:
    """Build the model-specific fal subscribe arguments."""
    image_size = params.get("image_size", "square_hd")
    num_steps = int(params.get("num_inference_steps", 4))

    # Family: kontext (single + multi)
    if "kontext/multi" in model:
        out = {"prompt": prompt, "aspect_ratio": "1:1", "num_images": 1}
        if refs:
            out["image_urls"] = refs[:4]
        return out
    if "kontext" in model:
        out = {"prompt": prompt, "aspect_ratio": "1:1", "num_images": 1}
        if refs:
            out["image_url"] = refs[0]
        return out

    # Family: flux-2 edit (multi-ref, up to 10)
    if "flux-2/edit" in model or "flux-2-pro/edit" in model:
        return {
            "prompt": prompt,
            "image_urls": refs[:10] if refs else [],
            "aspect_ratio": "1:1",
            "num_images": 1,
        }

    # Family: gpt-image-1.5 — only accepts auto / 1024x1024 / 1536x1024 / 1024x1536
    _gpt_size = {
        "square_hd": "1024x1024", "square": "1024x1024",
        "landscape_16_9": "1536x1024", "landscape_4_3": "1536x1024",
        "portrait_16_9": "1024x1536", "portrait_4_3": "1024x1536",
    }.get(image_size, image_size if image_size in {"auto", "1024x1024", "1536x1024", "1024x1536"} else "1024x1024")
    if "gpt-image-1.5/edit" in model:
        return {
            "prompt": prompt,
            "image_urls": refs[:10] if refs else [],
            "quality": params.get("quality", "low"),
            "image_size": _gpt_size,
        }
    if "gpt-image-1.5" in model or "gpt-image-1" in model:
        return {"prompt": prompt, "quality": params.get("quality", "low"), "image_size": _gpt_size}

    # Family: Seedream
    if "seedream/v4/edit" in model:
        return {"prompt": prompt, "image_urls": refs[:10] if refs else [], "image_size": image_size}
    if "seedream" in model:
        return {"prompt": prompt, "image_size": image_size}

    # Family: Nano Banana
    if "nano-banana-pro/edit" in model:
        return {"prompt": prompt, "image_urls": refs[:6] if refs else [], "num_images": 1}
    if "nano-banana" in model:
        return {"prompt": prompt, "num_images": 1}

    # Default: flux/schnell or flux/dev — text only
    return {
        "prompt": prompt,
        "image_size": image_size,
        "num_inference_steps": num_steps,
    }


class FalImageClient:
    def __init__(self, api_key: str, default_model: str, price_per_image_usd: float) -> None:
        if api_key:
            os.environ["FAL_KEY"] = api_key
        self.default_model = default_model
        # Kept for backwards compat; ignored when DEFAULT_PRICES has the model.
        self.fallback_price = price_per_image_usd

    async def generate(
        self,
        prompt: str,
        model: str | None,
        params: dict[str, Any],
        refs: list[str] | None = None,
    ) -> tuple[bytes, str, dict[str, Any]]:
        """Return (image_bytes, content_type, cost_info)."""
        chosen = model or self.default_model
        refs = refs or []
        if refs and not _accepts_refs(chosen):
            log.info("dropping refs — model doesn't accept image refs", extra={"model": chosen})
            refs = []

        args = _build_arguments(chosen, prompt, params or {}, refs)
        result = await fal_client.subscribe_async(chosen, arguments=args)

        images = result.get("images") or []
        if not images and result.get("image"):
            images = [result["image"]]
        if not images:
            raise RuntimeError(f"fal: no images returned (raw: {result!r})")
        url = images[0].get("url")
        if not url:
            raise RuntimeError("fal: image has no url")
        async with httpx.AsyncClient(timeout=120.0) as client:
            r = await client.get(url)
            r.raise_for_status()
            content_type = r.headers.get("Content-Type", "image/png")
            data = r.content

        price = _price_for(chosen) or self.fallback_price
        cost = {
            "provider": "fal",
            "model": chosen,
            "units": 1.0,
            "unit_label": "images",
            "unit_cost_usd": price,
            "total_cost_usd": price,
        }
        return data, content_type, cost
