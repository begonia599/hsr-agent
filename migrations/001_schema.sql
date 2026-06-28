CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS characters (
    id              INT PRIMARY KEY,
    version         TEXT NOT NULL,
    release_at      TIMESTAMPTZ,
    icon_name       TEXT,
    rarity          SMALLINT NOT NULL CHECK (rarity IN (4, 5)),
    path            TEXT NOT NULL,
    element         TEXT NOT NULL,
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    name_ko         TEXT,
    name_ja         TEXT,
    desc_zh         TEXT,
    desc_en         TEXT,
    sp_need         INT,
    roles           TEXT[] NOT NULL DEFAULT '{}',
    raw_zh          JSONB NOT NULL,
    raw_en          JSONB NOT NULL,
    axes            JSONB NOT NULL DEFAULT '{}',
    skill_text_zh   TEXT NOT NULL DEFAULT '',
    skill_text_en   TEXT NOT NULL DEFAULT '',
    embedding       vector(1024),
    is_trailblazer  BOOLEAN NOT NULL DEFAULT FALSE,
    is_collab       BOOLEAN NOT NULL DEFAULT FALSE,
    is_variant      BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_chars_roles ON characters USING gin (roles);
CREATE INDEX IF NOT EXISTS idx_chars_axes ON characters USING gin (axes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_chars_skilltext_zh ON characters USING gin (skill_text_zh gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_chars_skilltext_en ON characters USING gin (skill_text_en gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_chars_embedding ON characters USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_chars_path ON characters (path);
CREATE INDEX IF NOT EXISTS idx_chars_element ON characters (element);
CREATE INDEX IF NOT EXISTS idx_chars_name_zh_trgm ON characters USING gin (name_zh gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_chars_name_en_trgm ON characters USING gin (name_en gin_trgm_ops);

CREATE TABLE IF NOT EXISTS character_axes (
    char_id   INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,
    stat      TEXT NOT NULL,
    target    TEXT NOT NULL DEFAULT '',
    value     NUMERIC,
    uptime    TEXT NOT NULL DEFAULT '',
    condition TEXT,
    PRIMARY KEY (char_id, kind, stat, target, uptime)
);

CREATE INDEX IF NOT EXISTS idx_caxes_kind_stat ON character_axes (kind, stat);
CREATE INDEX IF NOT EXISTS idx_caxes_kind_target ON character_axes (kind, target);

CREATE TABLE IF NOT EXISTS lightcones (
    id              INT PRIMARY KEY,
    version         TEXT NOT NULL,
    rarity          SMALLINT NOT NULL CHECK (rarity IN (3, 4, 5)),
    path            TEXT NOT NULL,
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    desc_zh         TEXT,
    desc_en         TEXT,
    raw_zh          JSONB NOT NULL,
    raw_en          JSONB,
    axes            JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1024)
);

CREATE INDEX IF NOT EXISTS idx_lc_path ON lightcones (path);
CREATE INDEX IF NOT EXISTS idx_lc_axes ON lightcones USING gin (axes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_lc_embedding ON lightcones USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_lc_name_zh_trgm ON lightcones USING gin (name_zh gin_trgm_ops);

CREATE TABLE IF NOT EXISTS relic_sets (
    id              INT PRIMARY KEY,
    version         TEXT NOT NULL,
    kind            TEXT NOT NULL,
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    set2_desc       TEXT,
    set4_desc       TEXT,
    raw_zh          JSONB NOT NULL,
    raw_en          JSONB,
    axes            JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1024)
);

CREATE INDEX IF NOT EXISTS idx_rset_axes ON relic_sets USING gin (axes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_rset_embedding ON relic_sets USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_rset_name_zh_trgm ON relic_sets USING gin (name_zh gin_trgm_ops);

CREATE TABLE IF NOT EXISTS character_recommendations (
    id                BIGSERIAL PRIMARY KEY,
    char_id           INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    recommend_kind    TEXT NOT NULL,
    item_id           INT,
    rank              INT NOT NULL DEFAULT 0,
    payload           JSONB
);

CREATE INDEX IF NOT EXISTS idx_crec_char ON character_recommendations (char_id, recommend_kind);

CREATE TABLE IF NOT EXISTS team_cooccur (
    char_a            INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    char_b            INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    weight            INT NOT NULL,
    is_main_lineup    BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (char_a, char_b)
);

CREATE INDEX IF NOT EXISTS idx_coocc_a ON team_cooccur (char_a, weight DESC);
CREATE INDEX IF NOT EXISTS idx_coocc_b ON team_cooccur (char_b, weight DESC);

CREATE TABLE IF NOT EXISTS items (
    id              INT PRIMARY KEY,
    item_sub_type   TEXT,
    purpose_type    INT,
    rarity          TEXT,
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    figure_stem     TEXT,
    raw_zh          JSONB,
    raw_en          JSONB
);

CREATE INDEX IF NOT EXISTS idx_items_name_zh_trgm ON items USING gin (name_zh gin_trgm_ops);

CREATE TABLE IF NOT EXISTS asset_paths (
    entity_kind   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    variant       TEXT NOT NULL,
    local_path    TEXT NOT NULL,
    cdn_url       TEXT NOT NULL,
    bytes         INT,
    PRIMARY KEY (entity_kind, entity_id, variant)
);

CREATE INDEX IF NOT EXISTS idx_assets_entity ON asset_paths (entity_kind, entity_id);

CREATE OR REPLACE VIEW v_provides AS
SELECT char_id, stat, target, value, uptime, condition
FROM character_axes
WHERE kind = 'provides';

CREATE OR REPLACE VIEW v_needs AS
SELECT char_id, stat, target, value, uptime, condition
FROM character_axes
WHERE kind = 'needs';

CREATE OR REPLACE VIEW v_team_atk_buffers AS
SELECT c.id, c.name_zh, c.rarity, ca.value, ca.uptime
FROM characters c
JOIN character_axes ca ON ca.char_id = c.id
WHERE ca.kind = 'provides'
  AND ca.stat = 'atk_percent'
  AND ca.target IN ('all_allies', 'self_and_allies')
ORDER BY ca.value DESC NULLS LAST;
