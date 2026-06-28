from __future__ import annotations

import argparse
import json
import os
import sys
import time
import traceback
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import psycopg
from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.llm_client import create_client, require_llm_config
from scripts.enrich import (
    enrich_character,
    enrich_character_openai,
    output_payload,
    write_json,
)
from scripts.load import clean_text, load_json
from scripts.load_axes import DEFAULT_DATABASE_URL, load_character_axes

DEFAULT_VERSION = "4.3.54"


def now() -> str:
    return datetime.now(UTC).isoformat(timespec="seconds")


class Logger:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)

    def write(self, message: str) -> None:
        line = f"{now()} {message}"
        print(line, flush=True)
        with self.path.open("a", encoding="utf-8") as fh:
            fh.write(line + "\n")


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


def write_state(path: Path, state: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def all_character_ids(data_dir: Path) -> list[int]:
    overview = load_json(data_dir / "character.json")
    return [int(char_id) for char_id in sorted(overview, key=int)]


def load_axes_file(out_path: Path, logger: Logger) -> None:
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            rows = load_character_axes(cur, out_path)
    logger.write(f"loaded axes {out_path.name} rows={rows}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Background-safe enrich worker.")
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
    parser.add_argument("--ids", nargs="*", help="Only process these ids.")
    parser.add_argument("--force", action="store_true", help="Overwrite existing enriched files.")
    parser.add_argument("--load-axes", action="store_true", help="Load each generated axes file into PG.")
    parser.add_argument("--max-attempts", type=int, default=3)
    parser.add_argument("--sleep-seconds", type=float, default=2.0)
    parser.add_argument("--log-file", type=Path, default=Path("logs/enrich_worker.log"))
    parser.add_argument("--state-file", type=Path, default=Path("logs/enrich_worker_state.json"))
    return parser.parse_args()


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    logger = Logger(args.log_file if args.log_file.is_absolute() else ROOT / args.log_file)
    state_file = args.state_file if args.state_file.is_absolute() else ROOT / args.state_file
    data_dir = args.data_dir if args.data_dir.is_absolute() else ROOT / args.data_dir
    out_dir = args.out_dir if args.out_dir.is_absolute() else ROOT / args.out_dir
    requested_ids = parse_ids(args.ids)

    config = require_llm_config()
    os.environ.pop("LLM_API_KEY", None)
    if config.api_format not in {"anthropic", "openai"}:
        raise RuntimeError("LLM_API_FORMAT must be 'anthropic' or 'openai'")

    client = create_client(config) if config.api_format == "anthropic" else None
    char_ids = all_character_ids(data_dir)
    if requested_ids is not None:
        char_ids = [char_id for char_id in char_ids if char_id in requested_ids]

    logger.write(
        f"worker started ids={len(char_ids)} model={config.model} format={config.api_format} "
        f"force={args.force} load_axes={args.load_axes}"
    )

    done: list[int] = []
    skipped: list[int] = []
    failed: dict[int, str] = {}

    for index, char_id in enumerate(char_ids, start=1):
        detail_path = data_dir / "zh" / "character" / f"{char_id}.json"
        out_path = out_dir / "character" / f"{char_id}.json"
        detail = load_json(detail_path)
        name = clean_text(detail.get("name"))

        if out_path.exists() and not args.force:
            skipped.append(char_id)
            logger.write(f"[{index}/{len(char_ids)}] skip {char_id} {name}")
            if args.load_axes:
                try:
                    load_axes_file(out_path, logger)
                except Exception as exc:  # noqa: BLE001
                    logger.write(f"load_axes failed for skipped {char_id}: {type(exc).__name__}: {exc}")
            continue

        last_error = ""
        for attempt in range(1, args.max_attempts + 1):
            try:
                logger.write(f"[{index}/{len(char_ids)}] enrich {char_id} {name} attempt={attempt}")
                if config.api_format == "openai":
                    axes = enrich_character_openai(config, char_id, detail)
                else:
                    assert client is not None
                    axes = enrich_character(client, config.model, char_id, detail)

                write_json(out_path, output_payload(char_id, detail, axes, config.model))
                logger.write(f"wrote {out_path.relative_to(ROOT)}")
                if args.load_axes:
                    load_axes_file(out_path, logger)
                done.append(char_id)
                last_error = ""
                break
            except Exception as exc:  # noqa: BLE001
                last_error = f"{type(exc).__name__}: {exc}"
                logger.write(f"error {char_id} attempt={attempt}: {last_error}")
                logger.write(traceback.format_exc().rstrip())
                if attempt < args.max_attempts:
                    time.sleep(args.sleep_seconds * attempt)

        if last_error:
            failed[char_id] = last_error

        write_state(
            state_file,
            {
                "updated_at": now(),
                "total": len(char_ids),
                "done": done,
                "skipped": skipped,
                "failed": failed,
            },
        )

    logger.write(f"worker finished done={len(done)} skipped={len(skipped)} failed={len(failed)}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())

