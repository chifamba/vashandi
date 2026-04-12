-- IVFFlat approximate nearest-neighbor indexes for cosine similarity search.
--
-- lists=100 is a sensible default for up to ~1M rows per namespace.
-- Tune with: SET ivfflat.probes = 10; at query time for recall vs speed tradeoff.
-- Rebuild with REINDEX after bulk loading significant data for optimal clustering.
--
-- NOTE: These indexes only cover rows where embedding IS NOT NULL.
-- Rows written before embeddings are computed remain reachable via keyword search.

CREATE INDEX IF NOT EXISTS idx_memory_entities_embedding_ivfflat
    ON memory_entities USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX IF NOT EXISTS idx_memory_versions_embedding_ivfflat
    ON memory_entity_versions USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
