from __future__ import annotations

import os
from collections.abc import Iterable, Mapping, Sequence
from contextlib import contextmanager
from typing import Any

import psycopg
from psycopg.rows import dict_row

DEFAULT_DATABASE_URL = "postgresql://hsr:hsr@localhost:55432/hsr_agent"


def database_url() -> str:
    return os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)


@contextmanager
def get_conn() -> Iterable[psycopg.Connection[dict[str, Any]]]:
    with psycopg.connect(database_url(), row_factory=dict_row) as conn:
        yield conn


def execute(sql: str, params: Sequence[Any] | Mapping[str, Any] | None = None) -> int:
    with get_conn() as conn:
        with conn.cursor() as cur:
            cur.execute(sql, params)
            return cur.rowcount


def fetch(sql: str, params: Sequence[Any] | Mapping[str, Any] | None = None) -> list[dict[str, Any]]:
    with get_conn() as conn:
        with conn.cursor() as cur:
            cur.execute(sql, params)
            return list(cur.fetchall())
