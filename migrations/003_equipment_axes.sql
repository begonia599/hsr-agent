CREATE TABLE IF NOT EXISTS equipment_axes (
    id          BIGSERIAL PRIMARY KEY,
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('lightcone', 'relic_set')),
    entity_id   INT NOT NULL,
    kind        TEXT NOT NULL,
    stat        TEXT NOT NULL,
    target      TEXT NOT NULL DEFAULT '',
    value       NUMERIC,
    uptime      TEXT NOT NULL DEFAULT '',
    condition   TEXT,
    source      TEXT,
    confidence  NUMERIC NOT NULL DEFAULT 0.0 CHECK (confidence >= 0 AND confidence <= 1),
    reviewed    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_equipment_axes_entity
    ON equipment_axes (entity_kind, entity_id);

CREATE INDEX IF NOT EXISTS idx_equipment_axes_kind_stat
    ON equipment_axes (kind, stat);

CREATE INDEX IF NOT EXISTS idx_equipment_axes_reviewed
    ON equipment_axes (reviewed, confidence DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_equipment_axes_unique
    ON equipment_axes (entity_kind, entity_id, kind, stat, target, uptime, coalesce(condition, ''));
