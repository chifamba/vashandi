# Developing

This project can run fully in local dev without setting up PostgreSQL manually.

## Deployment Modes

For mode definitions and intended CLI behavior, see `doc/DEPLOYMENT-MODES.md`.

Current implementation status:

- canonical model: `local_trusted` and `authenticated` (with `private/public` exposure)

## Prerequisites

- Node.js 20+
- pnpm 9+
- Go 1.25+ (for Go backend services in `vashandi/backend/` and `openbrain/`)

## Dependency Lockfile Policy

GitHub Actions owns `pnpm-lock.yaml`.

- Do not commit `pnpm-lock.yaml` in pull requests.
- Pull request CI validates dependency resolution when manifests change.
- Pushes to `master` regenerate `pnpm-lock.yaml` with `pnpm install --lockfile-only --no-frozen-lockfile`, commit it back if needed, and then run verification with `--frozen-lockfile`.

## Start Dev

From repo root:

```sh
pnpm install
pnpm dev
```

This starts:

- API server: `http://localhost:3100`
- UI: served by the API server in dev middleware mode (same origin as API)
- OpenBrain can be started separately from the monorepo `openbrain/` directory with `go test ./...`, `go build ./cmd/openbrain`, and `./openbrain serve`

See the **OpenBrain** section below for full OpenBrain dev commands.

`pnpm dev` runs the server in watch mode and restarts on changes from workspace packages (including adapter packages). Use `pnpm dev:once` to run without file watching.

`pnpm dev:once` auto-applies pending local migrations by default before starting the dev server.

`pnpm dev` and `pnpm dev:once` are now idempotent for the current repo and instance: if the matching Paperclip dev runner is already alive, Paperclip reports the existing process instead of starting a duplicate.

Inspect or stop the current repo's managed dev runner:

```sh
pnpm dev:list
pnpm dev:stop
```

`pnpm dev:once` now tracks backend-relevant file changes and pending migrations. When the current boot is stale, the board UI shows a `Restart required` banner. You can also enable guarded auto-restart in `Instance Settings > Experimental`, which waits for queued/running local agent runs to finish before restarting the dev server.

Tailscale/private-auth dev mode:

```sh
pnpm dev --tailscale-auth
```

This runs dev as `authenticated/private` and binds the server to `0.0.0.0` for private-network access.

Allow additional private hostnames (for example custom Tailscale hostnames):

```sh
pnpm paperclipai allowed-hostname dotta-macbook-pro
```

## One-Command Local Run

For a first-time local install, you can bootstrap and run in one command:

```sh
pnpm paperclipai run
```

`paperclipai run` does:

1. auto-onboard if config is missing
2. `paperclipai doctor` with repair enabled
3. starts the server when checks pass

## Docker Quickstart (No local Node install)

Build and run Paperclip in Docker:

```sh
docker build -t paperclip-local .
docker run --name paperclip \
  -p 3100:3100 \
  -e HOST=0.0.0.0 \
  -e PAPERCLIP_HOME=/paperclip \
  -v "$(pwd)/data/docker-paperclip:/paperclip" \
  paperclip-local
```

Or use Compose:

```sh
docker compose -f docker/docker-compose.quickstart.yml up --build
```

See `doc/DOCKER.md` for API key wiring (`OPENAI_API_KEY` / `ANTHROPIC_API_KEY`) and persistence details.

## Docker For Untrusted PR Review

For a separate review-oriented container that keeps `codex`/`claude` login state in Docker volumes and checks out PRs into an isolated scratch workspace, see `doc/UNTRUSTED-PR-REVIEW.md`.

## Database in Dev (Auto-Handled)

For local development, leave `DATABASE_URL` unset.
The server will automatically use embedded PostgreSQL and persist data at:

- `~/.paperclip/instances/default/db`

Override home and instance:

```sh
PAPERCLIP_HOME=/custom/path PAPERCLIP_INSTANCE_ID=dev pnpm paperclipai run
```

No Docker or external database is required for this mode.

## Storage in Dev (Auto-Handled)

For local development, the default storage provider is `local_disk`, which persists uploaded images/attachments at:

- `~/.paperclip/instances/default/data/storage`

Configure storage provider/settings:

```sh
pnpm paperclipai configure --section storage
```

## Default Agent Workspaces

When a local agent run has no resolved project/session workspace, Paperclip falls back to an agent home workspace under the instance root:

- `~/.paperclip/instances/default/workspaces/<agent-id>`

This path honors `PAPERCLIP_HOME` and `PAPERCLIP_INSTANCE_ID` in non-default setups.

For `codex_local`, Paperclip also manages a per-company Codex home under the instance root and seeds it from the shared Codex login/config home (`$CODEX_HOME` or `~/.codex`):

- `~/.paperclip/instances/default/companies/<company-id>/codex-home`

If the `codex` CLI is not installed or not on `PATH`, `codex_local` agent runs fail at execution time with a clear adapter error. Quota polling uses a short-lived `codex app-server` subprocess: when `codex` cannot be spawned, that provider reports `ok: false` in aggregated quota results and the API server keeps running (it must not exit on a missing binary).

## Worktree-local Instances

When developing from multiple git worktrees, do not point two Paperclip servers at the same embedded PostgreSQL data directory.

Instead, create a repo-local Paperclip config plus an isolated instance for the worktree:

```sh
paperclipai worktree init
# or create the git worktree and initialize it in one step:
pnpm paperclipai worktree:make paperclip-pr-432
```

This command:

- writes repo-local files at `.paperclip/config.json` and `.paperclip/.env`
- creates an isolated instance under `~/.paperclip-worktrees/instances/<worktree-id>/`
- when run inside a linked git worktree, mirrors the effective git hooks into that worktree's private git dir
- picks a free app port and embedded PostgreSQL port
- by default seeds the isolated DB in `minimal` mode from the current effective Paperclip instance/config (repo-local worktree config when present, otherwise the default instance) via a logical SQL snapshot

Seed modes:

- `minimal` keeps core app state like companies, projects, issues, comments, approvals, and auth state, preserves schema for all tables, but omits row data from heavy operational history such as heartbeat runs, wake requests, activity logs, runtime services, and agent session state
- `full` makes a full logical clone of the source instance
- `--no-seed` creates an empty isolated instance

After `worktree init`, both the server and the CLI auto-load the repo-local `.paperclip/.env` when run inside that worktree, so normal commands like `pnpm dev`, `paperclipai doctor`, and `paperclipai db:backup` stay scoped to the worktree instance.

Provisioned git worktrees also pause all seeded routines in the isolated worktree database by default. This prevents copied daily/cron routines from firing unexpectedly inside the new workspace instance during development.

That repo-local env also sets:

- `PAPERCLIP_IN_WORKTREE=true`
- `PAPERCLIP_WORKTREE_NAME=<worktree-name>`
- `PAPERCLIP_WORKTREE_COLOR=<hex-color>`

The server/UI use those values for worktree-specific branding such as the top banner and dynamically colored favicon.

Print shell exports explicitly when needed:

```sh
paperclipai worktree env
# or:
eval "$(paperclipai worktree env)"
```

### Worktree CLI Reference

**`pnpm paperclipai worktree init [options]`** — Create repo-local config/env and an isolated instance for the current worktree.

| Option | Description |
|---|---|
| `--name <name>` | Display name used to derive the instance id |
| `--instance <id>` | Explicit isolated instance id |
| `--home <path>` | Home root for worktree instances (default: `~/.paperclip-worktrees`) |
| `--from-config <path>` | Source config.json to seed from |
| `--from-data-dir <path>` | Source PAPERCLIP_HOME used when deriving the source config |
| `--from-instance <id>` | Source instance id (default: `default`) |
| `--server-port <port>` | Preferred server port |
| `--db-port <port>` | Preferred embedded Postgres port |
| `--seed-mode <mode>` | Seed profile: `minimal` or `full` (default: `minimal`) |
| `--no-seed` | Skip database seeding from the source instance |
| `--force` | Replace existing repo-local config and isolated instance data |

Examples:

```sh
paperclipai worktree init --no-seed
paperclipai worktree init --seed-mode full
paperclipai worktree init --from-instance default
paperclipai worktree init --from-data-dir ~/.paperclip
paperclipai worktree init --force
```

Repair an already-created repo-managed worktree and reseed its isolated instance from the main default install:

```sh
cd ~/.paperclip/worktrees/PAP-884-ai-commits-component
pnpm paperclipai worktree init --force --seed-mode minimal \
  --name PAP-884-ai-commits-component \
  --from-config ~/.paperclip/instances/default/config.json
```

That rewrites the worktree-local `.paperclip/config.json` + `.paperclip/.env`, recreates the isolated instance under `~/.paperclip-worktrees/instances/<worktree-id>/`, and preserves the git worktree contents themselves.

For an already-created worktree where you want to keep the existing repo-local config/env and only overwrite the isolated database, use `worktree reseed` instead. Stop the target worktree's Paperclip server first so the command can replace the DB safely.

**`pnpm paperclipai worktree reseed [options]`** — Re-seed an existing worktree-local instance from another Paperclip instance or worktree while preserving the target worktree's current config, ports, and instance identity.

| Option | Description |
|---|---|
| `--from <worktree>` | Source worktree path, directory name, branch name, or `current` |
| `--to <worktree>` | Target worktree path, directory name, branch name, or `current` (defaults to `current`) |
| `--from-config <path>` | Source config.json to seed from |
| `--from-data-dir <path>` | Source `PAPERCLIP_HOME` used when deriving the source config |
| `--from-instance <id>` | Source instance id when deriving the source config |
| `--seed-mode <mode>` | Seed profile: `minimal` or `full` (default: `full`) |
| `--yes` | Skip the destructive confirmation prompt |
| `--allow-live-target` | Override the guard that requires the target worktree DB to be stopped first |

Examples:

```sh
# From the main repo, reseed a worktree from the current default/master instance.
cd /path/to/paperclip
pnpm paperclipai worktree reseed \
  --from current \
  --to PAP-1132-assistant-ui-pap-1131-make-issues-comments-be-like-a-chat \
  --seed-mode full \
  --yes

# From inside a worktree, reseed it from the default instance config.
cd /path/to/paperclip/.paperclip/worktrees/PAP-1132-assistant-ui-pap-1131-make-issues-comments-be-like-a-chat
pnpm paperclipai worktree reseed \
  --from-instance default \
  --seed-mode full
```

**`pnpm paperclipai worktree:make <name> [options]`** — Create `~/NAME` as a git worktree, then initialize an isolated Paperclip instance inside it. This combines `git worktree add` with `worktree init` in a single step.

| Option | Description |
|---|---|
| `--start-point <ref>` | Remote ref to base the new branch on (e.g. `origin/main`) |
| `--instance <id>` | Explicit isolated instance id |
| `--home <path>` | Home root for worktree instances (default: `~/.paperclip-worktrees`) |
| `--from-config <path>` | Source config.json to seed from |
| `--from-data-dir <path>` | Source PAPERCLIP_HOME used when deriving the source config |
| `--from-instance <id>` | Source instance id (default: `default`) |
| `--server-port <port>` | Preferred server port |
| `--db-port <port>` | Preferred embedded Postgres port |
| `--seed-mode <mode>` | Seed profile: `minimal` or `full` (default: `minimal`) |
| `--no-seed` | Skip database seeding from the source instance |
| `--force` | Replace existing repo-local config and isolated instance data |

Examples:

```sh
pnpm paperclipai worktree:make paperclip-pr-432
pnpm paperclipai worktree:make my-feature --start-point origin/main
pnpm paperclipai worktree:make experiment --no-seed
```

**`pnpm paperclipai worktree env [options]`** — Print shell exports for the current worktree-local Paperclip instance.

| Option | Description |
|---|---|
| `-c, --config <path>` | Path to config file |
| `--json` | Print JSON instead of shell exports |

Examples:

```sh
pnpm paperclipai worktree env
pnpm paperclipai worktree env --json
eval "$(pnpm paperclipai worktree env)"
```

For project execution worktrees, Paperclip can also run a project-defined provision command after it creates or reuses an isolated git worktree. Configure this on the project's execution workspace policy (`workspaceStrategy.provisionCommand`). The command runs inside the derived worktree and receives `PAPERCLIP_WORKSPACE_*`, `PAPERCLIP_PROJECT_ID`, `PAPERCLIP_AGENT_ID`, and `PAPERCLIP_ISSUE_*` environment variables so each repo can bootstrap itself however it wants.

## Quick Health Checks

In another terminal:

```sh
curl http://localhost:3100/api/health
curl http://localhost:3100/api/companies
```

Expected:

- `/api/health` returns `{"status":"ok"}`
- `/api/companies` returns a JSON array

## Reset Local Dev Database

To wipe local dev data and start fresh:

```sh
rm -rf ~/.paperclip/instances/default/db
pnpm dev
```

## Optional: Use External Postgres

If you set `DATABASE_URL`, the server will use that instead of embedded PostgreSQL.

## Automatic DB Backups

Paperclip can run automatic DB backups on a timer. Defaults:

- enabled
- every 60 minutes
- retain 30 days
- backup dir: `~/.paperclip/instances/default/data/backups`

Configure these in:

```sh
pnpm paperclipai configure --section database
```

Run a one-off backup manually:

```sh
pnpm paperclipai db:backup
# or:
pnpm db:backup
```

Environment overrides:

- `PAPERCLIP_DB_BACKUP_ENABLED=true|false`
- `PAPERCLIP_DB_BACKUP_INTERVAL_MINUTES=<minutes>`
- `PAPERCLIP_DB_BACKUP_RETENTION_DAYS=<days>`
- `PAPERCLIP_DB_BACKUP_DIR=/absolute/or/~/path`

## Secrets in Dev

Agent env vars now support secret references. By default, secret values are stored with local encryption and only secret refs are persisted in agent config.

- Default local key path: `~/.paperclip/instances/default/secrets/master.key`
- Override key material directly: `PAPERCLIP_SECRETS_MASTER_KEY`
- Override key file path: `PAPERCLIP_SECRETS_MASTER_KEY_FILE`

Strict mode (recommended outside local trusted machines):

```sh
PAPERCLIP_SECRETS_STRICT_MODE=true
```

When strict mode is enabled, sensitive env keys (for example `*_API_KEY`, `*_TOKEN`, `*_SECRET`) must use secret references instead of inline plain values.

CLI configuration support:

- `pnpm paperclipai onboard` writes a default `secrets` config section (`local_encrypted`, strict mode off, key file path set) and creates a local key file when needed.
- `pnpm paperclipai configure --section secrets` lets you update provider/strict mode/key path and creates the local key file when needed.
- `pnpm paperclipai doctor` validates secrets adapter configuration and can create a missing local key file with `--repair`.

Migration helper for existing inline env secrets:

```sh
pnpm secrets:migrate-inline-env         # dry run
pnpm secrets:migrate-inline-env --apply # apply migration
```

## Company Deletion Toggle

Company deletion is intended as a dev/debug capability and can be disabled at runtime:

```sh
PAPERCLIP_ENABLE_COMPANY_DELETION=false
```

Default behavior:

- `local_trusted`: enabled
- `authenticated`: disabled

## CLI Client Operations

Paperclip CLI now includes client-side control-plane commands in addition to setup commands.

Quick examples:

```sh
pnpm paperclipai issue list --company-id <company-id>
pnpm paperclipai issue create --company-id <company-id> --title "Investigate checkout conflict"
pnpm paperclipai issue update <issue-id> --status in_progress --comment "Started triage"
```

Set defaults once with context profiles:

```sh
pnpm paperclipai context set --api-base http://localhost:3100 --company-id <company-id>
```

Then run commands without repeating flags:

```sh
pnpm paperclipai issue list
pnpm paperclipai dashboard get
```

See full command reference in `doc/CLI.md`.

## OpenClaw Invite Onboarding Endpoints

Agent-oriented invite onboarding now exposes machine-readable API docs:

- `GET /api/invites/:token` returns invite summary plus onboarding and skills index links.
- `GET /api/invites/:token/onboarding` returns onboarding manifest details (registration endpoint, claim endpoint template, skill install hints).
- `GET /api/invites/:token/onboarding.txt` returns a plain-text onboarding doc intended for both human operators and agents (llm.txt-style handoff), including optional inviter message and suggested network host candidates.
- `GET /api/skills/index` lists available skill documents.
- `GET /api/skills/paperclip` returns the Paperclip heartbeat skill markdown.

## OpenClaw Join Smoke Test

Run the end-to-end OpenClaw join smoke harness:

```sh
pnpm smoke:openclaw-join
```

What it validates:

- invite creation for agent-only join
- agent join request using `adapterType=openclaw`
- board approval + one-time API key claim semantics
- callback delivery on wakeup to a dockerized OpenClaw-style webhook receiver

Required permissions:

- This script performs board-governed actions (create invite, approve join, wakeup another agent).
- In authenticated mode, run with board auth via `PAPERCLIP_AUTH_HEADER` or `PAPERCLIP_COOKIE`.

Optional auth flags (for authenticated mode):

- `PAPERCLIP_AUTH_HEADER` (for example `Bearer ...`)
- `PAPERCLIP_COOKIE` (session cookie header value)

## OpenClaw Docker UI One-Command Script

To boot OpenClaw in Docker and print a host-browser dashboard URL in one command:

```sh
pnpm smoke:openclaw-docker-ui
```

This script lives at `scripts/smoke/openclaw-docker-ui.sh` and automates clone/build/config/start for Compose-based local OpenClaw UI testing.

Pairing behavior for this smoke script:

- default `OPENCLAW_DISABLE_DEVICE_AUTH=1` (no Control UI pairing prompt for local smoke; no extra pairing env vars required)
- set `OPENCLAW_DISABLE_DEVICE_AUTH=0` to require standard device pairing

Model behavior for this smoke script:

- defaults to OpenAI models (`openai/gpt-5.2` + OpenAI fallback) so it does not require Anthropic auth by default

State behavior for this smoke script:

- defaults to isolated config dir `~/.openclaw-paperclip-smoke`
- resets smoke agent state each run by default (`OPENCLAW_RESET_STATE=1`) to avoid stale provider/auth drift

Networking behavior for this smoke script:

- auto-detects and prints a Paperclip host URL reachable from inside OpenClaw Docker
- default container-side host alias is `host.docker.internal` (override with `PAPERCLIP_HOST_FROM_CONTAINER` / `PAPERCLIP_HOST_PORT`)
- if Paperclip rejects container hostnames in authenticated/private mode, allow `host.docker.internal` via `pnpm paperclipai allowed-hostname host.docker.internal` and restart Paperclip

---

## OpenBrain

OpenBrain is the memory OS for AI agents — a standalone Go service that runs alongside Vashandi. It lives at `openbrain/` in the monorepo root (one level above `vashandi/`).

### OpenBrain Architecture

| Layer | Technology |
|---|---|
| Language | Go (module `github.com/chifamba/vashandi/openbrain`) |
| HTTP framework | chi router |
| Database | PostgreSQL + pgvector (for semantic search) |
| Message queue | Redis (embedding cache + async ingest queue) |
| Protocols | REST/JSON, gRPC, MCP (stdio + HTTP/SSE) |
| Migrations | golang-migrate (versioned SQL in `openbrain/db/migrations/`) |
| CLI | cobra |
| Admin UI | React+Vite SPA (built to `openbrain/ui/dist/`, embedded in Go binary) |

### Prerequisites for OpenBrain

- Go 1.25+
- Node.js 20+ and npm (for rebuilding the Admin UI)
- Docker (for Postgres + pgvector)

### Quick Start (Docker Compose)

The easiest way to run the full stack including OpenBrain:

```sh
cd vashandi/docker
docker compose up --build
```

Services:
- Vashandi API: `http://localhost:3100`
- OpenBrain REST API: `http://localhost:3101`
- OpenBrain gRPC: `localhost:50051`
- Postgres (pgvector): `localhost:5432`
- Redis: `localhost:6379`

### Running OpenBrain Locally (without Docker)

1. **Start Postgres with pgvector:**

   ```sh
   docker run -d --name openbrain-pg \
     -e POSTGRES_USER=openbrain \
     -e POSTGRES_PASSWORD=openbrain \
     -e POSTGRES_DB=openbrain \
     -p 5433:5432 \
     pgvector/pgvector:pg17
   ```

2. **Start Redis:**

   ```sh
   docker run -d --name openbrain-redis -p 6379:6379 redis:8-alpine
   ```

3. **Build the Admin UI** (required before `go build` if the `ui/dist/` is missing or you have changed `ui/src/`):

   ```sh
   cd openbrain/ui
   npm ci
   npm run build
   ```

4. **Build and run OpenBrain:**

   ```sh
   cd openbrain
   go build ./cmd/openbrain
   DATABASE_URL=postgres://openbrain:openbrain@localhost:5433/openbrain?sslmode=disable \
   REDIS_URL=redis://localhost:6379 \
   OPENBRAIN_API_KEY=dev_secret_token \
   ./openbrain serve
   ```

5. **Health check:**

   ```sh
   curl http://localhost:3101/healthz
   # {"status":"ok"}
   ```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | PostgreSQL connection string (required for production) |
| `PORT` | `3101` | HTTP port |
| `GRPC_PORT` | `50051` | gRPC port |
| `OPENBRAIN_API_KEY` | `dev_secret_token` | Static API key for auth middleware |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection string |
| `OPENAI_API_KEY` | — | OpenAI key for async embedding generation |
| `EMBEDDING_MODEL` | `text-embedding-3-small` | OpenAI embedding model |
| `DB_MAX_CONNS` | `10` | PostgreSQL max connections |
| `DB_MIN_CONNS` | `2` | PostgreSQL min connections |
| `DB_MAX_CONN_IDLE_SECS` | `300` | Max idle time for a connection (seconds) |
| `DB_MAX_CONN_LIFETIME_SECS` | `3600` | Max lifetime for a connection (seconds) |

### Common Dev Commands

```sh
# From openbrain/ directory:

# Run all tests (uses in-memory SQLite — no DB required)
go test ./...

# Run tests with verbose output
go test -v ./...

# Build the binary (requires ui/dist/ to exist)
go build ./cmd/openbrain

# Build and run the server
go run ./cmd/openbrain serve

# Format code
gofmt -w ./cmd ./db ./internal ./jobs ./redis

# Run the CLI help
go run ./cmd/openbrain --help
```

### CLI Commands

The `openbrain` binary serves both as the server and the CLI tool. Use `--base-url` and `--token` flags to point at a running instance:

```sh
# General flags (apply to all CLI commands)
--base-url    Base URL of the OpenBrain server (default: http://localhost:3101)
--token       Bearer token for authentication

# Memory management
openbrain memory list --namespace <company-id> [--type fact|decision|adr] [--tier 0-3]
openbrain memory get <entity-id>
openbrain memory add --type <type> --content "..." [--tier 1]
openbrain memory forget <entity-id>
openbrain memory search "<query>" [--top-k 10]
openbrain memory approve <proposal-id>

# Audit log
openbrain audit export --format jsonld|sqlite --out ./audit.export

# Health
openbrain health

# Repository convention sync daemon
openbrain watch --dir ./

# Generate a scoped JWT-like token
openbrain token --namespace <company-id> [--agent-id <id>] [--trust-tier 2]
```

### Admin Web UI

OpenBrain ships a React-based admin UI embedded directly in the binary. After starting the server, navigate to:

```
http://localhost:3101/admin/
```

On first visit, enter your **Base URL** and **API Token** (e.g., `dev_secret_token` for local dev). The UI provides:

- **Dashboard** — memory counts, tier distribution, stale memory ratio, knowledge gap count, top accessed entities, daydream trigger
- **Memories** — browse/filter/search all memory entities with expandable details
- **Proposals** — review curator proposals; approve or reject with one click
- **Audit Log** — view the tamper-evident audit log in chronological order
- **Agents** — view registered agents and their trust tiers

The admin UI source lives at `openbrain/ui/src/`. After editing the source, rebuild the dist:

```sh
cd openbrain/ui
npm run build
```

Then rebuild the Go binary to pick up the updated assets:

```sh
cd openbrain
go build ./cmd/openbrain
```

### MCP Server

OpenBrain exposes an MCP (Model Context Protocol) server for LLM agent integrations:

**stdio transport** (default, for local tool use):
```sh
./openbrain                        # starts MCP on stdin/stdout when invoked by an MCP host
```

**HTTP/SSE transport** (for remote or Docker-based deployment):
```
GET  /mcp        → info
GET  /mcp/sse    → SSE stream
POST /mcp/message → tool call endpoint
```

**Available MCP tools:**
| Tool | Description |
|---|---|
| `memory_search` | Semantic + keyword search across memories |
| `memory_note` | Ingest a new memory note |
| `memory_forget` | Soft-delete a memory by ID |
| `memory_correct` | Append a correction to an existing memory |
| `memory_browse` | Browse memories with optional filters |
| `context_compile` | Compile a context packet for an agent given a task query |

### gRPC API

```
service MemoryService {
  rpc Ingest(IngestRequest) returns (IngestResponse)
  rpc Query(QueryRequest) returns (QueryResponse)
  rpc Forget(ForgetRequest) returns (ForgetResponse)
}
```

Proto definition: `openbrain/proto/v1/memory.proto`

### REST API Summary

All endpoints except `/healthz` and `/admin/*` require `Authorization: Bearer <token>`.

**Memory CRUD** (`/api/v1/memories`):
```
POST   /api/v1/memories                         # create
GET    /api/v1/memories                         # browse (?namespaceId=&tier=&entityType=)
POST   /api/v1/memories/search                  # semantic search
GET    /api/v1/memories/:id                     # get by ID
PATCH  /api/v1/memories/:id                     # update
DELETE /api/v1/memories/:id                     # soft delete
GET    /api/v1/memories/:id/versions            # list version history
POST   /api/v1/memories/:id/rollback            # rollback to prior version
```

**Graph edges** (`/api/v1/memories/edges`):
```
POST   /api/v1/memories/edges                   # create edge
GET    /api/v1/memories/:id/edges               # get edges for entity
DELETE /api/v1/memories/edges/:edgeId           # delete edge
```

**Context** (`/api/v1/context`):
```
POST   /api/v1/context/compile                  # compile context packet
GET    /api/v1/context/pending                  # get pending proactive context
```

**Admin** (`/api/v1/admin`):
```
GET    /api/v1/admin/dashboard                  # dashboard metrics
POST   /api/v1/admin/daydream                   # trigger curator
GET    /api/v1/admin/proposals                  # list curator proposals
```

**Agents** (`/api/v1/agents`):
```
GET    /api/v1/agents                           # list agents in namespace
GET    /api/v1/agents/:id                       # get agent
PATCH  /api/v1/agents/:id                       # update trust tier / recall profile
```

**Audit** (`/api/v1/audit`):
```
GET    /api/v1/audit/log                        # browse audit log
GET    /api/v1/audit/export                     # export as jsonld or sqlite
```

**Internal** (`/internal/v1/`): Vashandi→OpenBrain calls; require a service token:
```
POST   /internal/v1/namespaces                  # create namespace (on company creation)
DELETE /internal/v1/namespaces/:id              # archive namespace (on company deletion)
POST   /internal/v1/namespaces/:id/agents       # register agent
DELETE /internal/v1/namespaces/:id/agents/:agentId
POST   /internal/v1/namespaces/:id/triggers/:triggerType
POST   /internal/v1/namespaces/:id/sync         # trigger brain.md sync
```

### Database Migrations

OpenBrain uses golang-migrate for versioned SQL migrations. Migrations run automatically on server startup before GORM AutoMigrate.

Migration files: `openbrain/db/migrations/`

To add a migration:
1. Create `openbrain/db/migrations/NNNNNN_description.up.sql` and `.down.sql`
2. Restart the server — migrations run automatically

### Running Tests

All tests use SQLite in-memory — no Postgres or Redis required:

```sh
cd openbrain
go test ./...
```

Individual packages:
```sh
go test ./cmd/openbrain/...    # server + CLI tests
go test ./internal/brain/...   # (no test files yet)
go test ./internal/mcp/...     # MCP server tests
go test ./jobs/...             # curator job tests
go test ./db/models/...        # model tests
go test ./redis/...            # Redis key tests
```

### OpenBrain in CI

The PR workflow (`.github/workflows/pr.yml`) runs:

1. **Build OpenBrain UI**: `cd openbrain/ui && npm ci && npm run build`
2. **Verify OpenBrain**: `go test ./...` and `go build ./cmd/openbrain`

If you add new UI dependencies, commit the updated `package-lock.json`. The `dist/` directory is committed so the Go binary always compiles without needing to run `npm run build` first.

### Vashandi ↔ OpenBrain Integration

See `openbrain/docs/vashandi-integration-contract.yaml` for the formal OpenAPI specification of the Vashandi→OpenBrain internal API.

Key integration points:
- **Company creation** → Vashandi calls `POST /internal/v1/namespaces` with `companyId` as the namespace ID
- **Agent registration** → Vashandi calls `POST /internal/v1/namespaces/:id/agents`
- **Run start / complete** → Vashandi calls `POST /internal/v1/namespaces/:id/triggers/run_start|run_complete`
- **Task checkout** → Vashandi calls `POST /internal/v1/namespaces/:id/triggers/checkout`

### Repository Convention Files

Agents and developers can drop knowledge into an `.openbrain/` directory within any project:

```
.openbrain/
  brain.md       → curated knowledge; ingested as L2 memory entities on sync
  session.md     → current session notes; ingested as L1 memory entities
```

Start the sync daemon to watch for changes:

```sh
openbrain watch --dir /path/to/project
```

Or trigger a one-off sync via the API:

```sh
curl -X POST http://localhost:3101/internal/v1/namespaces/<id>/sync \
  -H "Authorization: Bearer <token>" \
  -d '{"dir":"/path/to/project"}'
```
