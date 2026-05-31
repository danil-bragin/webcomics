"""OpenRouter LLM provider using the OpenAI SDK."""
from __future__ import annotations

import json
import logging
from typing import Any

from openai import AsyncOpenAI

log = logging.getLogger(__name__)


DEFAULT_SYSTEM_PROMPT = (
    "You write short 3- to 10-panel webcomic / meme scripts. "
    "Output STRICT JSON with shape: "
    '{"panels":[{"index":int,"prompt":"<image prompt for this panel>","caption":"<optional short caption>"}]}'
    " The prompt for each panel must be a vivid, self-contained description "
    "suitable for a text-to-image model. Keep prompts under 200 chars."
)


class OpenRouterClient:
    def __init__(self, api_key: str, base_url: str, default_model: str) -> None:
        self.client = AsyncOpenAI(api_key=api_key, base_url=base_url)
        self.default_model = default_model

    async def generate_script(
        self,
        prompt: str,
        system_prompt: str | None,
        model: str | None,
        panel_count: int,
    ) -> tuple[dict[str, Any], dict[str, Any]]:
        """Return (parsed_payload, raw_usage)."""
        sys = system_prompt or DEFAULT_SYSTEM_PROMPT
        if panel_count > 0:
            sys = f"{sys}\nProduce exactly {panel_count} panels."
        chosen_model = model or self.default_model
        resp = await self.client.chat.completions.create(
            model=chosen_model,
            messages=[
                {"role": "system", "content": sys},
                {"role": "user", "content": prompt},
            ],
            response_format={"type": "json_object"},
            temperature=0.9,
        )
        content = resp.choices[0].message.content or "{}"
        try:
            parsed = json.loads(content)
        except json.JSONDecodeError as e:
            log.error("invalid json from model", extra={"err": str(e), "content": content[:200]})
            raise
        usage = {}
        if resp.usage:
            usage = {
                "prompt_tokens": resp.usage.prompt_tokens,
                "completion_tokens": resp.usage.completion_tokens,
                "total_tokens": resp.usage.total_tokens,
            }
            # OpenRouter adds a non-standard `cost` field on the usage object.
            cost = getattr(resp.usage, "cost", None)
            if cost is not None:
                usage["cost_usd"] = float(cost)
        usage["model"] = chosen_model
        return parsed, usage

    async def generate_caption(
        self,
        panels: list[dict],
        platforms: list[str],
        plot_premise: str,
        plot_beats: list[dict],
        characters: list[dict],
        model: str | None,
        language: str = "en",
    ) -> tuple[dict[str, Any], dict[str, Any]]:
        """Compose social-post captions for one or more platforms.

        Returns ({platform: {caption, title, hashtags}}, usage).
        Each platform gets a tone hint baked into the system prompt so we
        don't produce hashtag-stuffed YouTube descriptions or essay-style
        tweets.
        """
        chosen_model = model or self.default_model
        platforms = platforms or ["generic"]
        platform_list = ", ".join(platforms)
        chars = ", ".join(c.get("name", "") for c in (characters or []) if c.get("name"))
        beat_lines = "\n".join(
            f"- {b.get('name', '')}: {b.get('description', '')}"
            for b in (plot_beats or [])
            if b.get("description")
        )
        panels_text = "\n".join(
            f"{i + 1}. {p.get('caption') or p.get('prompt', '')}"
            for i, p in enumerate(panels or [])
        )
        lang_names = {"en": "English", "ru": "Russian", "fr": "French"}
        lang_name = lang_names.get(language, "English")
        lang_rule = (
            ""
            if language == "en"
            else f"\nLANGUAGE: Write every title, description, caption, tag, and hashtag in {lang_name}. "
                 f"Hashtags also use {lang_name} words (no English mixing).\n"
        )
        system = (
            "You are a top-performing short-form creator writing real publication metadata for a webcomic video.\n"
            + lang_rule
            + ""
            "Write as a human creator. NEVER mention that the content is AI-generated, automated, a test, "
            "a draft, experimental, or any version number. NEVER use the words 'AI', 'generated', 'auto', "
            "'test', 'v1', 'v2', 'version', 'draft', 'experimental'.\n"
            "Tone: confident, playful, audience-first, hooks viewers in the first 3 words.\n"
            f"Target platforms: {platform_list}.\n\n"
            "PLATFORM RULES:\n"
            " - youtube: title <=80 chars (punchy + curiosity gap, no clickbait shouting), "
            "description 1-3 short paragraphs (NO mention of AI), 5-8 SEO tags (lowercase, single words or 2-grams), "
            "hashtags MUST include 'Shorts' + 2 niche tags relevant to the actual content.\n"
            " - instagram: caption <=2200 chars w/ light line-breaks for readability, 10-20 hashtags inline.\n"
            " - tiktok: caption <=150 chars (concise), 4-6 hashtags.\n"
            " - facebook: caption <=1500 chars w/ 3-5 hashtags; tone slightly more story-like than IG.\n"
            " - twitter: tweet <=270 chars, <=3 hashtags.\n\n"
            "AUDIENCE DETERMINATION (always include):\n"
            " Decide whether the video qualifies as 'made for kids' under YouTube COPPA rules.\n"
            " Default to FALSE unless the content is explicitly targeted at children under 13 (nursery rhymes, "
            "kids' cartoons, educational pre-school content, simple characters with no irony/sarcasm).\n"
            " Mature/dark/satirical humor, alcohol/relationship references, sarcasm, irony => not for kids.\n"
            " Include short one-sentence reasoning that a human reviewer can verify.\n\n"
            "Return STRICT JSON matching this schema:\n"
            '{\n'
            '  \"audience\": {\"made_for_kids\": false, \"confidence\": 0.0-1.0, \"reasoning\": \"...\"},\n'
            '  \"hook\": \"opening 1-line attention grab\",\n'
            '  \"platforms\": {\n'
            '    \"<platform>\": {\"title\": \"...\", \"description\": \"...\", \"caption\": \"...\", \"tags\": [\"...\"], \"hashtags\": [\"...\"]}\n'
            '  }\n'
            '}\n'
            "Only include the platforms requested above. 'description' is for long-form platforms (YouTube); "
            "'caption' is for short-form (IG, TikTok, X). 'tags' is YouTube tags array (no # prefix). 'hashtags' "
            "is the inline-display hashtag set."
        )
        user = (
            (f"Story premise: {plot_premise}\n\n" if plot_premise else "")
            + (f"Plot beats:\n{beat_lines}\n\n" if beat_lines else "")
            + (f"Characters: {chars}\n\n" if chars else "")
            + (f"Panels:\n{panels_text}\n\n" if panels_text else "")
            + "Compose publication metadata per platform."
        )
        resp = await self.client.chat.completions.create(
            model=chosen_model,
            messages=[
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            response_format={"type": "json_object"},
            temperature=0.7,
        )
        content = resp.choices[0].message.content or "{}"
        try:
            parsed = json.loads(content)
        except json.JSONDecodeError as e:
            log.error("invalid caption json", extra={"err": str(e), "content": content[:200]})
            raise
        usage = {}
        if resp.usage:
            usage = {
                "prompt_tokens": resp.usage.prompt_tokens,
                "completion_tokens": resp.usage.completion_tokens,
                "total_tokens": resp.usage.total_tokens,
            }
            cost = getattr(resp.usage, "cost", None)
            if cost is not None:
                usage["cost_usd"] = float(cost)
        usage["model"] = chosen_model
        return parsed, usage
