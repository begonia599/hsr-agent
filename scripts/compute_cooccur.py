from __future__ import annotations

import argparse
import json
import os
from collections import defaultdict
from itertools import combinations
from pathlib import Path

import psycopg
from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"
DEFAULT_VERSION = "4.3.54"


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def unique_ints(values) -> list[int]:
    seen: set[int] = set()
    out: list[int] = []
    for value in values or []:
        try:
            item = int(value)
        except (TypeError, ValueError):
            continue
        if item not in seen:
            seen.add(item)
            out.append(item)
    return out


def add_pair(
    weights: dict[tuple[int, int], int],
    main_flags: dict[tuple[int, int], bool],
    a: int,
    b: int,
    weight: int,
    is_main_lineup: bool,
) -> None:
    if a == b:
        return
    for key in [(a, b), (b, a)]:
        weights[key] += weight
        main_flags[key] = main_flags[key] or is_main_lineup


def build_cooccur(data_dir: Path, valid_ids: set[int]) -> tuple[dict[tuple[int, int], int], dict[tuple[int, int], bool]]:
    weights: dict[tuple[int, int], int] = defaultdict(int)
    main_flags: dict[tuple[int, int], bool] = defaultdict(bool)

    for path in sorted((data_dir / "zh" / "character").glob("*.json"), key=lambda p: int(p.stem)):
        detail = load_json(path)
        core_id = int(path.stem)
        if core_id not in valid_ids:
            continue

        for team in detail.get("teams") or []:
            if not isinstance(team, dict):
                continue
            avatar_id = int(team.get("avatar_id") or core_id)
            if avatar_id not in valid_ids:
                avatar_id = core_id

            base_weight = 2 if int(team.get("position") or 0) == 1 else 1
            lineup = [avatar_id] + unique_ints(team.get("member_list"))
            lineup = [char_id for char_id in lineup if char_id in valid_ids]
            for a, b in combinations(lineup, 2):
                add_pair(weights, main_flags, a, b, base_weight, True)

            for backup_key in ["backup_list1", "backup_list2", "backup_list3"]:
                for backup_id in unique_ints(team.get(backup_key)):
                    if backup_id in valid_ids:
                        add_pair(weights, main_flags, avatar_id, backup_id, 1, False)

    return weights, main_flags


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compute team co-occurrence from nanoka teams.")
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

    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT id FROM characters")
            valid_ids = {row[0] for row in cur.fetchall()}

            weights, main_flags = build_cooccur(data_dir, valid_ids)
            cur.execute("DELETE FROM team_cooccur")
            cur.executemany(
                """
                INSERT INTO team_cooccur (char_a, char_b, weight, is_main_lineup)
                VALUES (%s, %s, %s, %s)
                """,
                [
                    (a, b, weight, main_flags[(a, b)])
                    for (a, b), weight in sorted(weights.items())
                    if a in valid_ids and b in valid_ids
                ],
            )

    print(f"team_cooccur: {len(weights)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

