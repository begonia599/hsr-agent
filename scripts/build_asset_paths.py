from __future__ import annotations

import argparse
import json
import os
from pathlib import Path

import psycopg
from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"
DEFAULT_VERSION = "4.3.54"
STATIC_ASSET_ROOT = "https://static.nanoka.cc/assets/hsr"


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def rel_path(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def static_url(asset_root: Path, path: Path) -> str:
    return f"{STATIC_ASSET_ROOT}/{path.relative_to(asset_root).as_posix()}"


def add_file(rows: list[tuple[str, str, str, str, str, int]], entity_kind: str, entity_id: str, variant: str, path: Path, url: str) -> None:
    if path.exists():
        rows.append((entity_kind, entity_id, variant, rel_path(path), url, path.stat().st_size))


def add_static(rows: list[tuple[str, str, str, str, str, int]], asset_root: Path, entity_kind: str, entity_id: str, variant: str, path: Path) -> None:
    add_file(rows, entity_kind, entity_id, variant, path, static_url(asset_root, path))


def build_rows(data_dir: Path) -> list[tuple[str, str, str, str, str, int]]:
    asset_root = data_dir / "assets" / "hsr"
    rows: list[tuple[str, str, str, str, str, int]] = []

    characters = load_json(data_dir / "character.json")
    lightcones = load_json(data_dir / "lightcone.json")
    relic_sets = load_json(data_dir / "relicset.json")
    zh_items = load_json(data_dir / "zh" / "item.json")

    for char_id in sorted(characters, key=int):
        add_static(rows, asset_root, "character", char_id, "round", asset_root / "avatarroundicon" / f"{char_id}.webp")
        add_static(rows, asset_root, "character", char_id, "shop", asset_root / "avatarshopicon" / f"{char_id}.webp")
        add_static(rows, asset_root, "character", char_id, "avatar", asset_root / "avataricon" / "avatar" / f"{char_id}.webp")
        add_static(rows, asset_root, "character", char_id, "drawcard", asset_root / "avatardrawcard" / f"{char_id}.webp")

        og_path = asset_root / "og" / f"{char_id}.png"
        add_file(rows, "character", char_id, "og", og_path, f"https://hsr.nanoka.cc/character/{char_id}/og.png")

        for rank in range(1, 7):
            rank_path = asset_root / "rank" / "_dependencies" / "textures" / char_id / f"{char_id}_Rank_{rank}.webp"
            add_static(rows, asset_root, "character", char_id, f"rank_{rank}", rank_path)

    for lc_id in sorted(lightcones, key=int):
        add_static(rows, asset_root, "lightcone", lc_id, "medium", asset_root / "lightconemediumicon" / f"{lc_id}.webp")
        add_static(rows, asset_root, "lightcone", lc_id, "maxfigure", asset_root / "lightconemaxfigures" / f"{lc_id}.webp")

    for relic_id in sorted(relic_sets, key=int):
        add_static(rows, asset_root, "relic_set", relic_id, "figure", asset_root / "itemfigures" / f"{relic_id}.webp")

    for item_id, item in sorted(zh_items.items(), key=lambda kv: int(kv[0])):
        icon_path = item.get("item_figure_icon_path") or ""
        stem = Path(icon_path).stem
        if stem:
            add_static(rows, asset_root, "item", item_id, "figure", asset_root / "itemfigures" / f"{stem}.webp")

    for path in sorted((asset_root / "pathicon").glob("*.webp")):
        add_static(rows, asset_root, "path", path.stem, "icon", path)

    for path in sorted((asset_root / "element").glob("*.webp")):
        add_static(rows, asset_root, "element", path.stem, "icon", path)

    for path in sorted((asset_root / "relicfigures").glob("*.webp")):
        slot = path.stem.removeprefix("IconRelic")
        add_static(rows, asset_root, "slot", slot, "icon", path)

    for path in sorted((asset_root / "skillicons").glob("*.webp")):
        add_static(rows, asset_root, "skill_icon", path.stem, "icon", path)

    seen: set[tuple[str, str, str]] = set()
    unique_rows: list[tuple[str, str, str, str, str, int]] = []
    for row in rows:
        key = row[0], row[1], row[2]
        if key not in seen:
            seen.add(key)
            unique_rows.append(row)
    return unique_rows


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build asset_paths from local nanoka assets.")
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

    rows = build_rows(data_dir)
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            cur.execute("DELETE FROM asset_paths")
            cur.executemany(
                """
                INSERT INTO asset_paths
                    (entity_kind, entity_id, variant, local_path, cdn_url, bytes)
                VALUES (%s, %s, %s, %s, %s, %s)
                """,
                rows,
            )

    print(f"asset_paths: {len(rows)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

