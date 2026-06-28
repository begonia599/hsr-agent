from __future__ import annotations

import argparse
import json
import os
import sys
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import httpx
from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.llm_client import DEFAULT_LLM_MODEL, create_client, require_llm_config
from schemas.axes_vocab import CHARACTER_AXES_INPUT_SCHEMA, normalize_axes, tool_schema, vocab_prompt
from scripts.load import build_skill_text, clean_text, load_json, normalize_name

DEFAULT_VERSION = "4.3.54"
SAMPLE_IDS = [1309, 1102, 1213, 1306]

SYSTEM_PROMPT = """你是崩坏星穹铁道数据分析师。
你的任务是把国服中文角色技能、星魂、行迹文本抽取成结构化 axes。

硬规则:
1. 只使用工具 schema 和受控词表里的枚举值。
2. 不要按英文名或英文机制改写;角色名、技能名、机制描述以国服中文为准。
3. 数值统一为小数,例如 50% 写 0.5;不确定数值可以不填 value。
4. 复杂触发条件写入 condition,不要发明新的 stat/target/uptime。
5. 不确定的能力宁可少填,不要为了填满而猜。
6. roles 只描述角色长期定位;tags 描述队伍风格或资源倾向。
7. 击杀/消灭敌人用 uptime=on_kill;击破韧性才用 on_break。
8. 「附加伤害」用 stat=additional_dmg;只有明确写「追加攻击」才用 fua_dmg。
"""


def character_prompt(char_id: int, detail: dict[str, Any], skill_text: str) -> str:
    return f"""请抽取这个角色的 axes。

受控词表:
{vocab_prompt()}

角色 id: {char_id}
角色名: {normalize_name(detail.get("name"), "zh")}
命途: {detail.get("base_type")}
元素: {detail.get("damage_type")}
能量需求: {detail.get("sp_need")}

中文技能/星魂/行迹文本:
{skill_text}
"""


def parse_ids(raw_ids: list[str] | None) -> list[int]:
    if not raw_ids:
        return []
    ids: list[int] = []
    for raw in raw_ids:
        for part in raw.split(","):
            part = part.strip()
            if part:
                ids.append(int(part))
    return ids


def select_ids(data_dir: Path, requested: list[int], run_all: bool) -> list[int]:
    if requested:
        return requested
    if run_all:
        overview = load_json(data_dir / "character.json")
        return [int(char_id) for char_id in sorted(overview, key=int)]
    return SAMPLE_IDS


def extract_tool_payload(response: Any) -> dict[str, Any]:
    for block in response.content:
        if getattr(block, "type", None) == "tool_use" and getattr(block, "name", None) == "emit_character_axes":
            return dict(block.input)
    raise RuntimeError("LLM response did not contain emit_character_axes tool_use")


def enrich_character(client: Any, model: str, char_id: int, detail: dict[str, Any]) -> dict[str, Any]:
    skill_text = build_skill_text(detail)
    prompt = character_prompt(char_id, detail, skill_text)
    schema = tool_schema()
    response = client.messages.create(
        model=model,
        max_tokens=4096,
        system=SYSTEM_PROMPT,
        messages=[{"role": "user", "content": prompt}],
        tools=[schema],
        tool_choice={"type": "tool", "name": schema["name"]},
    )
    payload = extract_tool_payload(response)
    return normalize_axes(payload)


def openai_chat_url(base_url: str) -> str:
    base = base_url.rstrip("/")
    if base.endswith("/v1"):
        return f"{base}/chat/completions"
    return f"{base}/v1/chat/completions"


def enrich_character_openai(config: Any, char_id: int, detail: dict[str, Any]) -> dict[str, Any]:
    skill_text = build_skill_text(detail)
    prompt = character_prompt(char_id, detail, skill_text)
    tool = {
        "type": "function",
        "function": {
            "name": "emit_character_axes",
            "description": "Return normalized character roles, axes, and team-style tags.",
            "parameters": CHARACTER_AXES_INPUT_SCHEMA,
        },
    }
    payload = {
        "model": config.model,
        "temperature": 0,
        "max_tokens": 4096,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT + "\n你必须调用 emit_character_axes 工具,不要输出普通文本。"},
            {"role": "user", "content": prompt},
        ],
        "tools": [tool],
        "tool_choice": {"type": "function", "function": {"name": "emit_character_axes"}},
    }
    headers = {"Authorization": f"Bearer {config.api_key}", "Content-Type": "application/json"}
    response = None
    max_attempts = 6
    for attempt in range(1, max_attempts + 1):
        try:
            with httpx.Client(timeout=180) as http:
                response = http.post(openai_chat_url(config.base_url), headers=headers, json=payload)
            if response.status_code in {429, 500, 502, 503, 504} and attempt < max_attempts:
                print(f"retry {attempt}/{max_attempts}: HTTP {response.status_code}")
                time.sleep(5 * attempt)
                continue
            response.raise_for_status()
            break
        except httpx.TransportError as exc:
            if attempt >= max_attempts:
                raise
            print(f"retry {attempt}/{max_attempts}: {type(exc).__name__}: {exc}")
            time.sleep(5 * attempt)
    if response is None:
        raise RuntimeError("OpenAI-compatible request did not produce a response")
    body = response.json()
    message = body["choices"][0]["message"]
    tool_calls = message.get("tool_calls") or []
    if not tool_calls:
        raise RuntimeError(f"OpenAI-compatible response did not contain tool_calls: {body}")
    arguments = tool_calls[0]["function"]["arguments"]
    if isinstance(arguments, str):
        arguments = json.loads(arguments)
    return normalize_axes(arguments)


def output_payload(char_id: int, detail: dict[str, Any], axes: dict[str, Any], model: str) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "entity_kind": "character",
        "id": char_id,
        "name_zh": normalize_name(detail.get("name"), "zh"),
        "path": detail.get("base_type"),
        "element": detail.get("damage_type"),
        "model": model,
        "generated_at": datetime.now(UTC).isoformat(),
        "axes": axes,
    }


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Enrich character text into structured axes.")
    parser.add_argument("--version", default=os.getenv("HSR_DATA_VERSION", DEFAULT_VERSION))
    parser.add_argument(
        "--data-dir",
        type=Path,
        default=Path(os.getenv("HSR_DATA_DIR", f"nanoka_hsr/{DEFAULT_VERSION}")),
    )
    parser.add_argument(
        "--out-dir",
        type=Path,
        default=Path(os.getenv("HSR_ENRICHED_DIR", f"enriched/{DEFAULT_VERSION}")),
    )
    parser.add_argument("--ids", nargs="*", help="Character ids, comma-separated or space-separated.")
    parser.add_argument("--all", action="store_true", help="Process every character.")
    parser.add_argument("--force", action="store_true", help="Overwrite existing enriched files.")
    parser.add_argument("--dry-run", action="store_true", help="Print prompt preview without calling LLM.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    data_dir = args.data_dir if args.data_dir.is_absolute() else ROOT / args.data_dir
    out_dir = args.out_dir if args.out_dir.is_absolute() else ROOT / args.out_dir
    char_ids = select_ids(data_dir, parse_ids(args.ids), args.all)

    client = None
    config = None
    model = os.getenv("LLM_MODEL", DEFAULT_LLM_MODEL)
    if not args.dry_run:
        config = require_llm_config()
        if config.api_format not in {"anthropic", "openai"}:
            raise RuntimeError("LLM_API_FORMAT must be 'anthropic' or 'openai'")
        if config.api_format == "anthropic":
            client = create_client(config)
        model = config.model

    for char_id in char_ids:
        detail_path = data_dir / "zh" / "character" / f"{char_id}.json"
        if not detail_path.exists():
            raise FileNotFoundError(detail_path)
        detail = load_json(detail_path)
        out_path = out_dir / "character" / f"{char_id}.json"
        if out_path.exists() and not args.force:
            print(f"skip {char_id} {clean_text(detail.get('name'))}: {out_path}")
            continue

        skill_text = build_skill_text(detail)
        if args.dry_run:
            prompt = character_prompt(char_id, detail, skill_text)
            print(f"\n--- dry-run {char_id} {clean_text(detail.get('name'))} ---")
            print(prompt[:4000])
            if len(prompt) > 4000:
                print(f"\n... prompt truncated; full length={len(prompt)} chars")
            continue

        assert config is not None
        if config.api_format == "openai":
            axes = enrich_character_openai(config, char_id, detail)
        else:
            assert client is not None
            axes = enrich_character(client, model, char_id, detail)
        write_json(out_path, output_payload(char_id, detail, axes, model))
        print(f"wrote {out_path.relative_to(ROOT)}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
