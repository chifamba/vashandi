# Vashandi Implementation Plan

Date: 2026-04-12
Status: Active — Pending Implementation

*This document is the single source of truth for all pending Vashandi implementation work. It covers everything required to take Vashandi from its current V1 baseline to a fully mature control plane.*

See also: [OpenBrain Implementation Plan](./openbrain-implementation-plan.md)

---

## 1. Current State Assessment

| Area | Status |
|---|---|
| Companies CRUD | ✅ Implemented |
| Agents CRUD + org tree | ✅ Implemented |
| Issues/tasks + comments | ✅ Implemented |
| Heartbeat invocation (process + http adapters) | ✅ Implemented |
| Cost events ingestion + rollups | ✅ Implemented |
| Budget enforcement (agent-level hard stop) | ✅ Implemented |
| Approval gates (hire, CEO strategy) | ✅ Implemented |
| Activity log | ✅ Implemented |
| Agent API keys | ✅ Implemented |
| Board auth (local_trusted + authenticated modes) | ✅ Implemented |
| Documents + revisions | ✅ Implemented |
| Plugin system | ✅ Implemented (runtime + SDK) |
| Go backend port | ✅ Partial — models and routes ported; HTTP server, middleware, CLI, WebSocket, and UI serving remain |
| Asset/attachment storage | ⚠️ Partial — image-centric, non-image MIME incomplete |
| Multi-agent parallel dispatch | ❌ Not started |
| Teams & team-scoped auth | ❌ Not started |
| Memory service surface | ❌ Not started (spec written in 2026-03-17) |
| MCP tool management | ❌ Not started |
| Observability dashboarding | ❌ Not started |
| Guided onboarding V2 | ❌ Not started (spec written) |
| CEO Chat / command composer | ❌ Not started (spec written) |

---

## 2. Pending Implementation Checklist

### Phase V0 — V1 Completion & Stabilization

- [ ] **V0.1: Complete Vashandi Go Backend Port**
  - Implement Go HTTP server (chi router) with all V1 REST routes from `SPEC-implementation.md §10`
  - Port all middleware: auth (local_trusted + authenticated modes), CORS, logging (slog), error handling
  - Port CLI commands (`onboard`, `dev`, `doctor`) using cobra + charmbracelet/huh
  - Port WebSocket heartbeat ticks and live update streams
  - Serve React UI dist from Go static file handler
  - Port adapter implementations: process, http (V1 built-ins)
  - Port plugin loader to Go host boundary
  - Integrate Redis for WebSocket routing and caching
  - Update GitHub Actions CI to build and test Go binary
  - Update Dockerfile to multi-stage Go build
- [ ] **V0.2: Budget Policies & Enforcement**
  - Add project-level budget model: `project_budgets` table (`project_id`, `monthly_cents`, `spent_monthly_cents`, `alert_threshold_pct`)
  - Separate **spend budgets** (hard policy) from **usage quotas** (advisory visibility)
  - Generate `approval(type=budget_breach)` when hard limit is hit
  - Add budget breach incident tracking to prevent duplicate alerts within same breach window
  - Extend dashboard payload with project budget utilization
  - Board UI: budget settings page showing agent + project budgets
- [ ] **V0.3: Billing Ledger Normalization**
  - Extend `cost_events` with: `upstream_provider`, `biller`, `billing_type` (enum: `metered_api | subscription_included | subscription_overage | aggregator`), `billed_amount_cents`
  - Add `billing_ledger` table for account-level charges (platform fees, top-ups, credits)
  - Reporting reads only from `cost_events` + `billing_ledger`, not from `heartbeat_runs.usage_json`
  - `/api/companies/:companyId/costs/summary` returns normalized spend breakdown by provider, biller, and billing type
- [ ] **V0.4: Asset System Completion**
  - Accept all MIME types (not only images) for issue attachments
  - Fix comment-level attachment rendering in UI
  - Show file metadata, download, and inline preview for text/markdown/PDF/HTML files
  - Introduce `artifact` kind on assets: `task_output | generated_doc | preview_link | workspace_file`
  - Add artifact browser to issue and run detail pages
- [ ] **V0.5: Guided Onboarding V2**
  - Replace configuration-first onboarding with 4-question interview flow
  - `GET /api/onboarding/recommendation` — returns suggested deployment mode and adapter based on detected environment
  - `GET /api/onboarding/llm-handoff.txt` — returns a ready-to-paste Claude/Codex setup prompt
  - Auto-create starter objects on completion: company, company goal, CEO agent, first report, first task
  - Detect installed CLIs (claude, codex, cursor, openclaw)
  - End onboarding with a real first task visible in the board

### Phase V1 — Control Plane Hardening

- [ ] **V1.1: Multi-Agent Parallel Dispatch** (Issue #31, first half)
  - Remove serialized task completion constraint; support concurrent active runs per company
  - Add dispatch coordinator: tracks which agents have active runs, routes new heartbeat ticks
  - Implement parallel task checkout: multiple agents can hold `in_progress` issues simultaneously
  - Add `max_concurrent_runs` per agent (configurable, default 1)
  - Dashboard aggregation updated to show all active runs concurrently
  - Activity log correctly attributes concurrent run events
  - Schema: `agents.max_concurrent_runs int not null default 1`
- [ ] **V1.2: Cross-Agent Handoff & State Management** (Issue #31, second half)
  - Agent A can formally hand off an issue to Agent B: new issue status `in_handoff`
  - `POST /issues/:issueId/handoff` with `{ targetAgentId, summary, context }` payload
  - Handoff transfers assignee atomically; prior context snapshot preserved in issue comments
  - Full traceability: activity log records both sides of handoff
  - `GET /companies/:companyId/issues?status=in_handoff&targetAgent=me`
  - Schema: `issues.status` enum adds `in_handoff`; new `issue_handoffs` table
- [ ] **V1.3: Teams as First-Class Entity** (Issue #32, first half)
  - Introduce `teams` table: `id`, `company_id`, `name`, `description`, `lead_agent_id` nullable
  - Introduce `team_memberships` table: `id`, `company_id`, `team_id`, `agent_id`, `role` (enum: `lead | member`)
  - Teams are organizational groupings within a company (do not replace the org tree)
  - Board can create, update, and archive teams
  - UI: team management page under company settings
- [ ] **V1.4: Team-Scoped Authorization** (Issue #32, second half)
  - Scope agent API key access to team-owned resources (optional flag per company)
  - Team budget: `team_budgets` table; enforcement mirrors agent-level policy
  - Agent cannot access issues assigned to agents outside their team when team isolation mode is on
  - Board always has cross-team access (unchanged)

### Phase V2 — Intelligence Layer

- [ ] **V2.1: Memory Service Surface** (Issue #33)
  - Data model: `memory_bindings`, `memory_binding_targets`, `memory_operations` tables
  - API: `GET/POST /companies/:companyId/memory/bindings`, `PATCH/DELETE /memory/bindings/:bindingId`, `GET /companies/:companyId/memory/operations`, `POST /companies/:companyId/memory/{query,write,forget}`
  - Automatic hooks: pre-run hydrate (before heartbeat invoke) and post-run capture (after run completes)
  - Issue comment capture when binding has `captureComments=true`
  - Plugin adapter contract: `MemoryAdapter` interface; built-in local provider (markdown + optional embedding) as zero-config default
  - Memory cost integration: every `MemoryUsage` record creates a `cost_events` entry with `billing_type=memory_operation`
  - Circuit breaker: system degrades gracefully if OpenBrain (memory provider) is down
- [ ] **V2.2: Team-Scoped Memory Isolation** (Issue #33)
  - Memory operations carry team namespace if agent is team-member and team isolation is active
  - Pre-run hydrate scoped to agent's team by default; configurable per binding
  - `memory_operations` log includes `team_id` field when team context is active
- [ ] **V2.3: MCP Tool Access Control** (Issue #34, first requirement)
  - `mcp_tool_definitions` table: `id`, `company_id`, `name`, `description`, `schema jsonb`, `source`
  - `mcp_entitlement_profiles` table: `id`, `company_id`, `name`, `tool_ids uuid[]`
  - `agent_mcp_entitlements` table: `id`, `company_id`, `agent_id`, `profile_id`
  - MCP Hub: company-scoped registry of available MCP tools
  - `GET /agents/:agentId/mcp-tools` returns accessible tool list; 403 for tools outside entitlement
- [ ] **V2.4: MCP Governance UI & Runtime Auditing** (Issue #34, second requirement)
  - Board UI page: MCP Tools (browse, enable, assign profiles)
  - All tool invocations logged to `activity_log` with `action=mcp_tool_invoked`
  - `GET /companies/:companyId/mcp-audit` — filtered log of tool invocations
- [ ] **V2.5: MCP Hub Injection for Adapters** (Issue #34, third requirement)
  - On heartbeat invoke, include MCP tool definitions in agent context payload for supported adapters
  - Claude, Codex, and Cursor adapters receive MCP Hub config in `fat` context mode
  - Adapter config: `mcpHubEnabled: boolean`, `mcpToolFilter: string[]`
- [ ] **V2.6: CEO Chat / Board Command Composer**
  - Add `issues.kind` column: enum `task | strategy | question | decision`
  - Add `issues.scope` column: enum `company | project | issue | agent`
  - Add `issues.target_agent_id` fk null (for directed board→agent threads)
  - Add `issue_comments.intent` column: enum `hint | correction | board_question | board_decision | status`
  - Board UI: global command composer with mode selector (`ask | task | decision`)
  - "Ask CEO" from dashboard → creates `strategy` issue scoped to company, CEO picks up on next heartbeat
  - Thread can produce tasks, approvals, artifacts via board one-click actions

### Phase V3 — Platform Maturity

- [ ] **V3.1: Platform Observability Dashboarding** (Issue #35)
  - Company-level dashboard endpoint: active/running/paused/error agent counts, open/in-progress/blocked/done issue counts, month-to-date spend and budget utilization, pending approvals count, team health summary, memory hit rate, MCP invocation counts
  - `GET /api/platform/metrics` (board-only, across all companies)
  - Live activity feed via WebSocket: `ws://host/api/ws/companies/:companyId/activity`
  - `RunSummary` view model: headline, objective, completed steps, delegated work, artifacts, warnings — computed server-side from run logs
- [ ] **V3.2: Live Org Visibility + Explainability**
  - Org chart with live status indicators per agent
  - Agent page: status card, current issue, plan checklist, latest artifacts, summary of last run, expandable raw trace
  - Run page: Summary → Steps → Raw transcript (3-layer progressive disclosure)
- [ ] **V3.3: Company Import/Export V2**
  - Template export: agent definitions, org chart, adapter configs, role descriptions, optional seed tasks
  - Snapshot export: full state including task progress and agent status
  - `POST /companies/:companyId/export` — returns portable JSON artifact
  - `POST /companies/import` — creates company from template or snapshot
  - ClipHub foundations: company template registry (local manifest; hosted marketplace is future)
- [ ] **V3.4: Agent Evals Framework**
  - Eval run model: structured test case → agent invocation → assertion set
  - `eval_suites` and `eval_runs` tables
  - Board UI: eval run history and pass/fail summary
  - Integration with heartbeat system: eval invocations use same adapter contract

---

## 3. Detailed Phase Descriptions

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

#### V0.2 — Budget Policies & Enforcement

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

#### V0.3 — Billing Ledger Normalization

- Extend `cost_events` with: `upstream_provider`, `biller`, `billing_type` (enum: `metered_api | subscription_included | subscription_overage | aggregator`), `billed_amount_cents`
- Add `billing_ledger` table for account-level charges not attributable to a single inference (platform fees, top-ups, credits)
- Reporting reads only from `cost_events` + `billing_ledger`, not from `heartbeat_runs.usage_json`
- `/api/companies/:companyId/costs/summary` returns normalized spend breakdown by provider, biller, and billing type

#### V0.4 — Asset System Completion

- Accept all MIME types (not only images) for issue attachments
- Fix comment-level attachment rendering in UI
- Show file metadata, download, and inline preview for text/markdown/PDF/HTML files
- Introduce `artifact` kind on assets: `task_output | generated_doc | preview_link | workspace_file`
- Add artifact browser to issue and run detail pages

#### V0.5 — Guided Onboarding V2

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

#### V2.1 — Memory Service Surface (Issue #33)

This implements the control-plane memory layer. The actual memory provider is OpenBrain (see [OpenBrain Implementation Plan](./openbrain-implementation-plan.md)).

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

**Circuit breaker:** System degrades gracefully if OpenBrain is down; memory operations silently skip rather than blocking the heartbeat.

#### V2.2 — Team-Scoped Memory Isolation (Issue #33)

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

#### V2.6 — CEO Chat / Board Command Composer

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

#### V3.2 — Live Org Visibility + Explainability

- Org chart with live status indicators per agent (active/running/paused/error/idle)
- Agent page: status card, current issue, plan checklist, latest artifacts, summary of last run, expandable raw trace
- Run page: Summary → Steps → Raw transcript (3-layer progressive disclosure)
- `RunSummary` computed server-side from run logs; no additional persistence

#### V3.3 — Company Import/Export V2

- Template export: agent definitions, org chart, adapter configs, role descriptions, optional seed tasks
- Snapshot export: full state including task progress and agent status
- `POST /companies/:companyId/export` — returns portable JSON artifact
- `POST /companies/import` — creates company from template or snapshot
- ClipHub foundations: company template registry (local manifest; hosted marketplace is future)

#### V3.4 — Agent Evals Framework

- Eval run model: structured test case → agent invocation → assertion set
- `eval_suites` and `eval_runs` tables
- Board UI: eval run history and pass/fail summary
- Integration with heartbeat system: eval invocations use same adapter contract

---

## 4. Phase Summary

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
| V2.1 | Memory surface | V0 complete, OpenBrain OB-1 |
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

## 5. API Contracts

All new endpoints follow existing auth and error conventions from `SPEC-implementation.md §10`. Base path: `/api`.

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

**New approval types:**
```ts
type ApprovalType =
  | "hire_agent"
  | "approve_ceo_strategy"
  | "memory_curator_proposal"    // NEW: routes OpenBrain curator proposals to board
  | "budget_breach";             // NEW: board notification on budget hard stop
```

---

## 6. Non-Functional Requirements

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

---

## 7. Key Decisions

| # | Decision | Recommendation | Phase Affected |
|---|---|---|---|
| DECISION-11 | Vashandi Go port strategy | **Keep TS + Go running side by side** until full parity, then cut over | V0.1 |

---

## 8. Assumptions

1. **Vashandi V1 is functionally correct.** The existing TypeScript server implementation for companies, agents, issues, heartbeats, cost events, and approvals is functionally correct. The Go port is a language migration, not a rewrite.

2. **Team isolation is optional and company-configurable.** Not all companies need team-level access control. The V1.4 design makes it a company-level flag so simple deployments are not burdened.

3. **CEO Chat (V2.6) will be backed by Vashandi issues, not a separate message table.** This means the CEO agent picks up strategy threads via the existing heartbeat + task checkout path; no new real-time push mechanism is required for V1 of this feature.

4. **The memory service surface (V2.1) depends on OpenBrain OB-1 being deployed.** V2.1 ships with a built-in local provider as the zero-config default, so it can be developed independently, but full OpenBrain integration requires OpenBrain OB-1+ to be running.

---

*End of plan.*
