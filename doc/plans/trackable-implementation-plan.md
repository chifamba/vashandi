# Vashandi + OpenBrain Consolidated Master Plan

Date: 2026-04-12
Status: Active Implementation Plan

*This document is the single source of truth for the Vashandi and OpenBrain implementation, consolidating the Complete Platform Plan, Roadmap Gaps, and Trackable Checklists.*

---

## 1. Trackable Implementation Checklist (with Gaps and Architecture Updates)

# Vashandi + OpenBrain Trackable Implementation Plan

Date: 2026-04-12
Status: Active Implementation Plan

This document synthesizes the phases outlined in the Complete Platform Plan (2026-04-11) and the Gap Analysis (2026-04-11) into a single, trackable checklist. It also updates architectural decisions to incorporate Redis and multi-protocol support for OpenBrain.

---

## Architectural Updates

### 1. OpenBrain Multi-Protocol Support
To ensure maximum compatibility and integration flexibility, OpenBrain will support three protocols simultaneously:
- **HTTP/REST (JSON):** Primary protocol for internal monorepo communication (e.g., Vashandi internal API calls) and external web clients.
- **gRPC:** High-performance, low-latency binary protocol for bulk ingest, large context compilations, and future Rust bindings.
- **MCP (Model Context Protocol):** Native support for standardized LLM interactions, allowing immediate use of OpenBrain tools by standard MCP clients.

*Note: The previous decision (DECISION-05) prioritizing REST only is hereby overridden.*

### 2. Redis Caching & Message Broker Integration
**Recommendation:** Integrating a Redis cache is highly advised to meet NFRs (e.g., UI read latency, embedding cache).
**Image/Version:** `redis:8-alpine`

**Usage in Vashandi:**
- **WebSockets Pub/Sub:** Scalable event routing for real-time board updates.
- **Rate Limiting & Throttling:** Managing agent API call quotas and budget hard stops.
- **Query Caching:** Caching hot paths (e.g., company dashboards, active agent lists).

**Usage in OpenBrain:**
- **Embedding Cache:** Fast retrieval of previously computed LLM embeddings to save costs and latency.
- **Async Queues:** Processing offline memory compilation, background deduplication jobs, and graph maintenance without blocking the main event loop.

---

## Phase 0 — Foundation (Blockers)

These items must be completed before starting OpenBrain epic work.

- [x] **Task 0.1: Bootstrap OpenBrain Project Structure (GAP-02)**
  - Create standard Go module in `openbrain/` (`go mod init github.com/chifamba/vashandi/openbrain`).
  - Add `openbrain` to the monorepo root `vashandi/go.work`. Note: `go.work` is currently located in `vashandi/`, so we must reference `../openbrain`.
  - Configure `pnpm-workspace.yaml` and CI pipelines if needed.
- [x] **Task 0.2: Define OpenBrain Service Topology & Redis Setup (GAP-08)**
  - Update `docker-compose.yml` to include `redis:8-alpine`.
  - Update `docker-compose.yml` to run OpenBrain as a separate service alongside PostgreSQL/pgvector and Vashandi.
- [x] **Task 0.3: Define Vashandi ↔ OpenBrain Integration Interface (GAP-01)**
  - Document the exact gRPC protobufs and REST OpenAPI specs for the integration.
  - Detail HTTP/REST, gRPC, and MCP interfaces.
- [x] **Task 0.4: OpenBrain Company-Scoped Memory Namespacing (GAP-03)**
  - Define schema enforcing row-level `namespace_id` (mapping to Vashandi `company_id`) in Postgres and Redis keys.

## Phase 1 — V1 Completion & Wiring

- [x] **Task 1.1: Complete Vashandi Go Backend Port (V0.1)**
  - [x] Complete DB model ports (approval comments, logos, etc.).
  - Implement Go HTTP server with all V1 REST routes.
  - Integrate Redis for WebSocket routing and caching.
- [x] **Task 1.2: Vashandi Memory Plugin: OpenBrain Adapter (GAP-04)**
  - [x] Implement the `MemoryAdapter` interface to bridge Vashandi and OpenBrain via REST/gRPC.
- [x] **Task 1.3: Agent Identity Federation (GAP-05)**
  - [x] Define auth mechanism for external API access to OpenBrain (e.g., Agent-scoped JWT).
- [x] **Task 1.4: Budget Policies & Enforcement (V0.2) + OpenBrain Costs (GAP-07)**
  - [x] Implement project-level budgets.
  - [x] Surface OpenBrain context compilation costs in Vashandi's budget engine.

## Phase 2 — Safety, Storage & Core Governance

- [x] **Task 2.1: Implement OpenBrain Vector Storage & Graph Schema (GAP-14)**
  - Create Postgres tables with pgvector IVFFlat indexes and adjacency logic.
- [x] **Task 2.2: Async Job Infrastructure (Redis)**
  - Setup `redis:8-alpine` message queues in OpenBrain for tier promotion and ingest batches.
- [x] **Task 2.3: Sync Agent Lifecycle Events (GAP-06)**
  - Trigger Vashandi webhooks/gRPC calls to OpenBrain on agent creation/archival.
- [x] **Task 2.4: Fallback Strategy (GAP-10)**
  - [x] Implement circuit breakers in Vashandi's adapter so system degrades gracefully if OpenBrain is down.
- [x] **Task 2.5: Integration Tests (GAP-15)**
  - [x] Build cross-system contract verification test suite.

## Phase 3 — Intelligence Layer & Polish

- [x] **Task 3.1: Proactive Context Delivery**
  - Implement pre-run hydration and post-run capture endpoints.
- [x] **Task 3.2: LLM Curator Agent & Approvals (GAP-16)**
  - Implement Curator Agent logic in OpenBrain.
  - Route Curator promotion/deduplication proposals to Vashandi UI for human approval.
- [x] **Task 3.3: CEO Chat Ingestion (GAP-12)**
  - Build pipeline to classify and ingest high-value strategy context from Vashandi to OpenBrain.
- [x] **Task 3.4: Local Dev Setup (GAP-09) & Initial Onboarding (GAP-13)**
  - Seed new company namespaces from `brain.md` and default READMEs.
  - Finalize combined dev commands (`pnpm dev` handling Go OpenBrain + Go Vashandi + Node UI).
- [x] **Task 3.5: External API Stability (GAP-11)**
  - Publish stable v1 REST, gRPC, and MCP endpoints.


---

## 2. Gap Analysis (Historical Context)

# Vashandi + OpenBrain Roadmap Gap Analysis

Date: 2026-04-11
Status: Proposed issues for review
Source: Gap analysis against open issues #31–44

---

## Summary

The 14 existing open issues cover two distinct bodies of work:

- **Vashandi (issues #31–35)**: Multi-agent orchestration, teams/auth, agent memory, MCP management, observability.
- **OpenBrain (issues #36–44)**: 5 epics covering storage, governance, context engine, self-evolution, and interfaces.

The gap analysis identified **16 missing issues** needed before either system goes deep into implementation. These are captured below and can be created in bulk using:

```bash
bash scripts/create-roadmap-gap-issues.sh
```

---

## Critical Gaps (Blockers)

These three must be resolved before any OpenBrain epic work begins, as everything else depends on them.

### GAP-01 — Define Vashandi ↔ OpenBrain Integration Interface
**Label**: `roadmap-gap, integration`

No issue defines the actual API/protocol boundary between Vashandi and OpenBrain. Issue #33 references "Open Brain Synchronization" from the Vashandi side only.

Needs to define:
- Protocol/transport (HTTP, gRPC, MCP, event stream)
- Minimal API contract (ingest, query, forget, context-packet)
- Which Vashandi lifecycle events trigger memory writes
- Caller identity and error contract
- How Vashandi entities (issue, comment, run, agent) map to OpenBrain memory entity types

---

### GAP-02 — Bootstrap OpenBrain Project Structure in Monorepo
**Label**: `roadmap-gap, openbrain`

OpenBrain has 9 epic issues but no physical location in the repo. No issue establishes where it lives, what its build tooling is, or how it relates to existing packages.

Needs to define:
- Directory placement (`packages/openbrain/`, `backend/openbrain/`, or top-level `openbrain/`)
- Language/runtime and fit with `go.work` and `pnpm-workspace.yaml`
- Shared type strategy with `packages/shared/`
- Build, typecheck, and test pipeline integration

---

### GAP-03 — OpenBrain Company-Scoped Memory Namespacing
**Label**: `roadmap-gap, openbrain, integration`

Vashandi is explicitly multi-company with strict data isolation. OpenBrain epics are written for a single-tenant system with no mention of company partitioning or isolation. This is a security-critical gap.

Needs to define:
- Namespace model (separate schemas, row-level `company_id`, separate vector namespaces)
- Where isolation is enforced (storage layer vs. API layer)
- Whether team-scoped boundaries from Vashandi issue #32 also apply in OpenBrain

---

## High Priority Gaps

### GAP-04 — Vashandi Memory Plugin: OpenBrain Provider Adapter
**Label**: `roadmap-gap, vashandi, integration`

`doc/memory-landscape.md` defines a two-layer memory model (control-plane binding + provider adapter). No issue exists for building the Vashandi-side plugin that wraps OpenBrain as a memory provider.

---

### GAP-05 — Agent Identity Federation: Vashandi ↔ OpenBrain Trust
**Label**: `roadmap-gap, integration`

Vashandi has agent API keys. OpenBrain has its own Agent Registry and Trust Tiers (issue #38). No issue defines how these two identity systems relate or how a credential flows across the boundary.

---

### GAP-06 — Sync Agent Lifecycle Events with OpenBrain Registry
**Label**: `roadmap-gap, integration`

When Vashandi creates or archives an agent, its OpenBrain namespace and registry entry are unaffected. This causes memory leakage and orphaned data.

Key lifecycle events: agent created → register; agent archived → close namespace; company archived → export/archive all memories.

---

### GAP-07 — Cost Model: Include OpenBrain Memory Operation Spend
**Label**: `roadmap-gap, vashandi`

Vashandi enforces hard budget stops on token spend. OpenBrain's Context Compilation (issue #40) consumes tokens for embedding, retrieval, and LLM curation. Without surfacing these costs into Vashandi's budget engine, memory spend is invisible and agents can bypass budget limits.

---

### GAP-08 — OpenBrain Deployment: Service Topology and Docker Configuration
**Label**: `roadmap-gap, openbrain, infrastructure`

OpenBrain requires pgvector. Vashandi uses embedded PGlite in dev. No issue defines whether OpenBrain is a sidecar, a separate service, a plugin process, or an embedded library. This affects every other integration and local dev issue.

---

## Medium Priority Gaps

### GAP-09 — Local Dev Setup: Run Vashandi + OpenBrain Together
**Label**: `roadmap-gap, developer-experience`

No issue covers the combined local developer experience — updated docs, `pnpm dev` integration, environment variables, Docker Compose dev profile, and health checks for both services.

---

### GAP-10 — OpenBrain Unavailability: Agent Fallback Strategy
**Label**: `roadmap-gap, vashandi, resilience`

No defined behavior when OpenBrain is unavailable. Options: silent skip, queue writes/skip reads, fail fast, circuit breaker. This is especially important given OpenBrain's proactive context delivery model (issue #41).

---

### GAP-11 — OpenBrain External Service API: Versioning and Stability Contract
**Label**: `roadmap-gap, openbrain`

The vision is for OpenBrain to serve "other services in the future" beyond Vashandi. Without API versioning and a stability contract, OpenBrain will be Vashandi-specific and hard to generalize.

---

### GAP-12 — CEO Chat Context: OpenBrain Knowledge Ingestion
**Label**: `roadmap-gap, integration`

CEO Chat (on the Vashandi roadmap) produces the highest-value knowledge for a memory system. No issue defines how CEO Chat outputs flow into OpenBrain — entity classification, provenance, human approval gate, and agent visibility scoping.

---

### GAP-13 — Company Onboarding: Seed OpenBrain Memory Namespace
**Label**: `roadmap-gap, vashandi`

New company namespaces start empty in OpenBrain. No issue covers initial knowledge bootstrap from `brain.md`, goal statement, project README, or company template memory entries. OpenBrain issue #44 covers repo-level sync but not the company-creation path.

---

### GAP-14 — OpenBrain Graph Schema: Entity Types, Edges, and Query API Design
**Label**: `roadmap-gap, openbrain`

Issue #36 mentions "hybrid vector-graph" but does not specify the graph schema, entity types, relationship types, or query API. This is foundational enough to warrant its own design issue before implementation begins.

---

### GAP-15 — Integration Tests: Vashandi ↔ OpenBrain Contract Verification
**Label**: `roadmap-gap, testing, integration`

No integration test strategy exists for cross-system behavior. Without it, the interface contract will drift. Key scenarios: task completion → memory ingest, company isolation, budget enforcement including memory costs, graceful fallback, agent archive → namespace closed.

---

### GAP-16 — Route OpenBrain Curator Proposals Through Vashandi Approval Gates
**Label**: `roadmap-gap, integration`

OpenBrain issue #42 requires human approval for Curator Agent proposals. Vashandi already has an approval gate system. Building a separate approval UI in OpenBrain fragments the operator's attention. Curator proposals should route through Vashandi's existing approval workflow.

---

## Recommended Sequencing

```
Phase 0 — Foundation (GAP-01, GAP-02, GAP-03)
  └─ Nothing else can start without these

Phase 1 — Wiring (GAP-04, GAP-05, GAP-08)
  └─ Depends on Phase 0; enables local dev

Phase 2 — Safety (GAP-06, GAP-07, GAP-10, GAP-15)
  └─ Lifecycle integrity, cost visibility, resilience, test coverage

Phase 3 — Polish (GAP-09, GAP-11, GAP-12, GAP-13, GAP-14, GAP-16)
  └─ DX, external API, CEO Chat, onboarding, graph spec, approval routing
```


---

## 3. Complete Platform Plan Details & NFRs

# Complete Platform Plan — Vashandi & OpenBrain

**Date:** 2026-04-11
**Status:** Proposed — awaiting human confirmation of highlighted decisions
**Scope:** Both projects in the monorepo: `vashandi/` (Paperclip control plane) and `openbrain/` (memory OS)
**Source inputs:** Open issues #31–44, gap analysis `2026-04-11-vashandi-openbrain-roadmap-gaps.md`, `vashandi/doc/SPEC.md`, `vashandi/doc/SPEC-implementation.md`, `vashandi/doc/PRODUCT.md`, `vashandi/doc/memory-landscape.md`, `vashandi/doc/plans/2026-03-17-memory-service-surface-api.md`, `vashandi/doc/plans/2026-03-13-features.md`, `vashandi/doc/plans/2026-03-14-budget-policies-and-enforcement.md`, `vashandi/doc/plans/2026-03-14-billing-ledger-and-reporting.md`, `vashandi/PORT_TO_GOLANG_PLAN.md`

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Current State Assessment](#2-current-state-assessment)
3. [Stack Decisions](#3-stack-decisions)
4. [Vashandi (Paperclip) — Complete Plan](#4-vashandi-paperclip--complete-plan)
5. [OpenBrain — Complete Plan](#5-openbrain--complete-plan)
6. [Cross-Project Integration Roadmap](#6-cross-project-integration-roadmap)
7. [API Contracts](#7-api-contracts)
8. [Non-Functional Requirements (NFRs)](#8-non-functional-requirements-nfrs)
9. [Key Decisions Requiring Human Confirmation](#9-key-decisions-requiring-human-confirmation)
10. [Assumptions](#10-assumptions)

---

## 1. Executive Summary

The monorepo hosts two products with a strong integration story:

- **Vashandi (Paperclip)** — the control plane for autonomous AI companies: org charts, tasks, heartbeats, budgets, approvals, governance. It is already partially implemented (V1 baseline). The remaining open work covers multi-agent orchestration, teams, memory integration, MCP tool management, and observability.

- **OpenBrain** — a memory OS for AI agents: hybrid vector-graph persistent memory, tiered memory lifecycle, agent identity/trust governance, proactive context delivery, and a self-healing curator agent. It is greenfield — 9 epic issues exist but no implementation yet.

The two products must be designed and sequenced together because OpenBrain is the memory provider for Vashandi agents, and Vashandi is OpenBrain's primary tenant and governance host. Neither product is complete without the other.

**Priority ordering:**
1. Finish and stabilize Vashandi V1 (the foundation everything runs on).
2. Bootstrap OpenBrain structure and define the integration interface (blockers before any OpenBrain epic work).
3. Implement OpenBrain core storage and governance in parallel with Vashandi advanced features.
4. Wire the two systems together through Vashandi's memory plugin system.
5. Build OpenBrain's intelligence layer (context engine, curator) and Vashandi's advanced platform features (observability, MCP governance, teams) once the foundation is solid.

---

## 2. Current State Assessment

### Vashandi

| Area | Status |
|---|---|
| Companies CRUD | ✅ Implemented |
| Agents CRUD + org tree | ✅ Implemented |
| Issues/tasks + comments | ✅ Implemented |
| Heartbeat invocation (process + http adapters) | ✅ Implemented |
| Cost events ingestion + rollups | ✅ Implemented |
| Budget enforcement (agent-level hard stop) | ✅ Partial — no project budgets, no approval on breach |
| Approval gates (hire, CEO strategy) | ✅ Implemented |
| Activity log | ✅ Implemented |
| Agent API keys | ✅ Implemented |
| Board auth (local_trusted + authenticated modes) | ✅ Implemented |
| Asset/attachment storage | ✅ Partial — image-centric, non-image MIME incomplete |
| Documents + revisions | ✅ Implemented |
| Plugin system | ✅ Implemented (runtime + SDK) |
| Go backend port | 🔄 In progress — DB models partially done, server not yet started |
| Multi-agent parallel dispatch | ❌ Not started |
| Teams & team-scoped auth | ❌ Not started |
| Memory service surface | ❌ Not started (spec written in 2026-03-17) |
| MCP tool management | ❌ Not started |
| Observability dashboarding | ❌ Not started |
| Guided onboarding V2 | ❌ Not started (spec written) |
| CEO Chat / command composer | ❌ Not started (spec written) |

### OpenBrain

| Area | Status |
|---|---|
| Repo location (`openbrain/`) | ✅ Directory exists, README only |
| Go module initialization | ❌ Not started |
| PostgreSQL + pgvector storage | ❌ Not started |
| Typed memory entity schema | ❌ Not started |
| Multi-tier memory model (L0–L3) | ❌ Not started |
| Agent Registry + trust tiers | ❌ Not started |
| Immutable audit log | ❌ Not started |
| Context compilation engine | ❌ Not started |
| Proactive context delivery | ❌ Not started |
| LLM Curator Agent | ❌ Not started |
| CLI | ❌ Not started |
| MCP server | ❌ Not started |
| Repository convention sync (brain.md) | ❌ Not started |

---

## 3. Stack Decisions

### 3.1 Vashandi Stack (Confirmed / In Progress)

| Layer | Technology |
|---|---|
| API server | Node.js/TypeScript (Express) → **migrating to Go 1.26+** (chi router, pgx, sqlc) |
| UI | React + Vite + TanStack Query — unchanged |
| Database | PostgreSQL 18-alpine (embedded PGlite for local dev, Docker Postgres for prod-like) |
| ORM/query layer | Drizzle (TS side) + pgx/sqlc (Go side) |
| Migrations | Drizzle Kit (TS) / golang-migrate (Go) |
| Realtime | WebSockets (gorilla/websocket in Go) |
| CLI | cobra + charmbracelet/huh (Go) |
| Plugin runtime | TypeScript plugin SDK (existing) — Go plugin host TBD |
| Testing | Vitest (unit), Playwright (e2e) |
| Infra | solution currently hosted using docker compose |

### 3.2 OpenBrain Stack

> **⚠ DECISION-01 — Language for OpenBrain:** Go is recommended. Rationale: aligns with the Vashandi Go migration direction, integrates cleanly into the existing `go.work` workspace, produces single-binary deployments, and has excellent concurrency for background maintenance and ingestion jobs. Rust was considered for vector performance but adds Rust toolchain requirements and significant ecosystem divergence. TypeScript was considered for dev speed but conflicts with the performance and binary packaging goals. **Requires human confirmation.**

> **⚠ DECISION-02 — Vector storage:** PostgreSQL + pgvector is recommended. It reuses the existing Postgres instance (one less service to operate), supports the single-node Docker deployment requirement from issue #36, and pgvector is sufficient for the scale of a per-company memory store. A dedicated vector database (Qdrant, Weaviate) can be added later as a pluggable backend behind the storage adapter contract. **Requires human confirmation.**

> **⚠ DECISION-03 — Graph storage:** Postgres adjacency tables with recursive CTEs are recommended for V1 (facts/decisions as nodes, typed relationships as edge rows). The issues mention "hybrid vector-graph" but the graph component is for relationship tracking (entity→entity), not complex graph traversal. Apache AGE or Neo4j can be considered later. **Requires human confirmation.**

> **⚠ DECISION-04 — OpenBrain service topology:** Separate service (not embedded library, not Vashandi plugin process). Reasons: OpenBrain has its own background jobs (curator agent, proactive delivery triggers, async ingestion), its own data model, and is designed to serve agents and services beyond Vashandi. It runs as a Docker sidecar in local dev and as a standalone service in production. Vashandi calls OpenBrain over HTTP (internal REST). **Requires human confirmation.**

> **⚠ DECISION-05 — OpenBrain primary API protocol:** REST/JSON over HTTP for both internal (Vashandi→OpenBrain) and external (CLI, third-party) access. gRPC is added later if latency profiling shows it is necessary. The MCP server is a thin wrapper over the REST API. **Requires human confirmation.**

| Layer | Technology |
|---|---|
| Language | Go (pending DECISION-01) |
| HTTP framework | chi router (consistent with Vashandi Go port) |
| Vector storage | PostgreSQL + pgvector (DECISION-02) |
| Graph/relational storage | PostgreSQL adjacency tables (DECISION-03) |
| Service topology | Separate service, Docker sidecar for dev (DECISION-04) |
| API | REST/JSON (DECISION-05) |
| MCP transport | stdio + HTTP/SSE |
| Migrations | golang-migrate |
| CLI | cobra + charmbracelet/huh |
| Testing | Go standard testing + testify |
| Build integration | `go.work` at monorepo root |

### 3.3 Monorepo Workspace Integration

```
go.work                     ← already exists, covers vashandi/backend
  uses ./vashandi/backend
  uses ./openbrain           ← add this

pnpm-workspace.yaml         ← unchanged (UI only)
```

OpenBrain module: `github.com/chifamba/vashandi/openbrain`
OpenBrain lives at: `openbrain/` (monorepo root, directory already exists)

---

## 4. Vashandi (Paperclip) — Complete Plan

### Phase V0 — V1 Completion & Stabilization

**Goal:** Ship a fully stable, complete V1 before layering advanced features on top.

#### V0.1 — Go Backend Port (completion of in-progress work)

- Complete remaining DB model ports: `approval_comments`, `company_logos`, `company_memberships`, `company_secret_versions`, `company_skills`, `document_revisions`, `feedback_exports`, `feedback_votes`
- Implement Go HTTP server (chi router) with all V1 REST routes from `SPEC-implementation.md §10`
- Port all middleware: auth (local_trusted + authenticated modes), CORS, logging (slog), error handling
- Port CLI commands (`onboard`, `dev`, `doctor`) using cobra + charmbracelet/huh
- Port WebSocket heartbeat ticks and live update streams
- Serve React UI dist from Go static file handler
- Port adapter implementations: process, http (V1 built-ins)
- Port plugin loader to Go host boundary
- Update GitHub Actions CI to build and test Go binary
- Update Dockerfile to multi-stage Go build

**Acceptance criteria:**
- `go build ./...` passes cleanly
- All existing Vitest unit tests pass pointing at Go server
- Playwright e2e tests pass pointing at Go server
- Docker image builds and health endpoint responds

#### V0.2 — Budget Policies & Enforcement (from `2026-03-14-budget-policies-and-enforcement.md`)

- Add project-level budget model: `project_budgets` table (`project_id`, `monthly_cents`, `spent_monthly_cents`, `alert_threshold_pct`)
- Separate **spend budgets** (hard policy) from **usage quotas** (advisory visibility)
- Generate `approval(type=budget_breach)` when hard limit is hit, not just auto-pause
- Add budget breach incident tracking to prevent duplicate alerts within same breach window
- Extend dashboard payload with project budget utilization
- Board UI: budget settings page showing agent + project budgets

**Acceptance criteria:**
- Agent hitting budget creates approval entry visible on board
- Project budget pause stops all issue checkout for issues in that project
- Soft alert at configurable threshold (default 80%) emits activity event

#### V0.3 — Billing Ledger Normalization (from `2026-03-14-billing-ledger-and-reporting.md`)

- Extend `cost_events` with: `upstream_provider`, `biller`, `billing_type` (enum: `metered_api | subscription_included | subscription_overage | aggregator`), `billed_amount_cents`
- Add `billing_ledger` table for account-level charges not attributable to a single inference (platform fees, top-ups, credits)
- Reporting reads only from `cost_events` + `billing_ledger`, not from `heartbeat_runs.usage_json`
- `/api/companies/:companyId/costs/summary` returns normalized spend breakdown by provider, biller, and billing type

#### V0.4 — Asset System Completion (from `2026-03-13-features.md §4`)

- Accept all MIME types (not only images) for issue attachments
- Fix comment-level attachment rendering in UI
- Show file metadata, download, and inline preview for text/markdown/PDF/HTML files
- Introduce `artifact` kind on assets: `task_output | generated_doc | preview_link | workspace_file`
- Add artifact browser to issue and run detail pages

#### V0.5 — Guided Onboarding V2 (from `2026-03-13-features.md §1`)

- Replace configuration-first onboarding with 4-question interview flow
- `GET /api/onboarding/recommendation` — returns suggested deployment mode and adapter based on detected environment
- `GET /api/onboarding/llm-handoff.txt` — returns a ready-to-paste Claude/Codex setup prompt
- Auto-create starter objects on completion: company, company goal, CEO agent, first report, first task
- Detect installed CLIs (claude, codex, cursor, openclaw)
- End onboarding with a real first task visible in the board

**Acceptance criteria:**
- Fresh install with a supported local runtime completes without manual JSON/env editing
- Board shows a live first task before user leaves onboarding

---

### Phase V1 — Control Plane Hardening (Issues #31, #32)

**Goal:** Enable multi-agent workflows and introduce teams as a first-class organizational boundary.

#### V1.1 — Multi-Agent Parallel Dispatch (Issue #31, first half)

- Remove serialized task completion constraint; support concurrent active runs per company
- Add dispatch coordinator: tracks which agents have active runs, routes new heartbeat ticks
- Implement parallel task checkout: multiple agents can hold `in_progress` issues simultaneously (within company)
- Add `max_concurrent_runs` per agent (configurable, default 1, V1 max TBD)
- Dashboard aggregation updated to show all active runs concurrently
- Activity log correctly attributes concurrent run events

**Schema additions:**
- `agents.max_concurrent_runs` int not null default 1

**Invariants preserved:**
- Single assignee per issue (unchanged)
- Atomic checkout contract unchanged (409 on conflict)
- Budget enforcement applies per-run-start, not per-agent globally

#### V1.2 — Cross-Agent Handoff & State Management (Issue #31, second half)

- Agent A can formally hand off an issue to Agent B: new issue status `in_handoff`
- Handoff API: `POST /issues/:issueId/handoff` with `{ targetAgentId, summary, context }` payload
- Handoff transfers assignee atomically; prior context snapshot preserved in issue comments
- Full traceability: activity log records both sides of handoff
- Agents can query their handoff inbox: `GET /companies/:companyId/issues?status=in_handoff&targetAgent=me`

**Schema additions:**
- `issues.status` enum adds `in_handoff`
- `issue_handoffs` table: `id`, `company_id`, `issue_id`, `from_agent_id`, `to_agent_id`, `summary`, `context_snapshot`, `created_at`

#### V1.3 — Teams as First-Class Entity (Issue #32, first half)

- Introduce `teams` table: `id`, `company_id`, `name`, `description`, `lead_agent_id` nullable
- Introduce `team_memberships` table: `id`, `company_id`, `team_id`, `agent_id`, `role` (enum: `lead | member`)
- Teams are organizational groupings within a company — they do not replace the org tree but can cross it
- Board can create, update, and archive teams
- UI: team management page under company settings

**Schema additions:**
- `teams`: `id uuid pk`, `company_id uuid fk`, `name text`, `description text null`, `lead_agent_id uuid fk null`, `status enum(active|archived)`, `created_at`, `updated_at`
- `team_memberships`: `id uuid pk`, `company_id uuid fk`, `team_id uuid fk`, `agent_id uuid fk`, `role enum(lead|member)`, `joined_at timestamptz`
- Index: `team_memberships(company_id, team_id)`, `team_memberships(company_id, agent_id)`

#### V1.4 — Team-Scoped Authorization (Issue #32, second half)

- Scope agent API key access to team-owned resources (optional flag per company)
- Team budget: team can have its own monthly budget; enforcement mirrors agent-level policy
- `team_budgets` table analogous to project budgets
- Agent cannot access issues assigned to agents outside their team when team isolation mode is on
- Board always has cross-team access (unchanged)

---

### Phase V2 — Intelligence Layer (Issues #33, #34)

**Goal:** Surface memory, context, and MCP tool governance into the control plane.

#### V2.1 — Memory Service Surface (Issue #33, from `2026-03-17-memory-service-surface-api.md`)

This implements the control-plane memory layer. The actual memory provider is OpenBrain (see Phase OB3).

**Data model additions:**
- `memory_bindings`: `id`, `company_id`, `key text`, `provider_plugin_id`, `config jsonb`, `enabled bool`
- `memory_binding_targets`: `id`, `company_id`, `binding_id`, `target_type enum(company|agent)`, `target_id uuid`
- `memory_operations`: `id`, `company_id`, `binding_id`, `operation_type enum(write|query|forget|browse|correct)`, `scope jsonb`, `source_ref jsonb`, `usage jsonb`, `success bool`, `error text null`, `created_at`

**API additions:**
- `GET /companies/:companyId/memory/bindings`
- `POST /companies/:companyId/memory/bindings`
- `PATCH /memory/bindings/:bindingId`
- `DELETE /memory/bindings/:bindingId`
- `GET /companies/:companyId/memory/operations` (log browser)
- `POST /companies/:companyId/memory/query` (proxied to provider)
- `POST /companies/:companyId/memory/write` (proxied to provider)
- `POST /companies/:companyId/memory/forget` (proxied to provider)

**Automatic hooks (implemented in this phase):**
- Pre-run hydrate: before heartbeat invoke, query active binding with `intent=agent_preamble`
- Post-run capture: after run completes, write summary tied to run
- Issue comment capture: when binding has `captureComments=true`, write selected comments

**Plugin adapter contract:** Follows `MemoryAdapter` interface from `2026-03-17-memory-service-surface-api.md`. Built-in local provider (markdown + optional embedding) ships as the zero-config default.

**Memory cost integration:** Every `MemoryUsage` record returned by the adapter creates a `cost_events` entry with `billing_type=memory_operation`.

#### V2.2 — Team-Scoped Memory Isolation (Issue #33, third requirement)

- Memory operations carry team namespace if agent is team-member and team isolation is active
- Pre-run hydrate scoped to agent's team by default; configurable per binding
- `memory_operations` log includes `team_id` field when team context is active

#### V2.3 — MCP Tool Access Control (Issue #34, first requirement)

- `mcp_tool_definitions` table: `id`, `company_id`, `name`, `description`, `schema jsonb`, `source` (enum: `built_in|plugin|external`)
- `mcp_entitlement_profiles` table: `id`, `company_id`, `name`, `tool_ids uuid[]`
- `agent_mcp_entitlements` table: `id`, `company_id`, `agent_id`, `profile_id`
- MCP Hub concept: a company-scoped registry of available MCP tools
- Agent API includes tool entitlement: `GET /agents/:agentId/mcp-tools` returns accessible tool list
- Agents outside their entitlement receive 403 on tool invocation attempt

#### V2.4 — MCP Governance UI & Runtime Auditing (Issue #34, second requirement)

- Board UI page: MCP Tools (browse, enable, assign profiles)
- All tool invocations logged to `activity_log` with `action=mcp_tool_invoked`, `details={toolName, agentId, runId}`
- Activity feed shows tool invocation events
- `GET /companies/:companyId/mcp-audit` — filtered log of tool invocations

#### V2.5 — MCP Hub Injection for Adapters (Issue #34, third requirement)

- On heartbeat invoke, include MCP tool definitions in agent context payload for supported adapters
- Claude, Codex, and Cursor adapters receive MCP Hub config in `fat` context mode
- Adapter config: `mcpHubEnabled: boolean`, `mcpToolFilter: string[]`

#### V2.6 — CEO Chat / Board Command Composer (from `2026-03-13-features.md §2`)

- Add `issues.kind` column: enum `task | strategy | question | decision`
- Add `issues.scope` column: enum `company | project | issue | agent`
- Add `issues.target_agent_id` fk null (for directed board→agent threads)
- Add `issue_comments.intent` column: enum `hint | correction | board_question | board_decision | status`
- Board UI: global command composer with mode selector (`ask | task | decision`)
- "Ask CEO" from dashboard → creates `strategy` issue scoped to company, CEO picked up on next heartbeat
- Thread can produce tasks, approvals, artifacts via board one-click actions

---

### Phase V3 — Platform Maturity (Issue #35 + remaining features)

**Goal:** Production-grade observability, export/import, evals, and ClipHub foundations.

#### V3.1 — Platform Observability Dashboarding (Issue #35)

- Company-level dashboard endpoint returns: active/running/paused/error agent counts, open/in-progress/blocked/done issue counts, month-to-date spend and budget utilization, pending approvals count, team health summary (if teams enabled), memory operation counts and hit rate, MCP tool invocation counts
- New aggregated metric endpoint: `GET /api/platform/metrics` (board-only) — across all companies: total agents, total active runs, total spend MTD, error rate
- Live activity feed via WebSocket: `ws://host/api/ws/companies/:companyId/activity`
- Run summary view model (`RunSummary`): headline, objective, completed steps, delegated work, artifacts, warnings — derived from run logs without a separate persistence layer in V1

#### V3.2 — Live Org Visibility + Explainability (from `2026-03-13-features.md §3`)

- Org chart with live status indicators per agent (active/running/paused/error/idle)
- Agent page: status card, current issue, plan checklist, latest artifacts, summary of last run, expandable raw trace
- Run page: Summary → Steps → Raw transcript (3-layer progressive disclosure)
- `RunSummary` computed server-side from run logs; no additional persistence

#### V3.3 — Company Import/Export V2 (from `2026-03-13-company-import-export-v2.md`)

- Template export: agent definitions, org chart, adapter configs, role descriptions, optional seed tasks
- Snapshot export: full state including task progress and agent status
- `POST /companies/:companyId/export` — returns portable JSON artifact
- `POST /companies/import` — creates company from template or snapshot
- ClipHub foundations: company template registry (local manifest; hosted marketplace is future)

#### V3.4 — Agent Evals Framework (from `2026-03-13-agent-evals-framework.md`)

- Eval run model: structured test case → agent invocation → assertion set
- `eval_suites` and `eval_runs` tables
- Board UI: eval run history and pass/fail summary
- Integration with heartbeat system: eval invocations use same adapter contract

---

### Vashandi Phase Summary

| Phase | Focus | Depends On |
|---|---|---|
| V0.1 | Go port completion | — |
| V0.2 | Budget enforcement | V0.1 |
| V0.3 | Billing ledger | V0.1 |
| V0.4 | Asset system | V0.1 |
| V0.5 | Onboarding V2 | V0.1 |
| V1.1 | Parallel dispatch | V0 complete |
| V1.2 | Cross-agent handoff | V1.1 |
| V1.3 | Teams | V0 complete |
| V1.4 | Team auth | V1.3 |
| V2.1 | Memory surface | V0 complete, OB Phase OB2 |
| V2.2 | Team memory isolation | V2.1, V1.3 |
| V2.3 | MCP tool access | V0 complete |
| V2.4 | MCP governance UI | V2.3 |
| V2.5 | MCP hub injection | V2.3 |
| V2.6 | CEO chat / command composer | V0 complete |
| V3.1 | Observability | V1 + V2 |
| V3.2 | Org explainability | V3.1 |
| V3.3 | Import/export V2 | V0 complete |
| V3.4 | Agent evals | V0 complete |

---

## 5. OpenBrain — Complete Plan

### Phase OB-0 — Bootstrap & Cross-System Foundation (GAP-01, GAP-02, GAP-03)

**Goal:** Unblock all subsequent OpenBrain epic work by resolving the three critical gaps identified in the roadmap gap analysis.

#### OB-0.1 — Bootstrap Go Module & Project Structure (GAP-02)

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

- Add `openbrain` to monorepo `go.work`
- Add OpenBrain service to root-level `docker-compose.yml` dev profile
- CI: add `go build ./openbrain/...` and `go test ./openbrain/...` steps

#### OB-0.2 — Define Vashandi↔OpenBrain Integration Interface (GAP-01)

This is the formal API contract between the two systems. See [§7 API Contracts](#7-api-contracts) for the full specification.

Summary of decisions:
- Transport: HTTP/JSON (internal REST)
- Auth: Vashandi generates a service token per company at company creation time; stored as `company_secrets` in Vashandi; passed as `Authorization: Bearer <service-token>` on OpenBrain internal calls
- Vashandi lifecycle events that trigger OpenBrain calls: agent created, agent archived, company archived, run completed (post-run capture), run starting (pre-run hydrate)
- Entity mapping: Vashandi `agent` → OpenBrain `registered_agent`; Vashandi `company` → OpenBrain `namespace`; Vashandi `issue` → memory `source_ref` of kind `issue`; Vashandi `heartbeat_run` → memory `source_ref` of kind `run`

#### OB-0.3 — Company-Scoped Memory Namespacing (GAP-03)

> **⚠ DECISION-06 — Namespace isolation model:** Row-level isolation using `namespace_id` (maps 1:1 to Vashandi `company_id`) is recommended. All queries include mandatory `namespace_id` predicate enforced at the storage layer, not the API layer. This means even an API bug cannot return cross-namespace data if the storage functions enforce it. Separate Postgres schemas per company are rejected as operationally expensive at scale. **Requires human confirmation.**

- Every OpenBrain table includes `namespace_id uuid not null`
- Storage layer functions accept `namespace_id` as a non-optional parameter
- API layer extracts `namespace_id` from the service token (token is scoped to one company)
- Index: every table has `(namespace_id, ...)` composite index as primary access path
- Teams within a company: sub-namespace via `team_id` column (optional second isolation dimension)

---

### Phase OB-1 — Core Storage Infrastructure (Issue #36)

**Goal:** Establish the hybrid vector-graph persistence layer with full CRUD for all typed memory entities.

#### OB-1.1 — PostgreSQL + pgvector Setup

- Require pgvector extension: `CREATE EXTENSION IF NOT EXISTS vector;`
- Docker Compose dev profile: Postgres 16 with pgvector pre-installed
- Migration 0001: extension, namespace table, and base entity tables
- Connection pool via pgxpool; configurable pool size

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

#### OB-1.3 — CRUD Operations

- `POST /api/v1/memories` — create entity (with or without embedding)
- `GET /api/v1/memories/:id` — get entity by id
- `PATCH /api/v1/memories/:id` — update entity (creates version record)
- `DELETE /api/v1/memories/:id` — soft delete (sets `is_deleted=true`, creates version record)
- `GET /api/v1/memories` — browse with filters: entity_type, tier, team_id, date range
- `POST /api/v1/memories/search` — vector similarity search + optional keyword filter
- `POST /api/v1/memories/edges` — create relationship
- `GET /api/v1/memories/:id/edges` — get related entities

**Embedding generation:** Configurable embedding provider. Default: OpenAI text-embedding-3-small (1536d). Embedding happens asynchronously on write; entity is immediately queryable by keyword until embedding completes.

**Single-node Docker deployment acceptance criteria:**
- `docker compose up openbrain` brings up service with Postgres+pgvector; no external dependencies
- All CRUD operations functional in single-container mode

---

### Phase OB-2 — Multi-Tier Memory Lifecycle (Issue #37)

**Goal:** Implement L0→L3 tier logic for memory relevance management.

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
  - L2→L3: **requires human approval** (routed through Vashandi approval gate, see GAP-16)
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

### Phase OB-3 — Agent Identity & Governance (Issues #38, #39)

**Goal:** Establish agent registry with trust tiers and an immutable audit trail.

#### OB-3.1 — Agent Registry (Issue #38)

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

- Lower-trust agents: content is automatically redacted for L2/L3 entities above their tier
- Mandatory registration: any API call from an unregistered agent returns 403
- `POST /internal/v1/namespaces/:namespaceId/agents` — register agent (called by Vashandi on agent creation)
- `DELETE /internal/v1/namespaces/:namespaceId/agents/:agentId` — deregister (called by Vashandi on agent archive)

#### OB-3.2 — Immutable Audit Log (Issue #39)

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
  request_meta   jsonb,                      -- source IP, MCP session ID, run ID
  created_at     timestamptz not null default now()
);
```

**Tamper-evidence:** Each log row includes a `chain_hash` computed as `SHA-256(prev_chain_hash || id || created_at || after_hash)`. Verification function checks the chain is unbroken.

**Export:** `GET /api/v1/audit/export?format=jsonld|sqlite` — exportable for external audit tools.

**Immutability enforcement:** Write path to `memory_audit_log` is append-only at the application level. DB user for OpenBrain has no DELETE or UPDATE privilege on `memory_audit_log`.

---

### Phase OB-4 — Context Engine (Issues #40, #41)

**Goal:** Generate task-specific context packets within 500ms; deliver context proactively on environment triggers.

#### OB-4.1 — Context Compilation & Token Budgeting (Issue #40)

**Retrieval algorithm:**
1. Vector similarity search: top-K candidates by cosine similarity to task query (default K=50)
2. Re-rank by: tier weight (L3=4x, L2=2x, L1=1x, L0=0.5x), recency decay, agent recall profile preferences
3. Token budget enforcement: pack highest-ranked snippets until agent's `token_limit` is reached
4. Format output per agent's `recall_profile.format` (markdown | json | xml)

**API:**
- `POST /api/v1/context/compile`
  - body: `{ agentId, taskQuery, intent, tokenBudget?, includeTypes? }`
  - response: `{ snippets[], profileSummary?, tokenCount, latencyMs, usage }`
- Latency target: < 500ms at p95 for up to 10,000 entities per namespace

**Optimization strategies:**
- Embedding cache: skip re-embedding for repeated queries within 5 minutes (LRU cache)
- IVFFlat index with `lists=100` (tunable)
- Async pre-embedding on write path; compile blocks only when embedding is missing

#### OB-4.2 — Proactive Context Delivery (Issue #41)

Proactive context means OpenBrain pushes context to agents based on environment triggers rather than waiting for agent requests.

**Trigger types:**
| Trigger | Source | Action |
|---|---|---|
| Session start | Vashandi: heartbeat invoke | Pre-run hydrate context packet |
| Task checkout | Vashandi: `POST /issues/:issueId/checkout` | Task-specific memory surfacing |
| Branch creation | Git webhook or Vashandi run context | ADR + constraint surfacing |
| Test failure | Vashandi run output | Related past failures + fixes |
| Git commit/push | Git webhook or Vashandi run summary | Post-run memory capture |

**Integration model:** Vashandi calls `POST /internal/v1/namespaces/:id/triggers/:triggerType` with event payload. OpenBrain prepares context packet and optionally pushes via callback URL or stores for next agent poll.

**Scope:** This phase implements trigger ingestion and context packet preparation only. Agent-side delivery is via the existing `fat context` mode in Vashandi heartbeat invocations.

---

### Phase OB-5 — Self-Evolution & Curation (Issue #42)

**Goal:** The Curator Agent maintains memory health without degrading human oversight.

#### OB-5.1 — LLM Curator Agent (Gachlaw)

The Curator Agent is a background process within OpenBrain, not a Vashandi agent. It uses an LLM to reason over memory and produce proposals — but cannot self-approve.

**Curator actions (all require approval before execution):**
- De-duplicate: detect near-duplicate entities (cosine similarity > 0.95), propose merge
- Synthesize: group related L1 entities into a new L2 entity, propose promotion
- Conflict detection: identify contradicting entities (via edge type or LLM classification), propose resolution
- Knowledge gap detection: identify questions frequently asked by agents with empty recall, report as gaps
- Demotion: propose L2→L1 demotion for entities unused for 60 days

**Approval routing:**
> **⚠ DECISION-07 — Curator proposal routing:** Curator proposals will be reviewed and approved via the OpenBrain Modern Admin Web UI, which includes a dedicated UI panel for memory proposals and dashboard views. **Requires human confirmation.**

**Weekly Memory Health Report:**
- Generated each Monday (UTC)
- Includes: stale memory ratio, curator proposal acceptance rate, knowledge gap count, top accessed entities, entity type distribution by tier
- Stored as a Vashandi `document` attached to a system-created `strategy` issue for board visibility

---

### Phase OB-6 — Integration Interfaces (Issues #43, #44)

**Goal:** Provide standard CLI and MCP interfaces for human and agent interaction.

#### OB-6.1 — CLI for Human Memory Management (Issue #43, first requirement)

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

Human approval workflow: CLI-prompted review of pending curator proposals before submitting to Vashandi.

#### OB-6.2 — MCP Server (Issue #43, second requirement)

MCP server exposes OpenBrain tools to MCP-compatible agents (Claude, Cline, Cursor, etc.).

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

All tool calls log to `memory_audit_log`. Trust tier enforcement applies per agent's registration.

#### OB-6.3 — Repository Convention Synchronization (Issue #44)

- Watch `brain.md` (curated knowledge) and `session.md` (working task) within `.openbrain/` directories
- Changes to `brain.md` → ingest into OpenBrain as L2 entities with `provenance.kind = file_sync`
- Changes to OpenBrain entities promoted to L2/L3 → optionally write back to `brain.md` (configurable)
- `.openbrain/` directory is the local project-level OpenBrain context; supports multiple projects per namespace
- File watcher implemented as optional CLI daemon: `openbrain watch --dir ./`

---


### Phase OB-7 — Modern Admin Web UI

**Goal:** Provide a modern UI with full admin capabilities to manage all aspects of OpenBrain.

#### OB-7.1 — Dashboard and Metrics
- Modern UI (React-based, aligning with Vashandi's stack).
- Dashboard to display various useful metrics about the brain, including "thoughts" and "memories".
- Capability to manually trigger "day dreaming" (synthesis, deduplication, and conflict resolution by the LLM curator).

#### OB-7.2 — Administration and Maintenance
- Full admin ability to manage all aspects of OpenBrain, including direct CRUD operations on memories, namespaces, and agent registries.
- Ability to manage and monitor various maintenance jobs for the brain.

### OpenBrain Phase Summary

| Phase | Focus | Issues/Gaps | Depends On |
|---|---|---|---|
| OB-0.1 | Bootstrap Go module | GAP-02 | — |
| OB-0.2 | Vashandi↔OpenBrain interface | GAP-01 | OB-0.1, V0 complete |
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

This maps the 16 roadmap gaps onto the phase plan above.

### Phase I-0 — Blockers (must precede all OpenBrain epic work)

| Gap | Work | Addressed In |
|---|---|---|
| GAP-01 | Vashandi↔OpenBrain API contract | OB-0.2 |
| GAP-02 | OpenBrain Go module bootstrap | OB-0.1 |
| GAP-03 | Company-scoped namespace isolation | OB-0.3 |

### Phase I-1 — Wiring (enables local dev + basic integration)

| Gap | Work | Addressed In |
|---|---|---|
| GAP-04 | Vashandi memory plugin wrapping OpenBrain | V2.1 (MemoryAdapter backed by OpenBrain) |
| GAP-05 | Agent identity federation | OB-3.1 (Vashandi agent_id in registered_agents) |
| GAP-08 | Docker service topology | OB-0.1 (docker-compose.dev.yml) |

### Phase I-2 — Safety (lifecycle integrity, cost, resilience, testing)

| Gap | Work | Addressed In |
|---|---|---|
| GAP-06 | Agent lifecycle → OpenBrain namespace | OB-0.2 (Vashandi lifecycle webhooks) |
| GAP-07 | Memory operation costs in Vashandi budget | V2.1 (MemoryUsage → cost_events) |
| GAP-10 | OpenBrain unavailability fallback | V2.1 (circuit breaker in memory service surface) |
| GAP-15 | Integration tests | After I-1 complete: contract test suite in `tests/integration/` |

### Phase I-3 — Polish

| Gap | Work | Addressed In |
|---|---|---|
| GAP-09 | Local dev DX | OB-0.1 + docker-compose + updated DEVELOPING.md |
| GAP-11 | OpenBrain API versioning | All OpenBrain REST endpoints under `/api/v1/`; stability contract added to AGENTS.md |
| GAP-12 | CEO Chat → OpenBrain ingestion | V2.6 (CEO Chat) + OB-4.2 (post-thread capture) |
| GAP-13 | Company onboarding memory bootstrap | V0.5 (Onboarding V2) seeds brain.md; OB-6.3 ingests it |
| GAP-14 | OpenBrain graph schema design | OB-1.2 (adjacency table schema), formalized in `openbrain/doc/GRAPH-SCHEMA.md` |
| GAP-16 | Curator proposals through Vashandi approvals | OB-5 (curator) + V2.3 adjacent (new approval type `memory_curator_proposal`) |

---

## 7. API Contracts

### 7.1 Vashandi REST API Additions (to existing `/api` base)

All new endpoints follow existing auth and error conventions from `SPEC-implementation.md §10`.

```
# Memory service (V2.1)
GET    /api/companies/:companyId/memory/bindings
POST   /api/companies/:companyId/memory/bindings
PATCH  /api/memory/bindings/:bindingId
DELETE /api/memory/bindings/:bindingId
GET    /api/companies/:companyId/memory/operations
POST   /api/companies/:companyId/memory/query
POST   /api/companies/:companyId/memory/write
POST   /api/companies/:companyId/memory/forget

# MCP governance (V2.3–V2.5)
GET    /api/companies/:companyId/mcp/tools
POST   /api/companies/:companyId/mcp/tools
GET    /api/companies/:companyId/mcp/profiles
POST   /api/companies/:companyId/mcp/profiles
PATCH  /api/mcp/profiles/:profileId
GET    /api/agents/:agentId/mcp-tools
GET    /api/companies/:companyId/mcp-audit

# Teams (V1.3–V1.4)
GET    /api/companies/:companyId/teams
POST   /api/companies/:companyId/teams
GET    /api/teams/:teamId
PATCH  /api/teams/:teamId
POST   /api/teams/:teamId/members
DELETE /api/teams/:teamId/members/:agentId

# Multi-agent handoff (V1.2)
POST   /api/issues/:issueId/handoff
GET    /api/companies/:companyId/issues?status=in_handoff

# Budget additions (V0.2)
GET    /api/companies/:companyId/budgets/projects
POST   /api/companies/:companyId/budgets/projects
PATCH  /api/budgets/projects/:budgetId

# Platform metrics (V3.1)
GET    /api/platform/metrics           # board-only, cross-company

# Onboarding (V0.5)
GET    /api/onboarding/recommendation
GET    /api/onboarding/llm-handoff.txt
POST   /api/onboarding/complete
```

### 7.2 OpenBrain External REST API (`/api/v1/`)

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

### 7.3 OpenBrain Internal API (`/internal/v1/`) — Vashandi↔OpenBrain Only

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

### 7.4 Vashandi Memory Plugin Adapter Contract

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

### 7.5 New Vashandi Approval Types

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

## 8. Non-Functional Requirements (NFRs)

### 8.1 Vashandi NFRs

| NFR | Target | Enforcement |
|---|---|---|
| Board UI read latency (p99) | < 200ms | DB indexes on all hot query paths |
| API error rate | < 0.1% under normal load | Health monitoring + alerts |
| Heartbeat scheduler drift | < 5s | In-process scheduler; skip on overload |
| Budget enforcement lag | ≤ 1 scheduler tick after limit breach | Checked on every cost event ingest |
| Cross-company data isolation | Zero tolerance | `company_id` on every query; integration tests |
| All mutations write to `activity_log` | 100% of mutating endpoints | Enforced in service layer, not route layer |
| API key storage | Hash only (bcrypt), never plaintext after creation | TS/Go service contract |
| Agent handoff traceability | 100% of handoffs logged | Atomic write: handoff + activity log |
| Concurrent test isolation | No shared state between tests | Per-test DB schemas or transactions |

### 8.2 OpenBrain NFRs

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
| Curator proposals never self-approved | 100% | Proposals route through Vashandi approval gate |
| Memory operation logging | 100% of reads, writes, deletes | Audit log in every storage function |

### 8.3 Cross-System NFRs

| NFR | Target | Enforcement |
|---|---|---|
| OpenBrain unavailability impact | Degraded mode only (Vashandi continues) | Circuit breaker in V2.1 memory plugin |
| Memory costs visible in Vashandi | Every adapter MemoryUsage → cost_events | Memory plugin adapter responsibility |
| Service token security | Tokens stored as company_secrets, hashed | Existing Vashandi secret manager |
| Agent archive → namespace cleanup | < 24h after archive | Async job triggered by Vashandi lifecycle webhook |
| Integration test coverage | Key scenarios: task→memory ingest, company isolation, budget enforcement, fallback | `tests/integration/vashandi-openbrain/` |
| API versioning | All OpenBrain external endpoints under `/api/v1/` | Router prefix; v2 added only on breaking changes |

---

## 9. Key Decisions Requiring Human Confirmation

The following decisions were made by this analysis. Each needs confirmation before the affected phase begins.

| # | Decision | Recommendation | Alternatives | Phase Affected |
|---|---|---|---|---|
| DECISION-01 | OpenBrain language | **Go** | Rust (performance), TypeScript (ecosystem) | OB-0.1 and all subsequent |
| DECISION-02 | Vector storage | **PostgreSQL + pgvector** | Qdrant, Weaviate, Chroma | OB-1.1 and all subsequent |
| DECISION-03 | Graph storage | **Postgres adjacency tables** (V1) | Apache AGE, Neo4j | OB-1.2 and graph queries |
| DECISION-04 | OpenBrain service topology | **Separate service (Docker sidecar)** | Embedded library, Vashandi plugin process | OB-0.1, GAP-08 |
| DECISION-05 | OpenBrain API protocol | **REST/JSON** (gRPC later if needed) | gRPC from day one, MCP-native | OB-0.2, all API surfaces |
| DECISION-06 | Namespace isolation model | **Row-level `namespace_id`** enforced at storage layer | Separate Postgres schemas, separate DBs | OB-0.3 and all OpenBrain tables |
| DECISION-07 | Curator proposal routing | **OpenBrain Modern Admin UI** | Through Vashandi approval gates | OB-5, OB-7 |
| DECISION-08 | Embedding model/dimension | **OpenAI text-embedding-3-small (1536d)** as default | Cohere embed-v3, local Ollama embeddings, 768d alternatives | OB-1.2 and schema |
| DECISION-09 | OpenBrain auth for external API | **Agent-scoped JWT tokens** issued by OpenBrain, validated against registered_agents | Vashandi API key passthrough, per-namespace static keys | OB-3.1 and all API surfaces |
| DECISION-10 | L2→L3 promotion approval flow | **Routes through Vashandi board approval** | OpenBrain-internal approval, human CLI only | OB-2.2 and V2 integration |
| DECISION-11 | Vashandi Go port strategy | **Keep TS + Go running side by side** until full parity, then cut over | Cut over route by route, Cut over at once | V0.1 |

---

## 10. Assumptions

These assumptions are made in this plan. If any are incorrect, the affected sections must be revised.

1. **Vashandi V1 is functionally correct.** The existing TypeScript server implementation for companies, agents, issues, heartbeats, cost events, and approvals is functionally correct. The Go port is a language migration, not a rewrite.

2. **pgvector is sufficient for per-company scale.** At the expected scale of 10K–100K memory entities per company namespace, a single PostgreSQL instance with pgvector IVFFlat index provides < 500ms context compilation. This assumption should be validated with a load test before OB-4.1 is considered complete.

3. **OpenBrain is co-deployed with Vashandi.** The recommended topology has both services running in the same Docker Compose environment in development and in the same infrastructure in production. If they must deploy independently (different teams, different clusters), the internal API will need additional security hardening beyond service tokens.

4. **The `go.work` workspace can accommodate both `vashandi/backend` and `openbrain`.** Current `go.work` already covers `vashandi/backend`. Adding `openbrain/` should be straightforward. Any module name conflicts will require resolution.

5. **Team isolation is optional and company-configurable.** Not all companies need team-level access control. The V1.4 design makes it a company-level flag so simple deployments are not burdened.

6. **OpenBrain has its own standalone UI for governance.** A modern React-based UI will be built to manage memories, namespaces, agent registries, and trigger maintenance tasks like "day dreaming", along with handling approval workflows.

7. **Embedding calls are to an external provider (OpenAI by default).** Local embedding (Ollama) is a future option. The initial deployment requires an embedding API key.

8. **CEO Chat (V2.6) will be backed by Vashandi issues, not a separate message table.** This means the CEO agent picks up strategy threads via the existing heartbeat + task checkout path; no new real-time push mechanism is required for V1 of this feature.

9. **The Rust bindings mentioned in issue #43 are deferred.** Rust bindings for high-performance memory access are not included in this plan. They are a future optimization once the Go service is established and performance is measured.

10. **OpenBrain's "Gachlaw" curator agent uses an external LLM (not a self-hosted model).** The curator calls an LLM API (configurable provider, default OpenAI) for de-duplication synthesis and conflict detection. This is a cost-bearing operation tracked via OpenBrain's own memory operation cost accounting.

11. **The `docs/plans` path referenced in the problem statement maps to `doc/plans/` at the monorepo root.** This plan is placed there. Existing `vashandi/doc/plans/` continues to hold Vashandi-specific implementation plans.

---

*End of plan. All decisions marked ⚠ DECISION-N require human confirmation before the affected phase begins. Send confirmations or corrections and this plan will be updated accordingly.*
