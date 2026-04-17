# Go Backend-Only Features

This document describes features that are implemented exclusively in the Go backend and are not exposed through the Node.js Drizzle schema.

## Overview

The Vashandi architecture uses a dual-runtime approach:
- **Node.js/TypeScript**: Board UI, development middleware, and certain deferred areas
- **Go**: API server, orchestration, performance-critical operations, and core business logic

Some features are implemented only in Go and use separate database tables that are not mirrored in the Drizzle ORM schema. These features are accessed via the Go API routes.

## Teams (Go-only)

**Status**: Backend-specific feature

Teams provide a way to organize agents into logical groups with shared budgets and leadership.

### Tables

| Table | Purpose |
|-------|---------|
| `teams` | Team definitions with company scope |
| `team_memberships` | Agent-to-team assignments |
| `team_budgets` | Spending limits per team |

### Models

- `Team` — Group of agents with a lead agent and status
- `TeamMembership` — Links agents to teams with roles
- `TeamBudget` — Budget limits and periods for teams

### API Routes

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/companies/{companyId}/teams` | `TeamsHandler` |
| GET | `/api/teams/{teamId}` | `TeamHandler` |

### Rationale

Teams are a Go-only feature because:
1. Team operations are tightly coupled with budget governance, which runs in Go
2. The Node.js board UI doesn't currently expose team management surfaces
3. Keeping teams in Go simplifies the control-plane implementation

Teams may be promoted to the Drizzle schema in the future if:
- A UI surface for team management is added
- Cross-platform consistency becomes a priority

## MCP Governance (Go-only)

**Status**: Backend-specific feature

MCP (Model Context Protocol) governance tables manage tool definitions and agent entitlements for MCP-compatible tools.

### Tables

| Table | Purpose |
|-------|---------|
| `mcp_tool_definitions` | Tool schemas and metadata |
| `mcp_entitlement_profiles` | Named collections of allowed tools |
| `agent_mcp_entitlements` | Links agents to entitlement profiles |

### Models

- `MCPToolDefinition` — Tool metadata, JSON schema, and source
- `MCPEntitlementProfile` — Named profile with a list of tool IDs
- `AgentMCPEntitlement` — Maps an agent to a profile

### API Routes

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/companies/{companyId}/mcp/tools` | `MCPToolsHandler` |
| GET | `/api/companies/{companyId}/mcp/profiles` | `MCPProfilesHandler` |
| GET | `/api/agents/{agentId}/mcp-tools` | `AgentMCPToolsHandler` |

### Rationale

MCP governance is Go-only because:
1. Tool entitlement checks happen at agent runtime (Go)
2. MCP server integration is a Go-side concern
3. The feature is experimental and may evolve

## Memory Service (Go-only control plane)

**Status**: Backend-specific feature (control plane)

The memory service control plane manages memory bindings and operation logging. Actual memory storage and retrieval is delegated to OpenBrain or plugin providers.

### Tables

| Table | Purpose |
|-------|---------|
| `memory_bindings` | Provider configurations per company |
| `memory_binding_targets` | Target overrides (agent, project) |
| `memory_operations` | Audit log of memory operations |

### Models

- `MemoryBinding` — Points to a memory provider with configuration
- `MemoryBindingTarget` — Target-specific binding overrides
- `MemoryOperation` — Operation log entry with scope and usage

### API Routes

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/companies/{companyId}/memory/bindings` | `MemoryBindingsHandler` |
| GET | `/api/companies/{companyId}/memory/operations` | `MemoryOperationsHandler` |
| GET | `/api/companies/{companyId}/memory/audit/export` | `ExportAuditHandler` |

### Rationale

Memory service tables are Go-only because:
1. Memory operations are tightly integrated with agent run lifecycle (Go)
2. OpenBrain integration uses Go service-to-service calls
3. The feature is designed per the memory service plan (doc/plans/2026-03-17-memory-service-surface-api.md)

## Migration Notes

These tables are created in Go migration `0049_memory_mcp_teams_tables.sql`.

If any of these features need to be exposed to Node.js in the future:
1. Create corresponding Drizzle schema files in `packages/db/src/schema/`
2. Add exports to `packages/db/src/schema/index.ts`
3. Generate a Drizzle migration
4. Ensure Go and Node.js schemas stay synchronized

## Related Documentation

- [Memory Service Plan](./plans/2026-03-17-memory-service-surface-api.md)
- [Vashandi-OpenBrain Integration](./plans/2026-04-12-vashandi-openbrain-integration-contract.md)
- [SPEC-implementation.md](./SPEC-implementation.md)
