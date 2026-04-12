-- Enable pgvector extension for vector similarity search
CREATE EXTENSION IF NOT EXISTS vector;

-- Namespaces (1:1 with Vashandi companies)
CREATE TABLE IF NOT EXISTS namespaces (
    id          text PRIMARY KEY,
    company_id  text NOT NULL,
    team_id     text,
    settings    jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_namespaces_company_id ON namespaces(company_id);

-- Core memory entity table
CREATE TABLE IF NOT EXISTS memory_entities (
    id               text PRIMARY KEY,
    namespace_id     text NOT NULL,
    team_id          text,
    entity_type      text NOT NULL DEFAULT 'note',
    sync_path        text,
    title            text,
    text             text NOT NULL,
    -- Embedding stored as pgvector; NULL until async embedding job populates it
    embedding        vector(1536),
    provenance       jsonb NOT NULL DEFAULT '{}',
    identity         jsonb NOT NULL DEFAULT '{}',
    metadata         jsonb NOT NULL DEFAULT '{}',
    tier             integer NOT NULL DEFAULT 0,
    version          integer NOT NULL DEFAULT 1,
    is_deleted       boolean NOT NULL DEFAULT false,
    access_count     integer NOT NULL DEFAULT 0,
    manual_promote   boolean NOT NULL DEFAULT false,
    last_accessed_at timestamptz,
    decay_at         timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_namespace_type_tier    ON memory_entities(namespace_id, entity_type, tier);
CREATE INDEX IF NOT EXISTS idx_memory_namespace_updated      ON memory_entities(namespace_id, is_deleted, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_entities_namespace_created ON memory_entities(namespace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_entities_sync_path     ON memory_entities(sync_path) WHERE sync_path IS NOT NULL;

-- Append-only version history table
CREATE TABLE IF NOT EXISTS memory_entity_versions (
    id            text PRIMARY KEY,
    namespace_id  text NOT NULL,
    entity_id     text NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    version       integer NOT NULL,
    title         text,
    text          text NOT NULL,
    embedding     vector(1536),
    metadata      jsonb NOT NULL DEFAULT '{}',
    provenance    jsonb NOT NULL DEFAULT '{}',
    identity      jsonb NOT NULL DEFAULT '{}',
    tier          integer NOT NULL DEFAULT 0,
    changed_by    jsonb NOT NULL DEFAULT '{}',
    change_reason text,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_versions_namespace_entity_version
    ON memory_entity_versions(namespace_id, entity_id, version);

-- Relationship graph (adjacency list)
CREATE TABLE IF NOT EXISTS memory_edges (
    id             text PRIMARY KEY,
    namespace_id   text NOT NULL,
    from_entity_id text NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    to_entity_id   text NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    edge_type      text NOT NULL DEFAULT 'relates_to',
    weight         float NOT NULL DEFAULT 1.0,
    metadata       jsonb NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_edges_namespace_from_type ON memory_edges(namespace_id, from_entity_id, edge_type);
CREATE INDEX IF NOT EXISTS idx_memory_edges_namespace_to_type   ON memory_edges(namespace_id, to_entity_id, edge_type);

-- Agent registry
CREATE TABLE IF NOT EXISTS registered_agents (
    id                text PRIMARY KEY,
    namespace_id      text NOT NULL,
    vashandi_agent_id text NOT NULL,
    name              text NOT NULL,
    trust_tier        integer NOT NULL DEFAULT 1,
    recall_profile    jsonb NOT NULL DEFAULT '{}',
    is_active         boolean NOT NULL DEFAULT true,
    registered_at     timestamptz NOT NULL DEFAULT now(),
    deregistered_at   timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_registered_agents_namespace_agent
    ON registered_agents(namespace_id, vashandi_agent_id);
-- Only one active registration per (namespace, vashandi_agent) pair
CREATE UNIQUE INDEX IF NOT EXISTS idx_registered_agents_namespace_agent_active
    ON registered_agents(namespace_id, vashandi_agent_id) WHERE is_active = true;

-- LLM curator proposals (require human approval before execution)
CREATE TABLE IF NOT EXISTS curator_proposals (
    id             text PRIMARY KEY,
    namespace_id   text NOT NULL,
    proposal_type  text NOT NULL,
    memory_ids     jsonb NOT NULL DEFAULT '[]',
    summary        text NOT NULL DEFAULT '',
    suggested_text text NOT NULL DEFAULT '',
    suggested_tier integer,
    status         text NOT NULL DEFAULT 'pending',
    details        jsonb NOT NULL DEFAULT '{}',
    reviewed_by    text,
    reviewed_at    timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_curator_proposals_namespace_status ON curator_proposals(namespace_id, status);

-- Immutable audit log (application-level append-only; DB role has no UPDATE/DELETE)
CREATE TABLE IF NOT EXISTS memory_audit_log (
    id           bigserial PRIMARY KEY,
    namespace_id text NOT NULL,
    agent_id     text,
    actor_kind   text NOT NULL,
    action       text NOT NULL,
    entity_id    text,
    entity_type  text,
    before_hash  text,
    after_hash   text,
    chain_hash   text,
    request_meta jsonb NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_audit_log_namespace ON memory_audit_log(namespace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_audit_log_entity    ON memory_audit_log(entity_id)  WHERE entity_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memory_audit_log_agent     ON memory_audit_log(agent_id)   WHERE agent_id  IS NOT NULL;

-- Proactive context packets (for agent pre-run hydration)
CREATE TABLE IF NOT EXISTS context_packets (
    id           text PRIMARY KEY,
    namespace_id text NOT NULL,
    agent_id     text NOT NULL,
    trigger_type text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}',
    expires_at   timestamptz,
    delivered_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_context_packets_namespace_agent ON context_packets(namespace_id, agent_id);
