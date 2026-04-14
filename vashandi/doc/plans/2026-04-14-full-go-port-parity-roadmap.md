# Full Go Port Parity Roadmap

This document catalogs every pending task required to achieve 100% feature-complete parity between the original Node.js TypeScript architecture (`vashandi`) and the new Go 1.24+ backend migration. 

---

## 1. Node.js vs Go API Route Deficit

The `vashandi/backend/server/routes` directory is missing critical domains that exist natively in `vashandi/server/src/routes`. The following TS route files must be ported to Go:

- **`access.ts` (94KB)**: Rebuild user and organization RBAC access controls.
- **`adapters.ts` (23KB)**: Expose adapter configuration and connection status.
- **`approvals.ts` (11KB)**: Implement human-in-the-loop task approval logic.
- **`assets.ts` (11KB)**: S3 and Local filesystem blob management.
- **`authz.ts`**: Identity verification and better-auth integration bindings.
- **`company-skills.ts` (10KB)**: Custom skill assignments.
- **`costs.ts` (11KB)**: Event telemetry and budgeting deductions.
- **`execution-workspaces.ts` (15KB)**: Temporary filesystem creation mapping.
- **`inbox-dismissals.ts`**: UI state notifications state.
- **`instance-settings.ts`**: Runtime config mutations.
- **`issues-checkout-wakeup.ts`**: Atomic issue locking sequence.
- **`llms.ts`**: Gateway configuration testing routes.
- **`org-chart-svg.ts` (43KB)**: Dynamic visualization generator.
- **`plugin-ui-static.ts` (17KB)**: Plugin system mountpoints.
- **`projects.ts` (15KB)**: Project boundaries and workspace configurations.
- **`routines.ts` (11KB)**: Time-based execution polling data.
- **`secrets.ts`**: AES Key management endpoints.
- **`sidebar-badges.ts`**: UI metric aggregators.

> **Note on Logical Depth:** Some ported services exist but lack deep parity. For instance, the original `issues.ts` route handler is ~93KB of dense validation and orchestration logic, whereas the current `issues.go` port is barely 3KB. These existing routes must be expanded heavily.

---

## 2. CLI Tooling & Architecture Restructuring

We recently implemented structural parity using Cobra (Phases 1-3), however, there are major internal stubs that hold back complete algorithmic parity:

- **Worktree Lifecycle**: `vashandi/backend/cmd/paperclipai/worktree.go` requires translating ~4,000 lines of complex AST mutation, Git merging, and filesystem rollback logic from `worktree.ts` and `worktree-merge-history-lib.ts`.
- **Routines Execution**: `routines.go` requires translating the recurring schedule tracking loops.
- **Configuration Parsing**: `configure.go` and `allowed_hostname.go` need actual database and `config.json` injection logic applied under the hood.
- **Agent CLI API Bindings**: The proxy SDK endpoints in `vashandi/backend/client` must be populated. Currently only `Company` and `Issue` are fully wired; we need wrappers for `agent`, `approval`, `activity`, `dashboard`, `plugin`, and `context`.

---

## 3. Native Agent Adapters (Heartbeat Engine)

The TS implementation executes `claude` (Claude Code) as a system sub-process dynamically. Because the Go port is shifting toward using official provider SDKs natively within the engine (`anthropic.Runner`), we must rebuild every provider:

- **Fully Implement Anthropic Runner**: The current `vashandi/backend/adapters/anthropic/runner.go` must ingest and parse OpenBrain memory injections, format native JSON schemas for tool-calling, execute those functions by invoking the `client` packages, and stream the standard output chunks back into the Drizzle/Gorm database.
- **OpenAI / Codex Adapter Migration**: Needs native `github.com/openai/openai-go` module support to achieve parity with `packages/adapters/codex-local`.
- **Gemini Adapter Migration**: Needs native `google.golang.org/genai` module support to reach parity with `packages/adapters/gemini-local`.
- **Cursor / Pi / OpenClaw Gateways**: Extend interfaces to support the legacy custom enterprise targets.

---

## 4. OpenBrain "Fat Context" Injection

OpenBrain provides the vector-search intelligence. The TS API does this via `packages/shared/memory/` and API integrations.

- **gRPC / Internal API**: Deepen the data-shapes traversing from the Go REST Client into OpenBrain. The TS adapter formats precise XML wrappers (`<memory>...</memory>`) that must be strictly replicated in Go before pushing the strings out to the LLM context limits. mTLS certificates must be systematically mounted using the `OPENBRAIN_CA_CERT` pattern natively into the Go HTTP configuration.
