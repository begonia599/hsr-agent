from __future__ import annotations

import argparse
import json
import os
import re
import sys
from collections import Counter, defaultdict
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import psycopg
from dotenv import load_dotenv
from psycopg.rows import dict_row

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.db import DEFAULT_DATABASE_URL
from schemas.axes_vocab import STATS, TAGS
from schemas.equipment_axes_vocab import normalize_equipment_axes
from scripts.load import clean_text, load_json, relic_kind

DEFAULT_VERSION = "4.3.54"
DEFAULT_RELIC_SAMPLE_IDS = [101, 102, 118, 120, 301, 312]
DEFAULT_LIGHTCONE_SAMPLE_IDS = [21002, 23005, 24002]

PLACEHOLDER_RE = re.compile(r"#(\d+)(?:\[[^\]]+\])?")
VALUE_RE = re.compile(r"(-?\d+(?:\.\d+)?)(%)?")
DEF_IGNORE_RE = re.compile(r"无视(?:其|目标)?\s*(-?\d+(?:\.\d+)?)%的防御力")

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
) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "entity_kind": entity_kind,
        "id": item_id,
        "name_zh": clean_text(row.get("zh")),
        "name_en": clean_text(row.get("en")),
        "path": row.get("baseType"),
        "kind": relic_kind(item_id, row) if entity_kind == "relic_set" else None,
        "version": version,
        "model": "rules-v1",
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
    parser.add_argument("--no-db-infer", action="store_true", help="Do not infer lightcone weak axes from DB recommendations.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    data_dir = args.data_dir if args.data_dir.is_absolute() else ROOT / args.data_dir
    out_dir = args.out_dir if args.out_dir.is_absolute() else ROOT / args.out_dir
    requested_ids = parse_ids(args.ids)
    kinds = ["lightcone", "relic_set"] if args.kind == "all" else [args.kind]

    lightcone_inference = {} if args.no_db_infer else db_lightcone_inference()
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
            axes = lightcone_axes(item_id, row, lightcone_inference) if kind == "lightcone" else relic_axes(item_id, row)
            payload = output_payload(kind, item_id, row, axes, args.version)
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
