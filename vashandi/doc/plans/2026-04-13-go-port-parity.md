# Plan: Go Backend Parity (Phase 1)

**Date**: 2026-04-13
**Status**: Proposed

This plan outlines the first phase of achieving functional parity between the TypeScript Node.js server and the Go backend. High-priority services (Heartbeat, Plugins) are prioritized to unblock internal security hardening (mTLS).

## User Review Required

> [!IMPORTANT]
> **Microservice Readiness**: We are implementing a modular `AgentRunner` interface. This allows us to start with in-process execution for parity while facilitating a future split into dedicated runner microservices for better scaling and isolation.

## Proposed Changes

### 1. Heartbeat System (Parity)
Port the orchestration logic that manages agent execution lifecycles.

#### [NEW] [backend/server/services/heartbeat.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/services/heartbeat.go)
-   `HeartbeatService`: Logic for `Wakeup`, `StartRun`, and `EndRun`.
-   `AgentRunner` Interface: Abstraction for process execution.
-   `InProcessRunner`: Implementation using Go's `os/exec`.

#### [NEW] [backend/server/routes/heartbeat.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/routes/heartbeat.go)
-   POST `/api/heartbeat/wakeup`: Trigger agent wake-ups.
-   GET `/api/heartbeat/runs`: List active and past runs.

### 2. Plugin System (Foundations)
Port the plugin discovery and metadata logic used by the board and agents.

#### [NEW] [backend/server/services/plugins.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/services/plugins.go)
-   `PluginService`: Registry and metadata validation.

#### [NEW] [backend/server/routes/plugins.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/routes/plugins.go)
-   GET `/api/plugins`: List available plugins and their capabilities.

### 3. Usage, Observability & Workspace (Phase 2)
Port the critical logic for tracking costs, capturing logs, and managing Git-backed workspaces.

#### [NEW] [backend/server/services/run_log_store.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/services/run_log_store.go)
-   Implement `RunLogStore` (LocalFile NDJSON implementation) to record agent stdout/stderr.

#### [NEW] [backend/server/services/costs.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/services/costs.go)
-   `CostService`: Handle `cost_events` creation and rolling spend aggregation on agents/companies.

#### [MODIFY] [backend/server/services/heartbeat.go](file:///Users/robert/projects/github.com/vashandi/vashandi/backend/server/services/heartbeat.go)
-   Integrate `RunLogStore` into the `LocalRunner`.
-   Add usage reporting (tokens/cost) at run completion.

### 3. OpenBrain Refinements (Already Approved)
-   Add `VoyageProvider` for Anthropic/Claude embeddings.
-   Expose advanced Ollama parameters.

## Open Questions
-   **Secret Resolution**: How deep should the first pass of `SecretService` go? (Node.js handles complex env-binding).
-   **Logging Parity**: Should Go logs be saved to the same file structure as Node.js, or should we move to- [x] Standardize Activity Log retrieval and filtering in `ActivityService`.

## Phase 5: Production Hardening & Full Parity (Audit Findings)

Based on a deep-dive audit of the Node.js implementation, the following gaps must be closed for a production-ready Go release:

- [ ] **Heartbeat Resilience**:
    - Implement `ReapOrphanedRuns` for handling system restarts/crashes.
    - Implement agent-scoped concurrency limits (`ClaimQueuedRun`).
- [ ] **Advanced Workspaces**:
    - Port the `git_worktree` strategy to allow efficient concurrent runs on the same project.
    - Implement the full `WorkspaceOperationService` for tracking state changes.
- [ ] **Budget Enforcement**:
    - Integrate the budget check natively into the heartbeat execution cycle.
    - Implement the incident reporting system for budget breaches.
- [ ] **Secret Management**:
    - Implement the `SecretProvider` registry and `ResolveAdapterConfigForRuntime`.
- [ ] **Session Compaction**:
    - Port the logic to summarize and compact run histories into higher-level session summaries.

## Verification Plan

### Automated Tests
-   `go test ./backend/server/services` for Heartbeat logic.
-   Integration tests using `httptest` for the new routes.

### Manual Verification
-   Trigger a "Manual Wake" from the UI and verify the Go backend starts the process and streams logs correctly.
