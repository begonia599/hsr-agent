CREATE TABLE IF NOT EXISTS embedding_metadata (
    entity_kind TEXT PRIMARY KEY CHECK (entity_kind IN ('character', 'lightcone', 'relic_set')),
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    dimensions  INT NOT NULL CHECK (dimensions > 0),
    quality     TEXT NOT NULL,
    rows        INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
