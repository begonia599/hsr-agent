from __future__ import annotations

import argparse
import json
import os
import re
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from dotenv import load_dotenv

try:
    import psycopg
    from psycopg.types.json import Jsonb
except ModuleNotFoundError:
    psycopg = None
    Jsonb = None

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"
DEFAULT_VERSION = "4.3.54"

COLOR_TAG_RE = re.compile(r"</?color(?:=[^>]*)?>", re.IGNORECASE)
UNBREAK_RE = re.compile(r"<unbreak>(.*?)</unbreak>", re.IGNORECASE | re.DOTALL)
RUBY_B_RE = re.compile(r"\{RUBY_B#([^}]*)\}")
RUBY_E_RE = re.compile(r"\{RUBY_E#\}")
HTML_TAG_RE = re.compile(r"<[^>]+>")


def require_psycopg() -> None:
    if psycopg is None or Jsonb is None:
        raise RuntimeError("psycopg is required for scripts/load.py. Install dependencies with `python -m pip install -e .`.")


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def clean_text(value: Any) -> str:
    if value is None:
        return ""
    text = str(value)
    text = text.replace("\\n", "\n")
    text = UNBREAK_RE.sub(r"\1", text)
    text = COLOR_TAG_RE.sub("", text)
    text = RUBY_B_RE.sub(r"\1", text)
    text = RUBY_E_RE.sub("", text)
    text = HTML_TAG_RE.sub("", text)
    return text.strip()


def rarity_from_enum(value: str | None) -> int:
    if not value:
        raise ValueError("missing rarity enum")
    match = re.search(r"(\d+)$", value)
    if not match:
        raise ValueError(f"cannot parse rarity from {value!r}")
    return int(match.group(1))


def release_to_datetime(value: Any) -> datetime | None:
    if value is None:
        return None
    return datetime.fromtimestamp(int(value), UTC)


def int_id(value: Any) -> int:
    return int(str(value))


def figure_stem(path_value: str | None) -> str | None:
    if not path_value:
        return None
    return Path(path_value).stem


def trailblazer_name(lang: str) -> str:
    if lang == "en":
        return "Trailblazer"
    return "开拓者"


def normalize_name(value: Any, lang: str = "zh") -> str:
    text = clean_text(value)
    if text == "{NICKNAME}":
        return trailblazer_name(lang)
    return text


def max_level_params(skill: dict[str, Any]) -> list[Any]:
    levels = skill.get("level") or {}
    if not isinstance(levels, dict) or not levels:
        return []
    numeric_keys = sorted((int(k) for k in levels if str(k).isdigit()), reverse=True)
    if not numeric_keys:
        return []
    row = levels.get(str(numeric_keys[0])) or {}
    return row.get("param_list") or []


def append_named_desc(lines: list[str], prefix: str, obj: dict[str, Any]) -> None:
    name = clean_text(obj.get("name") or obj.get("point_name"))
    desc = clean_text(obj.get("desc") or obj.get("point_desc"))
    params = obj.get("param_list") or obj.get("ParamList") or []
    if name or desc:
        block = [f"{prefix}: {name}".strip(), desc]
        if params:
            block.append(f"参数: {json.dumps(params, ensure_ascii=False)}")
        lines.append("\n".join(part for part in block if part))


def build_skill_text(detail: dict[str, Any]) -> str:
    lines: list[str] = []
    desc = clean_text(detail.get("desc"))
    if desc:
        lines.append(f"角色简介:\n{desc}")

    for skill in (detail.get("skills") or {}).values():
        if not isinstance(skill, dict):
            continue
        type_name = clean_text(skill.get("type_name") or skill.get("type"))
        name = clean_text(skill.get("name"))
        skill_desc = clean_text(skill.get("desc"))
        simple_desc = clean_text(skill.get("simple_desc"))
        params = max_level_params(skill)
        block = [f"技能[{type_name}]: {name}".strip(), skill_desc]
        if simple_desc:
            block.append(f"简述: {simple_desc}")
        if params:
            block.append(f"满级参数: {json.dumps(params, ensure_ascii=False)}")
        lines.append("\n".join(part for part in block if part))

    for rank_no, rank in sorted((detail.get("ranks") or {}).items(), key=lambda kv: int(kv[0])):
        if isinstance(rank, dict):
            append_named_desc(lines, f"星魂{rank_no}", rank)

    for point_key, levels in sorted((detail.get("skill_trees") or {}).items()):
        if not isinstance(levels, dict):
            continue
        for level_key, point in sorted(levels.items(), key=lambda kv: int(kv[0])):
            if not isinstance(point, dict):
                continue
            name = clean_text(point.get("point_name"))
            desc = clean_text(point.get("point_desc"))
            params = point.get("param_list") or []
            if name or desc:
                block = [f"行迹[{point_key}.{level_key}]: {name}".strip(), desc]
                if params:
                    block.append(f"参数: {json.dumps(params, ensure_ascii=False)}")
                lines.append("\n".join(part for part in block if part))

    return "\n\n".join(lines)


def relic_kind(relic_id: int, set_data: dict[str, Any]) -> str:
    set_bonus = set_data.get("set") or {}
    if "4" in set_bonus:
        return "cavern"
    if "2" in set_bonus and relic_id >= 300:
        return "planar"
    return "cavern" if relic_id < 300 else "planar"


def relic_desc(set_data: dict[str, Any], bonus: str, lang: str) -> str | None:
    row = (set_data.get("set") or {}).get(bonus)
    if not isinstance(row, dict):
        return None
    return clean_text(row.get(lang))


def insert_recommendations(cur: psycopg.Cursor, char_id: int, detail: dict[str, Any]) -> None:
    for rank, lightcone_id in enumerate(detail.get("lightcones") or []):
        cur.execute(
            """
            INSERT INTO character_recommendations
                (char_id, recommend_kind, item_id, rank, payload)
            VALUES (%s, 'lightcone', %s, %s, %s)
            """,
            (char_id, int(lightcone_id), rank, Jsonb({"source": "detail.lightcones"})),
        )

    relics = detail.get("relics") or {}
    for kind, source_key in [("relic_set4", "set4_id_list"), ("relic_set2", "set2_id_list")]:
        for rank, relic_id in enumerate(relics.get(source_key) or []):
            cur.execute(
                """
                INSERT INTO character_recommendations
                    (char_id, recommend_kind, item_id, rank, payload)
                VALUES (%s, %s, %s, %s, %s)
                """,
                (
                    char_id,
                    kind,
                    int(relic_id),
                    rank,
                    Jsonb({"source": f"detail.relics.{source_key}"}),
                ),
            )

    for key in [
        "property_list3",
        "property_list4",
        "property_list5",
        "property_list6",
        "property_list",
        "sub_affix_property_list",
        "score_rank_list",
    ]:
        payload = relics.get(key)
        if payload:
            cur.execute(
                """
                INSERT INTO character_recommendations
                    (char_id, recommend_kind, item_id, rank, payload)
                VALUES (%s, %s, NULL, 0, %s)
                """,
                (char_id, key, Jsonb(payload)),
            )


def load_characters(cur: psycopg.Cursor, data_dir: Path, version: str) -> int:
    overview = load_json(data_dir / "character.json")
    zh_dir = data_dir / "zh" / "character"
    en_dir = data_dir / "en" / "character"
    loaded = 0

    cur.execute("DELETE FROM character_recommendations")

    for char_id_text, meta in sorted(overview.items(), key=lambda kv: int(kv[0])):
        char_id = int_id(char_id_text)
        zh = load_json(zh_dir / f"{char_id}.json")
        en_path = en_dir / f"{char_id}.json"
        en = load_json(en_path) if en_path.exists() else {}

        raw_en = en or {}
        cur.execute(
            """
            INSERT INTO characters (
                id, version, release_at, icon_name, rarity, path, element,
                name_zh, name_en, name_ko, name_ja, desc_zh, desc_en, sp_need,
                roles, raw_zh, raw_en, axes, skill_text_zh, skill_text_en,
                is_trailblazer, is_collab, is_variant
            )
            VALUES (
                %(id)s, %(version)s, %(release_at)s, %(icon_name)s, %(rarity)s,
                %(path)s, %(element)s, %(name_zh)s, %(name_en)s, %(name_ko)s,
                %(name_ja)s, %(desc_zh)s, %(desc_en)s, %(sp_need)s, '{}',
                %(raw_zh)s, %(raw_en)s, '{}', %(skill_text_zh)s, %(skill_text_en)s,
                %(is_trailblazer)s, %(is_collab)s, %(is_variant)s
            )
            ON CONFLICT (id) DO UPDATE SET
                version = EXCLUDED.version,
                release_at = EXCLUDED.release_at,
                icon_name = EXCLUDED.icon_name,
                rarity = EXCLUDED.rarity,
                path = EXCLUDED.path,
                element = EXCLUDED.element,
                name_zh = EXCLUDED.name_zh,
                name_en = EXCLUDED.name_en,
                name_ko = EXCLUDED.name_ko,
                name_ja = EXCLUDED.name_ja,
                desc_zh = EXCLUDED.desc_zh,
                desc_en = EXCLUDED.desc_en,
                sp_need = EXCLUDED.sp_need,
                raw_zh = EXCLUDED.raw_zh,
                raw_en = EXCLUDED.raw_en,
                skill_text_zh = EXCLUDED.skill_text_zh,
                skill_text_en = EXCLUDED.skill_text_en,
                is_trailblazer = EXCLUDED.is_trailblazer,
                is_collab = EXCLUDED.is_collab,
                is_variant = EXCLUDED.is_variant
            """,
            {
                "id": char_id,
                "version": version,
                "release_at": release_to_datetime(meta.get("release")),
                "icon_name": zh.get("avatar_vo_tag") or meta.get("icon"),
                "rarity": rarity_from_enum(zh.get("rarity") or meta.get("rank")),
                "path": zh.get("base_type") or meta.get("baseType"),
                "element": zh.get("damage_type") or meta.get("damageType"),
                "name_zh": normalize_name(zh.get("name") or meta.get("zh"), "zh"),
                "name_en": normalize_name(en.get("name") or meta.get("en") or "", "en"),
                "name_ko": normalize_name(meta.get("ko"), "ko"),
                "name_ja": normalize_name(meta.get("ja"), "ja"),
                "desc_zh": clean_text(zh.get("desc")),
                "desc_en": clean_text(en.get("desc")),
                "sp_need": zh.get("sp_need"),
                "raw_zh": Jsonb(zh),
                "raw_en": Jsonb(raw_en),
                "skill_text_zh": build_skill_text(zh),
                "skill_text_en": build_skill_text(en) if en else "",
                "is_trailblazer": 8001 <= char_id <= 8010,
                "is_collab": char_id in {1014, 1015},
                "is_variant": 1506 <= char_id <= 1510,
            },
        )
        insert_recommendations(cur, char_id, zh)
        loaded += 1

    return loaded


def load_lightcones(cur: psycopg.Cursor, data_dir: Path, version: str) -> int:
    lightcones = load_json(data_dir / "lightcone.json")
    loaded = 0
    for lc_id_text, row in sorted(lightcones.items(), key=lambda kv: int(kv[0])):
        lc_id = int_id(lc_id_text)
        cur.execute(
            """
            INSERT INTO lightcones (
                id, version, rarity, path, name_zh, name_en, desc_zh, desc_en,
                raw_zh, raw_en, axes
            )
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, '{}')
            ON CONFLICT (id) DO UPDATE SET
                version = EXCLUDED.version,
                rarity = EXCLUDED.rarity,
                path = EXCLUDED.path,
                name_zh = EXCLUDED.name_zh,
                name_en = EXCLUDED.name_en,
                desc_zh = EXCLUDED.desc_zh,
                desc_en = EXCLUDED.desc_en,
                raw_zh = EXCLUDED.raw_zh,
                raw_en = EXCLUDED.raw_en
            """,
            (
                lc_id,
                version,
                rarity_from_enum(row.get("rank")),
                row.get("baseType") or "",
                clean_text(row.get("zh")),
                clean_text(row.get("en")),
                clean_text(row.get("zh_desc") or row.get("desc")),
                clean_text(row.get("en_desc") or row.get("desc")),
                Jsonb(row),
                Jsonb(row),
            ),
        )
        loaded += 1
    return loaded


def load_relic_sets(cur: psycopg.Cursor, data_dir: Path, version: str) -> int:
    relic_sets = load_json(data_dir / "relicset.json")
    loaded = 0
    for relic_id_text, row in sorted(relic_sets.items(), key=lambda kv: int(kv[0])):
        relic_id = int_id(relic_id_text)
        cur.execute(
            """
            INSERT INTO relic_sets (
                id, version, kind, name_zh, name_en, set2_desc, set4_desc,
                raw_zh, raw_en, axes
            )
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, '{}')
            ON CONFLICT (id) DO UPDATE SET
                version = EXCLUDED.version,
                kind = EXCLUDED.kind,
                name_zh = EXCLUDED.name_zh,
                name_en = EXCLUDED.name_en,
                set2_desc = EXCLUDED.set2_desc,
                set4_desc = EXCLUDED.set4_desc,
                raw_zh = EXCLUDED.raw_zh,
                raw_en = EXCLUDED.raw_en
            """,
            (
                relic_id,
                version,
                relic_kind(relic_id, row),
                clean_text(row.get("zh")),
                clean_text(row.get("en")),
                relic_desc(row, "2", "zh"),
                relic_desc(row, "4", "zh"),
                Jsonb(row),
                Jsonb(row),
            ),
        )
        loaded += 1
    return loaded


def load_items(cur: psycopg.Cursor, data_dir: Path) -> int:
    zh_items = load_json(data_dir / "zh" / "item.json")
    en_items_path = data_dir / "en" / "item.json"
    en_items = load_json(en_items_path) if en_items_path.exists() else {}
    loaded = 0
    for item_id_text, zh in sorted(zh_items.items(), key=lambda kv: int(kv[0])):
        item_id = int_id(item_id_text)
        en = en_items.get(item_id_text) or {}
        cur.execute(
            """
            INSERT INTO items (
                id, item_sub_type, purpose_type, rarity, name_zh, name_en,
                figure_stem, raw_zh, raw_en
            )
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (id) DO UPDATE SET
                item_sub_type = EXCLUDED.item_sub_type,
                purpose_type = EXCLUDED.purpose_type,
                rarity = EXCLUDED.rarity,
                name_zh = EXCLUDED.name_zh,
                name_en = EXCLUDED.name_en,
                figure_stem = EXCLUDED.figure_stem,
                raw_zh = EXCLUDED.raw_zh,
                raw_en = EXCLUDED.raw_en
            """,
            (
                item_id,
                zh.get("item_sub_type"),
                zh.get("purpose_type"),
                zh.get("rarity"),
                clean_text(zh.get("item_name")),
                clean_text(en.get("item_name") or zh.get("item_name")),
                figure_stem(zh.get("item_figure_icon_path")),
                Jsonb(zh),
                Jsonb(en),
            ),
        )
        loaded += 1
    return loaded


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Load nanoka HSR data into PostgreSQL.")
    parser.add_argument("--version", default=os.getenv("HSR_DATA_VERSION", DEFAULT_VERSION))
    parser.add_argument(
        "--data-dir",
        type=Path,
        default=Path(os.getenv("HSR_DATA_DIR", f"nanoka_hsr/{DEFAULT_VERSION}")),
    )
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    data_dir = args.data_dir
    if not data_dir.is_absolute():
        data_dir = ROOT / data_dir
    if not data_dir.exists():
        raise FileNotFoundError(f"data dir not found: {data_dir}")

    require_psycopg()
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            counts = {
                "characters": load_characters(cur, data_dir, args.version),
                "lightcones": load_lightcones(cur, data_dir, args.version),
                "relic_sets": load_relic_sets(cur, data_dir, args.version),
                "items": load_items(cur, data_dir),
            }

    for name, count in counts.items():
        print(f"{name}: {count}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
