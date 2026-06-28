from __future__ import annotations

import hashlib
import os
import sys
from pathlib import Path

import psycopg

try:
    from dotenv import load_dotenv
except ImportError:  # pragma: no cover - optional during bootstrap
    load_dotenv = None

ROOT = Path(__file__).resolve().parents[1]
MIGRATIONS_DIR = ROOT / "migrations"
DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"


def load_environment() -> None:
    if load_dotenv is not None:
        load_dotenv(ROOT / ".env")


def migration_files() -> list[Path]:
    return sorted(MIGRATIONS_DIR.glob("*.sql"))


def checksum(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def main() -> int:
    load_environment()
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)

    files = migration_files()
    if not files:
        print(f"No migration files found in {MIGRATIONS_DIR}")
        return 1

    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS schema_migrations (
                    filename TEXT PRIMARY KEY,
                    checksum TEXT NOT NULL,
                    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
                )
                """
            )

            for path in files:
                sql = path.read_text(encoding="utf-8")
                digest = checksum(sql)
                cur.execute(
                    "SELECT checksum FROM schema_migrations WHERE filename = %s",
                    (path.name,),
                )
                row = cur.fetchone()

                if row is not None:
                    if row[0] != digest:
                        raise RuntimeError(
                            f"Migration checksum mismatch for {path.name}; "
                            "create a new migration instead of editing an applied one."
                        )
                    print(f"skip {path.name}")
                    continue

                print(f"apply {path.name}")
                cur.execute(sql)
                cur.execute(
                    "INSERT INTO schema_migrations (filename, checksum) VALUES (%s, %s)",
                    (path.name, digest),
                )

    print("migrations complete")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
