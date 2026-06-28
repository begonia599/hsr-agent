CREATE TABLE IF NOT EXISTS character_effect_sources (
    id              BIGSERIAL PRIMARY KEY,
    character_id    INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    source_kind     TEXT NOT NULL,
    source_key      TEXT NOT NULL,
    name_zh         TEXT NOT NULL,
    source_text_zh  TEXT NOT NULL,
    game_version    TEXT NOT NULL,
    source_hash     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(character_id, source_kind, source_key, game_version)
);

CREATE INDEX IF NOT EXISTS idx_effect_sources_character
    ON character_effect_sources (character_id, source_kind);

CREATE INDEX IF NOT EXISTS idx_effect_sources_hash
    ON character_effect_sources (source_hash);

CREATE TABLE IF NOT EXISTS character_modifiers (
    id              BIGSERIAL PRIMARY KEY,
    source_id       BIGINT NOT NULL REFERENCES character_effect_sources(id) ON DELETE CASCADE,
    target_scope    TEXT NOT NULL,
    stat_key        TEXT NOT NULL,
    value           NUMERIC,
    value_unit      TEXT NOT NULL,
    modifier_zone   TEXT NOT NULL,
    attack_tag      TEXT,
    element_key     TEXT,
    target_path     TEXT,
    condition_text  TEXT,
    condition_jsonb JSONB NOT NULL DEFAULT '{}',
    duration_key    TEXT,
    stack_rule      TEXT,
    confidence      NUMERIC NOT NULL DEFAULT 0.0 CHECK (confidence >= 0 AND confidence <= 1),
    reviewed        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_modifiers_source
    ON character_modifiers (source_id);

CREATE INDEX IF NOT EXISTS idx_modifiers_stat_zone
    ON character_modifiers (stat_key, modifier_zone);

CREATE INDEX IF NOT EXISTS idx_modifiers_target_scope
    ON character_modifiers (target_scope);

CREATE INDEX IF NOT EXISTS idx_modifiers_attack_tag
    ON character_modifiers (attack_tag);

CREATE INDEX IF NOT EXISTS idx_modifiers_condition
    ON character_modifiers USING gin (condition_jsonb jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_modifiers_reviewed
    ON character_modifiers (reviewed, confidence DESC);

CREATE OR REPLACE VIEW v_character_modifiers AS
SELECT
    c.id AS character_id,
    c.name_zh AS character_name_zh,
    s.id AS source_id,
    s.source_kind,
    s.source_key,
    s.name_zh AS source_name_zh,
    m.id AS modifier_id,
    m.target_scope,
    m.stat_key,
    m.value,
    m.value_unit,
    m.modifier_zone,
    m.attack_tag,
    m.element_key,
    m.target_path,
    m.condition_text,
    m.condition_jsonb,
    m.duration_key,
    m.stack_rule,
    m.confidence,
    m.reviewed
FROM character_modifiers m
JOIN character_effect_sources s ON s.id = m.source_id
JOIN characters c ON c.id = s.character_id;
