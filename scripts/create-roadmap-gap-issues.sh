#!/usr/bin/env bash
# create-roadmap-gap-issues.sh
#
# Creates the 16 roadmap gap issues identified in the Vashandi + OpenBrain
# gap analysis. Run this from a machine with `gh` authenticated and write
# access to the repo.
#
# Usage:
#   gh auth login          # if not already authenticated
#   bash scripts/create-roadmap-gap-issues.sh
#
# All issues are created with a label of "roadmap-gap". The script creates
# the label if it does not already exist.

set -euo pipefail

REPO="chifamba/vashandi"

echo "==> Ensuring 'roadmap-gap' label exists..."
gh label create "roadmap-gap" \
  --repo "$REPO" \
  --description "Gap identified in the Vashandi + OpenBrain roadmap" \
  --color "D93F0B" 2>/dev/null || echo "    (label already exists, continuing)"

echo "==> Ensuring 'integration' label exists..."
gh label create "integration" \
  --repo "$REPO" \
  --description "Cross-system integration concern" \
  --color "0075CA" 2>/dev/null || echo "    (label already exists, continuing)"

echo "==> Ensuring 'openbrain' label exists..."
gh label create "openbrain" \
  --repo "$REPO" \
  --description "OpenBrain memory system" \
  --color "008672" 2>/dev/null || echo "    (label already exists, continuing)"

echo "==> Ensuring 'vashandi' label exists..."
gh label create "vashandi" \
  --repo "$REPO" \
  --description "Vashandi agent orchestration platform" \
  --color "E4E669" 2>/dev/null || echo "    (label already exists, continuing)"

echo ""
echo "==> Creating issues..."
echo ""

# ─── CRITICAL ────────────────────────────────────────────────────────────────

echo "[1/16] Define Vashandi <-> OpenBrain Integration Interface..."
gh issue create \
  --repo "$REPO" \
  --title "[Integration] Define Vashandi ↔ OpenBrain Integration Interface" \
  --label "roadmap-gap,integration,openbrain,vashandi" \
  --body "## Gap: Critical — No Integration Contract Exists

The existing roadmap issues for Vashandi (#31–35) and OpenBrain (#36–44) treat both systems in isolation. Issue #33 references 'Open Brain Synchronization' from the Vashandi side only, but no issue defines the actual interface between the two systems.

## What Needs to Be Defined

- **Protocol/transport**: Does Vashandi call OpenBrain over HTTP/gRPC/MCP, or does OpenBrain pull from Vashandi events?
- **API contract**: The minimal set of operations Vashandi will call on OpenBrain (ingest, query, forget, get-context-packet).
- **Event model**: Which Vashandi lifecycle events trigger a memory write (task completion, heartbeat, approval, comment)?
- **Caller identity**: Which component initiates a cross-system call — the agent adapter, the Vashandi server, or a background sync job?
- **Error contract**: What happens when a cross-system call fails — retry, queue, or silent skip?
- **Schema alignment**: How Vashandi entities (issue, comment, run, agent) map to OpenBrain memory entity types.

## Acceptance Criteria

- A written interface spec (ADR or design doc) is agreed upon before either system implements the bridge.
- Both Vashandi and OpenBrain teams can develop independently against the spec.
- The spec addresses synchronous vs. asynchronous interactions explicitly.
- The spec is versioned so breaking changes can be managed."

echo "[2/16] Bootstrap OpenBrain Project Structure in Monorepo..."
gh issue create \
  --repo "$REPO" \
  --title "[OpenBrain] Bootstrap OpenBrain Project Structure in Monorepo" \
  --label "roadmap-gap,openbrain" \
  --body "## Gap: Critical — OpenBrain Has No Physical Home in This Repo

All 9 OpenBrain epic issues (#36–44) describe features but none of them establishes where OpenBrain lives in the monorepo, what its build tooling is, or how it relates to existing packages.

## What Needs to Be Done

- **Directory placement**: Decide whether OpenBrain lives under \`packages/openbrain/\`, \`backend/openbrain/\`, or a top-level \`openbrain/\` directory.
- **Language / runtime**: Confirm the primary implementation language (TypeScript, Go, Rust, or mixed) and how it fits with the existing \`go.work\` workspace and \`pnpm-workspace.yaml\`.
- **Package scaffolding**: Initialize the package with the appropriate manifest (\`package.json\`, \`go.mod\`, etc.) and add it to the workspace.
- **Shared type bindings**: Decide whether OpenBrain shares types with \`packages/shared/\` or maintains its own schema package.
- **Build pipeline**: Add OpenBrain to the top-level build, typecheck, and test commands.
- **CI integration**: Ensure OpenBrain is included in CI runs.

## Acceptance Criteria

- \`openbrain\` appears in the monorepo workspace and compiles/builds with zero errors.
- A \`README.md\` inside the OpenBrain directory explains what it is and how to run it.
- Other packages can import OpenBrain types without path hacks."

echo "[3/16] OpenBrain Company-Scoped Memory Namespacing..."
gh issue create \
  --repo "$REPO" \
  --title "[OpenBrain] Company-Scoped Memory Namespacing and Multi-Tenancy" \
  --label "roadmap-gap,openbrain,integration" \
  --body "## Gap: Critical — OpenBrain Issues Are Written for a Single-Tenant System

Vashandi is explicitly multi-company with strict data isolation ('every entity is company-scoped'). OpenBrain epic issues (#36–44) make no mention of multi-tenancy, company partitioning, or isolation. This is a security and architecture gap.

## What Needs to Be Defined

- **Namespace model**: How does OpenBrain partition memory by Vashandi company ID? (e.g., separate schemas, row-level \`company_id\` columns, or separate vector namespaces per company)
- **Isolation enforcement**: Where is the isolation enforced — at the OpenBrain storage layer, the API layer, or the Vashandi adapter?
- **Cross-company memory**: Can any memory ever cross company boundaries (shared knowledge bases, public templates)? If so, what governance applies?
- **Agent scoping within a company**: How does OpenBrain namespace memory by agent within a company?
- **Team scoping**: Vashandi issue #32 introduces team-scoped access control — does OpenBrain need to honor team boundaries in addition to company boundaries?

## Acceptance Criteria

- Memory written for Company A cannot be read by agents of Company B under any query path.
- Isolation is enforced at the storage layer, not only at the API boundary.
- The data model documents which fields form the isolation key."

# ─── HIGH ─────────────────────────────────────────────────────────────────────

echo "[4/16] Vashandi Memory Plugin: OpenBrain Provider Adapter..."
gh issue create \
  --repo "$REPO" \
  --title "[Vashandi] Memory Plugin: OpenBrain Provider Adapter" \
  --label "roadmap-gap,vashandi,integration,openbrain" \
  --body "## Gap: High — No Vashandi-Side Adapter Wires OpenBrain into the Memory Plugin System

\`doc/memory-landscape.md\` establishes a two-layer memory model:
1. Control-plane binding layer (Vashandi's responsibility)
2. Provider adapter layer (pluggable)

Vashandi issue #33 mentions 'Open Brain Synchronization' but there is no issue for building the actual Vashandi-side plugin or adapter that wraps OpenBrain as a memory provider.

## What Needs to Be Built

- **Memory provider interface**: Define the minimal Vashandi memory provider contract (\`ingest\`, \`query\`, \`forget\`, \`context-assembly\`).
- **OpenBrain adapter**: Implement an adapter that satisfies the provider interface by calling OpenBrain's API/MCP server.
- **Company binding**: The adapter must pass the active Vashandi \`company_id\` on every call to enforce isolation.
- **Plugin registration**: Register the adapter through the existing Vashandi plugin system so it can be enabled/disabled per company.
- **Fallback behavior**: Define what happens when OpenBrain is unavailable (see separate resilience issue).
- **Cost pass-through**: Surface OpenBrain's token consumption into Vashandi's cost model (see separate cost issue).

## Acceptance Criteria

- A Vashandi company can be configured to use OpenBrain as its memory provider.
- Memory ingestion happens automatically on relevant lifecycle events without manual agent intervention.
- The adapter is testable independently of a running OpenBrain instance (mock/stub interface)."

echo "[5/16] Agent Identity Federation..."
gh issue create \
  --repo "$REPO" \
  --title "[Integration] Agent Identity Federation: Vashandi ↔ OpenBrain Trust" \
  --label "roadmap-gap,integration,openbrain,vashandi" \
  --body "## Gap: High — Two Separate Identity Systems With No Defined Relationship

Vashandi has agent API keys (hashed at rest, company-scoped). OpenBrain has its own Agent Registry and Trust Tier system (issue #38). There is no issue addressing how these two identity systems relate.

## Questions to Answer

- **Credential issuance**: When an agent authenticated in Vashandi calls OpenBrain, what credential does it present? A Vashandi-issued token? An OpenBrain-issued token derived from the Vashandi identity?
- **Trust tier mapping**: How does a Vashandi agent's role (e.g., CEO, engineer, read-only) map to OpenBrain's trust tiers (Read, Write, Promote, Delete)?
- **Token lifecycle**: When a Vashandi agent API key is rotated or revoked, is its OpenBrain credential also invalidated?
- **Human vs. agent access**: Vashandi board operators (humans) need to manage OpenBrain memory via the CLI (issue #43). What credential do they use?
- **Service-to-service auth**: When Vashandi's server calls OpenBrain on behalf of a task (not a specific agent), what identity does it use?

## Acceptance Criteria

- A single authentication event in Vashandi is sufficient to authorize OpenBrain memory access.
- No manual credential management is required for agents to use OpenBrain through Vashandi.
- Agent key revocation in Vashandi propagates to OpenBrain within a defined SLA."

echo "[6/16] Agent Lifecycle Synchronization..."
gh issue create \
  --repo "$REPO" \
  --title "[Integration] Sync Agent Lifecycle Events with OpenBrain Registry" \
  --label "roadmap-gap,integration,openbrain,vashandi" \
  --body "## Gap: High — Agent Create/Archive in Vashandi Has No Effect on OpenBrain

When Vashandi creates or archives an agent, its OpenBrain memory namespace and registry entry are unaffected. This creates memory leakage and orphaned data.

## Lifecycle Events to Handle

| Vashandi Event | Required OpenBrain Action |
|---|---|
| Agent created | Register agent in OpenBrain registry; create memory namespace |
| Agent archived | Mark OpenBrain namespace as inactive; optionally archive or delete memories |
| Agent reassigned to different company | Move memory namespace; enforce new company isolation |
| Agent role/trust changed | Update OpenBrain trust tier to match |
| Company archived | Archive or export all memories for the company |

## Design Considerations

- Should lifecycle sync be synchronous (blocking agent creation until OpenBrain confirms) or async (fire-and-forget with reconciliation)?
- What happens when OpenBrain is unavailable during an agent create? Does the Vashandi operation fail or proceed with a deferred sync?
- How are orphaned memories detected and cleaned up if sync fails?

## Acceptance Criteria

- Creating an agent in Vashandi results in a registered OpenBrain entry for that agent.
- Archiving an agent in Vashandi results in the OpenBrain namespace being closed.
- Lifecycle events are logged in Vashandi's activity log with OpenBrain operation outcomes."

echo "[7/16] Cost Tracking for OpenBrain Memory Operations..."
gh issue create \
  --repo "$REPO" \
  --title "[Vashandi] Cost Model: Include OpenBrain Memory Operation Spend" \
  --label "roadmap-gap,vashandi,openbrain" \
  --body "## Gap: High — OpenBrain Memory Costs Are Invisible to Vashandi's Budget Engine

Vashandi tracks every token/API cost and enforces hard budget stops. OpenBrain's Context Compilation (issue #40) implies token consumption for embedding, retrieval, and LLM curation. Without surfacing these costs into Vashandi's cost model, memory spend is invisible to budget enforcement.

## What Needs to Be Built

- **Cost event schema extension**: Extend Vashandi's cost event model to include a \`memory_operation\` source type alongside existing agent/task cost events.
- **OpenBrain cost reporting**: Define how OpenBrain reports token/compute costs back to Vashandi per operation (ingest, query, curation).
- **Budget attribution**: Attribute memory costs to the correct agent, task, and company for budget rollups.
- **Dashboard visibility**: Surface memory costs in the Vashandi board UI alongside agent execution costs.
- **Budget enforcement**: Ensure memory spend counts toward the hard-stop budget limit — an agent cannot bypass the budget by offloading work to OpenBrain.

## Acceptance Criteria

- The Vashandi cost dashboard shows a breakdown that includes memory operation spend.
- Budget policies apply to total spend (execution + memory combined).
- OpenBrain cost reporting is optional/graceful-degraded when OpenBrain does not support it."

echo "[8/16] OpenBrain Deployment Topology..."
gh issue create \
  --repo "$REPO" \
  --title "[Infrastructure] OpenBrain Deployment: Service Topology and Docker Configuration" \
  --label "roadmap-gap,openbrain,infrastructure" \
  --body "## Gap: High — No Decision on How OpenBrain Runs Alongside Vashandi

OpenBrain requires PostgreSQL with pgvector (issue #36). Vashandi uses embedded PGlite in dev and a standard Postgres in production. No issue addresses the deployment relationship between the two systems.

## Topology Options to Evaluate

1. **Sidecar in the same Docker Compose**: OpenBrain shares the same Docker Compose stack as Vashandi, with a shared or separate Postgres instance.
2. **Separate deployable service**: OpenBrain is an independently deployed service with its own process, port, and database.
3. **Vashandi plugin process**: OpenBrain runs as a child process managed by Vashandi's plugin system.
4. **Embedded library**: OpenBrain's core runs in-process inside Vashandi with no separate network hop.

## Questions to Answer

- Does OpenBrain need its own pgvector-enabled Postgres, or can it share Vashandi's instance with a separate schema?
- In local dev, does \`pnpm dev\` start OpenBrain automatically or is it an opt-in service?
- What ports does OpenBrain expose and how are they configured?
- How does the Vashandi dev environment discover OpenBrain's address?
- What is the production deployment model (Docker image, Helm chart, etc.)?

## Acceptance Criteria

- A documented topology decision with rationale.
- \`docker-compose.yml\` updated to include OpenBrain in the standard stack.
- Local dev can run both Vashandi and OpenBrain with a single command.
- Production deployment docs updated."

# ─── MEDIUM ───────────────────────────────────────────────────────────────────

echo "[9/16] Local Dev Setup: Run Vashandi + OpenBrain Together..."
gh issue create \
  --repo "$REPO" \
  --title "[DX] Local Dev Setup: Run Vashandi + OpenBrain Together" \
  --label "roadmap-gap,developer-experience" \
  --body "## Gap: Medium — No Developer Experience for Running Both Projects Locally

Vashandi's dev setup (\`pnpm dev\`) is zero-config. Introducing OpenBrain adds a pgvector-enabled Postgres requirement, a separate CLI/MCP server, and potentially a separate process. There is no issue for the 'both projects running locally' developer experience.

## What Needs to Be Done

- **Onboarding docs**: Update \`doc/DEVELOPING.md\` and \`README.md\` to document the combined setup.
- **\`pnpm dev\` integration**: Decide whether \`pnpm dev\` starts OpenBrain automatically or if a separate command is needed.
- **Environment variables**: Document which \`\`.env\` variables are needed for OpenBrain connectivity and defaults.
- **Docker Compose dev profile**: Add an optional dev profile that includes OpenBrain + pgvector.
- **Monorepo scripts**: Add a combined health check that verifies both Vashandi and OpenBrain are running.
- **First-run experience**: Ensure \`npx paperclipai onboard --yes\` works with or without OpenBrain present.

## Acceptance Criteria

- A developer who has cloned the repo can run both Vashandi and OpenBrain locally in under 10 minutes following the docs.
- The setup works without OpenBrain (graceful degradation).
- \`curl http://localhost:3100/api/health\` and the OpenBrain equivalent both return 200 after setup."

echo "[10/16] Resilience / Fallback When OpenBrain is Unavailable..."
gh issue create \
  --repo "$REPO" \
  --title "[Vashandi] OpenBrain Unavailability: Agent Fallback Strategy" \
  --label "roadmap-gap,vashandi,integration,resilience" \
  --body "## Gap: Medium — No Defined Behavior When OpenBrain is Down

If OpenBrain's MCP server or API is unavailable, Vashandi agents still need to operate. No issue defines the graceful degradation contract. This is especially important given the proactive context delivery model in OpenBrain issue #41.

## Options to Evaluate

| Strategy | Trade-off |
|---|---|
| **Silent skip**: Proceed without memory context | Agent may make poor decisions without historical context |
| **Queue writes, proceed without reads**: Buffer ingestion, skip retrieval | May operate stale; queue can grow unbounded |
| **Fail fast**: Block agent heartbeat until OpenBrain recovers | Safe but reduces autonomy; risks cascading pauses |
| **Circuit breaker**: Auto-disable memory after N failures, re-enable on recovery | Balanced; needs configurable thresholds |

## What Needs to Be Defined

- Default fallback behavior (must be explicitly chosen, not left undefined).
- Whether the Vashandi board is notified when OpenBrain becomes unavailable.
- How write-queuing works if chosen (persistence, ordering, max queue size).
- Whether fallback behavior is configurable per company.
- Recovery behavior when OpenBrain comes back online (replay queued writes).

## Acceptance Criteria

- A documented fallback policy is implemented and tested.
- The Vashandi activity log records when OpenBrain was unavailable and what fallback action was taken.
- Agents continue to function (possibly with degraded context) when OpenBrain is down."

echo "[11/16] OpenBrain External Service API Contract..."
gh issue create \
  --repo "$REPO" \
  --title "[OpenBrain] External Service API: Versioning and Stability Contract" \
  --label "roadmap-gap,openbrain" \
  --body "## Gap: Medium — OpenBrain's Future Consumers Have No API Contract

The vision is for OpenBrain to serve 'other services in the future' beyond Vashandi. Without an external API versioning strategy, OpenBrain's interface will be Vashandi-specific and hard to generalize later.

## What Needs to Be Established

- **API versioning scheme**: How are breaking changes signaled? (\`/v1/\`, header-based, or semver package versions)
- **Stability tiers**: Which endpoints are stable vs. experimental?
- **OpenAPI/schema**: A machine-readable schema for external consumers to generate clients from.
- **Consumer registration**: Should external consumers register (like agents in the Agent Registry) or is the API open?
- **Rate limiting and quotas**: How are non-Vashandi consumers subject to resource limits?
- **Documentation**: A minimal external-facing API reference separate from internal Vashandi integration docs.

## Design Principle

OpenBrain should be designed as a service Vashandi _uses_, not a service Vashandi _owns_. The API should be usable by a hypothetical third system without Vashandi involvement.

## Acceptance Criteria

- OpenBrain exposes a versioned API (\`/v1/\`) from day one.
- The API schema is published alongside the service.
- At least one integration test uses the OpenBrain API without any Vashandi-specific headers or context."

echo "[12/16] CEO Chat → OpenBrain Knowledge Ingestion..."
gh issue create \
  --repo "$REPO" \
  --title "[Integration] CEO Chat Context: OpenBrain Knowledge Ingestion" \
  --label "roadmap-gap,integration,openbrain,vashandi" \
  --body "## Gap: Medium — CEO Chat Outputs Have No Path Into OpenBrain

Vashandi's README roadmap lists 'CEO Chat' as a future item. CEO-level strategy discussions represent the highest-value knowledge for a memory system — strategic decisions, rationale, constraints, and pivots. Without a defined path from CEO Chat outputs to OpenBrain ingestion, this context is lost.

## What Needs to Be Designed

- **Ingestion trigger**: When a CEO Chat session ends or a strategic decision is recorded, what triggers OpenBrain ingestion?
- **Entity classification**: How does OpenBrain classify CEO Chat content — as \`decision\` entities, \`constraint\` entities, \`task\` entities, or \`fact\` entities (per the typed memory schema in issue #36)?
- **Provenance**: The OpenBrain memory record should link back to the Vashandi issue/comment/chat session it came from.
- **Human approval gate**: Strategic decisions ingested from CEO Chat may warrant human confirmation before being promoted to long-term memory.
- **Agent visibility**: Which agents can query CEO Chat-derived memories? (Visibility scoping)

## Acceptance Criteria

- A CEO Chat session that produces a strategic decision results in a memory record in OpenBrain.
- The memory record has provenance linking back to the source conversation.
- The board can review and approve/reject CEO Chat memories before they are promoted."

echo "[13/16] Company Onboarding: Seed OpenBrain Memory Namespace..."
gh issue create \
  --repo "$REPO" \
  --title "[Vashandi] Company Onboarding: Seed OpenBrain Memory Namespace" \
  --label "roadmap-gap,vashandi,openbrain" \
  --body "## Gap: Medium — New Company's OpenBrain Memory Namespace Starts Empty

When a new company is created in Vashandi, its OpenBrain memory namespace is empty. There is no issue for initial knowledge bootstrap. OpenBrain issue #44 covers repo-level \`brain.md\` sync but not the company-creation onboarding path.

## What Needs to Be Built

- **Onboarding wizard integration**: During Vashandi company creation, offer an optional step to seed OpenBrain with initial context.
- **Import sources**: Support ingesting from \`brain.md\`, project README, goal statement, or uploaded documents.
- **Company template bootstrap**: When importing a company from a Vashandi template, carry over relevant memory entries to OpenBrain.
- **Goal-derived memories**: Automatically create an OpenBrain \`fact\` or \`constraint\` memory from the company's top-level goal statement.
- **Empty-state guidance**: If OpenBrain is connected but empty, surface a prompt in the Vashandi UI for the board to add initial context.

## Acceptance Criteria

- A newly created company has at least a goal-derived memory entry in OpenBrain.
- The board can initiate a bulk import of initial context during or after company creation.
- Seeding is non-blocking — company creation succeeds even if seeding fails."

echo "[14/16] OpenBrain Graph Schema: Entity Types, Edges, and Query API..."
gh issue create \
  --repo "$REPO" \
  --title "[OpenBrain] Graph Schema: Entity Types, Edges, and Query API Design" \
  --label "roadmap-gap,openbrain" \
  --body "## Gap: Medium — OpenBrain Graph Layer Is Mentioned But Not Specified

OpenBrain issue #36 mentions a 'hybrid vector-graph' storage layer but does not define the graph schema, the relationship types between memory entities, or the query interface. For a system this central, the graph model is substantial enough to warrant a dedicated design issue.

## What Needs to Be Specified

### Entity Types
- What are the first-class entity types? (fact, decision, task, constraint, ADR, risk, person, project, etc.)
- Which fields are mandatory for all entity types (provenance, timestamp, tier, type, identity)?
- How are custom/extension entity types registered?

### Edges / Relationships
- What relationship types exist between entities? (derives-from, contradicts, supports, blocks, owned-by, etc.)
- Are edges directional or bidirectional?
- Can edges carry metadata (confidence score, timestamp)?

### Query API
- What is the traversal API — Cypher-like, GraphQL, or a custom DSL?
- Can queries combine vector similarity with graph traversal in a single operation?
- What are the performance targets for common query patterns?

### Storage Implementation
- Which graph storage backend satisfies the requirements (Apache AGE for Postgres, neo4j, or in-process)?
- How does the graph layer integrate with the pgvector store from issue #36?

## Acceptance Criteria

- A documented graph schema with at least 6 entity types and 8 relationship types.
- A query API spec (even if informal) that covers the most common retrieval patterns.
- A storage backend decision with rationale."

echo "[15/16] Integration Testing: Vashandi ↔ OpenBrain Contract Verification..."
gh issue create \
  --repo "$REPO" \
  --title "[Testing] Integration Tests: Vashandi ↔ OpenBrain Contract Verification" \
  --label "roadmap-gap,testing,integration" \
  --body "## Gap: Medium — No Integration Test Strategy for Cross-System Behavior

All existing test issues and existing tests are scoped to individual systems. Without cross-system integration tests, the Vashandi ↔ OpenBrain interface contract will drift silently.

## Test Scenarios Needed

| Scenario | What It Verifies |
|---|---|
| Task completion → OpenBrain ingest | Memory is written when a Vashandi task completes |
| Agent heartbeat → context retrieval | Context packet is delivered before an agent's next heartbeat |
| Company isolation | Agent A cannot read memories scoped to Company B |
| Budget enforcement includes memory costs | Memory spend counts toward the budget hard stop |
| Agent archive → namespace closed | Archived agent's OpenBrain namespace is no longer writable |
| OpenBrain unavailable → graceful fallback | Vashandi agents continue operating when OpenBrain is down |
| Memory promotion via Curator Agent | Curator proposal routes through Vashandi approval gate |

## Infrastructure Requirements

- A test harness that spins up both Vashandi and OpenBrain together.
- Ability to mock OpenBrain at the adapter boundary for unit-level tests.
- A contract test suite that can run against a real OpenBrain instance.

## Acceptance Criteria

- At least the company isolation and graceful fallback scenarios have automated integration tests.
- Tests run in CI on every PR that touches either system.
- Test failures produce actionable output identifying which contract was violated."

echo "[16/16] Route OpenBrain Curator Proposals Through Vashandi Approvals..."
gh issue create \
  --repo "$REPO" \
  --title "[Integration] Route OpenBrain Curator Proposals Through Vashandi Approval Gates" \
  --label "roadmap-gap,integration,openbrain,vashandi" \
  --body "## Gap: Medium — OpenBrain Needs a Human Approval Surface; Vashandi Already Has One

OpenBrain issue #42 requires human approval for significant memory changes made by the Curator Agent (Gachlaw). Vashandi already has an approval gate system (board approvals for agent hires, CEO strategy proposals, etc.). Building a separate approval surface in OpenBrain would duplicate this and fragment the human operator's attention.

## What Needs to Be Designed

- **Proposal routing**: When the Curator Agent proposes a significant memory change, it creates a Vashandi approval request (the same mechanism as CEO strategy approvals).
- **Approval payload**: The Vashandi approval UI shows the proposed memory change with context — what memory is being modified, why, and what the current state is.
- **Human response propagation**: When the board approves or rejects in Vashandi, the decision is propagated back to OpenBrain to execute or discard the proposed change.
- **Scope of 'significant'**: Define what memory changes require human approval vs. which are automatic (low-risk de-duplication vs. deletion of an ADR).
- **Async handling**: Curator proposals should not block ongoing agent work — they queue for human review.

## Why This Matters

- Keeps the board's approval workflow in one place (Vashandi UI).
- Avoids the human operator needing to monitor two separate UIs for approvals.
- Reuses existing Vashandi governance infrastructure rather than re-implementing it in OpenBrain.

## Acceptance Criteria

- A Curator Agent proposal appears as a pending approval in the Vashandi board UI.
- Approving or rejecting in Vashandi triggers the corresponding action in OpenBrain.
- The activity log in Vashandi records memory governance decisions with OpenBrain operation outcomes."

echo ""
echo "==> All 16 issues created successfully!"
echo "    View them at: https://github.com/$REPO/issues?q=label%3Aroadmap-gap"
