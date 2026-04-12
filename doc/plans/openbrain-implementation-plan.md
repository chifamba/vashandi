# OpenBrain Implementation Plan

Date: 2026-04-12
Status: Active — Substantially Implemented (see §1 for accurate current-state table)

*This document is the single source of truth for all pending OpenBrain implementation work. It is derived from the consolidated master plan and covers everything required to take OpenBrain from its current prototype state to a fully operational memory OS.*

See also: [Vashandi Implementation Plan](./trackable-implementation-plan.md)

---

## 1. Current State Assessment

*Last updated: 2026-04-12. The codebase is substantially further along than previous assessments indicated. The table below reflects the actual state as verified against the code in `openbrain/`.*

| Area | Status |
|---|---|
| Repo location (`openbrain/`) | ✅ Fully structured: REST, gRPC, MCP, jobs, models, proto, redis, docs, Dockerfile |
| Go module initialization | ✅ Implemented (`github.com/chifamba/vashandi/openbrain`) |
| PostgreSQL storage (GORM AutoMigrate) | ✅ Implemented (all tables: memory_entities, edges, versions, agents, audit_log, proposals, context_packets, namespaces) |
| pgvector / IVFFlat indexes | ❌ Not implemented — embeddings stored as JSONB; cosine similarity computed in-process |
| Typed memory entity schema | ✅ Implemented (all models in `db/models/`) |
| Multi-tier memory model (L0–L3) | ✅ Implemented — tier enforcement, promotion logic, decay logic, versioning, rollback |
| Agent Registry + trust tiers | ✅ Implemented — RegisterAgent, DeregisterAgent, trust tier permissions, redaction |
| Immutable audit log | ✅ Implemented — chain-hash tamper evidence, append-only application layer; export as JSON-LD + SQLite |
| Context compilation engine | ✅ Implemented — vector+lexical+recency+tier scoring, token budget enforcement, format rendering |
| Proactive context delivery | ✅ Implemented — all 5 trigger types: run_start, run_complete, checkout, branch_creation, test_failure |
| LLM Curator Agent (Gachlaw) | ✅ Implemented — dedup, synthesis, demotion, gap detection proposals; weekly health report; requires human approval |
| Redis queue | ✅ Implemented (`jobs/queue.go`) — embedding/ingest queue workers; embedding cache stub present |
| CLI | ✅ All commands implemented: `memory list/get/add/forget/search/approve`, `audit export`, `health`, `watch`, `token` |
| MCP server | ✅ stdio + HTTP/SSE transports; all 6 tools: `memory_search`, `memory_note`, `memory_forget`, `memory_correct`, `memory_browse`, `context_compile` |
| gRPC server | ✅ Implemented — Ingest, Query, Forget via proto/v1 |
| Repository convention sync (brain.md) | ✅ Implemented — `SyncRepositoryDir`, `WatchRepositoryDir`, `.openbrain/brain.md` → L2 ingest, session.md → L1 |
| Admin Web UI | 🟡 Minimal — server-rendered HTML admin page at `/admin`; full React admin UI not yet built |
| OpenBrain in Vashandi Docker Compose | ✅ `openbrain` service added to `vashandi/docker/docker-compose.yml` |
| golang-migrate SQL migrations | ❌ Not implemented — uses GORM AutoMigrate; no versioned SQL migration files |
| pgxpool connection pool | ❌ Not implemented — uses GORM default connection pool |
| CI build/test step for OpenBrain | ❌ Not added to CI |
| DEVELOPING.md update for OpenBrain | 🟡 Partial — brief mention exists; full combined dev commands not documented |
| Vashandi↔OpenBrain integration contract doc | ❌ Not formally documented; internal API endpoints implemented but no contract document |

---

## 2. Stack Decisions

### Architectural Decisions (all require human confirmation unless marked Confirmed)

> **⚠ DECISION-01 — Language for OpenBrain (✅ Confirmed):** Go. Aligns with the Vashandi Go migration direction, integrates into the existing `go.work` workspace, produces single-binary deployments, and has excellent concurrency for background maintenance and ingestion jobs.

> **⚠ DECISION-02 — Vector storage:** PostgreSQL + pgvector is recommended. Reuses the existing Postgres instance, supports single-node Docker deployment, and pgvector is sufficient for per-company memory store scale. A dedicated vector database (Qdrant, Weaviate) can be added later as a pluggable backend.

> **⚠ DECISION-03 — Graph storage:** Postgres adjacency tables with recursive CTEs for V1 (facts/decisions as nodes, typed relationships as edge rows). Apache AGE or Neo4j can be considered later.

> **⚠ DECISION-04 — OpenBrain service topology (✅ Confirmed):** Separate service (not embedded library, not Vashandi plugin process). OpenBrain has its own background jobs (curator agent, proactive delivery triggers, async ingestion), its own data model, and is designed to serve agents and services beyond Vashandi. Runs as a Docker sidecar in local dev and as a standalone service in production.

> **⚠ DECISION-05 — OpenBrain primary API protocol (✅ Overridden: Multi-Protocol):** REST/JSON + gRPC + MCP simultaneously. HTTP/REST for internal monorepo communication and external web clients. gRPC for high-performance bulk ingest and large context compilations. MCP for standardized LLM interactions.

> **⚠ DECISION-06 — Namespace isolation model:** Row-level isolation using `namespace_id` (maps 1:1 to Vashandi `company_id`). All queries include mandatory `namespace_id` predicate enforced at the storage layer. Separate Postgres schemas per company are rejected as operationally expensive.

> **⚠ DECISION-07 — Curator proposal routing:** Curator proposals will be reviewed and approved via the OpenBrain Modern Admin Web UI, which includes a dedicated panel for memory proposals and dashboard views.

> **⚠ DECISION-08 — Embedding model/dimension:** OpenAI text-embedding-3-small (1536d) as default. Cohere embed-v3 and local Ollama embeddings are future alternatives.

> **⚠ DECISION-09 — OpenBrain auth for external API:** Agent-scoped JWT tokens issued by OpenBrain, validated against `registered_agents`.

> **⚠ DECISION-10 — L2→L3 promotion approval flow:** Routes through Vashandi board approval.

### Stack Summary

| Layer | Technology |
|---|---|
| Language | Go (DECISION-01) |
| HTTP framework | chi router (consistent with Vashandi Go port) |
| Vector storage | PostgreSQL + pgvector (DECISION-02) |
| Graph/relational storage | PostgreSQL adjacency tables (DECISION-03) |
| Service topology | Separate service, Docker sidecar for dev (DECISION-04) |
| Primary API | REST/JSON + gRPC + MCP (DECISION-05) |
| MCP transport | stdio + HTTP/SSE |
| Migrations | golang-migrate |
| CLI | cobra + charmbracelet/huh |
| Redis | redis:8-alpine — embedding cache, async queues |
| Testing | Go standard testing + testify |
| Build integration | `vashandi/go.work` workspace |

### Monorepo Workspace Integration

```
vashandi/go.work            ← already exists
  uses ./backend/shared
  uses ./backend/db
  uses ./backend/server
  uses ./backend/cmd/paperclipai
  uses ../openbrain

pnpm-workspace.yaml         ← unchanged (UI only)
```

OpenBrain module: `github.com/chifamba/vashandi/openbrain`
OpenBrain lives at: `openbrain/` (monorepo root — directory already exists).

### Redis Integration

**Usage in OpenBrain:**
- **Embedding Cache:** Fast retrieval of previously computed LLM embeddings to save costs and latency.
- **Async Queues:** Processing offline memory compilation, background deduplication jobs, and graph maintenance without blocking the main event loop.

---

## 3. Pending Implementation Checklist

### Phase OB-0 — Bootstrap & Cross-System Foundation

These items must be completed before starting any other OpenBrain epic work.

- [x] **OB-0.1: Bootstrap Go Module & Project Structure**
  - [x] Create standard Go module in `openbrain/` (`go mod init github.com/chifamba/vashandi/openbrain`)
  - [x] Scaffold directory layout: `cmd/openbrain/`, `internal/{brain,mcp}/`, `db/models/`, `jobs/`, `redis/`, `proto/v1/`, `docs/`
  - [x] Add `openbrain` to the Vashandi workspace `go.work`
  - [x] Add OpenBrain service to the Vashandi Docker Compose dev stack (`vashandi/docker/docker-compose.yml`)
  - [ ] CI: add `go build ./openbrain/...` and `go test ./openbrain/...` steps
  - [x] Update `DEVELOPING.md` with combined dev commands (partial — brief mention present)
- [ ] **OB-0.2: Define Vashandi↔OpenBrain Integration Interface**
  - [ ] Document exact HTTP/REST, gRPC, and MCP interfaces (internal API endpoints are implemented but no formal contract document)
  - [ ] Define service token strategy: Vashandi generates a service token per company at creation, stored as `company_secrets`
  - [ ] Map Vashandi lifecycle events to OpenBrain calls: agent created, agent archived, company archived, run completed, run starting
  - [ ] Map entity types: Vashandi `agent` → OpenBrain `registered_agent`; Vashandi `company` → OpenBrain `namespace`
- [x] **OB-0.3: Company-Scoped Memory Namespacing**
  - [x] Define schema enforcing row-level `namespace_id` in all Postgres tables and Redis keys
  - [x] Storage layer functions accept `namespace_id` as a non-optional parameter
  - [x] API layer extracts `namespace_id` from the service token (token is scoped to one company via `maybeNamespaceAuthorized`)
  - [x] Every table has `(namespace_id, ...)` composite index as primary access path

### Phase OB-1 — Core Storage Infrastructure

- [x] **OB-1.1: PostgreSQL Setup**
  - [x] GORM AutoMigrate creates all tables on startup
  - [x] Dockerfile and Docker Compose entry for Postgres+OpenBrain
  - [ ] Docker Compose dev profile: Postgres 16 with pgvector pre-installed (currently uses standard Postgres)
  - [ ] golang-migrate versioned SQL migration files (using GORM AutoMigrate instead)
  - [ ] pgxpool connection pool with configurable pool size (using GORM default pool)
  - [ ] pgvector extension: `CREATE EXTENSION IF NOT EXISTS vector;`
- [x] **OB-1.2: Typed Memory Entity Schema**
  - [x] `memory_entities` table with all fields (GORM model in `db/models/memory.go`)
  - [x] `memory_edges` adjacency table for relationship graph
  - [x] `memory_entity_versions` append-only version history table
  - [x] Composite indexes on namespace_id paths (GORM index tags)
  - [ ] IVFFlat index with `lists=100` for pgvector (embeddings stored as JSONB; in-process cosine used instead)
- [x] **OB-1.3: CRUD Operations**
  - [x] `POST /api/v1/memories` — create entity
  - [x] `GET /api/v1/memories/:id` — get entity by id
  - [x] `PATCH /api/v1/memories/:id` — update entity (creates version record)
  - [x] `DELETE /api/v1/memories/:id` — soft delete
  - [x] `GET /api/v1/memories` — browse with filters
  - [x] `POST /api/v1/memories/search` — vector similarity search + keyword filter (in-process scoring)
  - [x] `POST /api/v1/memories/edges` — create relationship
  - [x] `GET /api/v1/memories/:id/edges` — get related entities
  - [ ] Async embedding generation via OpenAI (currently uses local FNV hash-based embeddings)

### Phase OB-2 — Multi-Tier Memory Lifecycle

- [x] **OB-2.1: Tier Model & Data**
  - [x] Implement L0 (Ephemeral, 24h TTL), L1 (Working, 30 days), L2 (Reference, indefinite), L3 (Core, permanent)
  - [x] `tier` column on `memory_entities` enforces model; default L0
- [x] **OB-2.2: Promotion Logic**
  - [x] Background job (every 6h): L0→L1 on 3+ accesses within 24h or manual flag; L1→L2 on 5+ accesses or curator proposal
  - [x] Promotion creates version record with `change_reason = "tier_promotion"`
  - [ ] L2→L3 requires human approval via Vashandi board (L2→L3 promotion gating not yet wired to Vashandi)
- [x] **OB-2.3: Decay Logic**
  - [x] Daily decay job: L0 auto-delete after TTL; L1 demote to L0 if not accessed within 30 days
  - [x] L2/L3 no automatic decay; demotion proposals via curator
  - [x] `stale_memory_ratio` tracked in dashboard metrics
- [x] **OB-2.4: Versioning & Rollback**
  - [x] Every entity update creates immutable version record in `memory_entity_versions`
  - [x] `POST /api/v1/memories/:id/rollback` — restores prior version
  - [x] `GET /api/v1/memories/:id/versions` — list version history

### Phase OB-3 — Agent Identity & Governance

- [x] **OB-3.1: Agent Registry + Trust Tiers**
  - [x] `registered_agents` table (GORM model)
  - [x] Trust tier permissions: Tier 1 (Read), Tier 2 (Contributor), Tier 3 (Curator), Tier 4 (Admin)
  - [x] Namespace authorization middleware enforces token→namespace scoping (unregistered agents get 403 on namespace mismatch)
  - [x] `POST /internal/v1/namespaces/:namespaceId/agents` — register agent
  - [x] `DELETE /internal/v1/namespaces/:namespaceId/agents/:agentId` — deregister agent
  - [x] Lower-trust agents: content redacted for L2/L3 entities above their tier
- [x] **OB-3.2: Immutable Audit Log**
  - [x] `memory_audit_log` table (append-only at application level)
  - [x] Chain hash computed as `SHA-256(prev_chain_hash || namespace_id || created_at || after_hash)`
  - [x] Every read, write, update, delete, promote, rollback, forget, search operation logged
  - [x] `GET /api/v1/audit/log` — filtered log browser
  - [x] `GET /api/v1/audit/export?format=jsonld|sqlite` — export for external audit tools

### Phase OB-4 — Context Engine

- [x] **OB-4.1: Context Compilation & Token Budgeting**
  - [x] Retrieval algorithm: semantic+lexical+recency+tier scoring, re-rank, token budget enforcement, format per agent's recall profile
  - [x] `POST /api/v1/context/compile` — body: `{ agentId, taskQuery, intent, tokenBudget?, includeTypes? }`, response: `{ snippets[], profileSummary?, tokenCount, latencyMs, usage }`
  - [ ] Target: < 500ms at p95 for up to 10,000 entities (no load test yet; in-process scoring is not O(1))
  - [ ] Embedding cache via Redis (Redis queue exists but LRU embedding cache not yet wired)
  - [ ] IVFFlat index (pending pgvector integration)
- [x] **OB-4.2: Proactive Context Delivery**
  - [x] Trigger ingestion endpoint: `POST /internal/v1/namespaces/:id/triggers/:triggerType`
  - [x] Trigger type: `run_start` (pre-run hydrate)
  - [x] Trigger type: `run_complete` (post-run capture)
  - [x] Trigger type: `checkout` (task-specific surfacing)
  - [x] Trigger type: `branch_creation` (ADR surfacing)
  - [x] Trigger type: `test_failure` (related failures)
  - [x] Context packet preparation and storage for next agent poll (`GET /api/v1/context/pending`)
  - [ ] Integration with Vashandi heartbeat `fat context` mode (Vashandi side not yet wired)

### Phase OB-5 — Self-Evolution & Curation

- [x] **OB-5.1: LLM Curator Agent (Gachlaw)**
  - [x] Background process within OpenBrain (weekly via `StartBackgroundJobs`)
  - [x] De-duplicate: detect near-duplicates (cosine similarity > 0.95), propose merge
  - [x] Synthesize: group related L1 entities into new L2 entity, propose promotion
  - [x] Demotion: propose L2→L1 for entities unused 60 days
  - [x] Knowledge gap detection: identify frequently-asked queries with empty recall
  - [x] All proposals require approval before execution (`ResolveProposal`)
  - [x] Proposals routed to admin endpoint for review (`GET /admin/proposals`, `POST .../proposals/:id/resolve`)
  - [x] Weekly Memory Health Report stored as memory entity
  - [ ] Conflict detection: identify contradicting entities via LLM (not yet implemented; dedup uses cosine only)
  - [ ] LLM API integration for curator synthesis (currently uses heuristic text summarization, not an LLM call)

### Phase OB-6 — Integration Interfaces

- [x] **OB-6.1: CLI for Human Memory Management**
  - [x] Command: `memory list`
  - [x] Command: `memory get`
  - [x] Command: `memory add`
  - [x] Command: `memory forget`
  - [x] Command: `memory search`
  - [x] Command: `memory approve`
  - [x] Command: `audit export`
  - [x] Command: `health`
  - [x] Command: `watch` (repository dir sync daemon)
  - [x] Command: `token` (generate scoped JWT-like token)
  - [x] Human approval workflow: CLI-prompted review of pending curator proposals via `memory approve`
- [x] **OB-6.2: MCP Server**
  - [x] Transport: stdio (default)
  - [x] Transport: HTTP/SSE (`/mcp`, `/mcp/message`, `/mcp/sse`)
  - [x] Tool: `memory_search`
  - [x] Tool: `memory_note`
  - [x] Tool: `memory_forget`
  - [x] Tool: `memory_correct`
  - [x] Tool: `memory_browse`
  - [x] Tool: `context_compile`
  - [x] All tool calls log to `memory_audit_log` via service layer
  - [ ] Trust tier enforcement in MCP layer (MCP handlers use TrustTier 4 unconditionally)
- [x] **OB-6.3: Repository Convention Synchronization**
  - [x] Watch `.openbrain/brain.md` and `.openbrain/session.md` within directories
  - [x] Changes to `brain.md` → ingest as L2 entities with `provenance.kind = file_sync`
  - [x] `session.md` → L1 entities with `provenance.kind = session`
  - [x] CLI daemon: `openbrain watch --dir ./` polls on configurable interval
  - [x] Server endpoint: `POST /internal/v1/namespaces/:id/sync` triggers sync
  - [ ] L2/L3 promotions → optional write-back to `brain.md` (not yet implemented)

### Phase OB-7 — Modern Admin Web UI

- [x] **OB-7.1: Dashboard and Metrics (partial)**
  - [x] `GET /api/v1/admin/dashboard` — JSON dashboard metrics endpoint
  - [x] Server-rendered admin HTML page at `GET /admin` showing dashboard + proposals
  - [x] Dashboard displays: memories, tier distribution, stale memory ratio, proposal acceptance rate, knowledge gap count, top accessed entities
  - [x] `POST /api/v1/admin/daydream` — manually trigger curator generation
  - [ ] Full modern React-based UI (not yet built; current UI is server-rendered HTML)
- [x] **OB-7.2: Administration and Maintenance (partial via REST)**
  - [x] Full admin CRUD on memories via REST API (`/api/v1/memories`)
  - [x] Memory proposal review via REST (`GET/POST /admin/proposals`, `.../proposals/:id/resolve`)
  - [x] Background jobs running: promotion (6h), decay (24h), health report (7d)
  - [ ] Dedicated admin UI panels for proposal review, namespace management, and job monitoring
  - [ ] L2→L3 promotion approval routing to Vashandi board (DECISION-10)

---

## 4. Detailed Phase Descriptions

### Phase OB-0 — Bootstrap & Cross-System Foundation

#### OB-0.1 — Bootstrap Go Module & Project Structure

Target directory layout:

```
openbrain/
  go.mod                 (module github.com/chifamba/vashandi/openbrain)
  cmd/
    openbrain/           (main binary: server + CLI combined)
      main.go
  internal/
    server/              (HTTP handlers)
    storage/             (db layer: pgx, migrations)
    memory/              (core memory operations)
    context/             (context compilation engine)
    curator/             (LLM curator agent)
    registry/            (agent registry)
    audit/               (immutable audit log)
    mcp/                 (MCP server)
  pkg/
    api/                 (public API types, shared with clients)
  migrations/            (golang-migrate SQL files)
  docker/
    Dockerfile
    docker-compose.dev.yml
```

#### OB-0.2 — Define Vashandi↔OpenBrain Integration Interface

Summary of decisions:
- Transport: HTTP/JSON (internal REST)
- Auth: Vashandi generates a service token per company at company creation time; stored as `company_secrets` in Vashandi; passed as `Authorization: Bearer <service-token>` on OpenBrain internal calls
- Vashandi lifecycle events that trigger OpenBrain calls: agent created, agent archived, company archived, run completed (post-run capture), run starting (pre-run hydrate)
- Entity mapping: Vashandi `agent` → OpenBrain `registered_agent`; Vashandi `company` → OpenBrain `namespace`; Vashandi `issue` → memory `source_ref` of kind `issue`; Vashandi `heartbeat_run` → memory `source_ref` of kind `run`

#### OB-0.3 — Company-Scoped Memory Namespacing

- Every OpenBrain table includes `namespace_id uuid not null`
- Storage layer functions accept `namespace_id` as a non-optional parameter
- API layer extracts `namespace_id` from the service token (token is scoped to one company)
- Index: every table has `(namespace_id, ...)` composite index as primary access path
- Teams within a company: sub-namespace via `team_id` column (optional second isolation dimension)

---

### Phase OB-1 — Core Storage Infrastructure

#### OB-1.2 — Typed Memory Entity Schema

```sql
-- Core entity table
CREATE TABLE memory_entities (
  id             uuid primary key default gen_random_uuid(),
  namespace_id   uuid not null,            -- maps to Vashandi company_id
  team_id        uuid,                     -- optional team sub-namespace
  entity_type    text not null,            -- fact | decision | task | constraint | adr | note
  title          text,
  content        text not null,
  embedding      vector(1536),             -- OpenAI ada-002 / configurable dimension
  provenance     jsonb not null,           -- source_ref: kind, entity ids, timestamps
  identity       jsonb not null,           -- created_by_agent_id, created_via (mcp|api|cli|auto)
  metadata       jsonb,                    -- arbitrary provider/user metadata
  tier           int not null default 0,   -- 0=L0 (ephemeral) .. 3=L3 (core knowledge)
  version        int not null default 1,
  is_deleted     bool not null default false,
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now()
);

-- Relationship graph (adjacency)
CREATE TABLE memory_edges (
  id             uuid primary key default gen_random_uuid(),
  namespace_id   uuid not null,
  from_entity_id uuid not null references memory_entities(id),
  to_entity_id   uuid not null references memory_entities(id),
  edge_type      text not null,            -- supports | contradicts | refines | relates_to | supersedes
  weight         float not null default 1.0,
  metadata       jsonb,
  created_at     timestamptz not null default now()
);

-- Version history (append-only)
CREATE TABLE memory_entity_versions (
  id             uuid primary key default gen_random_uuid(),
  namespace_id   uuid not null,
  entity_id      uuid not null references memory_entities(id),
  version        int not null,
  content        text not null,
  embedding      vector(1536),
  changed_by     jsonb not null,
  change_reason  text,
  created_at     timestamptz not null default now()
);
```

**Required indexes:**
```sql
CREATE INDEX ON memory_entities (namespace_id, entity_type, tier);
CREATE INDEX ON memory_entities (namespace_id, is_deleted, updated_at DESC);
CREATE INDEX ON memory_entities USING ivfflat (embedding vector_cosine_ops);
CREATE INDEX ON memory_edges (namespace_id, from_entity_id, edge_type);
CREATE INDEX ON memory_edges (namespace_id, to_entity_id, edge_type);
CREATE INDEX ON memory_entity_versions (namespace_id, entity_id, version);
```

**Embedding generation:** Configurable embedding provider. Default: OpenAI text-embedding-3-small (1536d). Embedding happens asynchronously on write; entity is immediately queryable by keyword until embedding completes.

**Single-node Docker deployment acceptance criteria:**
- `docker compose up openbrain` brings up service with Postgres+pgvector; no external dependencies
- All CRUD operations functional in single-container mode

---

### Phase OB-2 — Multi-Tier Memory Lifecycle

#### OB-2.1 — Tier Model Definition

| Tier | Label | Description | Default TTL | Promotion Trigger |
|---|---|---|---|---|
| L0 | Ephemeral | Session-only, not indexed for long-term retrieval | 24h | explicit note or repeated access |
| L1 | Working | Active project context, readily accessible | 30 days | accessed 3+ times or manually promoted |
| L2 | Reference | Stable project knowledge, indexed | indefinite | curator agent synthesis or manual |
| L3 | Core | Foundational facts, ADRs, constraints | permanent until explicit forget | human approval required |

#### OB-2.2 — Promotion Logic

- Background job runs every 6 hours; configurable interval
- Promotion rules:
  - L0→L1: entity accessed ≥ 3 times within 24h, OR manually flagged
  - L1→L2: entity accessed ≥ 5 times within 30 days, OR curator proposes
  - L2→L3: **requires human approval** (routed through Vashandi approval gate)
- Promotion creates a version record with `change_reason = "tier_promotion"`

#### OB-2.3 — Decay Logic

- Decay job runs daily
- Decay rules:
  - L0: auto-delete after TTL (default 24h); configurable per namespace
  - L1: demote to L0 if not accessed within 30 days; configurable
  - L2/L3: no automatic decay; curator agent may propose demotion (human approval required)
- Decay metrics tracked: `stale_memory_ratio` (L0 decayed / total created L0)

#### OB-2.4 — Versioning & Rollback

- Every entity update creates an immutable version record in `memory_entity_versions`
- `POST /api/v1/memories/:id/rollback` — restores a prior version (creates a new version record, does not delete existing versions)
- Self-deletion: `DELETE /api/v1/memories/:id` sets `is_deleted=true` and records deletion in version history; hard delete is admin-only

---

### Phase OB-3 — Agent Identity & Governance

#### OB-3.1 — Agent Registry

```sql
CREATE TABLE registered_agents (
  id                uuid primary key default gen_random_uuid(),
  namespace_id      uuid not null,
  vashandi_agent_id uuid not null,          -- Vashandi agent.id
  name              text not null,
  trust_tier        int not null default 1, -- 1=Read, 2=Write, 3=Promote, 4=Delete
  recall_profile    jsonb not null,         -- verbosity, format, token_limit, preferred_types
  is_active         bool not null default true,
  registered_at     timestamptz not null default now(),
  deregistered_at   timestamptz
);

-- Unique: one registration per Vashandi agent per namespace
CREATE UNIQUE INDEX ON registered_agents (namespace_id, vashandi_agent_id) WHERE is_active = true;
```

**Trust tier permissions:**
| Tier | Label | Read | Write (L0/L1) | Write (L2/L3) | Promote | Delete |
|---|---|---|---|---|---|---|
| 1 | Read | ✅ | ❌ | ❌ | ❌ | ❌ |
| 2 | Contributor | ✅ | ✅ | ❌ | ❌ | ❌ |
| 3 | Curator | ✅ | ✅ | propose only | propose only | propose only |
| 4 | Admin | ✅ | ✅ | ✅ | ✅ | ✅ (soft) |

#### OB-3.2 — Immutable Audit Log

```sql
CREATE TABLE memory_audit_log (
  id             bigserial primary key,
  namespace_id   uuid not null,
  agent_id       uuid,                       -- registered_agents.id; null for system/human
  actor_kind     text not null,              -- agent | human | system | curator
  action         text not null,              -- read | write | update | delete | promote | rollback | forget | search
  entity_id      uuid,
  entity_type    text,
  before_hash    text,                       -- SHA-256 of content before action
  after_hash     text,                       -- SHA-256 of content after action
  chain_hash     text,                       -- SHA-256(prev_chain_hash || id || created_at || after_hash)
  request_meta   jsonb,                      -- source IP, MCP session ID, run ID
  created_at     timestamptz not null default now()
);
```

**Immutability enforcement:** Write path to `memory_audit_log` is append-only at the application level. DB user for OpenBrain has no DELETE or UPDATE privilege on `memory_audit_log`.

---

### Phase OB-4 — Context Engine

#### OB-4.1 — Context Compilation & Token Budgeting

**Retrieval algorithm:**
1. Vector similarity search: top-K candidates by cosine similarity to task query (default K=50)
2. Re-rank by: tier weight (L3=4x, L2=2x, L1=1x, L0=0.5x), recency decay, agent recall profile preferences
3. Token budget enforcement: pack highest-ranked snippets until agent's `token_limit` is reached
4. Format output per agent's `recall_profile.format` (markdown | json | xml)

**Optimization strategies:**
- Embedding cache: skip re-embedding for repeated queries within 5 minutes (LRU cache, backed by Redis)
- IVFFlat index with `lists=100` (tunable)
- Async pre-embedding on write path; compile blocks only when embedding is missing

#### OB-4.2 — Proactive Context Delivery

**Trigger types:**
| Trigger | Source | Action |
|---|---|---|
| Session start | Vashandi: heartbeat invoke | Pre-run hydrate context packet |
| Task checkout | Vashandi: `POST /issues/:issueId/checkout` | Task-specific memory surfacing |
| Branch creation | Git webhook or Vashandi run context | ADR + constraint surfacing |
| Test failure | Vashandi run output | Related past failures + fixes |
| Git commit/push | Git webhook or Vashandi run summary | Post-run memory capture |

---

### Phase OB-5 — Self-Evolution & Curation

#### OB-5.1 — LLM Curator Agent (Gachlaw)

The Curator Agent is a background process within OpenBrain, not a Vashandi agent. It uses an LLM to reason over memory and produce proposals — but cannot self-approve.

**Curator actions (all require approval before execution):**
- De-duplicate: detect near-duplicate entities (cosine similarity > 0.95), propose merge
- Synthesize: group related L1 entities into a new L2 entity, propose promotion
- Conflict detection: identify contradicting entities (via edge type or LLM classification), propose resolution
- Knowledge gap detection: identify questions frequently asked by agents with empty recall, report as gaps
- Demotion: propose L2→L1 demotion for entities unused for 60 days

**Weekly Memory Health Report:**
- Generated each Monday (UTC)
- Includes: stale memory ratio, curator proposal acceptance rate, knowledge gap count, top accessed entities, entity type distribution by tier
- Stored as a Vashandi `document` attached to a system-created `strategy` issue for board visibility

---

### Phase OB-6 — Integration Interfaces

#### OB-6.1 — CLI for Human Memory Management

```sh
openbrain memory list --namespace <company-id> [--type fact|decision|adr] [--tier 0-3]
openbrain memory get <entity-id>
openbrain memory add --type <type> --content "..." [--tier 1]
openbrain memory forget <entity-id>
openbrain memory search "<query>" [--top-k 10]
openbrain memory approve <proposal-id>   # routes to Vashandi approval gate
openbrain audit export --format jsonld|sqlite --out ./audit.jsonld
openbrain health
```

#### OB-6.2 — MCP Server

**Transport:** stdio (default for local use) + HTTP/SSE (for remote or docker-based deployment)

**Exposed tools:**
```
memory_search      { query: string, topK?: int, agentId: string, namespaceId: string }
memory_note        { content: string, type: string, agentId: string, namespaceId: string }
memory_forget      { entityId: string, agentId: string, namespaceId: string }
memory_correct     { entityId: string, correction: string, agentId: string, namespaceId: string }
memory_browse      { filters?, agentId: string, namespaceId: string }
context_compile    { taskQuery: string, tokenBudget?: int, agentId: string, namespaceId: string }
```

#### OB-6.3 — Repository Convention Synchronization

- Watch `brain.md` (curated knowledge) and `session.md` (working task) within `.openbrain/` directories
- Changes to `brain.md` → ingest into OpenBrain as L2 entities with `provenance.kind = file_sync`
- Changes to OpenBrain entities promoted to L2/L3 → optionally write back to `brain.md` (configurable)
- `.openbrain/` directory is the local project-level OpenBrain context; supports multiple projects per namespace
- File watcher implemented as optional CLI daemon: `openbrain watch --dir ./`

---

### Phase OB-7 — Modern Admin Web UI

#### OB-7.1 — Dashboard and Metrics

- Modern React-based UI (aligning with Vashandi's stack)
- Dashboard with: thoughts, memories, tier distribution by namespace, stale memory ratio, curator proposal acceptance rate, knowledge gap count, top accessed entities
- Capability to manually trigger "day dreaming" (synthesis, deduplication, and conflict resolution by the LLM curator)

#### OB-7.2 — Administration and Maintenance

- Full admin CRUD on memories, namespaces, and agent registries
- Memory proposal review panel for curator proposals (DECISION-07)
- Monitor and manage maintenance jobs: promotion scheduler, decay job, audit export
- L2→L3 promotion approval interface (routes decisions back to Vashandi board approval gate per DECISION-10)

---

## 5. Phase Summary

| Phase | Focus | Issues/Gaps | Depends On |
|---|---|---|---|
| OB-0.1 | Bootstrap Go module & project structure | GAP-02 | — |
| OB-0.2 | Vashandi↔OpenBrain integration interface | GAP-01 | OB-0.1 |
| OB-0.3 | Company namespace isolation | GAP-03 | OB-0.1 |
| OB-1.1 | PostgreSQL + pgvector setup | #36 | OB-0 complete |
| OB-1.2 | Memory entity schema | #36 | OB-1.1 |
| OB-1.3 | CRUD operations | #36 | OB-1.2 |
| OB-2 | Multi-tier memory lifecycle | #37 | OB-1 complete |
| OB-3.1 | Agent registry + trust tiers | #38 | OB-1 complete, OB-0.2 |
| OB-3.2 | Immutable audit log | #39 | OB-1 complete |
| OB-4.1 | Context compilation | #40 | OB-1, OB-2, OB-3 |
| OB-4.2 | Proactive context delivery | #41 | OB-4.1, OB-0.2 |
| OB-5 | LLM Curator Agent | #42 | OB-2, OB-3, OB-4 |
| OB-6.1 | CLI | #43 | OB-1, OB-3 |
| OB-6.2 | MCP server | #43 | OB-1, OB-3, OB-4 |
| OB-6.3 | Repo convention sync | #44 | OB-6.1 |
| OB-7 | Modern Admin Web UI | New | OB-1, OB-3, OB-5 |

---

## 6. Cross-Project Integration Roadmap

This maps the 16 roadmap gaps onto the OpenBrain phase plan.

### Phase I-0 — Blockers (must precede all OpenBrain epic work)

| Gap | Work | Addressed In |
|---|---|---|
| GAP-01 | Vashandi↔OpenBrain API contract | OB-0.2 |
| GAP-02 | OpenBrain Go module bootstrap | OB-0.1 |
| GAP-03 | Company-scoped namespace isolation | OB-0.3 |

### Phase I-1 — Wiring (enables local dev + basic integration)

| Gap | Work | Addressed In |
|---|---|---|
| GAP-04 | Vashandi memory plugin wrapping OpenBrain | Vashandi V2.1 (MemoryAdapter backed by OpenBrain) |
| GAP-05 | Agent identity federation | OB-3.1 (Vashandi agent_id in registered_agents) |
| GAP-08 | Docker service topology | OB-0.1 (docker-compose.dev.yml) |

### Phase I-2 — Safety (lifecycle integrity, cost, resilience, testing)

| Gap | Work | Addressed In |
|---|---|---|
| GAP-06 | Agent lifecycle → OpenBrain namespace | OB-0.2 (Vashandi lifecycle webhooks) |
| GAP-07 | Memory operation costs in Vashandi budget | Vashandi V2.1 (MemoryUsage → cost_events) |
| GAP-10 | OpenBrain unavailability fallback | Vashandi V2.1 (circuit breaker in memory service surface) |
| GAP-15 | Integration tests | After I-1 complete: contract test suite in `tests/integration/` |

### Phase I-3 — Polish

| Gap | Work | Addressed In |
|---|---|---|
| GAP-09 | Local dev DX | OB-0.1 + docker-compose + updated DEVELOPING.md |
| GAP-11 | OpenBrain API versioning | All OpenBrain REST endpoints under `/api/v1/`; stability contract |
| GAP-12 | CEO Chat → OpenBrain ingestion | Vashandi V2.6 (CEO Chat) + OB-4.2 (post-thread capture) |
| GAP-13 | Company onboarding memory bootstrap | Vashandi V0.5 (Onboarding V2) seeds brain.md; OB-6.3 ingests it |
| GAP-14 | OpenBrain graph schema design | OB-1.2 (adjacency table schema), formalized in `openbrain/doc/GRAPH-SCHEMA.md` |
| GAP-16 | Curator proposals through Vashandi approvals | OB-5 (curator) + Vashandi V2.3 adjacent (new approval type `memory_curator_proposal`) |

---

## 7. API Contracts

### 7.1 OpenBrain External REST API (`/api/v1/`)

Auth: `Authorization: Bearer <agent-or-user-token>` — token identifies namespace + agent.

```
# Memory CRUD
POST   /api/v1/memories
GET    /api/v1/memories/:id
PATCH  /api/v1/memories/:id
DELETE /api/v1/memories/:id
GET    /api/v1/memories
POST   /api/v1/memories/search
POST   /api/v1/memories/:id/rollback

# Graph edges
POST   /api/v1/memories/edges
GET    /api/v1/memories/:id/edges
DELETE /api/v1/memories/edges/:edgeId

# Context
POST   /api/v1/context/compile

# Audit
GET    /api/v1/audit/log
GET    /api/v1/audit/export

# Agent registry (human-managed)
GET    /api/v1/agents
GET    /api/v1/agents/:agentId
PATCH  /api/v1/agents/:agentId    # update trust tier or recall profile

# Health
GET    /api/v1/health
```

### 7.2 OpenBrain Internal API (`/internal/v1/`) — Vashandi↔OpenBrain Only

Auth: `Authorization: Bearer <vashandi-service-token>` — one token per company, generated at company creation.

```
# Namespace lifecycle
POST   /internal/v1/namespaces                           # called on Vashandi company creation
DELETE /internal/v1/namespaces/:namespaceId              # called on Vashandi company archive → exports memories

# Agent lifecycle
POST   /internal/v1/namespaces/:namespaceId/agents       # called on Vashandi agent creation
DELETE /internal/v1/namespaces/:namespaceId/agents/:id   # called on Vashandi agent archive

# Triggers (proactive context)
POST   /internal/v1/namespaces/:namespaceId/triggers/run_start    # pre-run hydrate
POST   /internal/v1/namespaces/:namespaceId/triggers/run_complete # post-run capture
POST   /internal/v1/namespaces/:namespaceId/triggers/checkout     # task checkout context

# Bulk ingest (from Vashandi memory plugin)
POST   /internal/v1/namespaces/:namespaceId/memories/ingest-batch
```

### 7.3 Vashandi Memory Plugin Adapter Contract

Implements the `MemoryAdapter` interface defined in `vashandi/doc/plans/2026-03-17-memory-service-surface-api.md`. The OpenBrain provider adapter translates calls to internal API endpoints.

```ts
// Existing contract (from 2026-03-17 plan) — unchanged
interface MemoryAdapter {
  key: string;
  capabilities: MemoryAdapterCapabilities;
  write(req: MemoryWriteRequest): Promise<{ records?, usage? }>;
  query(req: MemoryQueryRequest): Promise<MemoryContextBundle>;
  get(handle: MemoryRecordHandle, scope: MemoryScope): Promise<MemorySnippet | null>;
  forget(handles: MemoryRecordHandle[], scope: MemoryScope): Promise<{ usage? }>;
}
```

### 7.4 New Vashandi Approval Types

```ts
type ApprovalType =
  | "hire_agent"
  | "approve_ceo_strategy"
  | "memory_curator_proposal"    // NEW: routes OpenBrain curator proposals to board
  | "budget_breach";             // NEW: board notification on budget hard stop
```

`memory_curator_proposal` payload shape:
```json
{
  "proposalKind": "deduplicate | synthesize | conflict_resolve | promote | demote",
  "entityIds": ["uuid"],
  "proposalText": "...",
  "evidenceSummary": "...",
  "openbrainProposalId": "uuid"
}
```

---

## 8. Non-Functional Requirements

### 8.1 OpenBrain NFRs

| NFR | Target | Enforcement |
|---|---|---|
| Context compilation latency (p95) | < 500ms (issue #40) | Embedding cache + IVFFlat index |
| Memory CRUD latency (p95) | < 100ms | Composite indexes on namespace_id paths |
| Memory namespace isolation | Zero cross-namespace data leakage | namespace_id enforced in every storage function |
| Audit log immutability | Append-only; chain hash verifiable | DB role has no DELETE/UPDATE on audit table |
| Audit log export | JSON-LD + SQLite (issue #39) | Export endpoint |
| Stale memory ratio (L0) | Tracked as metric, not bounded | Weekly health report |
| Single-node Docker deployment | Required (issue #36) | Docker Compose dev profile ships with service |
| Agent registration requirement | 403 for unregistered agents | Middleware on all agent-origin API calls |
| Curator proposals never self-approved | 100% | Proposals route through admin UI approval panel |
| Memory operation logging | 100% of reads, writes, deletes | Audit log in every storage function |

### 8.2 Cross-System NFRs

| NFR | Target | Enforcement |
|---|---|---|
| OpenBrain unavailability impact | Degraded mode only (Vashandi continues) | Circuit breaker in Vashandi V2.1 memory plugin |
| Memory costs visible in Vashandi | Every adapter MemoryUsage → cost_events | Memory plugin adapter responsibility |
| Service token security | Tokens stored as company_secrets, hashed | Existing Vashandi secret manager |
| Agent archive → namespace cleanup | < 24h after archive | Async job triggered by Vashandi lifecycle webhook |
| Integration test coverage | Key scenarios: task→memory ingest, company isolation, budget enforcement, fallback | `tests/integration/vashandi-openbrain/` |
| API versioning | All OpenBrain external endpoints under `/api/v1/` | Router prefix; v2 added only on breaking changes |

---

## 9. Assumptions

1. **pgvector is sufficient for per-company scale.** At the expected scale of 10K–100K memory entities per company namespace, a single PostgreSQL instance with pgvector IVFFlat index provides < 500ms context compilation. This assumption should be validated with a load test before OB-4.1 is considered complete.

2. **OpenBrain is co-deployed with Vashandi.** The recommended topology has both services running in the same Docker Compose environment in development and in the same infrastructure in production. If they must deploy independently (different teams, different clusters), the internal API will need additional security hardening beyond service tokens.

3. **The `go.work` workspace can accommodate both `vashandi/backend` and `openbrain`.** Current `go.work` already covers `vashandi/backend`. Adding `openbrain/` should be straightforward.

4. **Team isolation is optional and company-configurable.** Not all companies need team-level access control. The sub-namespace `team_id` design makes it a per-binding configuration so simple deployments are not burdened.

5. **OpenBrain has its own standalone UI for governance (DECISION-07).** A modern React-based UI will be built to manage memories, namespaces, agent registries, curator proposals, and maintenance tasks.

6. **Embedding calls are to an external provider (OpenAI by default).** Local embedding (Ollama) is a future option. The initial deployment requires an embedding API key.

7. **The Rust bindings mentioned in issue #43 are deferred.** Rust bindings for high-performance memory access are not included in this plan. They are a future optimization once the Go service is established and performance is measured.

8. **OpenBrain's "Gachlaw" curator agent uses an external LLM (not a self-hosted model).** The curator calls an LLM API (configurable provider, default OpenAI) for de-duplication, synthesis, and conflict detection. This is a cost-bearing operation tracked via OpenBrain's own memory operation cost accounting.

---

*All decisions marked ⚠ DECISION-N require human confirmation before the affected phase begins. Send confirmations or corrections and this plan will be updated accordingly.*
