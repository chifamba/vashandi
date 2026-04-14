# Master Porting Plan: Paperclip to Go 1.24+

This document is the consolidated master tracker outlining the complete execution strategy, progress state, and the exhaustive 100% completion parity roadmap for porting the Vashandi/Paperclip monorepo (originally Node.js/TypeScript) natively to Go 1.24+.

## 1. Architecture Assessment & Technology Mapping

| Component                 | Current TypeScript Stack                                | Proposed Go 1.24+ Stack                                                  |
| ------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------ |
| **Monorepo Management**   | pnpm workspaces (`pnpm-workspace.yaml`)                 | Go Workspaces (`go.work`) alongside `pnpm` for UI                        |
| **Core Server / Routing** | Express.js + `tsx`                                      | Standard Library `net/http` (enhanced routing in Go 1.22+) or `go-chi/chi` |
| **Database & ORM**        | Drizzle ORM + PostgreSQL (`embedded-postgres`)          | `GORM` / raw `pgx`                                                       |
| **Migrations**            | Drizzle Kit                                             | `golang-migrate/migrate` or `goose`                                      |
| **Validation**            | Zod                                                     | `go-playground/validator/v10`                                            |
| **CLI Framework**         | Commander.js + `@clack/prompts`                         | `spf13/cobra` + `charmbracelet/huh` or `survey`                          |

---

## 2. Global Progress Tracker

| Phase | Description | Status |
|---|---|---|
| 1 | Go Workspace Initialization and Shared Models | Complete |
| 2 | Database Layer and GORM Migrations | Complete |
| 3 | Core Server Implementation (HTTP) | In Progress (Router initialized; Health, Dashboard, Activity, Goals routes ported. Large deficit remains, see below) |
| 4 | WebSockets and Realtime Functionality | Not Started |
| 5 | Adapters and Plugins Architecture (Heartbeat Runner) | In Progress (Anthropic native SDK base mapped, needs full LLM binding parity) |
| 6 | CLI Porting (Cobra Mapping) | Pending Internal Implementation (Commands are structurally registered but lack REST connections) |
| 7 | Testing, CI/CD, and Docker | Not Started |

---

## 3. 100% Parity Go Port Checklist

The following is the authoritative, exhaustive checklist for porting the final elements required to completely strip Node.js from the backend. 

### Part A: Server API Routes

[x] Node.js Route Migration: access.go
{Description: "Rebuild user and organization RBAC access controls mapping to access.ts",
TestCase: "Verify standard users cannot mutate company settings; verify workspace invites operate correctly",
Additional: "Porting logic from vashandi/server/src/routes/access.ts (94KB equivalent)"}

[x] Node.js Route Migration: adapters.go
{Description: "Rewrite endpoint logic exposing adapter configuration and connection test states",
TestCase: "Verify GET request retrieves status for Anthropic and OpenAI adapter health",
Additional: "Porting logic from adapters.ts"}

[x] Node.js Route Migration: approvals.go
{Description: "Handle AI task state transition requirements for human-in-the-loop interventions",
TestCase: "Trigger a required approval state on an Issue and verify the human PATCH response resumes the agent",
Additional: "Porting logic from approvals.ts"}

[x] Node.js Route Migration: assets.go
{Description: "Handle file upload, S3 bucket storage initialization, and presigned URLs",
TestCase: "Upload a file locally via REST and read the asset payload back",
Additional: "Porting logic from assets.ts"}

[x] Node.js Route Migration: authz.go
{Description: "Map Better-Auth primitives directly to Vashandi session management",
TestCase: "Validate JWT Bearer token across instance boundaries",
Additional: "Porting logic from authz.ts"}

[x] Node.js Route Migration: company_skills.go
{Description: "Translate custom skill assignments functionality into the Go data layer",
TestCase: "Assign a new CLI skill constraint to a Company and fetch assigned skill list",
Additional: "Porting logic from company-skills.ts"}

[x] Node.js Route Migration: costs.go
{Description: "Store event telemetry and process budget deduction tracking from LLM responses",
TestCase: "Verify cost accumulation accurately caps issue allocation limits",
Additional: "Porting logic from costs.ts"}

[x] Node.js Route Migration: execution_workspaces.go
{Description: "Translate execution workspace temporary filesystem creation and tear-down logic",
TestCase: "Verify worktree creation generates local /tmp folder constraints appropriately",
Additional: "Porting logic from execution-workspaces.ts"}

[x] Node.js Route Migration: inbox_dismissals.go
{Description: "Manage notification reads and dismissal states for the Operator UI",
TestCase: "Check off notification ID and verify status changes to read=true",
Additional: "Porting logic from inbox-dismissals.ts"}

[x] Node.js Route Migration: instance_settings.go
{Description: "Translate dynamic API configurations to mutate config.json state at runtime",
TestCase: "Update allowed DB retention days and review system config sync",
Additional: "Porting logic from instance-settings.ts"}

[x] Node.js Route Migration: issues_checkout_wakeup.go
{Description: "Rebuild atomic issue locking and multi-scheduler wakeup operations",
TestCase: "Trigger multiple issue checkouts guaranteeing singular assignment locks",
Additional: "Porting logic from issues-checkout-wakeup.ts"}

[x] Node.js Route Migration: llms.go
{Description: "Manage gateway connection routing tests and LLM fallback handling",
TestCase: "Ping OpenAI fallback path when primary is down",
Additional: "Porting logic from llms.ts"}

[x] Node.js Route Migration: org_chart_svg.go
{Description: "Render dynamic SVG visualizations representing Company architecture topologies",
TestCase: "GET the SVG endpoint and verify valid DOM tree representing root Company users",
Additional: "Porting logic from org-chart-svg.ts (heavy 40KB+ generator)"}

[x] Node.js Route Migration: plugin_ui_static.go
{Description: "Re-implement static proxy layers mounting Plugin UI injection bundles",
TestCase: "Access plugin IFRAME context and verify CORS mappings",
Additional: "Porting logic from plugin-ui-static.ts"}

[x] Node.js Route Migration: projects.go
{Description: "Write project boundary enforcement, settings, and workspace isolation variables",
TestCase: "Create new Project namespace and restrict agent access to it natively locally",
Additional: "Porting logic from projects.ts"}

[x] Node.js Route Migration: routines.go
{Description: "Implement tracking endpoint for automated time-polled task executions",
TestCase: "Verify 30s recurrent routine schedules update task records",
Additional: "Porting logic from routines.ts"}

[x] Node.js Route Migration: secrets.go
{Description: "Port AES Key abstraction mapping and external keyfile bindings",
TestCase: "Encrypt a test string with local hardware path key and successfully decrypt payload",
Additional: "Porting logic from secrets.ts"}

[x] Node.js Route Migration: sidebar_badges.go
{Description: "Count aggregates and notification state for Sidebar React components",
TestCase: "Load initial UI render and verify correct inbox count payload",
Additional: "Porting logic from sidebar-badges.ts"}

[x] Complete Logic Mapping: issues.go Expansion
{Description: "Expand existing 3KB Go issues route to reach complete 93KB parity with Node structure",
TestCase: "Exhaustive suite checking validation limits, cascading task deletions, and issue relations",
Additional: "issues.go exists but needs vast logic expansion to match real-world complexity"}

### Part B: CLI Tooling & Workspaces

[x] Go CLI Parity: Internal Worktree parsing algorithms
{Description: "Port worktree-merge-history-lib.ts AST parsers into Go's worktree.go handler",
TestCase: "CLI worktree command successfully merges mock AST modification locally against target project repo",
Additional: "Vital for agent system interaction allowing safe code rollbacks"}

[x] Go CLI Parity: Configuration TUI mutation
{Description: "Add interactive TUI forms to config and allowed-hostname Go CLI endpoints",
TestCase: "CLI dynamically inputs user values mapping them permanently into config.json",
Additional: "Uses Cobra selection prompts"}

[x] Go CLI Parity: Full Agent Client SDK Proxies
{Description: "Populate Go HTTP proxy methods inside client_commands.go for agent, approval, activity, dashboard, plugin, and context namespaces",
TestCase: "Execute 'paperclipai context list' via the CLI resulting in a successful GET retrieval from Go API",
Additional: "Requires matching backend Go Server API readiness integration"}

### Part C: Heartbeat & OpenBrain Adapter Mappings

[x] Heartbeat Runner: Fully Implemented Anthropic Native Execution
{Description: "Bridge LLM tool parsing, payload streaming chunk management, and JSON validations using anthropic-sdk-go",
TestCase: "Execute an end-to-end conversation with standard Tool Use integration triggering activity.Log() successfully",
Additional: "Totally bypasses and replaces the previous Claude Code exec() shell wrapping methodology"}

[x] Heartbeat Runner: OpenAI Native Execution Adapter
{Description: "Instantiate official github.com/openai/openai-go hooks mapped cleanly into the AgentRunner interfaces",
TestCase: "Codex provider initializes and passes 10 standard tool payload configurations correctly formatted to the REST backend",
Additional: "Parity matching for the existing TS codex-local adapter"}

[x] Heartbeat Runner: Gemini Native Execution Adapter
{Description: "Instantiate google.golang.org/genai integration specifically tuned for GenAI system prompt formatting limitations",
TestCase: "Gemini provider executes context constraints payload yielding identical JSON to TS versions",
Additional: "Parity matching for the existing TS gemini-local adapter"}

[x] OpenBrain Integration: Fat Context injection
{Description: "Generate strict memory XML bindings directly injecting semantic vector search results into LLM boundary variables before execution",
TestCase: "Perform Agent Heartbeat invocation specifying an active TaskID and validating the Memory String hits the final LLM Context prompt securely via mTLS",
Additional: "Ensures Go agents do not lose temporal context limits previously guaranteed by OpenBrain."}
