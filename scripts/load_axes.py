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

from schemas.axes_vocab import normalize_axes

DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"
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
    for key in ["condition", "reason", "source"]:
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            parts.append(f"{key}: {value.strip()}")
    return "\n".join(parts) if parts else None


def axis_rows(char_id: int, axes: dict[str, Any]) -> list[tuple[int, str, str, str, Any, str, str | None]]:
    rows: list[tuple[int, str, str, str, Any, str, str | None]] = []
    for kind in ["provides", "needs", "restricts"]:
        for item in axes.get(kind) or []:
            rows.append(
                (
                    char_id,
                    kind,
                    item["stat"],
                    item.get("target") or "",
                    item.get("value"),
                    item.get("uptime") or "",
                    axis_condition(item),
                )
            )

    for tag in axes.get("tags") or []:
        rows.append((char_id, "tag", tag, "", None, "", None))

    return rows


def load_character_axes(cur: psycopg.Cursor, path: Path) -> int:
    payload = load_json(path)
    char_id = int(payload["id"])
    axes = normalize_axes(payload.get("axes") or payload)
    roles = axes.get("roles") or []

    cur.execute("SELECT 1 FROM characters WHERE id = %s", (char_id,))
    if cur.fetchone() is None:
        raise RuntimeError(f"character {char_id} from {path} is not loaded in database")

    cur.execute(
        """
        UPDATE characters
        SET axes = %s, roles = %s
        WHERE id = %s
        """,
        (Jsonb(axes), roles, char_id),
    )
    cur.execute("DELETE FROM character_axes WHERE char_id = %s", (char_id,))
    rows = axis_rows(char_id, axes)
    if rows:
        cur.executemany(
            """
            INSERT INTO character_axes
                (char_id, kind, stat, target, value, uptime, condition)
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (char_id, kind, stat, target, uptime) DO UPDATE SET
                value = EXCLUDED.value,
                condition = EXCLUDED.condition
            """,
            rows,
        )
    return len(rows)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Load enriched axes JSON into PostgreSQL.")
    parser.add_argument("--version", default=os.getenv("HSR_DATA_VERSION", DEFAULT_VERSION))
    parser.add_argument(
        "--enriched-dir",
        type=Path,
        default=Path(os.getenv("HSR_ENRICHED_DIR", f"enriched/{DEFAULT_VERSION}")),
    )
    parser.add_argument("--ids", nargs="*", help="Character ids, comma-separated or space-separated.")
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    enriched_dir = args.enriched_dir if args.enriched_dir.is_absolute() else ROOT / args.enriched_dir
    character_dir = enriched_dir / "character"
    requested_ids = parse_ids(args.ids)

    if not character_dir.exists():
        print(f"no enriched character dir: {character_dir}")
        return 0

    paths = sorted(character_dir.glob("*.json"), key=lambda p: int(p.stem))
    if requested_ids is not None:
        paths = [path for path in paths if int(path.stem) in requested_ids]

    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    loaded = 0
    rows = 0
    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            for path in paths:
                rows += load_character_axes(cur, path)
                loaded += 1
                print(f"loaded {path.relative_to(ROOT)}")

    print(f"characters_with_axes: {loaded}")
    print(f"character_axes rows: {rows}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
