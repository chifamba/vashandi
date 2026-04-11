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
