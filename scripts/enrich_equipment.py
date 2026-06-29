from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from collections import Counter, defaultdict
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import httpx
from dotenv import load_dotenv

try:
    import psycopg
    from psycopg.rows import dict_row
except ModuleNotFoundError:
    psycopg = None
    dict_row = None

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.llm_client import require_llm_config
from schemas.axes_vocab import STATS, TAGS
from schemas.equipment_axes_vocab import EQUIPMENT_AXES_INPUT_SCHEMA, equipment_vocab_prompt, normalize_equipment_axes
from scripts.load import (
    build_lightcone_desc,
    clean_text,
    lightcone_refinement,
    lightcone_refinement_params,
    load_json,
    load_lightcone_detail,
    relic_kind,
)

DEFAULT_VERSION = "4.3.54"
DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"
DEFAULT_RELIC_SAMPLE_IDS = [101, 102, 118, 120, 301, 312]
DEFAULT_LIGHTCONE_SAMPLE_IDS = [21002, 23005, 24002]

SYSTEM_PROMPT = """你是崩坏星穹铁道装备机制数据分析师。
你的任务是把国服中文光锥/遗器效果文本抽取成结构化 equipment axes。

硬规则:
1. 只使用工具 schema 和受控词表里的枚举值,不要发明新的 stat/target/uptime/tag。
2. 只基于给定中文文本和参数抽取,不要用游戏记忆补充不存在的效果。
3. 数值统一为小数,例如 24% 写 0.24;光锥默认抽取叠影1数值,除非文本明确要求其它叠影。
4. provides 表示装备实际提供的属性、伤害、资源、机制效果;needs 表示装备适合/要求的触发机制、角色类型或队伍环境;restricts 表示限制、阈值、不可叠加、站位、目标类型等。
5. 目标(target)按受益者或受影响对象填写:装备者用 self,全队用 all_allies,敌人 debuff 用 one_enemy/all_enemies。
6. 复杂触发条件原文摘要写入 condition;不确定数值可以不填 value。
7. 「附加伤害」用 additional_dmg;只有明确写「追加攻击」才用 fua_dmg。
8. tag 必须由文本显式机制触发:只有出现「追加攻击」才可填 fua_team;只有出现「持续伤害」才可填 dot_team;只有出现「超击破」才可填 super_break_team。
9. 叠层上限、持续回合这类说明写入 condition/notes,不要作为 restricts 的机制效果;只有阈值、站位、目标类型、不可叠加、同命途/同属性等才写 restricts。
10. 不确定的效果宁可少填,不要为了填满而猜。
"""

PLACEHOLDER_RE = re.compile(r"#(\d+)(?:\[[^\]]+\])?")
VALUE_RE = re.compile(r"(-?\d+(?:\.\d+)?)(%)?")
DEF_IGNORE_RE = re.compile(r"无视(?:其|目标)?\s*(-?\d+(?:\.\d+)?)%的防御力")
HP_RESTORE_RE = re.compile(r"回复等同于(?:自身|装备者)?生命上限\s*(-?\d+(?:\.\d+)?)%")

STAT_PATTERNS: list[tuple[str, str]] = [
    ("outgoing_heal", "治疗量"),
    ("shield_strength", "护盾量"),
    ("crit_rate", "暴击率"),
    ("crit_dmg", "暴击伤害"),
    ("break_eff", "击破特攻"),
    ("energy_regen", "能量恢复效率"),
    ("effect_hit", "效果命中"),
    ("effect_res", "效果抵抗"),
    ("speed_percent", "速度提高"),
    ("atk_percent", "攻击力"),
    ("hp_percent", "生命上限"),
    ("def_ignore", "无视防御力"),
    ("def_percent", "防御力"),
    ("basic_dmg", "普攻造成的伤害"),
    ("skill_dmg", "战技造成的伤害"),
    ("ult_dmg", "终结技造成的伤害"),
    ("fua_dmg", "追加攻击造成的伤害"),
    ("dot_dmg", "持续伤害"),
    ("super_break_dmg", "超击破伤害"),
    ("break_dmg", "击破伤害"),
    ("res_pen", "抗性穿透"),
    ("dmg_percent", "造成的伤害"),
]

ELEMENTS = ["物理", "火", "冰", "雷", "风", "量子", "虚数"]
DAMAGE_STATS = {
    "basic_dmg",
    "skill_dmg",
    "ult_dmg",
    "fua_dmg",
    "dot_dmg",
    "break_dmg",
    "super_break_dmg",
    "dmg_percent",
}
COMBINED_DAMAGE_PATTERNS: list[tuple[str, list[str]]] = [
    ("普攻和战技造成的伤害", ["basic_dmg", "skill_dmg"]),
    ("战技和终结技造成的伤害", ["skill_dmg", "ult_dmg"]),
    ("终结技和追加攻击造成的伤害", ["ult_dmg", "fua_dmg"]),
]


def parse_ids(raw_ids: list[str] | None) -> set[int] | None:
    if not raw_ids:
        return None
    ids: set[int] = set()
    for raw in raw_ids:
        for part in raw.split(","):
            part = part.strip()
            if part:
                ids.add(int(part))
    return ids


def select_ids(overview: dict[str, Any], requested: set[int] | None, run_all: bool, samples: list[int]) -> list[int]:
    if requested is not None:
        return sorted(requested)
    if run_all:
        return [int(item_id) for item_id in sorted(overview, key=int)]
    return samples


def openai_chat_url(base_url: str) -> str:
    base = base_url.rstrip("/")
    if base.endswith("/v1"):
        return f"{base}/chat/completions"
    return f"{base}/v1/chat/completions"


def tool_definition() -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": "emit_equipment_axes",
            "description": "Return normalized lightcone or relic-set provides/needs/restricts/tags.",
            "parameters": EQUIPMENT_AXES_INPUT_SCHEMA,
        },
    }


def lightcone_effect_text(data_dir: Path, item_id: int) -> tuple[dict[str, Any], str]:
    detail = load_lightcone_detail(data_dir, "zh", item_id)
    refinements = lightcone_refinement(detail)
    if not refinements:
        return detail, ""

    lines = [
        f"光锥名: {clean_text(detail.get('name'))}",
        f"命途: {detail.get('base_type')}",
        f"稀有度: {detail.get('rarity')}",
        f"技能名: {clean_text(refinements.get('name'))}",
        f"原始效果文本: {clean_text(refinements.get('desc'))}",
        f"叠影1渲染文本: {build_lightcone_desc(detail, '1')}",
    ]
    levels = refinements.get("level") or {}
    if isinstance(levels, dict):
        for level in ["1", "2", "3", "4", "5"]:
            row = levels.get(level) or {}
            if isinstance(row, dict) and row.get("param_list") is not None:
                lines.append(f"叠影{level}参数: {json.dumps(row.get('param_list') or [], ensure_ascii=False)}")
    return detail, "\n".join(part for part in lines if part.strip())


def relic_effect_text(item_id: int, row: dict[str, Any]) -> str:
    lines = [
        f"遗器套装名: {clean_text(row.get('zh'))}",
        f"类型: {relic_kind(item_id, row)}",
    ]
    set_bonus = row.get("set") or {}
    for bonus_key, bonus in sorted(set_bonus.items()):
        if not isinstance(bonus, dict):
            continue
        desc = clean_text(bonus.get("zh"))
        params = bonus.get("ParamList") or bonus.get("param_list") or []
        if desc:
            lines.append(f"{bonus_key}件套效果: {desc}")
        if params:
            lines.append(f"{bonus_key}件套参数: {json.dumps(params, ensure_ascii=False)}")
    return "\n".join(part for part in lines if part.strip())


def equipment_prompt(entity_kind: str, item_id: int, row: dict[str, Any], data_dir: Path) -> tuple[dict[str, Any], str]:
    if entity_kind == "lightcone":
        detail, text = lightcone_effect_text(data_dir, item_id)
        if not text:
            text = f"光锥名: {clean_text(row.get('zh'))}\n命途: {row.get('baseType')}\n效果文本缺失。"
        prompt = f"""请抽取这个光锥的 equipment axes。

受控词表:
{equipment_vocab_prompt()}

光锥 id: {item_id}

中文光锥详情:
{text}
"""
        return detail or row, prompt

    text = relic_effect_text(item_id, row)
    prompt = f"""请抽取这个遗器套装的 equipment axes。

受控词表:
{equipment_vocab_prompt()}

遗器套装 id: {item_id}

中文遗器套装详情:
{text}
"""
    return row, prompt


def extract_tool_arguments(body: dict[str, Any]) -> dict[str, Any]:
    message = body["choices"][0]["message"]
    tool_calls = message.get("tool_calls") or []
    if not tool_calls:
        raise RuntimeError(f"OpenAI-compatible response did not contain tool_calls: {body}")
    arguments = tool_calls[0]["function"]["arguments"]
    if isinstance(arguments, str):
        return json.loads(arguments)
    return dict(arguments)


def sanitize_equipment_axes(axes: dict[str, Any], source_text: str) -> dict[str, Any]:
    rules: list[tuple[list[str], set[str], set[str]]] = [
        (["追加攻击"], {"fua_team"}, {"fua_dmg", "fua_trigger"}),
        (["持续伤害"], {"dot_team"}, {"dot_dmg", "dot_trigger"}),
        (["超击破"], {"super_break_team"}, {"super_break_dmg"}),
        (["召唤物", "忆灵", "记忆灵"], {"summon_team"}, set()),
        (["护盾"], {"shield_dependent"}, {"shield_strength", "shield_apply"}),
        (["治疗", "回复生命", "恢复生命"], {"heal_dependent"}, {"outgoing_heal", "heal_percent", "heal_over_time"}),
        (["生命值降低", "消耗生命", "损失生命"], {"hp_loss_team"}, set()),
    ]
    blocked_tags: set[str] = set()
    blocked_stats: set[str] = set()
    for needles, tags, stats in rules:
        if any(needle in source_text for needle in needles):
            continue
        blocked_tags.update(tags)
        blocked_stats.update(stats)

    if blocked_tags:
        axes["tags"] = [tag for tag in axes.get("tags") or [] if tag not in blocked_tags]
    if blocked_stats:
        for key in ["provides", "needs", "restricts"]:
            axes[key] = [
                item
                for item in axes.get(key) or []
                if not isinstance(item, dict) or item.get("stat") not in blocked_stats
            ]
    axes["restricts"] = [
        item
        for item in axes.get("restricts") or []
        if not (
            isinstance(item, dict)
            and any(needle in str(item.get("condition", "")) for needle in ["最多叠加", "叠加上限", "上限"])
            and not any(needle in str(item.get("condition", "")) for needle in ["大于等于", "至少", "不少于", "无法叠加", "同属性", "同命途"])
        )
    ]
    if not any(isinstance(item, dict) and item.get("stat") == "heal_percent" for item in axes.get("provides") or []):
        match = HP_RESTORE_RE.search(source_text)
        if match:
            if "施放终结技" in source_text:
                uptime = "on_ult"
            elif "击破" in source_text:
                uptime = "on_break"
            else:
                uptime = "conditional"
            axes.setdefault("provides", []).append(
                {
                    "stat": "heal_percent",
                    "target": "self",
                    "value": float(match.group(1)) / 100,
                    "uptime": uptime,
                    "condition": "回复等同于自身生命上限一定比例的生命值。",
                    "source": "deterministic_postprocess",
                    "confidence": 0.95,
                }
            )
    return axes


def enrich_equipment_openai(config: Any, entity_kind: str, item_id: int, row: dict[str, Any], data_dir: Path) -> tuple[dict[str, Any], dict[str, Any]]:
    payload_row, prompt = equipment_prompt(entity_kind, item_id, row, data_dir)
    tool = tool_definition()
    payload = {
        "model": config.model,
        "temperature": 0,
        "max_tokens": 4096,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT + "\n你必须调用 emit_equipment_axes 工具,不要输出普通文本。"},
            {"role": "user", "content": prompt},
        ],
        "tools": [tool],
        "tool_choice": {"type": "function", "function": {"name": "emit_equipment_axes"}},
    }
    headers = {"Authorization": f"Bearer {config.api_key}", "Content-Type": "application/json"}
    response = None
    max_attempts = 6
    for attempt in range(1, max_attempts + 1):
        try:
            with httpx.Client(timeout=180) as http:
                response = http.post(openai_chat_url(config.base_url), headers=headers, json=payload)
            if response.status_code in {429, 500, 502, 503, 504} and attempt < max_attempts:
                print(f"retry {entity_kind} {item_id} {attempt}/{max_attempts}: HTTP {response.status_code}")
                time.sleep(5 * attempt)
                continue
            response.raise_for_status()
            break
        except httpx.TransportError as exc:
            if attempt >= max_attempts:
                raise
            print(f"retry {entity_kind} {item_id} {attempt}/{max_attempts}: {type(exc).__name__}: {exc}")
            time.sleep(5 * attempt)
    if response is None:
        raise RuntimeError("OpenAI-compatible request did not produce a response")
    axes = normalize_equipment_axes(extract_tool_arguments(response.json()))
    axes = sanitize_equipment_axes(axes, prompt)
    return payload_row, axes


def placeholder_value(raw_text: str, params: list[Any], start: int, fallback_index: int = 0) -> float | None:
    nearby = raw_text[start : start + 80]
    match = PLACEHOLDER_RE.search(nearby)
    if match:
        index = int(match.group(1)) - 1
    else:
        index = fallback_index
    if index < 0 or index >= len(params):
        return None
    value = params[index]
    if isinstance(value, (int, float)):
        return float(value)
    return None


def format_text(raw_text: str, params: list[Any]) -> str:
    text = clean_text(raw_text)

    def replace(match: re.Match[str]) -> str:
        index = int(match.group(1)) - 1
        if index < 0 or index >= len(params):
            return match.group(0)
        value = params[index]
        is_percent_placeholder = text[match.end() : match.end() + 1] == "%"
        if isinstance(value, (int, float)) and is_percent_placeholder:
            return f"{value * 100:g}"
        return f"{value:g}" if isinstance(value, (int, float)) else str(value)

    return PLACEHOLDER_RE.sub(replace, text)


def parsed_value(raw: str, is_percent: bool) -> float:
    value = float(raw)
    return value / 100 if is_percent else value


def first_value_after(text: str, position: int, max_chars: int = 80) -> float | None:
    match = VALUE_RE.search(text[position : position + max_chars])
    if not match:
        return None
    return parsed_value(match.group(1), match.group(2) == "%")


def clause_bounds(text: str, position: int) -> tuple[int, int]:
    start = max(text.rfind(mark, 0, position) for mark in ["。", "；", "\n"])
    end_candidates = [idx for idx in [text.find(mark, position) for mark in ["。", "；", "\n"]] if idx >= 0]
    end = min(end_candidates) if end_candidates else len(text)
    return start + 1, end


def add_axis(
    axes: dict[str, Any],
    kind: str,
    stat: str,
    *,
    target: str = "self",
    value: float | None = None,
    uptime: str = "passive",
    condition: str = "",
    source: str = "",
    confidence: float = 0.75,
) -> None:
    if stat not in STATS and kind != "tag":
        return
    if kind == "tag":
        if stat in TAGS and stat not in axes["tags"]:
            axes["tags"].append(stat)
        return
    item: dict[str, Any] = {
        "stat": stat,
        "target": target,
        "uptime": uptime,
        "source": source,
        "confidence": confidence,
    }
    if value is not None:
        item["value"] = value
    if condition:
        item["condition"] = condition
    axes[kind].append(item)


def target_from_text(text: str) -> str:
    if "我方全体" in text or "全体我方" in text:
        return "all_allies"
    if "我方其他" in text or "其他我方" in text or "其他角色" in text:
        return "all_allies"
    if "我方单体" in text or "技能目标" in text:
        return "one_ally"
    if "敌方全体" in text:
        return "all_enemies"
    if "敌方目标" in text:
        return "one_enemy"
    return "self"


def clause_around(text: str, position: int) -> str:
    start, end = clause_bounds(text, position)
    return text[start:end]


def target_from_position(text: str, position: int, stat: str = "") -> str:
    clause = clause_around(text, position)
    target = target_from_text(clause)
    if stat == "def_ignore":
        return "one_enemy"
    if target in {"all_allies", "one_ally", "self_and_allies"}:
        return target
    if stat in DAMAGE_STATS:
        return "self"
    if "装备者及其忆灵" in clause or "装备者和忆灵" in clause:
        return "self_and_allies"
    if "装备者" in clause:
        return "self"
    return target


def uptime_from_text(text: str) -> str:
    if "最多叠加" in text or "层" in text and "提高" in text:
        return "stack_based"
    if "战斗开始" in text or "进入战斗" in text:
        return "combat_start"
    if "施放终结技" in text:
        return "on_ult"
    if "施放战技" in text:
        return "on_skill"
    if "施放追加攻击" in text or "追加攻击时" in text:
        return "on_fua"
    if "击破" in text and "击破特攻" not in text:
        return "on_break"
    if "消灭" in text or "击杀" in text:
        return "on_kill"
    if "攻击后" in text or "施放攻击" in text:
        return "on_attack"
    if "当" in text or "若" in text or "大于等于" in text:
        return "conditional"
    return "passive"


def is_threshold_reference(text: str, position: int) -> bool:
    start, _ = clause_bounds(text, position)
    clause = clause_around(text, position)
    local_position = position - start
    for marker in ["大于等于", "不少于", "至少", "达到"]:
        marker_position = clause.find(marker)
        if marker_position < 0:
            continue
        condition_end = clause.find("时", marker_position)
        if condition_end < 0:
            condition_end = len(clause)
        if local_position < condition_end and "提高" not in clause[local_position:condition_end]:
            return True
    return False


def should_skip_stat(text: str, stat: str, position: int) -> bool:
    clause = clause_around(text, position)
    tail = clause[position - clause_bounds(text, position)[0] :]
    if is_threshold_reference(text, position):
        return True
    if "提高" not in tail and stat != "def_ignore":
        return True
    if stat == "def_percent" and ("无视" in clause or "防御力降低" in clause):
        return True
    if stat in DAMAGE_STATS and "无视" in clause:
        return True
    if any(needle in clause for needle, _ in COMBINED_DAMAGE_PATTERNS) and stat == "dmg_percent":
        return True
    if stat != "dmg_percent":
        return False
    window = text[max(0, position - 12) : position + 16]
    return any(
        needle in window
        for needle in ["普攻造成的伤害", "战技造成的伤害", "终结技造成的伤害", "追加攻击造成的伤害", "持续伤害", "击破伤害", "超击破伤害"]
    )


def add_tags_and_needs(axes: dict[str, Any], text: str, source: str) -> None:
    pairs = [
        ("追加攻击", "fua_team", "fua_dmg"),
        ("持续伤害", "dot_team", "dot_dmg"),
        ("超击破", "super_break_team", "super_break_dmg"),
        ("击破", "break_team", "break_eff"),
        ("终结技", "ult_team", "ult_dmg"),
        ("召唤物", "summon_team", ""),
        ("记忆灵", "summon_team", ""),
        ("护盾", "shield_dependent", "shield_strength"),
        ("治疗", "heal_dependent", "outgoing_heal"),
    ]
    for needle, tag, need_stat in pairs:
        if needle not in text:
            continue
        add_axis(axes, "tag", tag)
        if need_stat:
            add_axis(
                axes,
                "needs",
                need_stat,
                condition=f"装备效果文本包含「{needle}」,适合能稳定触发或利用该机制的角色。",
                source=source,
                confidence=0.6,
            )
    if "速度" in text:
        add_axis(axes, "tag", "speed_team")
    if "暴击" in text:
        add_axis(axes, "tag", "crit_scaler")
    if "攻击力" in text:
        add_axis(axes, "tag", "atk_scaler")
    if "生命上限" in text:
        add_axis(axes, "tag", "hp_scaler")
    if "防御力" in text and "无视" not in text and "防御力降低" not in text:
        add_axis(axes, "tag", "def_scaler")


def add_restrictions(axes: dict[str, Any], text: str, source: str) -> None:
    restriction_needles = [
        "大于等于",
        "至少",
        "同属性",
        "同命途",
        "不是编队中的第一位角色",
        "位于编队第一位",
        "无法叠加",
    ]
    for needle in restriction_needles:
        if needle in text:
            add_axis(
                axes,
                "restricts",
                "def_unique_buff",
                condition=f"条件限制: {text}",
                source=source,
                confidence=0.45,
            )
            return


def extract_effect_axes(raw_text: str, params: list[Any], source: str) -> dict[str, Any]:
    text = format_text(raw_text, params)
    axes: dict[str, Any] = {"provides": [], "needs": [], "restricts": [], "tags": []}
    target = target_from_text(text)
    uptime = uptime_from_text(text)

    for element in ELEMENTS:
        needle = f"{element}属性伤害"
        if needle in text:
            position = text.find(needle)
            clause = clause_around(text, position)
            add_axis(
                axes,
                "provides",
                "dmg_percent",
                target="self",
                value=first_value_after(text, position),
                uptime=uptime_from_text(clause),
                condition=f"{element}属性伤害提高。",
                source=source,
            )

    for match in DEF_IGNORE_RE.finditer(text):
        position = match.start()
        clause = clause_around(text, position)
        add_axis(
            axes,
            "provides",
            "def_ignore",
            target=target_from_position(text, position, "def_ignore"),
            value=parsed_value(match.group(1), True),
            uptime=uptime_from_text(clause),
            condition=clause,
            source=source,
        )

    for needle, stats in COMBINED_DAMAGE_PATTERNS:
        search_from = 0
        while True:
            position = text.find(needle, search_from)
            if position < 0:
                break
            if not should_skip_stat(text, stats[0], position):
                clause = clause_around(text, position)
                for stat in stats:
                    add_axis(
                        axes,
                        "provides",
                        stat,
                        target=target_from_position(text, position, stat),
                        value=first_value_after(text, position + len(needle)),
                        uptime=uptime_from_text(clause),
                        condition=clause,
                        source=source,
                    )
            search_from = position + len(needle)

    for stat, needle in STAT_PATTERNS:
        search_from = 0
        while True:
            position = text.find(needle, search_from)
            if position < 0:
                break
            if should_skip_stat(text, stat, position):
                search_from = position + len(needle)
                continue
            clause = clause_around(text, position)
            local_target = target_from_position(text, position, stat)
            add_axis(
                axes,
                "provides",
                stat,
                target=local_target,
                value=first_value_after(text, position),
                uptime=uptime_from_text(clause),
                condition=clause,
                source=source,
            )
            search_from = position + len(needle)

    if "恢复1个战技点" in text or "恢复1点战技点" in text:
        add_axis(
            axes,
            "provides",
            "sp_recovery",
            target="all_allies",
            value=1,
            uptime="combat_start" if "战斗开始" in text else uptime,
            condition=text,
            source=source,
        )

    if "行动提前" in text:
        add_axis(axes, "provides", "turn_advance", target=target, uptime=uptime, condition=text, source=source)

    add_tags_and_needs(axes, text, source)
    add_restrictions(axes, text, source)
    return axes


def merge_axes(base: dict[str, Any], extra: dict[str, Any]) -> None:
    for key in ["provides", "needs", "restricts"]:
        base[key].extend(extra.get(key) or [])
    for tag in extra.get("tags") or []:
        if tag not in base["tags"]:
            base["tags"].append(tag)


def relic_axes(relic_id: int, row: dict[str, Any]) -> dict[str, Any]:
    axes: dict[str, Any] = {"provides": [], "needs": [], "restricts": [], "tags": []}
    set_bonus = row.get("set") or {}
    for bonus_key, bonus in sorted(set_bonus.items()):
        if not isinstance(bonus, dict):
            continue
        raw_text = clean_text(bonus.get("zh"))
        params = bonus.get("ParamList") or []
        source = f"{bonus_key}pc"
        merge_axes(axes, extract_effect_axes(raw_text, params, source))
    if relic_kind(relic_id, row) == "planar":
        axes["notes"] = "位面饰品套装 axes 由中文 2 件套效果规则抽取。"
    else:
        axes["notes"] = "隧洞遗器套装 axes 由中文 2/4 件套效果规则抽取。"
    return normalize_equipment_axes(axes)


def db_lightcone_inference() -> dict[int, dict[str, Any]]:
    if psycopg is None or dict_row is None:
        print("warning: psycopg is not installed; skip lightcone weak inference from DB")
        return {}
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    if not database_url:
        return {}
    inference: dict[int, dict[str, Any]] = defaultdict(lambda: {"tags": Counter(), "needs": Counter(), "chars": []})
    try:
        with psycopg.connect(database_url, row_factory=dict_row) as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """
                    SELECT cr.item_id, c.id AS char_id, c.name_zh, c.axes
                    FROM character_recommendations cr
                    JOIN characters c ON c.id = cr.char_id
                    WHERE cr.recommend_kind = 'lightcone' AND cr.item_id IS NOT NULL
                    """
                )
                for row in cur.fetchall():
                    item_id = int(row["item_id"])
                    axes = row["axes"] or {}
                    inference[item_id]["chars"].append({"id": row["char_id"], "name_zh": row["name_zh"]})
                    for tag in axes.get("tags") or []:
                        if tag in TAGS:
                            inference[item_id]["tags"][tag] += 1
                    for need in axes.get("needs") or []:
                        stat = need.get("stat") if isinstance(need, dict) else None
                        if stat in STATS:
                            inference[item_id]["needs"][stat] += 1
    except psycopg.Error as exc:
        print(f"warning: cannot infer lightcone axes from DB recommendations: {exc}")
        return {}
    return inference


def lightcone_axes(lc_id: int, row: dict[str, Any], inference: dict[int, dict[str, Any]]) -> dict[str, Any]:
    axes: dict[str, Any] = {"provides": [], "needs": [], "restricts": [], "tags": []}
    desc = clean_text(row.get("zh_desc") or row.get("desc"))
    if desc:
        params = row.get("params") or row.get("ParamList") or []
        merge_axes(axes, extract_effect_axes(desc, params, "lightcone_effect"))
        axes["notes"] = "光锥 axes 由本地效果文本规则抽取。"
        return normalize_equipment_axes(axes)

    inferred = inference.get(lc_id) or {}
    tags: Counter[str] = inferred.get("tags") or Counter()
    needs: Counter[str] = inferred.get("needs") or Counter()
    for tag, _ in tags.most_common(6):
        add_axis(axes, "tag", tag)
    for stat, _ in needs.most_common(6):
        add_axis(
            axes,
            "needs",
            stat,
            condition="本地 lightcone.json 缺少效果文本;该需求由 nanoka 推荐角色的 needs 反推。",
            source="recommended_characters",
            confidence=0.35,
        )
    axes["notes"] = (
        "本地 lightcone.json 缺少光锥效果文本;当前 axes 由命途和 nanoka 推荐角色画像反推,"
        "只能用于弱检索/推荐解释,不能当作光锥机制事实。补抓光锥效果文本后需重跑。"
    )
    return normalize_equipment_axes(axes)


def output_payload(
    entity_kind: str,
    item_id: int,
    row: dict[str, Any],
    axes: dict[str, Any],
    version: str,
    model: str = "rules-v1",
) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "entity_kind": entity_kind,
        "id": item_id,
        "name_zh": clean_text(row.get("name") or row.get("zh")),
        "name_en": clean_text(row.get("en")),
        "path": row.get("base_type") or row.get("baseType"),
        "kind": relic_kind(item_id, row) if entity_kind == "relic_set" else None,
        "version": version,
        "model": model,
        "generated_at": datetime.now(UTC).isoformat(),
        "axes": axes,
    }


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Enrich lightcone and relic-set data into structured equipment axes.")
    parser.add_argument("--version", default=os.getenv("HSR_DATA_VERSION", DEFAULT_VERSION))
    parser.add_argument("--data-dir", type=Path, default=Path(os.getenv("HSR_DATA_DIR", f"nanoka_hsr/{DEFAULT_VERSION}")))
    parser.add_argument("--out-dir", type=Path, default=Path(os.getenv("HSR_ENRICHED_DIR", f"enriched/{DEFAULT_VERSION}")))
    parser.add_argument("--kind", choices=["all", "lightcone", "relic_set"], default="all")
    parser.add_argument("--ids", nargs="*", help="Equipment ids, comma-separated or space-separated.")
    parser.add_argument("--all", action="store_true", help="Process every selected equipment kind.")
    parser.add_argument("--force", action="store_true", help="Overwrite existing enriched files.")
    parser.add_argument("--dry-run", action="store_true", help="Print outputs without writing files.")
    parser.add_argument("--mode", choices=["rules", "llm"], default="rules", help="Use local rules or OpenAI-compatible LLM extraction.")
    parser.add_argument("--workers", type=int, default=1, help="Concurrent LLM requests when --mode llm.")
    parser.add_argument("--no-db-infer", action="store_true", help="Do not infer lightcone weak axes from DB recommendations.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    data_dir = args.data_dir if args.data_dir.is_absolute() else ROOT / args.data_dir
    out_dir = args.out_dir if args.out_dir.is_absolute() else ROOT / args.out_dir
    requested_ids = parse_ids(args.ids)
    kinds = ["lightcone", "relic_set"] if args.kind == "all" else [args.kind]

    config = None
    model = "rules-v1"
    if args.mode == "llm" and not args.dry_run:
        config = require_llm_config()
        if config.api_format != "openai":
            raise RuntimeError("scripts/enrich_equipment.py --mode llm currently requires LLM_API_FORMAT=openai")
        model = config.model

    lightcone_inference = {} if args.mode == "llm" or args.no_db_infer else db_lightcone_inference()

    if args.mode == "llm" and not args.dry_run and args.workers > 1:
        assert config is not None
        tasks: list[tuple[str, int, dict[str, Any], Path]] = []
        for kind in kinds:
            filename = "lightcone.json" if kind == "lightcone" else "relicset.json"
            overview = load_json(data_dir / filename)
            samples = DEFAULT_LIGHTCONE_SAMPLE_IDS if kind == "lightcone" else DEFAULT_RELIC_SAMPLE_IDS
            item_ids = select_ids(overview, requested_ids, args.all, samples)
            for item_id in item_ids:
                row = overview.get(str(item_id))
                if row is None:
                    raise KeyError(f"{kind} {item_id} not found in {filename}")
                out_path = out_dir / kind / f"{item_id}.json"
                if out_path.exists() and not args.force:
                    print(f"skip {kind} {item_id}: {out_path.relative_to(ROOT)}")
                    continue
                tasks.append((kind, item_id, row, out_path))

        def run_task(task: tuple[str, int, dict[str, Any], Path]) -> str:
            kind, item_id, row, out_path = task
            payload_row, axes = enrich_equipment_openai(config, kind, item_id, row, data_dir)
            payload = output_payload(kind, item_id, payload_row, axes, args.version, model)
            write_json(out_path, payload)
            return f"wrote {out_path.relative_to(ROOT)}"

        completed = 0
        with ThreadPoolExecutor(max_workers=max(1, args.workers)) as executor:
            futures = [executor.submit(run_task, task) for task in tasks]
            for future in as_completed(futures):
                print(future.result())
                completed += 1
                print(f"progress {completed}/{len(tasks)}")
        print(f"equipment enriched: {completed}")
        return 0

    total = 0
    for kind in kinds:
        filename = "lightcone.json" if kind == "lightcone" else "relicset.json"
        overview = load_json(data_dir / filename)
        samples = DEFAULT_LIGHTCONE_SAMPLE_IDS if kind == "lightcone" else DEFAULT_RELIC_SAMPLE_IDS
        item_ids = select_ids(overview, requested_ids, args.all, samples)
        for item_id in item_ids:
            row = overview.get(str(item_id))
            if row is None:
                raise KeyError(f"{kind} {item_id} not found in {filename}")
            out_path = out_dir / kind / f"{item_id}.json"
            if out_path.exists() and not args.force and not args.dry_run:
                print(f"skip {kind} {item_id}: {out_path.relative_to(ROOT)}")
                continue
            payload_row = row
            if args.mode == "llm":
                if args.dry_run:
                    payload_row, prompt = equipment_prompt(kind, item_id, row, data_dir)
                    print(f"\n--- dry-run {kind} {item_id} ---")
                    print(prompt[:4000])
                    if len(prompt) > 4000:
                        print(f"\n... prompt truncated; full length={len(prompt)} chars")
                    total += 1
                    continue
                assert config is not None
                payload_row, axes = enrich_equipment_openai(config, kind, item_id, row, data_dir)
            else:
                axes = lightcone_axes(item_id, row, lightcone_inference) if kind == "lightcone" else relic_axes(item_id, row)
            payload = output_payload(kind, item_id, payload_row, axes, args.version, model)
            if args.dry_run:
                print(json.dumps(payload, ensure_ascii=False, indent=2))
            else:
                write_json(out_path, payload)
                print(f"wrote {out_path.relative_to(ROOT)}")
            total += 1
    print(f"equipment enriched: {total}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
