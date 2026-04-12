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

- [ ] **Task 0.1: Bootstrap OpenBrain Project Structure (GAP-02)**
  - Create standard Go module in `openbrain/` (`go mod init github.com/chifamba/vashandi/openbrain`).
  - Add `openbrain` to the monorepo root `vashandi/go.work`. Note: `go.work` is currently located in `vashandi/`, so we must reference `../openbrain`.
  - Configure `pnpm-workspace.yaml` and CI pipelines if needed.
- [ ] **Task 0.2: Define OpenBrain Service Topology & Redis Setup (GAP-08)**
  - Update `docker-compose.yml` to include `redis:8-alpine`.
  - Update `docker-compose.yml` to run OpenBrain as a separate service alongside PostgreSQL/pgvector and Vashandi.
- [ ] **Task 0.3: Define Vashandi ↔ OpenBrain Integration Interface (GAP-01)**
  - Document the exact gRPC protobufs and REST OpenAPI specs for the integration.
  - Detail HTTP/REST, gRPC, and MCP interfaces.
- [ ] **Task 0.4: OpenBrain Company-Scoped Memory Namespacing (GAP-03)**
  - Define schema enforcing row-level `namespace_id` (mapping to Vashandi `company_id`) in Postgres and Redis keys.

## Phase 1 — V1 Completion & Wiring

- [ ] **Task 1.1: Complete Vashandi Go Backend Port (V0.1)**
  - Complete DB model ports (approval comments, logos, etc.).
  - Implement Go HTTP server with all V1 REST routes.
  - Integrate Redis for WebSocket routing and caching.
- [ ] **Task 1.2: Vashandi Memory Plugin: OpenBrain Adapter (GAP-04)**
  - Implement the `MemoryAdapter` interface to bridge Vashandi and OpenBrain via REST/gRPC.
- [ ] **Task 1.3: Agent Identity Federation (GAP-05)**
  - Define auth mechanism for external API access to OpenBrain (e.g., Agent-scoped JWT).
- [ ] **Task 1.4: Budget Policies & Enforcement (V0.2) + OpenBrain Costs (GAP-07)**
  - Implement project-level budgets.
  - Surface OpenBrain context compilation costs in Vashandi's budget engine.

## Phase 2 — Safety, Storage & Core Governance

- [ ] **Task 2.1: Implement OpenBrain Vector Storage & Graph Schema (GAP-14)**
  - Create Postgres tables with pgvector IVFFlat indexes and adjacency logic.
- [ ] **Task 2.2: Async Job Infrastructure (Redis)**
  - Setup `redis:8-alpine` message queues in OpenBrain for tier promotion and ingest batches.
- [ ] **Task 2.3: Sync Agent Lifecycle Events (GAP-06)**
  - Trigger Vashandi webhooks/gRPC calls to OpenBrain on agent creation/archival.
- [ ] **Task 2.4: Fallback Strategy (GAP-10)**
  - Implement circuit breakers in Vashandi's adapter so system degrades gracefully if OpenBrain is down.
- [ ] **Task 2.5: Integration Tests (GAP-15)**
  - Build cross-system contract verification test suite.

## Phase 3 — Intelligence Layer & Polish

- [ ] **Task 3.1: Proactive Context Delivery**
  - Implement pre-run hydration and post-run capture endpoints.
- [ ] **Task 3.2: LLM Curator Agent & Approvals (GAP-16)**
  - Implement Curator Agent logic in OpenBrain.
  - Route Curator promotion/deduplication proposals to Vashandi UI for human approval.
- [ ] **Task 3.3: CEO Chat Ingestion (GAP-12)**
  - Build pipeline to classify and ingest high-value strategy context from Vashandi to OpenBrain.
- [ ] **Task 3.4: Local Dev Setup (GAP-09) & Initial Onboarding (GAP-13)**
  - Seed new company namespaces from `brain.md` and default READMEs.
  - Finalize combined dev commands (`pnpm dev` handling Go OpenBrain + Go Vashandi + Node UI).
- [ ] **Task 3.5: External API Stability (GAP-11)**
  - Publish stable v1 REST, gRPC, and MCP endpoints.
