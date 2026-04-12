# Vashandi ↔ OpenBrain Integration Contract

Date: 2026-04-12

This document specifies the full integration contract between the Vashandi control plane and the OpenBrain memory service, and defines the three infrastructure upgrades required to make that integration production-ready:

1. **pgxpool** connection pool
2. **golang-migrate** SQL migrations
3. **pgvector / IVFFlat** semantic index

---

## 1. System Topology

```
┌─────────────────────────────────────────────────────────┐
│  Vashandi (Node.js / TypeScript)                        │
│  port 3100                                              │
│                                                         │
│  ┌─────────────┐   ┌─────────────┐   ┌───────────────┐ │
│  │ API server  │   │ Agent adapters│  │ Plugin runtime│ │
│  └──────┬──────┘   └──────┬───────┘  └───────┬───────┘ │
│         │                 │                   │         │
└─────────┼─────────────────┼───────────────────┼─────────┘
          │  REST / gRPC    │  REST             │  REST
          ▼                 ▼                   ▼
┌──────────────────────────────────────────────────────────┐
│  OpenBrain (Go)                                          │
│  HTTP port 3101   gRPC port 50051   MCP stdio/HTTP       │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────┐  │
│  │  REST router │  │  gRPC server │  │  MCP server    │  │
│  └──────┬───────┘  └──────┬───────┘  └────────┬───────┘  │
│         └─────────────────┴──────────────────-─┘         │
│                           │                              │
│                    brain.Service                         │
│                           │                              │
│               pgxpool  (jackc/pgx v5)                   │
│                           │                              │
└───────────────────────────┼──────────────────────────────┘
                            │
                    PostgreSQL + pgvector
```

---

## 2. Authentication Contract

### 2.1 Vashandi → OpenBrain (server-to-server)

All Vashandi calls to OpenBrain must authenticate as a service actor with the highest trust tier (4).

**Token format:** `OPENBRAIN_SIGNING_SECRET` shared symmetric secret, or a scoped HMAC token issued by OpenBrain.

**Token structure (scoped):**
```
openbrain.<base64url-JSON-claims>.<HMAC-SHA256-hex-signature>
```

Claims JSON:
```json
{
  "namespaceId": "<company-id>",
  "agentId":     "<vashandi-agent-id>",
  "trustTier":   4,
  "actorKind":   "service",
  "name":        "vashandi-control-plane"
}
```

**Header:** `Authorization: Bearer <token>`

**Optional supplemental headers:**
| Header | Purpose |
|--------|---------|
| `X-OpenBrain-Actor-Kind` | Override actor kind when not in scoped token |
| `X-OpenBrain-Agent-ID` | Override agent id |
| `X-OpenBrain-Trust-Tier` | Override trust tier (1–4) |

### 2.2 Agent → OpenBrain (via Vashandi proxy)

Vashandi issues scoped tokens to agents with:
- `trustTier`: 1 (default) or higher if agent is elevated
- `actorKind`: `"agent"`
- `namespaceId`: company-id the agent belongs to
- `agentId`: agent's Vashandi id

These tokens constrain namespace access inside OpenBrain without requiring agents to know the signing secret.

### 2.3 Namespace → Company mapping

Every Vashandi company maps 1:1 to an OpenBrain namespace.  
`namespace_id = company_id` (string UUID).

Vashandi must call `POST /internal/v1/namespaces` when a company is created, and `DELETE /internal/v1/namespaces/{namespaceId}` when a company is soft-deleted.

---

## 3. REST API Contract

Base URL: `http://<openbrain-host>:3101`

### 3.1 Lifecycle endpoints (internal — Vashandi only)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/internal/v1/namespaces` | Create or ensure namespace |
| `DELETE` | `/internal/v1/namespaces/{namespaceId}` | Archive namespace and export |
| `POST` | `/internal/v1/namespaces/{namespaceId}/agents` | Register or update a Vashandi agent |
| `DELETE` | `/internal/v1/namespaces/{namespaceId}/agents/{agentId}` | Deregister agent |
| `POST` | `/internal/v1/namespaces/{namespaceId}/triggers/{triggerType}` | Fire lifecycle trigger |
| `POST` | `/internal/v1/namespaces/{namespaceId}/sync` | Sync a repository path |

**Trigger types:**
- `pre_run` — compile and return context before an agent run starts
- `post_run` — capture run summary after completion
- `issue_comment` — capture a new issue comment
- `error` — log a run error as a memory

### 3.2 Memory CRUD (agents and service)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/memories` | Create memory |
| `GET` | `/api/v1/memories` | Browse (paginated, filterable) |
| `POST` | `/api/v1/memories/search` | Semantic + lexical search |
| `GET` | `/api/v1/memories/{id}` | Fetch single memory |
| `PATCH` | `/api/v1/memories/{id}` | Update (text, tier, metadata) |
| `DELETE` | `/api/v1/memories/{id}` | Soft-delete |
| `GET` | `/api/v1/memories/{id}/versions` | Version history |
| `POST` | `/api/v1/memories/{id}/rollback` | Roll back to prior version |
| `POST` | `/api/v1/memories/edges` | Create a graph edge |
| `DELETE` | `/api/v1/memories/edges/{edgeId}` | Remove edge |
| `GET` | `/api/v1/memories/{id}/edges` | List edges for a memory |

### 3.3 Context compilation

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/context/compile` | Compile token-budgeted context block |
| `GET` | `/api/v1/context/pending` | Fetch pending context packets |

**Compile request body:**
```json
{
  "namespaceId":   "company-uuid",
  "agentId":       "agent-uuid",
  "taskQuery":     "fix CI failure in postgres migration",
  "intent":        "agent_preamble",
  "tokenBudget":   800,
  "includeTypes":  ["fact", "decision", "adr"]
}
```

### 3.4 Curator / admin

| Method | Path |
|--------|------|
| `GET` | `/api/v1/admin/proposals` |
| `POST` | `/api/v1/namespaces/{namespaceId}/proposals/{proposalId}/resolve` |
| `GET` | `/api/v1/admin/dashboard` |
| `POST` | `/api/v1/admin/daydream` |
| `GET` | `/api/v1/audit/log` |
| `GET` | `/api/v1/audit/export` |

### 3.5 Health

| Method | Path | Auth |
|--------|------|------|
| `GET` | `/healthz` | None |
| `GET` | `/api/v1/health` | Bearer |

---

## 4. gRPC Contract

Service: `openbrain.v1.MemoryService`  
Port: 50051 (default)

```protobuf
service MemoryService {
  rpc Ingest (IngestRequest) returns (IngestResponse);
  rpc Query  (QueryRequest)  returns (QueryResponse);
  rpc Forget (ForgetRequest) returns (ForgetResponse);
}

message MemoryRecord {
  string id   = 1;
  string text = 2;
  map<string, string> metadata = 3;
}
```

**Use case:** high-throughput batch ingestion from Vashandi post-run capture.  
Authentication is handled at the transport level via shared secret or mTLS (TBD).

---

## 5. MCP Contract

OpenBrain exposes an MCP server on the same HTTP port as REST, plus via stdio.

**Endpoints:**
- `POST /mcp` — message endpoint
- `GET /mcp/sse` — SSE discovery stream
- `POST /mcp/message` — SSE fallback

**Tools exposed:**
| Tool | Description |
|------|-------------|
| `memory_search` | Semantic + lexical search |
| `memory_note` | Write a new memory |
| `memory_forget` | Soft-delete a memory |
| `memory_correct` | Patch text of an existing memory |
| `memory_browse` | Browse memories by type, tier, or date |
| `context_compile` | Return a token-budgeted context block |

MCP calls are authenticated with the same scoped bearer tokens and are audited via `memory_audit_log`.

---

## 6. Data Model

### 6.1 Core tables (OpenBrain-owned)

| Table | Purpose |
|-------|---------|
| `namespaces` | One row per Vashandi company; indexed on `company_id` |
| `memory_entities` | The primary memory corpus |
| `memory_entity_versions` | Immutable version snapshots |
| `memory_edges` | Graph relationships between memories |
| `registered_agents` | Vashandi agents registered in a namespace |
| `context_packets` | Pending context delivery payloads |
| `curator_proposals` | Curator-generated merge / promote proposals |
| `memory_audit_log` | Append-only tamper-evident log; `chain_hash` links entries |

### 6.2 `memory_entities` key columns

| Column | Type | Notes |
|--------|------|-------|
| `id` | `uuid` | Primary key |
| `namespace_id` | `uuid` | FK → namespaces.id, indexed |
| `entity_type` | `text` | `fact`, `decision`, `adr`, `note`, `error`, etc. |
| `text` | `text` | Full content |
| `embedding` | `vector(1536)` | pgvector column (see §8) |
| `tier` | `int` | 0 = ephemeral, 1 = working, 2 = long-term |
| `version` | `int` | Auto-incremented on every update |
| `provenance` | `jsonb` | Source reference from Vashandi |
| `identity` | `jsonb` | Actor reference |
| `metadata` | `jsonb` | Free-form key/value |
| `is_deleted` | `bool` | Soft-delete flag |
| `decay_at` | `timestamptz` | Scheduled soft-delete for tier-0 |

### 6.3 Provenance shape (Vashandi → OpenBrain)

```json
{
  "kind":       "run",
  "companyId":  "company-uuid",
  "issueId":    "issue-uuid",
  "runId":      "run-uuid",
  "agentId":    "agent-uuid",
  "commentId":  null,
  "activityId": null
}
```

Supported `kind` values: `run`, `issue_comment`, `issue_document`, `issue`, `activity`, `manual_note`, `external_document`.

---

## 7. Infrastructure Upgrade 1: pgxpool Connection Pool

### Problem

`cmd/openbrain/db.go` currently opens a single GORM connection via `gorm.Open(postgres.Open(dsn))` with no pool configuration. Under load this will exhaust server-side connections or serialize all DB operations.

### Target

Replace the plain GORM open with a `pgxpool`-backed connection using GORM's `pgx` driver. pgx v5 already ships with `jackc/puddle/v2` pooling.

### Implementation

**`cmd/openbrain/db.go` (updated)**

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5/stdlib"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func InitDB() *gorm.DB {
    dsn := envDefault("DATABASE_URL",
        "postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable")

    poolCfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        log.Fatalf("pgxpool parse config: %v", err)
    }
    poolCfg.MaxConns = int32(envInt("DB_MAX_CONNS", 20))
    poolCfg.MinConns = int32(envInt("DB_MIN_CONNS", 2))

    pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
    if err != nil {
        log.Fatalf("pgxpool connect: %v", err)
    }
    if err := pool.Ping(context.Background()); err != nil {
        log.Fatalf("pgxpool ping: %v", err)
    }

    sqlDB := stdlib.OpenDBFromPool(pool)
    db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
    if err != nil {
        log.Fatalf("gorm open: %v", err)
    }
    return db
}
```

**Environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable` | Full DSN |
| `DB_MAX_CONNS` | `20` | Max pool size |
| `DB_MIN_CONNS` | `2` | Idle minimum |

**go.mod additions:**

```
github.com/jackc/pgx/v5       (already indirect — promote to direct)
```

No new external dependency is required because `jackc/pgx/v5` is already an indirect dependency through `gorm.io/driver/postgres`.

---

## 8. Infrastructure Upgrade 2: golang-migrate SQL Migrations

### Problem

`brain.Service.AutoMigrate()` calls GORM's `AutoMigrate`, which:
- cannot safely modify existing columns
- has no rollback capability
- makes it impossible to reproduce the exact schema from scratch
- conflates schema migration with the application binary

### Target

Replace `AutoMigrate` with explicit SQL migration files managed by `golang-migrate/migrate/v4`, with PostgreSQL as the driver and migrations embedded in the binary via `embed.FS`.

### Migration directory structure

```
openbrain/
  db/
    migrations/
      000001_initial_schema.up.sql
      000001_initial_schema.down.sql
      000002_add_pgvector_embedding.up.sql
      000002_add_pgvector_embedding.down.sql
      000003_add_context_packets.up.sql
      000003_add_context_packets.down.sql
      000004_add_audit_chain_hash.up.sql
      000004_add_audit_chain_hash.down.sql
```

### Migration runner

**`db/migrate.go`**

```go
package db

import (
    "database/sql"
    "embed"
    "fmt"

    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/postgres"
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

func Migrate(sqlDB *sql.DB) error {
    src, err := iofs.New(MigrationsFS, "migrations")
    if err != nil {
        return fmt.Errorf("migrations source: %w", err)
    }
    drv, err := postgres.WithInstance(sqlDB, &postgres.Config{})
    if err != nil {
        return fmt.Errorf("migrate driver: %w", err)
    }
    m, err := migrate.NewWithInstance("iofs", src, "postgres", drv)
    if err != nil {
        return fmt.Errorf("migrate init: %w", err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("migrate up: %w", err)
    }
    return nil
}
```

**`cmd/openbrain/db.go` (updated call site)**

```go
sqlDB := stdlib.OpenDBFromPool(pool)
if err := db.Migrate(sqlDB); err != nil {
    log.Fatalf("migration failed: %v", err)
}
```

**`000001_initial_schema.up.sql` (representative excerpt)**

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS namespaces (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   UUID NOT NULL,
    team_id      UUID,
    settings     JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_namespaces_company_id ON namespaces(company_id);

CREATE TABLE IF NOT EXISTS memory_entities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    namespace_id    UUID NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    team_id         UUID,
    entity_type     TEXT NOT NULL,
    sync_path       TEXT,
    title           TEXT,
    text            TEXT NOT NULL,
    embedding       JSONB NOT NULL DEFAULT '[]',   -- replaced in migration 002
    provenance      JSONB NOT NULL DEFAULT '{}',
    identity        JSONB NOT NULL DEFAULT '{}',
    metadata        JSONB NOT NULL DEFAULT '{}',
    tier            INT NOT NULL DEFAULT 0,
    version         INT NOT NULL DEFAULT 1,
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    access_count    INT NOT NULL DEFAULT 0,
    manual_promote  BOOLEAN NOT NULL DEFAULT FALSE,
    last_accessed_at TIMESTAMPTZ,
    decay_at        TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- (additional tables: memory_entity_versions, memory_edges,
--  registered_agents, context_packets, curator_proposals,
--  memory_audit_log follow the same pattern)
```

**go.mod additions:**

```
github.com/golang-migrate/migrate/v4 v4.18.1
```

---

## 9. Infrastructure Upgrade 3: pgvector / IVFFlat Indexes

### Problem

`memory_entities.embedding` is stored as `JSONB` (`[]float32` serialized as a JSON array). This means:
- semantic similarity is computed in Go application code using cosine distance over deserialized slices — O(n) full-table scan for every search
- the column type gives the database no opportunity to accelerate nearest-neighbour queries
- stored vectors are ~4× larger than native binary format

### Target

- Enable the `pgvector` extension.
- Change `embedding` from `JSONB` to `vector(1536)` (OpenAI `text-embedding-3-small` dimension; parameterize for other models).
- Build an `IVFFlat` index for approximate nearest-neighbour (ANN) search.
- Keep the Euclidean / cosine operator consistent with the embedding model in use.

### Migration 002 — enable pgvector

**`000002_add_pgvector_embedding.up.sql`**

```sql
CREATE EXTENSION IF NOT EXISTS vector;

-- Add native vector column alongside existing jsonb column.
ALTER TABLE memory_entities ADD COLUMN IF NOT EXISTS embedding_vec vector(1536);

-- Backfill: convert existing jsonb arrays to vector.
-- Only safe when all existing rows have valid float arrays.
-- Run as a background job for large tables; here shown for fresh installs.
UPDATE memory_entities
    SET embedding_vec = embedding::text::vector
    WHERE embedding != '[]' AND embedding_vec IS NULL;

-- Once backfill is verified, swap columns.
ALTER TABLE memory_entities DROP COLUMN IF EXISTS embedding;
ALTER TABLE memory_entities RENAME COLUMN embedding_vec TO embedding;
ALTER TABLE memory_entities ALTER COLUMN embedding SET NOT NULL;

-- IVFFlat index (cosine distance).
-- lists = sqrt(row_count) is a good starting heuristic.
-- Tune after the table has at least 10 000 rows.
CREATE INDEX IF NOT EXISTS idx_memory_entities_embedding_ivfflat
    ON memory_entities
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
```

**`000002_add_pgvector_embedding.down.sql`**

```sql
DROP INDEX IF EXISTS idx_memory_entities_embedding_ivfflat;
ALTER TABLE memory_entities ALTER COLUMN embedding TYPE JSONB
    USING embedding::text::jsonb;
DROP EXTENSION IF EXISTS vector;
```

### GORM model update

```go
// db/models/memory.go
import "github.com/pgvector/pgvector-go"

type Memory struct {
    // ...
    Embedding pgvector.Vector `gorm:"type:vector(1536)" json:"-"`
    // ...
}
```

**go.mod addition:**

```
github.com/pgvector/pgvector-go v0.2.2
```

### Search query (cosine similarity)

```go
// In brain.Service.SearchMemories — replace Go-side cosine loop with:
db.WithContext(ctx).
    Raw(`SELECT *, 1 - (embedding <=> ?) AS score
         FROM memory_entities
         WHERE namespace_id = ? AND is_deleted = false
         ORDER BY embedding <=> ?
         LIMIT ?`,
        pgvector.NewVector(queryEmbedding),
        namespaceID,
        pgvector.NewVector(queryEmbedding),
        limit,
    ).Scan(&results)
```

Operators:
- `<=>` — cosine distance (requires `vector_cosine_ops` index)
- `<->` — L2 Euclidean distance (requires `vector_l2_ops` index)
- `<#>` — negative inner product (requires `vector_ip_ops` index)

### IVFFlat tuning guidance

| Parameter | Guidance |
|-----------|----------|
| `lists` | `sqrt(total_rows)` at index build time; re-index as table grows |
| `ivfflat.probes` | Set `SET ivfflat.probes = 10` at session level for better recall (default 1) |
| Minimum rows | Effective below 1 000 rows; for < 1 000 rows consider `hnsw` or sequential scan |
| Alternative index | `CREATE INDEX ... USING hnsw (embedding vector_cosine_ops)` for better recall on moderate data sizes |

---

## 10. Environment Variables Reference

| Variable | Service | Default | Description |
|----------|---------|---------|-------------|
| `DATABASE_URL` | OpenBrain | `postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable` | PostgreSQL DSN |
| `DB_MAX_CONNS` | OpenBrain | `20` | pgxpool max connections |
| `DB_MIN_CONNS` | OpenBrain | `2` | pgxpool min idle connections |
| `PORT` | OpenBrain | `3101` | HTTP listen port |
| `GRPC_PORT` | OpenBrain | `50051` | gRPC listen port |
| `OPENBRAIN_API_KEY` | Both | *(required)* | Shared bearer secret (legacy) |
| `OPENBRAIN_SIGNING_SECRET` | Both | *(required in prod)* | HMAC secret for scoped tokens |
| `OPENBRAIN_URL` | Vashandi | `http://localhost:3101` | OpenBrain base URL |
| `OPENBRAIN_GRPC_URL` | Vashandi | `localhost:50051` | OpenBrain gRPC address |

---

## 11. Vashandi Integration Points

### 11.1 Company lifecycle

```
company.create  → POST /internal/v1/namespaces
                  body: { namespaceId, companyId, settings }

company.delete  → DELETE /internal/v1/namespaces/{companyId}
```

### 11.2 Agent lifecycle

```
agent.create    → POST /internal/v1/namespaces/{companyId}/agents
                  body: { agentId, name, trustTier, recallProfile }

agent.deactivate → DELETE /internal/v1/namespaces/{companyId}/agents/{agentId}
```

### 11.3 Run lifecycle hooks

```
run.start       → POST /internal/v1/namespaces/{companyId}/triggers/pre_run
                  body: { agentId, taskQuery, intent: "agent_preamble", tokenBudget }
                  response: context packet delivered to agent preamble

run.finish      → POST /internal/v1/namespaces/{companyId}/triggers/post_run
                  body: { agentId, summary, metadata: { runId, issueId } }

run.error       → POST /internal/v1/namespaces/{companyId}/triggers/error
                  body: { agentId, errorText, metadata: { runId } }
```

### 11.4 Issue comment / document capture

```
comment.create (when binding enabled)
                → POST /api/v1/memories
                  body: { namespaceId, entityType: "note", text, provenance: { kind: "issue_comment", ... } }
```

### 11.5 Token issuance

Vashandi issues scoped tokens server-side before proxying agent requests to OpenBrain:

```
token = HMAC-sign({ namespaceId: company.id, agentId: agent.id, trustTier: 1, actorKind: "agent" })
```

The signing secret is `OPENBRAIN_SIGNING_SECRET`, shared between both services.

---

## 12. Rollout Order

1. **Infrastructure first** — deploy OpenBrain with pgxpool + golang-migrate (migrations 001+). No vector column yet.
2. **Vashandi lifecycle hooks** — wire company create/delete and agent create/delete calls to `/internal/v1` endpoints.
3. **pgvector migration** — apply migration 002 against a deployed postgres with the `vector` extension enabled. Backfill embeddings for existing rows.
4. **Semantic search activation** — update `brain.Service.SearchMemories` to use vector operators instead of Go-side cosine loop.
5. **Run lifecycle triggers** — wire `pre_run` / `post_run` hooks in the Vashandi run orchestrator.
6. **MCP surface** — enable MCP tool exposure in agent adapters that support it.

---

## 13. Open Questions

- **Embedding model and dimension.** `vector(1536)` matches OpenAI `text-embedding-3-small`. If a different model is used the index must be rebuilt. The dimension should be a migration parameter or environment variable.
- **Token issuance location.** Does Vashandi issue tokens in the API server or in each adapter? Centralizing in the API server is simpler but requires the adapter to receive the token via the run context.
- **gRPC auth.** The current gRPC server has no authentication middleware. For production, add a unary interceptor that validates the same `OPENBRAIN_SIGNING_SECRET`.
- **IVFFlat vs HNSW.** For tables under ~100 K rows with moderate write throughput, `hnsw` (available in pgvector ≥ 0.5) often gives better recall without the separate build step.
- **Migration locking.** `golang-migrate` uses a single advisory lock. For multi-replica deployments, only one replica should run migrations (init container or one-shot job).
- **Cross-company memory.** Not supported. OpenBrain enforces namespace isolation on every query. Any proposal to share memory across companies must go through explicit export/import.
