from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import httpx
import psycopg
from dotenv import load_dotenv
from psycopg.types.json import Jsonb

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

from hsr_agent.db import get_conn
from hsr_agent.llm_client import create_client, require_llm_config
from schemas.modifier_vocab import (
    MODIFIER_RESPONSE_SCHEMA,
    normalize_modifier_payload,
    openai_tool_schema,
    tool_schema,
    vocab_prompt,
)
from scripts.load import clean_text, max_level_params, normalize_name

DEFAULT_VERSION = "4.3.54"
SAMPLE_IDS = [1306, 1309, 1303, 1101, 1205, 1308, 1005, 1310, 1203, 1217, 1304, 1208]
OPENAI_MAX_ATTEMPTS = 6
OPENAI_TRANSIENT_STATUS = {429, 500, 502, 503, 504, 524}
DSML_MODIFIERS_MARKER = '<｜DSML｜parameter name="modifiers" string="false">'
DSML_MODIFIERS_PARAM_MARKER = 'parameter name="modifiers" string="false">'

SYSTEM_PROMPT = """你是崩坏星穹铁道机制抽取员。
你的任务是从国服中文技能、行迹、星魂文本中抽取可计算的数值效果和 utility 效果。

硬规则:
0. 你必须在第一条响应中直接调用 emit_character_modifiers 工具。禁止输出自然语言、思考过程、逐条分析、Markdown 或解释。
1. 只使用受控词表,不要发明 stat_key、modifier_zone 或 attack_tag。
2. source_key 和 source_kind 必须原样来自输入 sources。
3. 百分比数值统一写小数,例如 50% 写 0.5;战技点、回合、能量、削韧等保持原始数值。
4. 不确定数值写 value=null,value_unit=unknown,并降低 confidence。
5. 「附加伤害」用 attack_tag=additional 或 stat_key=additional_dmg;只有明确写「追加攻击」才用 fua。
6. DoT、击破、超击破默认不吃暴击;不要把它们标成 crit 区。
7. 复杂条件写 condition_text,机器可读条件写 condition_jsonb。
8. 如果某条来源只有技能等级提升或没有明确机制,直接跳过。没有可抽取内容时返回 modifiers=[]。
"""

SKILL_TYPE_KIND = {
    "Normal": "basic",
    "BPSkill": "skill",
    "Ultra": "ult",
    "Talent": "talent",
    "Passive": "talent",
    "Maze": "technique",
    "QTE": "passive",
}

SKILL_TYPE_NAME_KIND = {
    "普攻": "basic",
    "战技": "skill",
    "终结技": "ult",
    "天赋": "talent",
    "秘技": "technique",
}

POINT_KIND = {
    "point01": "basic",
    "point02": "skill",
    "point03": "ult",
    "point04": "talent",
    "point05": "technique",
}


@dataclass(frozen=True)
class EffectSource:
    character_id: int
    source_kind: str
    source_key: str
    name_zh: str
    source_text_zh: str
    game_version: str

    @property
    def source_hash(self) -> str:
        return hashlib.sha256(self.source_text_zh.encode("utf-8")).hexdigest()


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


def source_text(parts: list[str], params: list[Any] | None = None) -> str:
    clean_parts = [clean_text(part) for part in parts if clean_text(part)]
    if params:
        clean_parts.append(f"参数: {json.dumps(params, ensure_ascii=False)}")
    return "\n".join(clean_parts).strip()


def iter_effect_sources(char_id: int, version: str, detail: dict[str, Any]) -> list[EffectSource]:
    sources: list[EffectSource] = []

    for skill_key, skill in sorted((detail.get("skills") or {}).items(), key=lambda kv: str(kv[0])):
        if not isinstance(skill, dict):
            continue
        type_name = clean_text(skill.get("type_name") or skill.get("type"))
        kind = SKILL_TYPE_KIND.get(str(skill.get("type")), SKILL_TYPE_NAME_KIND.get(type_name, "other"))
        name = clean_text(skill.get("name")) or f"skill_{skill_key}"
        text = source_text(
            [
                f"技能类型: {type_name}",
                f"名称: {name}",
                clean_text(skill.get("desc")),
                f"简述: {clean_text(skill.get('simple_desc'))}" if skill.get("simple_desc") else "",
            ],
            max_level_params(skill),
        )
        if text:
            sources.append(EffectSource(char_id, kind, str(skill_key), name, text, version))

    for rank_no, rank in sorted((detail.get("ranks") or {}).items(), key=lambda kv: int(kv[0])):
        if not isinstance(rank, dict):
            continue
        name = clean_text(rank.get("name")) or f"星魂{rank_no}"
        text = source_text([f"星魂{rank_no}: {name}", clean_text(rank.get("desc"))], rank.get("param_list") or [])
        if text:
            sources.append(EffectSource(char_id, "eidolon", str(rank_no), name, text, version))

    for point_key, levels in sorted((detail.get("skill_trees") or {}).items(), key=lambda kv: str(kv[0])):
        if not isinstance(levels, dict):
            continue
        for level_key, point in sorted(levels.items(), key=lambda kv: int(kv[0])):
            if not isinstance(point, dict):
                continue
            name = clean_text(point.get("point_name"))
            desc = clean_text(point.get("point_desc"))
            if not name and not desc:
                continue
            kind = POINT_KIND.get(str(point_key), "trace")
            source_key = f"{point_key}.{level_key}"
            text = source_text([f"行迹{source_key}: {name}", desc], point.get("param_list") or [])
            sources.append(EffectSource(char_id, kind, source_key, name or source_key, text, version))

    return sources


def fetch_characters(ids: list[int], run_all: bool, version: str) -> list[dict[str, Any]]:
    with get_conn() as conn:
        with conn.cursor() as cur:
            if ids:
                cur.execute(
                    """
                    SELECT id, name_zh, version, raw_zh
                    FROM characters
                    WHERE id = ANY(%s)
                    ORDER BY id
                    """,
                    (ids,),
                )
            elif run_all:
                cur.execute(
                    """
                    SELECT id, name_zh, version, raw_zh
                    FROM characters
                    WHERE version = %s
                    ORDER BY id
                    """,
                    (version,),
                )
            else:
                cur.execute(
                    """
                    SELECT id, name_zh, version, raw_zh
                    FROM characters
                    WHERE id = ANY(%s)
                    ORDER BY id
                    """,
                    (SAMPLE_IDS,),
                )
            return list(cur.fetchall())


def prompt_for_character(row: dict[str, Any], sources: list[EffectSource]) -> str:
    detail = row["raw_zh"]
    if isinstance(detail, str):
        detail = json.loads(detail)
    prompt_sources = [
        {
            "source_kind": src.source_kind,
            "source_key": src.source_key,
            "name_zh": src.name_zh,
            "text": src.source_text_zh,
        }
        for src in sources
        if is_prompt_relevant_source(src)
    ]
    return f"""请抽取这个角色的 modifiers。
必须直接调用 emit_character_modifiers,不要输出普通文本。

受控词表:
{vocab_prompt()}

角色 id: {row["id"]}
角色名: {normalize_name(detail.get("name"), "zh") or row["name_zh"]}
命途: {detail.get("base_type")}
元素: {detail.get("damage_type")}

sources:
{json.dumps(prompt_sources, ensure_ascii=False, indent=2)}
"""


def is_prompt_relevant_source(src: EffectSource) -> bool:
    text = src.source_text_zh
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    if len(lines) <= 1:
        return False
    if "等级+" in text and len(lines) <= 2 and "参数:" not in text:
        return False
    if src.source_kind == "trace" and "参数:" not in text and len(lines) <= 2:
        return False
    return True


def openai_chat_url(base_url: str) -> str:
    base = base_url.rstrip("/")
    if base.endswith("/v1"):
        return f"{base}/chat/completions"
    return f"{base}/v1/chat/completions"


class OpenAIResponseParseError(RuntimeError):
    pass


def compact_json(value: Any, limit: int = 2000) -> str:
    text = json.dumps(value, ensure_ascii=False, default=str)
    if len(text) <= limit:
        return text
    return f"{text[:limit]}...<truncated {len(text) - limit} chars>"


def build_openai_payload(config: Any, row: dict[str, Any], sources: list[EffectSource]) -> dict[str, Any]:
    return {
        "model": config.model,
        "temperature": 0,
        "max_tokens": 8192,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": prompt_for_character(row, sources)},
        ],
        "tools": [openai_tool_schema()],
        "tool_choice": {"type": "function", "function": {"name": "emit_character_modifiers"}},
    }


def parse_dsml_content(content: str) -> dict[str, Any] | None:
    marker_index = content.find(DSML_MODIFIERS_MARKER)
    marker_len = len(DSML_MODIFIERS_MARKER)
    if marker_index < 0:
        marker_index = content.find(DSML_MODIFIERS_PARAM_MARKER)
        marker_len = len(DSML_MODIFIERS_PARAM_MARKER)
    if marker_index < 0:
        return None
    fragment = content[marker_index + marker_len :].lstrip()
    try:
        modifiers, _ = json.JSONDecoder().raw_decode(fragment)
    except json.JSONDecodeError as exc:
        raise OpenAIResponseParseError("DSML modifiers JSON was incomplete or invalid") from exc
    if not isinstance(modifiers, list):
        raise OpenAIResponseParseError("DSML modifiers parameter was not an array")
    return {"modifiers": modifiers}


def parse_content_payload(content: str) -> dict[str, Any] | None:
    stripped = content.strip()
    if not stripped:
        return None

    dsml_payload = parse_dsml_content(stripped)
    if dsml_payload is not None:
        return dsml_payload

    if stripped.startswith("{"):
        try:
            loaded = json.loads(stripped)
        except json.JSONDecodeError as exc:
            raise OpenAIResponseParseError("assistant content JSON was incomplete or invalid") from exc
        if not isinstance(loaded, dict):
            raise OpenAIResponseParseError("assistant content JSON was not an object")
        return dict(loaded)

    return None


def parse_openai_message(message: dict[str, Any], body: dict[str, Any] | None = None) -> dict[str, Any]:
    tool_calls = message.get("tool_calls") or []
    if tool_calls:
        function = tool_calls[0].get("function") or {}
        arguments = function.get("arguments")
        if isinstance(arguments, str):
            try:
                loaded = json.loads(arguments)
            except json.JSONDecodeError as exc:
                raise OpenAIResponseParseError("tool call arguments JSON was incomplete or invalid") from exc
        elif isinstance(arguments, dict):
            loaded = arguments
        else:
            raise OpenAIResponseParseError("tool call arguments were missing")
        if not isinstance(loaded, dict):
            raise OpenAIResponseParseError("tool call arguments JSON was not an object")
        return dict(loaded)

    content = message.get("content")
    if isinstance(content, str):
        content_payload = parse_content_payload(content)
        if content_payload is not None:
            return content_payload

    if body is not None:
        raise OpenAIResponseParseError(
            f"OpenAI-compatible response did not contain parseable tool_calls: {compact_json(body)}"
        )
    raise OpenAIResponseParseError("OpenAI-compatible stream did not contain parseable tool_calls")


def apply_openai_stream_event(event: dict[str, Any], state: dict[str, Any]) -> None:
    for choice in event.get("choices") or []:
        finish_reason = choice.get("finish_reason")
        if finish_reason is not None:
            state["finish_reason"] = finish_reason

        delta = choice.get("delta") or {}
        content = delta.get("content")
        if isinstance(content, str):
            state["content_parts"].append(content)

        for tool_delta in delta.get("tool_calls") or []:
            index = int(tool_delta.get("index") or 0)
            tool_call = state["tool_calls"].setdefault(
                index,
                {"id": "", "type": "function", "function": {"name": "", "arguments": ""}},
            )
            if tool_delta.get("id"):
                tool_call["id"] = str(tool_delta["id"])
            if tool_delta.get("type"):
                tool_call["type"] = str(tool_delta["type"])

            function_delta = tool_delta.get("function") or {}
            if function_delta.get("name"):
                tool_call["function"]["name"] += str(function_delta["name"])
            if function_delta.get("arguments") is not None:
                tool_call["function"]["arguments"] += str(function_delta["arguments"])


def stream_state_to_message(state: dict[str, Any]) -> dict[str, Any]:
    return {
        "role": "assistant",
        "content": "".join(state["content_parts"]),
        "tool_calls": [call for _, call in sorted(state["tool_calls"].items())],
    }


def extract_openai_stream(config: Any, payload: dict[str, Any], headers: dict[str, str]) -> dict[str, Any]:
    stream_payload = dict(payload)
    stream_payload["stream"] = True
    response_timeout = httpx.Timeout(300.0, connect=30.0)
    url = openai_chat_url(config.base_url)
    last_error: Exception | None = None

    for attempt in range(1, OPENAI_MAX_ATTEMPTS + 1):
        state: dict[str, Any] = {"content_parts": [], "tool_calls": {}, "finish_reason": None}
        try:
            with httpx.Client(timeout=response_timeout) as http:
                with http.stream("POST", url, headers=headers, json=stream_payload) as response:
                    if response.status_code in OPENAI_TRANSIENT_STATUS and attempt < OPENAI_MAX_ATTEMPTS:
                        print(f"stream retry {attempt}/{OPENAI_MAX_ATTEMPTS}: HTTP {response.status_code}")
                        time.sleep(5 * attempt)
                        continue
                    if response.status_code >= 400:
                        error_text = response.read().decode("utf-8", errors="replace")
                        raise RuntimeError(
                            f"OpenAI-compatible stream request failed: "
                            f"HTTP {response.status_code}: {error_text[:2000]}"
                        )

                    for line in response.iter_lines():
                        if not line or line.startswith(":"):
                            continue
                        if not line.startswith("data:"):
                            continue
                        data = line[len("data:") :].strip()
                        if not data:
                            continue
                        if data == "[DONE]":
                            break
                        try:
                            event = json.loads(data)
                        except json.JSONDecodeError as exc:
                            raise OpenAIResponseParseError("OpenAI stream chunk JSON was invalid") from exc
                        apply_openai_stream_event(event, state)

            return parse_openai_message(stream_state_to_message(state))
        except httpx.TransportError as exc:
            last_error = exc
            if attempt >= OPENAI_MAX_ATTEMPTS:
                raise
            print(f"stream retry {attempt}/{OPENAI_MAX_ATTEMPTS}: {type(exc).__name__}: {exc}")
            time.sleep(5 * attempt)

    if last_error is not None:
        raise last_error
    raise RuntimeError("OpenAI-compatible stream request did not produce a response")


def extract_openai_non_stream(config: Any, payload: dict[str, Any], headers: dict[str, str]) -> dict[str, Any]:
    request_payload = dict(payload)
    request_payload.pop("stream", None)
    response = None
    for attempt in range(1, OPENAI_MAX_ATTEMPTS + 1):
        try:
            with httpx.Client(timeout=180) as http:
                response = http.post(openai_chat_url(config.base_url), headers=headers, json=request_payload)
            if response.status_code in OPENAI_TRANSIENT_STATUS and attempt < OPENAI_MAX_ATTEMPTS:
                print(f"retry {attempt}/{OPENAI_MAX_ATTEMPTS}: HTTP {response.status_code}")
                time.sleep(5 * attempt)
                continue
            if response.status_code >= 400:
                raise RuntimeError(
                    f"OpenAI-compatible request failed: HTTP {response.status_code}: {response.text[:2000]}"
                )
            response.raise_for_status()
            break
        except httpx.TransportError as exc:
            if attempt >= OPENAI_MAX_ATTEMPTS:
                raise
            print(f"retry {attempt}/{OPENAI_MAX_ATTEMPTS}: {type(exc).__name__}: {exc}")
            time.sleep(5 * attempt)
    if response is None:
        raise RuntimeError("OpenAI-compatible request did not produce a response")
    try:
        body = response.json()
    except ValueError as exc:
        raise RuntimeError(f"OpenAI-compatible response was not JSON: {response.text[:2000]}") from exc
    try:
        message = body["choices"][0]["message"]
    except (KeyError, IndexError, TypeError) as exc:
        raise OpenAIResponseParseError(f"OpenAI-compatible response shape was invalid: {compact_json(body)}") from exc
    return parse_openai_message(message, body)


def extract_openai(
    config: Any,
    row: dict[str, Any],
    sources: list[EffectSource],
    stream: bool = True,
) -> dict[str, Any]:
    payload = build_openai_payload(config, row, sources)
    headers = {"Authorization": f"Bearer {config.api_key}", "Content-Type": "application/json"}
    if not stream:
        return extract_openai_non_stream(config, payload, headers)
    try:
        return extract_openai_stream(config, payload, headers)
    except OpenAIResponseParseError as exc:
        print(f"stream parse fallback to non-stream: {exc}", file=sys.stderr)
        return extract_openai_non_stream(config, payload, headers)


def extract_anthropic(client: Any, model: str, row: dict[str, Any], sources: list[EffectSource]) -> dict[str, Any]:
    schema = tool_schema()
    response = client.messages.create(
        model=model,
        max_tokens=4096,
        system=SYSTEM_PROMPT,
        messages=[{"role": "user", "content": prompt_for_character(row, sources)}],
        tools=[schema],
        tool_choice={"type": "tool", "name": schema["name"]},
    )
    for block in response.content:
        if getattr(block, "type", None) == "tool_use" and getattr(block, "name", None) == "emit_character_modifiers":
            return dict(block.input)
    raise RuntimeError("LLM response did not contain emit_character_modifiers tool_use")


def upsert_sources(cur: psycopg.Cursor, sources: list[EffectSource]) -> dict[tuple[str, str], int]:
    source_ids: dict[tuple[str, str], int] = {}
    for src in sources:
        cur.execute(
            """
            INSERT INTO character_effect_sources
                (character_id, source_kind, source_key, name_zh, source_text_zh, game_version, source_hash)
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (character_id, source_kind, source_key, game_version)
            DO UPDATE SET
                name_zh = EXCLUDED.name_zh,
                source_text_zh = EXCLUDED.source_text_zh,
                source_hash = EXCLUDED.source_hash,
                updated_at = now()
            RETURNING id
            """,
            (
                src.character_id,
                src.source_kind,
                src.source_key,
                src.name_zh,
                src.source_text_zh,
                src.game_version,
                src.source_hash,
            ),
        )
        source_ids[(src.source_kind, src.source_key)] = int(cur.fetchone()["id"])
    return source_ids


def delete_existing_modifiers(cur: psycopg.Cursor, char_id: int) -> None:
    cur.execute(
        """
        DELETE FROM character_modifiers m
        USING character_effect_sources s
        WHERE m.source_id = s.id
          AND s.character_id = %s
        """,
        (char_id,),
    )


def insert_modifiers(cur: psycopg.Cursor, source_ids: dict[tuple[str, str], int], modifiers: list[dict[str, Any]]) -> None:
    for mod in modifiers:
        source_id = source_ids[(mod["source_kind"], mod["source_key"])]
        cur.execute(
            """
            INSERT INTO character_modifiers
                (source_id, target_scope, stat_key, value, value_unit, modifier_zone,
                 attack_tag, element_key, target_path, condition_text, condition_jsonb,
                 duration_key, stack_rule, confidence, reviewed)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, false)
            """,
            (
                source_id,
                mod["target_scope"],
                mod["stat_key"],
                mod["value"],
                mod["value_unit"],
                mod["modifier_zone"],
                mod["attack_tag"],
                mod["element_key"],
                mod["target_path"],
                mod["condition_text"],
                Jsonb(mod["condition_jsonb"]),
                mod["duration_key"],
                mod["stack_rule"],
                mod["confidence"],
            ),
        )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract character mechanics modifiers into PostgreSQL.")
    parser.add_argument("--version", default=os.getenv("HSR_DATA_VERSION", DEFAULT_VERSION))
    parser.add_argument("--ids", nargs="*", help="Character ids, comma-separated or space-separated.")
    parser.add_argument("--all", action="store_true", help="Process every character.")
    parser.add_argument("--dry-run", action="store_true", help="Print source extraction and prompt preview only.")
    parser.add_argument("--sources-only", action="store_true", help="Only upsert character_effect_sources; do not call LLM.")
    parser.add_argument("--continue-on-error", action="store_true", help="Log per-character extraction errors and continue.")
    parser.add_argument("--no-stream", action="store_true", help="Disable streaming for OpenAI-compatible extraction.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    rows = fetch_characters(parse_ids(args.ids), args.all, args.version)
    if not rows:
        print("no characters selected")
        return 1

    config = None
    client = None
    if not args.dry_run and not args.sources_only:
        config = require_llm_config()
        if config.api_format not in {"openai", "anthropic"}:
            raise RuntimeError("LLM_API_FORMAT must be 'openai' or 'anthropic'")
        if config.api_format == "anthropic":
            client = create_client(config)

    for row in rows:
        detail = row["raw_zh"]
        if isinstance(detail, str):
            detail = json.loads(detail)
        sources = iter_effect_sources(int(row["id"]), str(row["version"]), detail)
        valid_sources = {(src.source_kind, src.source_key) for src in sources}

        if args.dry_run:
            print(f"\n--- dry-run {row['id']} {row['name_zh']} sources={len(sources)} ---")
            for src in sources:
                print(f"[{src.source_kind}:{src.source_key}] {src.name_zh} hash={src.source_hash[:12]}")
                print(src.source_text_zh[:500])
            prompt = prompt_for_character(row, sources)
            print(f"\n--- prompt preview length={len(prompt)} ---")
            print(prompt[:5000])
            if len(prompt) > 5000:
                print("... prompt truncated")
            continue

        try:
            with get_conn() as conn:
                with conn.cursor() as cur:
                    source_ids = upsert_sources(cur, sources)
                    if args.sources_only:
                        print(f"upserted sources {row['id']} {row['name_zh']}: {len(source_ids)}")
                        continue

                    assert config is not None
                    if config.api_format == "openai":
                        payload = extract_openai(config, row, sources, stream=not args.no_stream)
                    else:
                        assert client is not None
                        payload = extract_anthropic(client, config.model, row, sources)
                    normalized = normalize_modifier_payload(payload, valid_sources)
                    delete_existing_modifiers(cur, int(row["id"]))
                    insert_modifiers(cur, source_ids, normalized["modifiers"])
                    print(
                        f"wrote modifiers {row['id']} {row['name_zh']}: "
                        f"sources={len(source_ids)} modifiers={len(normalized['modifiers'])}"
                    )
        except Exception as exc:
            if not args.continue_on_error:
                raise
            print(f"ERROR modifiers {row['id']} {row['name_zh']}: {type(exc).__name__}: {exc}", file=sys.stderr)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
