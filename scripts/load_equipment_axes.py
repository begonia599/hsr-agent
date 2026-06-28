from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Any

import psycopg
from dotenv import load_dotenv
from psycopg.types.json import Jsonb

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.db import DEFAULT_DATABASE_URL
from schemas.equipment_axes_vocab import normalize_equipment_axes

DEFAULT_VERSION = "4.3.54"


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


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


def axis_condition(item: dict[str, Any]) -> str | None:
    parts = []
    for key in ["condition", "reason"]:
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            parts.append(f"{key}: {value.strip()}")
    return "\n".join(parts) if parts else None


def axis_rows(entity_kind: str, entity_id: int, axes: dict[str, Any]) -> list[tuple[Any, ...]]:
    rows: list[tuple[Any, ...]] = []
    seen: set[tuple[Any, ...]] = set()

    def append(row: tuple[Any, ...]) -> None:
        fingerprint = (row[0], row[1], row[2], row[3], row[4], row[6], row[7] or "")
        if fingerprint in seen:
            return
        seen.add(fingerprint)
        rows.append(row)

    for kind in ["provides", "needs", "restricts"]:
        for item in axes.get(kind) or []:
            confidence = item.get("confidence")
            if not isinstance(confidence, (int, float)):
                confidence = 0.7 if kind == "provides" else 0.5
            append(
                (
                    entity_kind,
                    entity_id,
                    kind,
                    item["stat"],
                    item.get("target") or "",
                    item.get("value"),
                    item.get("uptime") or "",
                    axis_condition(item),
                    item.get("source") or "",
                    confidence,
                    False,
                )
            )
    for tag in axes.get("tags") or []:
        append((entity_kind, entity_id, "tag", tag, "", None, "", None, "", 0.6, False))
    return rows


def load_equipment_axes(cur: psycopg.Cursor, path: Path) -> int:
    payload = load_json(path)
    entity_kind = payload["entity_kind"]
    entity_id = int(payload["id"])
    axes = normalize_equipment_axes(payload.get("axes") or payload)
    table = {"lightcone": "lightcones", "relic_set": "relic_sets"}[entity_kind]

    cur.execute(f"SELECT 1 FROM {table} WHERE id = %s", (entity_id,))
    if cur.fetchone() is None:
        raise RuntimeError(f"{entity_kind} {entity_id} from {path} is not loaded in database")

    cur.execute(f"UPDATE {table} SET axes = %s WHERE id = %s", (Jsonb(axes), entity_id))
    cur.execute("DELETE FROM equipment_axes WHERE entity_kind = %s AND entity_id = %s", (entity_kind, entity_id))
    rows = axis_rows(entity_kind, entity_id, axes)
    if rows:
        cur.executemany(
            """
            INSERT INTO equipment_axes
                (entity_kind, entity_id, kind, stat, target, value, uptime, condition, source, confidence, reviewed)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
            """,
            rows,
        )
    return len(rows)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Load enriched equipment axes JSON into PostgreSQL.")
    parser.add_argument("--version", default=os.getenv("HSR_DATA_VERSION", DEFAULT_VERSION))
    parser.add_argument("--enriched-dir", type=Path, default=Path(os.getenv("HSR_ENRICHED_DIR", f"enriched/{DEFAULT_VERSION}")))
    parser.add_argument("--kind", choices=["all", "lightcone", "relic_set"], default="all")
    parser.add_argument("--ids", nargs="*", help="Equipment ids, comma-separated or space-separated.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    enriched_dir = args.enriched_dir if args.enriched_dir.is_absolute() else ROOT / args.enriched_dir
    requested_ids = parse_ids(args.ids)
    kinds = ["lightcone", "relic_set"] if args.kind == "all" else [args.kind]

    paths: list[Path] = []
    for kind in kinds:
        kind_dir = enriched_dir / kind
        if not kind_dir.exists():
            continue
        paths.extend(sorted(kind_dir.glob("*.json"), key=lambda p: int(p.stem)))
    if requested_ids is not None:
        paths = [path for path in paths if int(path.stem) in requested_ids]

    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    loaded = 0
    rows = 0
    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            for path in paths:
                rows += load_equipment_axes(cur, path)
                loaded += 1
                print(f"loaded {path.relative_to(ROOT)}")

    print(f"equipment_with_axes: {loaded}")
    print(f"equipment_axes rows: {rows}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
