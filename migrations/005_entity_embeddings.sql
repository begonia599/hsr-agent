CREATE TABLE IF NOT EXISTS entity_embeddings (
    entity_kind         TEXT NOT NULL CHECK (entity_kind IN ('character', 'lightcone', 'relic_set')),
    entity_id           INT NOT NULL,
    embedding_model_id  TEXT NOT NULL,
    provider            TEXT NOT NULL,
    model               TEXT NOT NULL,
    native_dimensions   INT NOT NULL CHECK (native_dimensions > 0),
    storage_dimensions  INT NOT NULL CHECK (storage_dimensions = 1024),
    projection_strategy TEXT NOT NULL DEFAULT 'none',
    quality             TEXT NOT NULL,
    content_hash        TEXT NOT NULL,
    embedding           vector(1024) NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (entity_kind, entity_id, embedding_model_id)
);

CREATE INDEX IF NOT EXISTS idx_entity_embeddings_kind_model
    ON entity_embeddings (entity_kind, embedding_model_id);

CREATE INDEX IF NOT EXISTS idx_entity_embeddings_model
    ON entity_embeddings (embedding_model_id);

CREATE INDEX IF NOT EXISTS idx_entity_embeddings_embedding
    ON entity_embeddings USING hnsw (embedding vector_cosine_ops);
