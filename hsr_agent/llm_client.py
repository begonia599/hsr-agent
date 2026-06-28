from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Any

try:
    import anthropic
except ModuleNotFoundError:  # OpenAI-compatible mode does not need this dependency.
    anthropic = None

DEFAULT_LLM_BASE_URL = "https://api.deepseek.com"
DEFAULT_LLM_MODEL = "deepseek-chat"
DEFAULT_LLM_API_FORMAT = "openai"


@dataclass(frozen=True)
class LLMConfig:
    base_url: str
    api_key: str
    model: str
    api_format: str


def load_llm_config() -> LLMConfig:
    return LLMConfig(
        base_url=os.getenv("LLM_BASE_URL", DEFAULT_LLM_BASE_URL),
        api_key=os.getenv("LLM_API_KEY", ""),
        model=os.getenv("LLM_MODEL", DEFAULT_LLM_MODEL),
        api_format=os.getenv("LLM_API_FORMAT", DEFAULT_LLM_API_FORMAT),
    )


def require_llm_config() -> LLMConfig:
    config = load_llm_config()
    if not config.api_key or config.api_key == "replace-me":
        raise RuntimeError(
            "LLM_API_KEY is not configured. Set it in .env or run enrich.py with --dry-run."
        )
    return config


def create_client(config: LLMConfig) -> Any:
    if anthropic is None:
        raise RuntimeError("anthropic package is required when LLM_API_FORMAT=anthropic")
    return anthropic.Anthropic(base_url=config.base_url, api_key=config.api_key)
