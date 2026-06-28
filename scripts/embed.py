from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import sys
from collections.abc import Iterable
from pathlib import Path
from typing import Any

import psycopg
from dotenv import load_dotenv
from psycopg.rows import dict_row

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.db import DEFAULT_DATABASE_URL
from scripts.load import clean_text

DIMENSIONS = 1024
MODEL_NAME = "local-hash-ngram-v1"

ASCII_TOKEN_RE = re.compile(r"[a-z0-9_]+", re.IGNORECASE)
CJK_RE = re.compile(r"[\u3400-\u9fff]+")

ALIASES: dict[str, tuple[str, ...]] = {
    "crit_rate": ("crit_rate", "暴击率", "双暴"),
    "crit_dmg": ("crit_dmg", "暴击伤害", "爆伤", "暴伤", "双暴"),
    "atk_percent": ("atk_percent", "攻击力", "攻击百分比", "atk"),
    "dmg_percent": ("dmg_percent", "增伤", "伤害提高", "造成的伤害提高"),
    "speed": ("speed", "速度", "spd"),
    "turn_advance": ("turn_advance", "行动提前", "拉条", "提前行动"),
    "energy_regen": ("energy_regen", "能量恢复效率", "充能", "回能", "能量"),
    "energy_restore": ("energy_restore", "恢复能量", "回能", "能量"),
    "sp_generation": ("sp_generation", "战技点上限", "产点", "sp"),
    "sp_recovery": ("sp_recovery", "恢复战技点", "回点", "sp"),
    "sp_consumption": ("sp_consumption", "消耗战技点", "耗点", "sp"),
    "fua": ("fua", "追加攻击", "追击", "fua_team", "fua_dmg"),
    "break": ("break", "击破", "超击破", "削韧", "break_specialist"),
    "dot": ("dot", "持续伤害", "dot_enabler", "dot_detonator"),
    "debuff": ("debuff", "负面效果", "减防", "易伤", "debuffer"),
    "sustain": ("sustain", "治疗", "护盾", "生存", "sustain_healer", "sustain_shielder"),
    "main_dps": ("main_dps", "主c", "主 C", "输出位"),
    "sub_dps": ("sub_dps", "副c", "副 C", "副输出"),
    "amplifier": ("amplifier", "同谐", "辅助", "拐"),
}


def parse_ids(values: list[str] | None) -> set[str] | None:
    if not values:
        return None
    out: set[str] = set()
    for value in values:
        for item in value.split(","):
            item = item.strip()
            if item:
                out.add(item)
    return out


def normalize_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, (dict, list)):
        value = json.dumps(value, ensure_ascii=False, sort_keys=True)
    return clean_text(value).lower()


def add_feature(vec: list[float], feature: str, weight: float) -> None:
    if not feature:
        return
    digest = hashlib.sha256(feature.encode("utf-8")).digest()
    bucket = int.from_bytes(digest[:4], "big") % DIMENSIONS
    sign = 1.0 if digest[4] & 1 else -1.0
    vec[bucket] += sign * weight


def text_features(text: str) -> Iterable[tuple[str, float]]:
    for token in ASCII_TOKEN_RE.findall(text):
        yield f"tok:{token}", 2.0
        for part in token.split("_"):
            if part:
                yield f"tok:{part}", 1.0

    for segment in CJK_RE.findall(text):
        chars = list(segment)
        for char in chars:
            yield f"c1:{char}", 0.15
        for n, weight in [(2, 0.8), (3, 1.0), (4, 0.6)]:
            if len(chars) < n:
                continue
            for index in range(len(chars) - n + 1):
                yield f"c{n}:{''.join(chars[index:index + n])}", weight


def alias_features(text: str) -> Iterable[tuple[str, float]]:
    compact = re.sub(r"\s+", "", text)
    for canonical, aliases in ALIASES.items():
        if any(alias.lower().replace(" ", "") in compact for alias in aliases):
            yield f"axis:{canonical}", 6.0
            for alias in aliases:
                yield f"alias:{alias.lower().replace(' ', '')}", 2.0


def embed_text(text: str) -> list[float]:
    normalized = normalize_text(text)
    vec = [0.0] * DIMENSIONS
    for feature, weight in text_features(normalized):
        add_feature(vec, feature, weight)
    for feature, weight in alias_features(normalized):
        add_feature(vec, feature, weight)

    norm = math.sqrt(sum(value * value for value in vec))
    if norm == 0:
        return vec
    return [value / norm for value in vec]


def vector_literal(vec: list[float]) -> str:
    return "[" + ",".join(f"{value:.8f}" for value in vec) + "]"


def row_text(kind: str, row: dict[str, Any]) -> str:
    if kind == "character":
        return "\n".join(
            [
                str(row["id"]),
                normalize_text(row["name_zh"]),
                normalize_text(row["name_en"]),
                normalize_text(row["path"]),
                normalize_text(row["element"]),
                normalize_text(row["roles"]),
                normalize_text(row["axes"]),
                normalize_text(row["skill_text_zh"]),
            ]
        )
    if kind == "lightcone":
        return "\n".join(
            [
                str(row["id"]),
                normalize_text(row["name_zh"]),
                normalize_text(row["name_en"]),
                normalize_text(row["path"]),
                normalize_text(row["desc_zh"]),
                normalize_text(row["axes"]),
            ]
        )
    if kind == "relic_set":
        return "\n".join(
            [
                str(row["id"]),
                normalize_text(row["name_zh"]),
                normalize_text(row["name_en"]),
                normalize_text(row["kind"]),
                normalize_text(row["set2_desc"]),
                normalize_text(row["set4_desc"]),
                normalize_text(row["axes"]),
            ]
        )
    raise ValueError(f"unknown kind {kind!r}")


def select_rows(cur: psycopg.Cursor[dict[str, Any]], kind: str, ids: set[str] | None, force: bool, limit: int) -> list[dict[str, Any]]:
    table_sql = {
        "character": """
            SELECT id, name_zh, name_en, path, element, roles, axes, skill_text_zh
            FROM characters
            WHERE (%(force)s OR embedding IS NULL)
              AND (%(ids)s::text[] IS NULL OR id::text = ANY(%(ids)s::text[]))
            ORDER BY id
            LIMIT %(limit)s
        """,
        "lightcone": """
            SELECT id, name_zh, name_en, path, desc_zh, axes
            FROM lightcones
            WHERE (%(force)s OR embedding IS NULL)
              AND (%(ids)s::text[] IS NULL OR id::text = ANY(%(ids)s::text[]))
            ORDER BY id
            LIMIT %(limit)s
        """,
        "relic_set": """
            SELECT id, name_zh, name_en, kind, set2_desc, set4_desc, axes
            FROM relic_sets
            WHERE (%(force)s OR embedding IS NULL)
              AND (%(ids)s::text[] IS NULL OR id::text = ANY(%(ids)s::text[]))
            ORDER BY id
            LIMIT %(limit)s
        """,
    }
    cur.execute(table_sql[kind], {"force": force, "ids": list(ids) if ids else None, "limit": limit})
    return list(cur.fetchall())


def update_embedding(cur: psycopg.Cursor[dict[str, Any]], kind: str, row_id: int, vec: list[float]) -> None:
    table = {
        "character": "characters",
        "lightcone": "lightcones",
        "relic_set": "relic_sets",
    }[kind]
    cur.execute(
        f"UPDATE {table} SET embedding = %s::vector WHERE id = %s",
        (vector_literal(vec), row_id),
    )


def process_kind(
    cur: psycopg.Cursor[dict[str, Any]],
    kind: str,
    ids: set[str] | None,
    force: bool,
    limit: int,
    dry_run: bool,
) -> int:
    rows = select_rows(cur, kind, ids, force, limit)
    if dry_run:
        print(f"{kind}: would embed {len(rows)} rows with {MODEL_NAME}/{DIMENSIONS}")
        if rows:
            preview = row_text(kind, rows[0])[:800].replace("\n", " ")
            print(f"{kind} preview id={rows[0]['id']}: {preview}")
        return len(rows)

    for row in rows:
        update_embedding(cur, kind, int(row["id"]), embed_text(row_text(kind, row)))
    print(f"{kind}: embedded {len(rows)} rows")
    return len(rows)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate local pgvector embeddings for loaded HSR data.")
    parser.add_argument("--kind", choices=["all", "character", "lightcone", "relic_set"], default="all")
    parser.add_argument("--ids", nargs="*", help="Optional ids, comma-separated or space-separated.")
    parser.add_argument("--force", action="store_true", help="Recompute rows that already have embeddings.")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be embedded without writing.")
    parser.add_argument("--limit", type=int, default=1_000_000, help="Maximum rows per kind.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    ids = parse_ids(args.ids)
    kinds = ["character", "lightcone", "relic_set"] if args.kind == "all" else [args.kind]
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)

    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            total = 0
            for kind in kinds:
                total += process_kind(cur, kind, ids, args.force, args.limit, args.dry_run)
            if args.dry_run:
                conn.rollback()
            else:
                conn.commit()

    print(f"total: {total}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
