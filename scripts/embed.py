from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import sys
import tempfile
import time
from collections.abc import Iterable
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import httpx
import psycopg
from dotenv import load_dotenv
from psycopg.rows import dict_row

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from hsr_agent.db import DEFAULT_DATABASE_URL
from scripts.load import clean_text

DEFAULT_DIMENSIONS = 1024
LOCAL_MODEL_NAME = "local-hash-ngram-v1"
TEXT_SCHEMA_VERSION = "entity-text-v1"

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


@dataclass(frozen=True)
class EmbeddingSettings:
    model_id: str
    provider: str
    base_url: str
    api_key: str
    model: str
    dimensions: int
    native_dimensions: int
    storage_dimensions: int
    projection_strategy: str
    encoding_format: str
    extra_headers: dict[str, str]
    batch_size: int

    @property
    def quality(self) -> str:
        if self.provider == "local_hash":
            return "lexical_hash"
        if self.provider == "openai_compatible":
            return "semantic"
        return "disabled"


def add_feature(vec: list[float], feature: str, weight: float, dimensions: int) -> None:
    if not feature:
        return
    digest = hashlib.sha256(feature.encode("utf-8")).digest()
    bucket = int.from_bytes(digest[:4], "big") % dimensions
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


def embed_text_local(text: str, dimensions: int) -> list[float]:
    normalized = normalize_text(text)
    vec = [0.0] * dimensions
    for feature, weight in text_features(normalized):
        add_feature(vec, feature, weight, dimensions)
    for feature, weight in alias_features(normalized):
        add_feature(vec, feature, weight, dimensions)

    norm = math.sqrt(sum(value * value for value in vec))
    if norm == 0:
        return vec
    return [value / norm for value in vec]


def vector_literal(vec: list[float]) -> str:
    return "[" + ",".join(f"{value:.8f}" for value in vec) + "]"


def embeddings_url(base_url: str) -> str:
    base = base_url.rstrip("/")
    if base.endswith("/v1"):
        return base + "/embeddings"
    return base + "/v1/embeddings"


def parse_extra_headers(raw: str) -> dict[str, str]:
    headers: dict[str, str] = {}
    for item in raw.split(","):
        item = item.strip()
        if not item:
            continue
        if ":" not in item:
            raise ValueError(f"invalid EMBEDDING_EXTRA_HEADERS item {item!r}, expected Name:value")
        name, value = item.split(":", 1)
        name = name.strip()
        value = value.strip()
        if name and value:
            headers[name] = value
    return headers


def csv_env(key: str) -> list[str]:
    return [item.strip() for item in os.getenv(key, "").split(",") if item.strip()]


def env_id(model_id: str) -> str:
    out: list[str] = []
    last_underscore = False
    for char in model_id.upper():
        if char.isalnum():
            out.append(char)
            last_underscore = False
            continue
        if not last_underscore:
            out.append("_")
            last_underscore = True
    return "".join(out).strip("_")


def infer_native_dimensions(model: str, storage_dimensions: int) -> int:
    name = model.lower()
    if "qwen3-embedding-8b" in name:
        return 4096
    if "qwen3-embedding-4b" in name:
        return 2560
    if "qwen3-embedding-0.6b" in name or "bge-m3" in name:
        return 1024
    return storage_dimensions


def default_projection_strategy(native_dimensions: int, storage_dimensions: int) -> str:
    if native_dimensions == storage_dimensions:
        return "none"
    if native_dimensions > storage_dimensions:
        return f"truncate_{storage_dimensions}"
    return "requested_dimensions"


def validate_settings(settings: EmbeddingSettings) -> EmbeddingSettings:
    if settings.dimensions <= 0:
        raise ValueError("embedding dimensions must be positive")
    if settings.storage_dimensions != DEFAULT_DIMENSIONS:
        raise ValueError(
            f"entity_embeddings stores vector({DEFAULT_DIMENSIONS}); configure {settings.model_id} dimensions to {DEFAULT_DIMENSIONS}"
        )
    if settings.provider == "openai_compatible":
        missing = [
            name
            for name, value in [
                ("base_url", settings.base_url),
                ("api_key", settings.api_key),
                ("model", settings.model),
            ]
            if not value
        ]
        if missing:
            raise ValueError(f"missing required embedding config for {settings.model_id}: {', '.join(missing)}")
    elif settings.provider != "local_hash":
        raise ValueError(f"unsupported embedding provider {settings.provider!r}")
    return settings


def load_catalog_settings(args: argparse.Namespace) -> EmbeddingSettings:
    model_id = args.model_id.strip()
    ids = csv_env("EMBEDDING_MODEL_IDS")
    if ids and model_id not in ids:
        raise ValueError(f"embedding model id {model_id!r} is not listed in EMBEDDING_MODEL_IDS")

    prefix = "EMBEDDING_MODEL_" + env_id(model_id) + "_"
    provider = (args.provider or os.getenv(prefix + "PROVIDER") or "openai_compatible").strip().lower()
    if provider == "local-hash-ngram-v1":
        provider = "local_hash"
    model = (args.model or os.getenv(prefix + "MODEL") or model_id).strip()
    storage_dimensions = int(args.dimensions or os.getenv(prefix + "DIMENSIONS") or DEFAULT_DIMENSIONS)
    native_dimensions = int(
        os.getenv(prefix + "NATIVE_DIMENSIONS") or infer_native_dimensions(model, storage_dimensions)
    )
    projection_strategy = (
        os.getenv(prefix + "PROJECTION_STRATEGY")
        or default_projection_strategy(native_dimensions, storage_dimensions)
    ).strip()
    settings = EmbeddingSettings(
        model_id=model_id,
        provider=provider,
        base_url=(args.base_url or os.getenv(prefix + "BASE_URL") or "").strip(),
        api_key=(os.getenv(prefix + "API_KEY") or "").strip(),
        model=model,
        dimensions=storage_dimensions,
        native_dimensions=native_dimensions,
        storage_dimensions=storage_dimensions,
        projection_strategy=projection_strategy,
        encoding_format=(args.encoding_format or os.getenv(prefix + "ENCODING_FORMAT") or "float").strip(),
        extra_headers=parse_extra_headers(os.getenv(prefix + "EXTRA_HEADERS", "")),
        batch_size=max(1, args.batch_size),
    )
    if settings.provider == "local_hash" and not settings.model:
        settings = EmbeddingSettings(
            model_id=settings.model_id,
            provider=settings.provider,
            base_url=settings.base_url,
            api_key=settings.api_key,
            model=LOCAL_MODEL_NAME,
            dimensions=settings.dimensions,
            native_dimensions=settings.native_dimensions,
            storage_dimensions=settings.storage_dimensions,
            projection_strategy=settings.projection_strategy,
            encoding_format=settings.encoding_format,
            extra_headers=settings.extra_headers,
            batch_size=settings.batch_size,
        )
    return validate_settings(settings)


def load_settings(args: argparse.Namespace) -> EmbeddingSettings:
    if args.model_id:
        return load_catalog_settings(args)

    provider = (args.provider or os.getenv("EMBEDDING_PROVIDER") or "local_hash").strip().lower()
    if provider == "local-hash-ngram-v1":
        provider = "local_hash"
    dimensions = int(args.dimensions or os.getenv("EMBEDDING_DIMENSIONS") or DEFAULT_DIMENSIONS)
    if dimensions <= 0:
        raise ValueError("EMBEDDING_DIMENSIONS must be positive")
    model = (args.model or os.getenv("EMBEDDING_MODEL") or "").strip()
    if provider == "local_hash" and not model:
        model = LOCAL_MODEL_NAME
    native_dimensions = infer_native_dimensions(model, dimensions)
    settings = EmbeddingSettings(
        model_id=os.getenv("EMBEDDING_ID", "legacy"),
        provider=provider,
        base_url=(args.base_url or os.getenv("EMBEDDING_BASE_URL") or "").strip(),
        api_key=os.getenv("EMBEDDING_API_KEY", "").strip(),
        model=model,
        dimensions=dimensions,
        native_dimensions=native_dimensions,
        storage_dimensions=dimensions,
        projection_strategy=default_projection_strategy(native_dimensions, dimensions),
        encoding_format=(args.encoding_format or os.getenv("EMBEDDING_ENCODING_FORMAT") or "float").strip(),
        extra_headers=parse_extra_headers(os.getenv("EMBEDDING_EXTRA_HEADERS", "")),
        batch_size=max(1, args.batch_size),
    )
    if settings.provider == "openai_compatible":
        missing = [
            name
            for name, value in [
                ("EMBEDDING_BASE_URL", settings.base_url),
                ("EMBEDDING_API_KEY", settings.api_key),
                ("EMBEDDING_MODEL", settings.model),
            ]
            if not value
        ]
        if missing:
            raise ValueError(f"missing required embedding config: {', '.join(missing)}")
    elif settings.provider != "local_hash":
        raise ValueError(f"unsupported EMBEDDING_PROVIDER {settings.provider!r}")
    return settings


def embed_texts_openai_compatible(texts: list[str], settings: EmbeddingSettings) -> list[list[float]]:
    payload: dict[str, Any] = {
        "input": texts,
        "model": settings.model,
        "dimensions": settings.dimensions,
    }
    if settings.encoding_format:
        payload["encoding_format"] = settings.encoding_format
    headers = {
        "Authorization": f"Bearer {settings.api_key}",
        "Content-Type": "application/json",
        **settings.extra_headers,
    }
    last_exc: Exception | None = None
    for attempt in range(1, 4):
        try:
            with httpx.Client(timeout=60) as client:
                res = client.post(embeddings_url(settings.base_url), headers=headers, json=payload)
            if res.status_code >= 500 and attempt < 3:
                time.sleep(attempt * 2)
                continue
            res.raise_for_status()
            body = res.json()
            rows = body.get("data")
            if not isinstance(rows, list):
                raise RuntimeError("embedding response missing data array")
            rows = sorted(rows, key=lambda item: int(item.get("index", 0)))
            vectors: list[list[float]] = []
            for item in rows:
                vec = item.get("embedding")
                if not isinstance(vec, list):
                    raise RuntimeError("embedding response item missing embedding")
                vector = [float(value) for value in vec]
                if len(vector) != settings.storage_dimensions:
                    if (
                        len(vector) > settings.storage_dimensions
                        and settings.projection_strategy.startswith("truncate_")
                    ):
                        vector = vector[: settings.storage_dimensions]
                    else:
                        raise RuntimeError(
                            f"embedding dimensions mismatch: got {len(vector)}, expected {settings.storage_dimensions}"
                        )
                if len(vector) != settings.storage_dimensions:
                    raise RuntimeError(
                        f"embedding dimensions mismatch after projection: got {len(vector)}, expected {settings.storage_dimensions}"
                    )
                vectors.append(vector)
            if len(vectors) != len(texts):
                raise RuntimeError(f"embedding count mismatch: got {len(vectors)}, expected {len(texts)}")
            return vectors
        except (httpx.HTTPError, RuntimeError, ValueError) as exc:
            last_exc = exc
            if attempt < 3:
                time.sleep(attempt * 2)
                continue
            raise
    raise RuntimeError(f"embedding request failed: {last_exc}")


def embed_texts(texts: list[str], settings: EmbeddingSettings) -> list[list[float]]:
    if settings.provider == "local_hash":
        return [embed_text_local(text, settings.dimensions) for text in texts]
    return embed_texts_openai_compatible(texts, settings)


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
        desc_zh = normalize_text(row["desc_zh"])
        axes_text = normalize_text(row["axes"]) if desc_zh else ""
        return "\n".join(
            [
                str(row["id"]),
                normalize_text(row["name_zh"]),
                normalize_text(row["name_en"]),
                normalize_text(row["path"]),
                desc_zh,
                axes_text,
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


def row_content_hash(kind: str, row: dict[str, Any]) -> str:
    text = row_text(kind, row)
    payload = "\0".join([TEXT_SCHEMA_VERSION, kind, text])
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def select_rows(
    cur: psycopg.Cursor[dict[str, Any]],
    kind: str,
    ids: set[str] | None,
    force: bool,
    limit: int,
    filter_old_embedding: bool,
) -> list[dict[str, Any]]:
    old_embedding_filter = "(%(force)s OR embedding IS NULL) AND " if filter_old_embedding else ""
    table_sql = {
        "character": f"""
            SELECT id, name_zh, name_en, path, element, roles, axes, skill_text_zh
            FROM characters
            WHERE {old_embedding_filter}(%(ids)s::text[] IS NULL OR id::text = ANY(%(ids)s::text[]))
            ORDER BY id
            LIMIT %(limit)s
        """,
        "lightcone": f"""
            SELECT id, name_zh, name_en, path, desc_zh, axes
            FROM lightcones
            WHERE {old_embedding_filter}(%(ids)s::text[] IS NULL OR id::text = ANY(%(ids)s::text[]))
            ORDER BY id
            LIMIT %(limit)s
        """,
        "relic_set": f"""
            SELECT id, name_zh, name_en, kind, set2_desc, set4_desc, axes
            FROM relic_sets
            WHERE {old_embedding_filter}(%(ids)s::text[] IS NULL OR id::text = ANY(%(ids)s::text[]))
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


def entity_embedding_current(
    cur: psycopg.Cursor[dict[str, Any]],
    kind: str,
    row_id: int,
    settings: EmbeddingSettings,
    content_hash: str,
) -> bool:
    cur.execute(
        """
        SELECT provider, model, native_dimensions, storage_dimensions, projection_strategy, quality, content_hash
        FROM entity_embeddings
        WHERE entity_kind = %s AND entity_id = %s AND embedding_model_id = %s
        """,
        (kind, row_id, settings.model_id),
    )
    row = cur.fetchone()
    if row is None:
        return False
    return (
        row["provider"] == settings.provider
        and row["model"] == settings.model
        and int(row["native_dimensions"]) == settings.native_dimensions
        and int(row["storage_dimensions"]) == settings.storage_dimensions
        and row["projection_strategy"] == settings.projection_strategy
        and row["quality"] == settings.quality
        and row["content_hash"] == content_hash
    )


def upsert_entity_embedding(
    cur: psycopg.Cursor[dict[str, Any]],
    kind: str,
    row_id: int,
    settings: EmbeddingSettings,
    content_hash: str,
    vec: list[float],
) -> None:
    cur.execute(
        """
        INSERT INTO entity_embeddings (
            entity_kind, entity_id, embedding_model_id,
            provider, model, native_dimensions, storage_dimensions, projection_strategy,
            quality, content_hash, embedding, updated_at
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s::vector, now())
        ON CONFLICT (entity_kind, entity_id, embedding_model_id) DO UPDATE
        SET provider = EXCLUDED.provider,
            model = EXCLUDED.model,
            native_dimensions = EXCLUDED.native_dimensions,
            storage_dimensions = EXCLUDED.storage_dimensions,
            projection_strategy = EXCLUDED.projection_strategy,
            quality = EXCLUDED.quality,
            content_hash = EXCLUDED.content_hash,
            embedding = EXCLUDED.embedding,
            updated_at = now()
        """,
        (
            kind,
            row_id,
            settings.model_id,
            settings.provider,
            settings.model,
            settings.native_dimensions,
            settings.storage_dimensions,
            settings.projection_strategy,
            settings.quality,
            content_hash,
            vector_literal(vec),
        ),
    )


def process_entity_kind(
    cur: psycopg.Cursor[dict[str, Any]],
    kind: str,
    ids: set[str] | None,
    force: bool,
    limit: int,
    dry_run: bool,
    settings: EmbeddingSettings,
    progress_file: Path | None = None,
) -> int:
    rows = select_rows(cur, kind, ids, True, limit, filter_old_embedding=False)
    pending: list[tuple[dict[str, Any], str, str]] = []
    skipped = 0
    for row in rows:
        text = row_text(kind, row)
        content_hash = hashlib.sha256("\0".join([TEXT_SCHEMA_VERSION, kind, text]).encode("utf-8")).hexdigest()
        if not force and entity_embedding_current(cur, kind, int(row["id"]), settings, content_hash):
            skipped += 1
            continue
        pending.append((row, text, content_hash))

    if dry_run:
        write_progress(progress_file, "dry_run", settings, kind, 0, len(pending), skipped, 0)
        print(
            f"{kind}/{settings.model_id}: would embed {len(pending)} rows, skipped {skipped}; "
            f"{settings.provider}/{settings.model}/{settings.storage_dimensions}/{settings.projection_strategy}"
        )
        if pending:
            preview = pending[0][1][:800].replace("\n", " ")
            print(f"{kind} preview id={pending[0][0]['id']}: {preview}")
        return len(pending)

    processed = 0
    write_progress(progress_file, "running", settings, kind, processed, len(pending), skipped, 0)
    for start in range(0, len(pending), settings.batch_size):
        batch = pending[start : start + settings.batch_size]
        vectors = embed_texts([item[1] for item in batch], settings)
        for (row, _text, content_hash), vector in zip(batch, vectors, strict=True):
            upsert_entity_embedding(cur, kind, int(row["id"]), settings, content_hash, vector)
            processed += 1
        write_progress(progress_file, "running", settings, kind, processed, len(pending), skipped, 0)
        print(
            f"{kind}/{settings.model_id}: processed {processed}/{len(pending)}, "
            f"skipped {skipped}, failed 0"
        )
    if not pending:
        print(f"{kind}/{settings.model_id}: processed 0, skipped {skipped}, failed 0")
    write_progress(progress_file, "complete", settings, kind, processed, len(pending), skipped, 0)
    return processed


def write_progress(
    path: Path | None,
    status: str,
    settings: EmbeddingSettings,
    kind: str,
    processed: int,
    total: int,
    skipped: int,
    failed: int,
) -> None:
    if path is None:
        return
    payload = {
        "status": status,
        "updated_at": datetime.now(timezone.utc).isoformat(),
        "model_id": settings.model_id,
        "provider": settings.provider,
        "model": settings.model,
        "storage_dimensions": settings.storage_dimensions,
        "projection_strategy": settings.projection_strategy,
        "kind": kind,
        "processed": processed,
        "total": total,
        "skipped": skipped,
        "failed": failed,
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile("w", encoding="utf-8", dir=path.parent, delete=False) as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
        tmp_name = fh.name
    Path(tmp_name).replace(path)


def process_kind(
    cur: psycopg.Cursor[dict[str, Any]],
    kind: str,
    ids: set[str] | None,
    force: bool,
    limit: int,
    dry_run: bool,
    settings: EmbeddingSettings,
) -> int:
    rows = select_rows(cur, kind, ids, force, limit, filter_old_embedding=True)
    if dry_run:
        print(f"{kind}: would embed {len(rows)} rows with {settings.provider}/{settings.model}/{settings.dimensions}")
        if rows:
            preview = row_text(kind, rows[0])[:800].replace("\n", " ")
            print(f"{kind} preview id={rows[0]['id']}: {preview}")
        return len(rows)

    for start in range(0, len(rows), settings.batch_size):
        batch = rows[start : start + settings.batch_size]
        texts = [row_text(kind, row) for row in batch]
        vectors = embed_texts(texts, settings)
        for row, vector in zip(batch, vectors, strict=True):
            update_embedding(cur, kind, int(row["id"]), vector)
    print(f"{kind}: embedded {len(rows)} rows")
    if ids is None:
        upsert_embedding_metadata(cur, kind, settings)
    return len(rows)


def upsert_embedding_metadata(cur: psycopg.Cursor[dict[str, Any]], kind: str, settings: EmbeddingSettings) -> None:
    table = {
        "character": "characters",
        "lightcone": "lightcones",
        "relic_set": "relic_sets",
    }[kind]
    cur.execute(f"SELECT count(*) FROM {table} WHERE embedding IS NOT NULL")
    rows = int(cur.fetchone()["count"])
    cur.execute(
        """
        INSERT INTO embedding_metadata (entity_kind, provider, model, dimensions, quality, rows, updated_at)
        VALUES (%s, %s, %s, %s, %s, %s, now())
        ON CONFLICT (entity_kind) DO UPDATE
        SET provider = EXCLUDED.provider,
            model = EXCLUDED.model,
            dimensions = EXCLUDED.dimensions,
            quality = EXCLUDED.quality,
            rows = EXCLUDED.rows,
            updated_at = now()
        """,
        (kind, settings.provider, settings.model, settings.dimensions, settings.quality, rows),
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate local pgvector embeddings for loaded HSR data.")
    parser.add_argument("--kind", choices=["all", "character", "lightcone", "relic_set"], default="all")
    parser.add_argument("--kinds", help="Comma-separated kinds. Overrides --kind when provided.")
    parser.add_argument("--model-id", help="Embedding model id from EMBEDDING_MODEL_IDS; writes entity_embeddings.")
    parser.add_argument("--ids", nargs="*", help="Optional ids, comma-separated or space-separated.")
    parser.add_argument("--force", action="store_true", help="Recompute rows that already have embeddings.")
    parser.add_argument("--resume", action="store_true", help="Resume entity_embeddings generation by skipping current rows.")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be embedded without writing.")
    parser.add_argument("--limit", type=int, default=1_000_000, help="Maximum rows per kind.")
    parser.add_argument("--provider", choices=["local_hash", "openai_compatible"], help="Override EMBEDDING_PROVIDER.")
    parser.add_argument("--base-url", help="Override EMBEDDING_BASE_URL.")
    parser.add_argument("--model", help="Override EMBEDDING_MODEL.")
    parser.add_argument("--dimensions", type=int, help="Override EMBEDDING_DIMENSIONS.")
    parser.add_argument("--encoding-format", help="Override EMBEDDING_ENCODING_FORMAT; use empty string to omit.")
    parser.add_argument("--batch-size", type=int, default=int(os.getenv("EMBEDDING_BATCH_SIZE", "16")), help="Embedding request batch size.")
    parser.add_argument("--progress-file", type=Path, help="Write JSON progress for background runs, e.g. logs/embed_progress.json.")
    return parser.parse_args()


def parse_kinds(args: argparse.Namespace) -> list[str]:
    allowed = {"character", "lightcone", "relic_set"}
    if args.kinds:
        raw = [item.strip() for item in args.kinds.split(",") if item.strip()]
        if "all" in raw:
            return ["character", "lightcone", "relic_set"]
        invalid = [item for item in raw if item not in allowed]
        if invalid:
            raise ValueError(f"unknown kind(s): {', '.join(invalid)}")
        return raw
    return ["character", "lightcone", "relic_set"] if args.kind == "all" else [args.kind]


def main() -> int:
    load_dotenv(ROOT / ".env")
    args = parse_args()
    settings = load_settings(args)
    ids = parse_ids(args.ids)
    kinds = parse_kinds(args)
    database_url = os.getenv("DATABASE_URL", DEFAULT_DATABASE_URL)
    use_entity_embeddings = bool(args.model_id)
    if ids is not None and not args.dry_run and not use_entity_embeddings:
        print("warning: --ids only updates selected rows and will not refresh embedding_metadata")

    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            total = 0
            for kind in kinds:
                if use_entity_embeddings:
                    total += process_entity_kind(cur, kind, ids, args.force, args.limit, args.dry_run, settings, args.progress_file)
                else:
                    total += process_kind(cur, kind, ids, args.force, args.limit, args.dry_run, settings)
            if args.dry_run:
                conn.rollback()
            else:
                conn.commit()

    print(f"total: {total}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
