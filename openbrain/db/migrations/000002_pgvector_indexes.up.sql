-- IVFFlat approximate nearest-neighbor indexes for cosine similarity search.
--
-- Tuning guidance:
--   lists = row_count / 1000  for tables up to 1M rows
--   lists = sqrt(row_count)   for tables over 1M rows
-- Default lists=100 is calibrated for ~100K rows per namespace. Rebuild after
-- significant bulk data loads with:  REINDEX INDEX idx_memory_entities_embedding_ivfflat;
-- Query-time probes (recall/speed tradeoff):  SET ivfflat.probes = 10;
--
-- NOTE: These indexes only cover rows where embedding IS NOT NULL.
-- Rows written before embeddings are computed remain reachable via keyword search.

CREATE INDEX IF NOT EXISTS idx_memory_entities_embedding_ivfflat
    ON memory_entities USING ivfflat (embedding public.vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX IF NOT EXISTS idx_memory_versions_embedding_ivfflat
    ON memory_entity_versions USING ivfflat (embedding public.vector_cosine_ops)
    WITH (lists = 100);
