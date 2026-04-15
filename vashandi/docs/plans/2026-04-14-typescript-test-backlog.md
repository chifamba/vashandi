# TypeScript Test Cases Backlog

Status: Active  
Owner: Engineering  
Date: 2026-04-14

## Summary

Inventory of existing Node/TypeScript tests across the monorepo and prioritised backlog of missing test cases. Tests live under Vitest and are run via `pnpm test:run` from the `vashandi/` root.

**Current test counts (as of 2026-04-14)**

| Area | Files |
|------|------:|
| `server/src/__tests__/` | 125 |
| `ui/src/` | 74 |
| `cli/src/__tests__/` | 21 |
| `packages/db/src/` | 4 |
| `packages/adapters/` | 9 |
| **Total** | **233** |

---

## Existing Test Coverage Inventory

### Server — Adapters

| File | What it covers |
|------|----------------|
| `claude-local-adapter.test.ts` | Max-turn detection, UI stdout parser, CLI formatter |
| `claude-local-adapter-environment.test.ts` | Environment variable resolution for Claude runs |
| `claude-local-execute.test.ts` | `--append-system-prompt-file`, session resume vs fresh, HOME/CONFIG_DIR logging |
| `claude-local-skill-sync.test.ts` | Skill sync for Claude local adapter |
| `codex-local-adapter.test.ts` | Parser, stale session detection, UI stdout parser, CLI formatter |
| `codex-local-skill-sync.test.ts` | Skill sync for Codex local adapter |
| `cursor-local-adapter.test.ts` | Parser, stale session, UI stdout parser, CLI formatter |
| `cursor-local-adapter-environment.test.ts` | Environment resolution for Cursor runs |
| `cursor-local-execute.test.ts` | Execute path |
| `cursor-local-skill-sync.test.ts` | Skill sync for Cursor local adapter |
| `cursor-local-skill-injection.test.ts` | Symlinking missing skills into Cursor skills home |
| `gemini-local-adapter.test.ts` | Parser, stale session, UI stdout parser, CLI formatter |
| `gemini-local-skill-sync.test.ts` | Skill sync for Gemini local adapter |
| `opencode-local-adapter.test.ts` | Parser, stale session, UI stdout parser, CLI formatter |
| `opencode-local-skill-sync.test.ts` | Skill sync for OpenCode local adapter |
| `openclaw-gateway-adapter.test.ts` | Gateway UI parser, execute flow, auto-approve pairing, test environment |
| `pi-local-execute.test.ts` | Pi exhausted-retry handling |
| `pi-local-skill-sync.test.ts` | Skill sync for Pi local adapter |
| `adapter-models.test.ts` | Model listing helpers |
| `adapter-session-codecs.test.ts` | Session encode/decode round-trips |
| `adapter-session-codecs.test.ts` | Session encode/decode round-trips |
| `agent-skill-contract.test.ts` | Skill contract shape validation |

### Server — Auth & Access

| File | What it covers |
|------|----------------|
| `agent-auth-jwt.test.ts` | JWT creation, verification, expiry, missing secret |
| `express5-auth-wildcard.test.ts` | Wildcard route auth in Express 5 |
| `cli-auth-routes.test.ts` | CLI auth challenge create/describe/approve/cancel |
| `invite-accept-gateway-defaults.test.ts` | Default fields on gateway invite accept |
| `invite-accept-replay.test.ts` | OpenClaw gateway invite replay eligibility, payload merging |
| `invite-join-grants.test.ts` | Permission grant assignment during join |
| `invite-join-manager.test.ts` | CEO resolution for join request assignment |
| `invite-onboarding-text.test.ts` | Onboarding message copy per adapter type |
| `openclaw-invite-prompt-route.test.ts` | CEO/board permission gating, invite summary |
| `board-auth` (CLI) | CLI board auth store CRUD |

### Server — Routes

| File | What it covers |
|------|----------------|
| `activity-routes.test.ts` | Activity list, issue activity, run activity |
| `agent-instructions-routes.test.ts` | Read/write/delete agent instruction files |
| `agent-skills-routes.test.ts` | Skill list/sync permissions per agent |
| `approval-routes-idempotency.test.ts` | Idempotent approve/reject |
| `company-branding-route.test.ts` | Branding update, logo asset |
| `company-portability-routes.test.ts` | Export preview, import preview, apply bundle |
| `company-skills-routes.test.ts` | Skill import, delete, permission gating |
| `instance-settings-routes.test.ts` | Admin vs non-admin access, guarded settings |
| `issue-activity-events-routes.test.ts` | Activity event creation on issue update |
| `issue-attachment-routes.test.ts` | Upload, download, cross-company block |
| `issue-comment-reopen-routes.test.ts` | Reopen via comment, interrupt active run |
| `issue-closed-workspace-routes.test.ts` | Reject comments/checkout for closed workspaces |
| `issue-dependency-wakeups-routes.test.ts` | Wake dependents on blocker completion, parent on children done |
| `issue-feedback-routes.test.ts` | Vote flush, board-only access, company scoping |
| `issue-telemetry-routes.test.ts` | Task-completed telemetry emission |
| `issue-update-comment-wakeup-routes.test.ts` | Comment-triggered heartbeat wakeup |
| `issues-checkout-wakeup.test.ts` | Self-checkout skip, cross-agent wake |
| `issues-goal-context-routes.test.ts` | Goal context surfacing, blocker summaries |
| `openclaw-invite-prompt-route.test.ts` | (see Auth section above) |
| `project-goal-telemetry-routes.test.ts` | Project/goal creation telemetry |
| `project-routes-env.test.ts` | Env binding normalisation on create/update |
| `routines-routes.test.ts` | CRUD, agent/board permission gating |
| `routines-e2e.test.ts` | Full routine trigger → issue run cycle with embedded Postgres |

### Server — Services

| File | What it covers |
|------|----------------|
| `budgets-service.test.ts` | Policy upsert, spend checking |
| `company-skills.test.ts` | Skill source parsing, directory discovery |
| `costs-service.test.ts` | Cost event creation, spend summary, quota integration |
| `execution-workspace-policy.test.ts` | Policy derivation, settings inheritance, legacy compat |
| `feedback-service.test.ts` | Vote storage, consent flow, trace bundles, artifact capture |
| `feedback-share-client.test.ts` | Shared bundle serialisation |
| `hire-hook.test.ts` | onHireApproved lifecycle, error tolerance |
| `issue-execution-policy.test.ts` | Policy normalisation, stage transitions, participant dedup |
| `issues-service.test.ts` | List filtering, create workspace inheritance, blockers, dependency wakeups |
| `issues-user-context.test.ts` | Read/unread derivation, timestamp fallbacks |
| `monthly-spend-service.test.ts` | Re-computation from current UTC month |
| `plugin-dev-watcher.test.ts` | Watch target resolution |
| `plugin-telemetry-bridge.test.ts` | Event prefixing, capability gating |
| `plugin-worker-manager.test.ts` | Stderr failure context |
| `quota-windows.test.ts` | toPercent, Claude/Codex token reading, quota parsing |
| `quota-windows-service.test.ts` | Timeout/race between slow adapters |
| `routines-service.test.ts` | Live-execution coalescing, trigger run lifecycle |
| `work-products.test.ts` | Primary work-product create with transaction |
| `workspace-runtime.test.ts` | Env sanitisation, symlink check, worktree creation |

### Server — Infrastructure

| File | What it covers |
|------|----------------|
| `app-hmr-port.test.ts` | HMR port resolution |
| `assets.test.ts` | Static asset serving |
| `attachment-types.test.ts` | Allowed MIME type gating |
| `dev-runner-output.test.ts` | Dev runner stdout/stderr formatting |
| `dev-runner-paths.test.ts` | Dev runner path resolution |
| `dev-server-status.test.ts` | Dev server health polling |
| `dev-watch-ignore.test.ts` | Glob ignore patterns for watch mode |
| `error-handler.test.ts` | Express error handler shape |
| `forbidden-tokens.test.ts` | Username token derivation and matching |
| `heartbeat-comment-wake-batching.test.ts` | Batch wakeup on comment activity (embedded Postgres) |
| `heartbeat-process-recovery.test.ts` | Orphaned process detection and recovery |
| `heartbeat-project-env.test.ts` | Project env overlay on adapter config |
| `log-redaction.test.ts` | Home-dir path and username redaction |
| `logger-tz.test.ts` | Logger timezone handling |
| `normalize-agent-mention-token.test.ts` | Agent mention token normalisation |
| `paperclip-env.test.ts` | Env var resolution order |
| `paperclip-skill-utils.test.ts` | Skill utility helpers |
| `private-hostname-guard.test.ts` | Private IP/hostname blocking |
| `redaction.test.ts` | Sensitive key and JWT redaction |
| `routine-run-telemetry.test.ts` | Routine run telemetry emission |
| `server-startup-feedback-export.test.ts` | Feedback export on startup |
| `storage-local-provider.test.ts` | Round-trip bytes, cross-company block |
| `telemetry-client-flush.test.ts` | Telemetry flush on shutdown |
| `ui-branding.test.ts` | UI branding asset injection |
| `worktree-config.test.ts` | Worktree configuration helpers |

### UI

| File | What it covers |
|------|----------------|
| `api/issues.test.ts` | `issuesApi.list` parentId parameter |
| `adapters/registry.test.ts` | Adapter registry lookup |
| `adapters/metadata.test.ts` | Adapter metadata helpers |
| `adapters/transcript.test.ts` | Transcript parsing helpers |
| `hooks/useKeyboardShortcuts.test.tsx` | Keyboard shortcut registration |
| `hooks/useCompanyPageMemory.test.ts` | Page memory persistence |
| `context/LiveUpdatesProvider.test.ts` | SSE connection lifecycle |
| `lib/activity-format.test.ts` | Activity event label formatting |
| `lib/agent-order.test.ts` | Agent list ordering |
| `lib/agent-skills-state.test.ts` | Agent skills state derivation |
| `lib/assignees.test.ts` | Assignee list helpers |
| `lib/comment-submit-draft.test.ts` | Comment draft persistence |
| `lib/company-export-selection.test.ts` | Export bundle selection logic |
| `lib/company-page-memory.test.ts` | Company page state memory |
| `lib/company-portability-sidebar.test.ts` | Portability sidebar state |
| `lib/company-routes.test.ts` | Company route path helpers |
| `lib/company-selection.test.ts` | Company selection helpers |
| `lib/document-revisions.test.ts` | Revision state derivation |
| `lib/groupBy.test.ts` | Generic groupBy utility |
| `lib/inbox.test.ts` | Inbox badge count, dismissal, unread/read, approvals, join requests |
| `lib/instance-settings.test.ts` | Instance settings helpers |
| `lib/issue-chat-messages.test.ts` | Chat message helpers |
| `lib/issue-execution-policy.test.ts` | UI-side policy helpers |
| `lib/issue-filters.test.ts` | Filter label, toggle, apply, count |
| `lib/issue-timeline-events.test.ts` | Timeline event derivation |
| `lib/issue-tree.test.ts` | Hierarchy building, orphan promotion |
| `lib/issueChatTranscriptRuns.test.ts` | Chat transcript run mapping |
| `lib/issueDetailBreadcrumb.test.ts` | Breadcrumb generation |
| `lib/keyboardShortcuts.test.ts` | Shortcut config |
| `lib/legacy-agent-config.test.ts` | Legacy config migration |
| `lib/main-content-focus.test.ts` | Focus management helpers |
| `lib/markdownPaste.test.ts` | Markdown paste handler |
| `lib/mention-aware-link-node.test.ts` | Mention link node |
| `lib/mention-deletion.test.ts` | Mention deletion in editor |
| `lib/model-utils.test.ts` | Model display name helpers |
| `lib/navigation-scroll.test.ts` | Scroll-restoration helpers |
| `lib/new-agent-runtime-config.test.ts` | New agent runtime config defaults |
| `lib/normalize-markdown.test.ts` | Markdown normalisation |
| `lib/onboarding-goal.test.ts` | Onboarding goal helpers |
| `lib/onboarding-launch.test.ts` | Onboarding launch gating |
| `lib/onboarding-route.test.ts` | Onboarding route resolution |
| `lib/optimistic-issue-comments.test.ts` | Optimistic comment updates |
| `lib/optimistic-issue-runs.test.ts` | Optimistic run upsert/remove |
| `lib/portable-files.test.ts` | Portable file helpers |
| `lib/project-order.test.ts` | Project ordering |
| `lib/project-workspaces-tab.test.ts` | Project workspaces tab state |
| `lib/recent-assignees.test.ts` | Recent assignee tracking |
| `lib/routine-trigger-patch.test.ts` | Routine trigger patch helpers |
| `lib/status-colors.test.ts` | Status colour mapping |
| `lib/timeAgo.test.ts` | Time-ago formatting |
| `lib/transcriptPresentation.test.ts` | Transcript rendering helpers |
| `lib/utils.test.ts` | Shared utility functions |
| `lib/worktree-branding.test.ts` | Worktree branding helpers |
| `lib/zip.test.ts` | Zip import/export helpers |
| `pages/GoalDetail.test.tsx` | Goal detail page render |
| `pages/Inbox.test.tsx` | Inbox page render |
| `pages/Routines.test.tsx` | Routines page render |
| `components/ApprovalPayload.test.tsx` | Approval payload display |
| `components/CommentThread.test.tsx` | Comment thread render |
| `components/InlineEditor.test.tsx` | Inline editor interactions |
| `components/IssueChatThread.test.tsx` | Issue chat thread render |
| `components/IssueDocumentsSection.test.tsx` | Issue documents section |
| `components/IssueProperties.test.tsx` | Issue properties panel |
| `components/IssueRow.test.tsx` | Issue row display |
| `components/IssuesList.test.tsx` | Issues list with filters |
| `components/IssueWorkspaceCard.test.tsx` | Workspace card display |
| `components/MarkdownBody.test.tsx` | Markdown body render |
| `components/MarkdownEditor.test.tsx` | Markdown editor interactions |
| `components/NewIssueDialog.test.tsx` | New issue dialog form |
| `components/RoutineRunVariablesDialog.test.tsx` | Routine run variables dialog |
| `components/RunInvocationCard.test.tsx` | Run invocation card |
| `components/SwipeToArchive.test.tsx` | Swipe-to-archive gesture |
| `components/transcript/RunTranscriptView.test.tsx` | Run transcript rendering |
| `components/transcript/useLiveRunTranscripts.test.tsx` | Live transcript streaming hook |

### CLI

| File | What it covers |
|------|----------------|
| `agent-jwt-env.test.ts` | Agent JWT environment variable injection |
| `allowed-hostname.test.ts` | Allowed hostname validation |
| `auth-command-registration.test.ts` | Auth command registration |
| `board-auth.test.ts` | Board auth store CRUD |
| `common.test.ts` | Common CLI helpers |
| `company.test.ts` | Company import routing, confirmation modes, dashboard URL, preview render |
| `company-delete.test.ts` | Company delete flow |
| `company-import-export-e2e.test.ts` | Export then import round-trip |
| `company-import-url.test.ts` | Import from URL resolution |
| `company-import-zip.test.ts` | Import from ZIP |
| `context.test.ts` | Client context store upsert/read/defaults |
| `data-dir.test.ts` | Data directory path resolution |
| `doctor.test.ts` | Doctor command checks |
| `feedback.test.ts` | Feedback command |
| `home-paths.test.ts` | Home directory path helpers |
| `http.test.ts` | HTTP client helpers |
| `onboard.test.ts` | Onboarding flow |
| `routines.test.ts` | `disableAllRoutinesInConfig` (embedded Postgres) |
| `telemetry.test.ts` | CLI telemetry emission |
| `worktree.test.ts` | Worktree path sanitisation, git args, seeding, branding |
| `worktree-merge-history.test.ts` | Merge history helpers |

### `packages/db`

| File | What it covers |
|------|----------------|
| `client.test.ts` | `applyPendingMigrations` with embedded Postgres |
| `embedded-postgres-error.test.ts` | Error formatting, log collection |
| `runtime-config.test.ts` | `resolveDatabaseTarget` precedence order |
| `backup-lib.test.ts` | Buffered text file writer, `runDatabaseBackup` (embedded Postgres) |

### `packages/adapters`

| File | What it covers |
|------|----------------|
| `codex-local/parse-stdout.test.ts` | Codex stdout parsing |
| `codex-local/quota-spawn-error.test.ts` | Quota spawn error detection |
| `codex-local/parse.test.ts` | Codex output parser |
| `openclaw-gateway/execute.test.ts` | Gateway execute flow |

---

## Backlog — Missing Tests

Priority: **P0** = critical path / high risk, **P1** = important, **P2** = nice to have.

---

### Server — Missing Route Tests

#### P0

- [x] **`agents` routes** (`server/src/routes/agents.ts`) — Go: `backend/server/routes/agents_crud_test.go`
  - CRUD (create, list, get, update, archive) ✓
  - Company scoping enforcement ✓
  - Permission gating: board admin vs non-admin vs agent callers (⚠ partial — authz context key bug blocks full test)
  - `reportsTo` hierarchy validation (todo)
  - Role uniqueness (CEO singleton) (todo)
  - `GET /agents/:id/models` — model list delegation to adapter (todo)

- [x] **`secrets` routes** (`server/src/routes/secrets.ts`) — Go: `backend/server/routes/secrets_test.go`
  - Create/update/delete company-level and agent-level secrets ✓
  - `local_encrypted` storage: verify ciphertext is stored, plaintext is not exposed ✓
  - `inline` secret key vs value scoping (todo)
  - Permission gating: admin vs non-admin (todo)

- [x] **`projects` routes** (`server/src/routes/projects.ts`) — Go: `backend/server/routes/projects_test.go`
  - CRUD (create, list, get, update, archive) ✓
  - Project member add/remove (todo)
  - Company scoping on all reads/writes ✓

#### P1

- [x] **`goals` routes** (`server/src/routes/goals.ts`) — Go: `backend/server/routes/goals_test.go`
  - CRUD (create, list, get, update, complete) ✓
  - Scoping to project and company ✓

- [x] **`authz` routes** (`server/src/routes/authz.ts`) — Go: `backend/server/routes/authz_test.go`
  - Grant/revoke permissions for principals (todo)
  - Board admin vs non-admin access ✓ (all tests pass after context key fix)
  - Agent-scope permission reads ✓

- [x] **`approvals` routes** (broader coverage beyond idempotency) — Go: `backend/server/routes/approvals_test.go`
  - List approvals for a company ✓
  - Comment thread on approvals ✓
  - `requestRevision` and `resubmit` flows ✓
  - Agent-only vs board-only approval actions (todo)

- [x] **`execution-workspaces` routes** (`server/src/routes/execution-workspaces.ts`) — Go: `backend/server/routes/execution_workspaces_test.go`
  - List and get workspace records ✓
  - Status and project filters ✓
  - Close workspace and linked issue rejection (partial — close readiness check ✓)
  - Runtime services action ✓
  - Workspace operations listing ✓

- [x] **`inbox-dismissals` routes** — Go: `backend/server/routes/inbox_dismissals_test.go`
  - Dismiss a run or alert ✓
  - Idempotent upsert ✓
  - User filtering ✓

#### P2

- [x] **`dashboard` routes** — summary/stats endpoint shape — Go: `backend/server/routes/dashboard_test.go`
  - Company-scoped dashboard summary ✓
  - Platform metrics ✓
- [x] **`plugins` routes** — install, uninstall, settings update, capability query — Go: `backend/server/routes/plugins_test.go`
  - ListPluginsHandler (empty, with plugins, content type) ✓
- [x] **`adapters` routes** — adapter listing, model introspection — Go: `backend/server/routes/adapters_test.go`
  - ListAdapters (builtin + plugin adapters) ✓
  - GetAdapterConfiguration (known/unknown) ✓
  - InstallAdapter, OverrideAdapter, ReloadAdapter, DeleteAdapter ✓
  - GetAdapterConfigSchema ✓
- [x] **`llms` routes** — model list by adapter type — Go: `backend/server/routes/llms_test.go`
  - ListAgentConfiguration ✓
  - ListAgentIcons ✓
  - Content-Type checks ✓
- [x] **`sidebar-badges` route** — badge count aggregation — Go: `backend/server/routes/sidebar_badges_test.go`
- [x] **`health` route** — 200 OK and database connectivity check — Go: `backend/server/routes/health_test.go`
- [x] **`org-chart-svg` route** — SVG generation from agent hierarchy — Go: `backend/server/routes/org_chart_svg_test.go`
  - Empty company ✓
  - Single agent rendering ✓
  - Hierarchy with edges ✓
  - Company scoping ✓
  - Nebula style variant ✓
  - Long name truncation ✓
  - PNG fallback to SVG ✓
  - htmlEscape helper ✓
- [x] **`companies` routes** — Go: `backend/server/routes/companies_test.go`
  - CRUD (list, get, update, delete) ✓
  - Branding update ✓
  - Stats endpoint ✓
  - Export/import stubs ✓
  - Filtered field enforcement ✓
- [x] **`routines` routes** — Go: `backend/server/routes/routines_test.go`
  - CRUD (list, get, create, update, delete) ✓
  - Triggers (create, delete, fire) ✓
  - Run listing ✓
  - Run-now action ✓
- [x] **`activity` routes** — Go: `backend/server/routes/activity_test.go`
  - List activity (company-scoped, entity-type filter) ✓
  - Create activity ✓
  - Issue activity listing ✓
  - Heartbeat run issues listing ✓
- [x] **`instance-settings` routes** — Go: `backend/server/routes/instance_settings_test.go`
  - Get/update general settings ✓
  - Get/update experimental settings ✓
- [x] **`teams` routes** — Go: `backend/server/routes/teams_test.go`
  - List teams (company-scoped) ✓
  - Get team ✓
- [x] **`costs` routes** — Go: `backend/server/routes/costs_test.go`
  - Cost summary ✓
  - Costs by agent ✓
  - Costs by provider ✓
  - Budget overview ✓
  - Budget policy update ✓
  - Finance events listing ✓
  - Finance summary ✓
- [x] **`access` routes** — Go: `backend/server/routes/access_test.go`
  - InviteAcceptHandler: success, not-found, expired, already-accepted ✓
  - CLIAuthChallengeHandler: create ✓
  - ResolveCLIAuthHandler: found, not-found ✓
  - ListJoinRequestsHandler: company scoping, status filter ✓
  - ClaimJoinRequestHandler: success, not-found ✓
  - BoardClaimTokenHandler: pending, not-found ✓
  - ListSkillsHandler: content type and body ✓
  - ListCompanyMembersHandler: company scoping ✓
  - GetCLIAuthMeHandler: returns actor info ✓
  - RevokeCLIAuthCurrentHandler: returns revoked status ✓

---

### Server — Missing Service Tests

#### P0

- [x] **`secrets` service** (`server/src/services/secrets.ts`) — Go: `backend/server/services/secrets_test.go`
  - `local_encrypted` encrypt/decrypt round-trip using `ENCRYPTION_SECRET` ✓ (in `backend/shared/crypto_test.go`)
  - `resolveAdapterConfigForRuntime` merges secret values into config ✓
  - `normalizeAdapterConfigForPersistence` strips plaintext values before storage (todo)
  - Secret version rotation ✓

- [x] **`access` service** (`server/src/services/access.ts`) — Go: `backend/server/services/access_test.go`
  - `canUser` permission evaluation
  - `hasPermission` with explicit grants
  - `ensureMembership` throws on non-members
  - Company-scoped membership lookup
  - Instance admin bypass

- [x] **`agents` service** (`server/src/services/agents.ts`) — Go: `backend/server/services/agents_test.go`
  - Create with automatic membership grant
  - `resolveByReference` — by id, by name, by shortname
  - Deduplication of agent names within a company
  - Archive/unarchive
  - Monthly spend re-computation (mirrors company service)

#### P1

- [x] **`issues` service** (`server/src/services/issues.ts`) — Go: `backend/server/services/issues_test.go`
  - ListIssues company scoping ✓
  - ListIssues status filter ✓
  - ListIssues assignee filter ✓
  - CreateIssue default status ✓
  - CreateIssue with project generates identifier ✓
  - CreateIssue activity logging (verified issue creation succeeds) ✓
  - TransitionStatus: valid transitions, side effects (StartedAt, CompletedAt, CancelledAt) ✓
  - TransitionStatus: same status no-op, invalid status, not-found ✓
  - Checkout: success, already-locked, same-run idempotent ✓
  - NormalizeAgentMentionToken: HTML entity unescaping ✓
- [x] **`budgets` service** (`server/src/services/budgets.ts`) — Go: `backend/server/services/budgets_test.go`
  - CheckProjectBudget: no policy (unlimited) ✓
  - CheckProjectBudget: within budget ✓
  - CheckProjectBudget: exceeds budget ✓
  - CheckProjectBudget: exactly at budget (blocked) ✓
  - CheckProjectBudget: inactive policy ignored ✓
- [x] **`costs` service** (`server/src/services/costs.ts`) — Go: `backend/server/services/costs_test.go`
  - CreateEvent: basic creation ✓
  - CreateEvent: updates agent spend ✓
  - CreateEvent: updates company spend ✓
  - CreateEvent: defaults OccurredAt ✓
- [x] **`plugins` service** — Go: `backend/server/services/plugins_test.go`
  - ListPlugins: empty, installed-only filter ✓
  - GetPluginManifest: found, not-found, invalid JSON ✓
  - UpdatePluginStatus: status change, activity logging ✓
- [x] **`goals` service** — CRUD, completion transitions, project linkage
- [x] **`projects` service** — CRUD, archived project filtering, workspace defaults
- [x] **`finance` service** (`server/src/services/finance.ts`) — debit/credit ledger, summary by biller/kind
- [x] **`issue-approvals` service** — linking approvals to issues, listing issues pending approval
- [x] **`issue-assignment-wakeup` service** — wakeup logic when an assignee changes
- [x] **`workspace-operations` service** — operation log writes, idempotency — Go: `backend/server/services/workspace_operations_test.go`
  - CreateRecorder ✓
  - Begin (create operation record) ✓
  - Finish success/error ✓ (⚠ 2 skipped on SQLite due to UUID PK generation — will pass on PostgreSQL)
  - Multiple sequential operations ✓
- [x] **`workspace-runtime-read-model` service** — derived workspace status from events
- [x] **`workspaces` service** — workspace directory resolution — Go: `backend/server/services/workspaces_test.go`
  - deriveRepoNameFromURL: https with/without .git, ssh, bare name, empty, nested path ✓
- [x] **`run-log-store` service** — append and list run log entries — Go: `backend/server/services/run_log_store_test.go`
  - Begin creates file ✓
  - Append and read round-trip ✓
  - Empty file ✓
  - Non-existent file ✓
  - Default base path ✓
  - Multiple runs ✓
- [x] **`cron` service** — nextRunAt computation, routine trigger firing cadence

#### P2

- [x] **`live-events` service** — SSE fan-out per company, client subscribe/unsubscribe
- [x] **`openbrain-client` service** — context compile request, token budget handling, graceful fallback when OpenBrain is unavailable
- [x] **`memory-adapter` service** — InjectContextIntoPrompt, xmlEscape, stringMapToAny — Go: `backend/server/services/memory_adapter_test.go`
  - InjectContextIntoPrompt: empty XML passthrough, XML injection with agent_memory wrapper ✓
  - xmlEscape: ampersand, angle brackets, quotes, empty ✓
  - stringMapToAny: populated map, empty map ✓
- [ ] **`dashboard` service** — stats aggregation
- [x] **`plugin-lifecycle` service** — install/uninstall state machine
- [x] **`plugin-manifest-validator` service** — schema validation, capability allow-list
- [x] **`plugin-config-validator` service** — config schema enforcement
- [x] **`plugin-capability-validator` service** — capability intersection checks
- [x] **`plugin-host-services` service** — tool dispatch, job scheduling delegation
- [x] **`plugin-job-coordinator` service** — job queue ordering and concurrency — Go: `backend/server/services/plugin_job_coordinator_test.go`
- [x] **`plugin-registry` service** — installed plugin lookup — Go: `backend/server/services/plugin_registry_test.go`
- [x] **`plugin-loader` service** — dynamic module loading, sandbox setup — Go: `backend/server/services/plugin_loader_test.go`
- [x] **`activity-log` service** — `logActivity` deduplication, payload shape, company scoping — Go: `backend/server/services/activity_test.go`
  - Log with basic fields ✓
  - Log with details JSON ✓
  - Log with agent and run ID ✓
  - List with company scoping ✓
  - List with entity type filter ✓
  - Default and custom limit ✓
- [x] **`feedback-redaction` service** — PII stripping from feedback bundles — Go: `backend/server/services/feedback_redaction_test.go`
- [x] **`github-fetch` service** — authenticated GitHub API calls, rate-limit handling — Go: `backend/server/services/github_fetch_test.go`
- [x] **`default-agent-instructions` service** — template expansion — Go: `backend/server/services/default_agent_instructions_test.go`

---

### Shared / Infrastructure — Golang Port

- [x] **`home_paths`** — path resolution helpers — Go: `backend/shared/home_paths_test.go`
  - ResolvePaperclipHomeDir: default, env override, tilde expansion ✓
  - ResolvePaperclipInstanceID: default, env override ✓
  - ResolvePaperclipInstanceRoot: combined resolution ✓
  - SanitizeFriendlyPathSegment: normal, spaces, special chars, empty, all-special, dots, underscores ✓
  - ResolveManagedProjectWorkspaceDir: path structure ✓
  - ResolveDefaultAgentWorkspaceDir: path structure ✓
  - PathSegmentRegex: valid and invalid patterns ✓
  - expandHomePrefix: tilde, tilde-slash, no tilde, empty ✓
- [x] **`crypto`** — encryption helpers — Go: `backend/shared/crypto_test.go` (previously completed)
  - DecryptLocalSecret round-trip, missing env var, wrong key size, invalid JSON, invalid base64 IV, tampered ciphertext ✓

### Routes — Additional Golang Port (2026-04-14)

- [x] **`company-skills` routes** — Go: `backend/server/routes/company_skills_test.go`
  - ListCompanySkills: company scoping, empty ✓
  - CreateCompanySkill: success, bad body ✓
  - GetCompanySkill: found, not-found, cross-company block ✓
  - DeleteCompanySkill ✓
  - GetCompanySkillUpdateStatus ✓
  - GetCompanySkillFiles ✓
  - InstallUpdateCompanySkill ✓
- [x] **`mcp-governance` routes** — Go: `backend/server/routes/mcp_governance_test.go`
  - MCPToolsHandler: company scoping, empty, missing companyId ✓
  - MCPProfilesHandler: company scoping, content type ✓
  - AgentMCPToolsHandler: no entitlements, missing agentId ✓
- [x] **`heartbeat` routes** — Go: `backend/server/routes/heartbeat_test.go`
  - ListHeartbeatRuns: company scoping, missing companyId, content type, descending order, empty ✓
- [x] **`issues-checkout-wakeup` routes** — Go: `backend/server/routes/issues_checkout_wakeup_test.go`
  - ShouldWakeAssigneeOnCheckout: nil issue, no assignee, same agent, different agent, not in progress, empty actor ✓
- [x] **`context` routes** — Go: `backend/server/routes/context_test.go`
  - RegisterContextRoutes: run_start, run_complete, checkout triggers ✓
  - PreRunHydrationHandler ✓
  - PostRunCaptureHandler ✓
- [x] **`issues` routes** (`server/src/routes/issues.ts`) — Go: `backend/server/routes/issues_test.go`
  - ListIssuesHandler (company scoping, status filter) ✓
  - ListAllIssuesHandler (all companies, status filter) ✓
  - GetIssueHandler (found, not-found) ✓
  - CreateIssueHandler (success with company scoping, bad body) ✓
  - UpdateIssueHandler (success, not-found) ✓
  - DeleteIssueHandler (soft-delete via hidden_at, not-found) ✓
  - TransitionIssueHandler (success, bad body) ✓
  - AddIssueCommentHandler & ListIssueCommentsHandler (create + list round-trip, issue not-found) ✓
  - CreateWorkProductHandler & ListWorkProductsHandler (create + list round-trip) ✓
  - BulkUpdateIssuesHandler (multiple updates with company scoping, empty IDs) ✓
  - ReleaseIssueHandler (success, not-found) ✓
  - ListIssueLabelsHandler (company scoping) ✓
  - CreateLabelHandler, DeleteLabelHandler ✓
  - Content-Type checks ✓
- [x] **`memory` routes** — Go: `backend/server/routes/memory_test.go`
  - MemoryBindingsHandler (returns empty array, content type) ✓
  - MemoryOperationsHandler (returns empty array, content type) ✓
- [x] **`chat` routes** — Go: `backend/server/routes/chat_test.go`
  - CeoChatIngestionHandler (bad body, returns ingested status) ✓
  - RegisterChatRoutes (route registration) ✓
- [x] **`handoff` routes** — Go: `backend/server/routes/handoff_test.go`
  - HandoffIssueHandler (success with assignee update, issue not-found, bad body) ✓
  - Handoff markdown stored on original run ✓
- [x] **`assets` routes** — Go: `backend/server/routes/assets_test.go`
  - GetAssetHandler (found, not-found) ✓
  - GetAssetContentHandler (found, not-found) ✓
  - GetAttachmentContentHandler (found, not-found) ✓
  - DeleteAttachmentHandler ✓

---

### UI — Missing Page Tests

#### P1

- [ ] **`IssueDetail` page** — renders title, description, assignee, status; comment submit; run start/stop
- [ ] **`AgentDetail` page** — renders agent properties, skills list, instructions view
- [ ] **`Approvals` page** — lists pending approvals; approve/reject actions
- [ ] **`CompanySettings` page** — renders settings tabs; branding update
- [ ] **`Issues` / `MyIssues` pages** — filtering, search, empty state

#### P2

- [ ] **`Goals` page** — goal tree render, create goal dialog
- [ ] **`Projects` page** — project list, create project dialog
- [ ] **`Agents` page** — agent list, invite agent flow
- [ ] **`Costs` page** — spend charts render
- [ ] **`PluginManager` page** — plugin list, install/uninstall button
- [ ] **`Org` / `OrgChart` page** — hierarchy SVG render
- [ ] **`Dashboard` page** — summary stats render
- [ ] **`CliAuth` page** — challenge approve flow
- [ ] **`Auth` / `InviteLanding` pages** — login form, invite acceptance
- [ ] **`CompanySkills` page** — skill list, import form
- [ ] **`InstanceSettings` pages** — admin-only gating in the UI

---

### UI — Missing Component Tests

#### P1

- [ ] **`CommandPalette`** — keyboard open/close, fuzzy search results, action dispatch
- [ ] **`FilterBar`** — filter chip render, add/remove filter, clear all
- [ ] **`AgentConfigForm`** — form validation, adapter type switching, secret fields masked
- [ ] **`EnvVarEditor`** — add/edit/delete env rows, key uniqueness validation
- [ ] **`CommentThread`** (extended) — reply nesting, markdown render, optimistic insert
- [ ] **`ApprovalCard`** — approve/reject/revision buttons, status badge

#### P2

- [ ] **`BudgetPolicyCard`** — displays limits, shows over-budget warning
- [ ] **`ActiveAgentsPanel`** — live agent count, per-agent run status
- [ ] **`GoalTree`** — tree expand/collapse, create child goal
- [ ] **`CompanySwitcher`** — company list, switch action
- [ ] **`AgentActionButtons`** — start/stop/cancel run actions
- [ ] **`ExecutionWorkspaceCloseDialog`** — confirmation modal flow
- [ ] **`ImageGalleryModal`** — image navigation, zoom
- [ ] **`DocumentDiffModal`** — diff rendering

---

### UI — Missing Lib Tests

#### P1

- [ ] **`lib/goals.ts`** (if exists) — goal state derivation
- [ ] **`lib/approval-utils.ts`** (if exists) — approval status helpers
- [ ] **`lib/execution-workspace-utils.ts`** (if exists) — workspace mode helpers

#### P2

- [ ] Additional edge cases in `lib/issue-filters.test.ts` — multi-value filter combinations
- [ ] Additional edge cases in `lib/inbox.test.ts` — join request badge interaction with dismissals

---

### CLI — Missing Tests

#### P1

- [ ] **`issues` command** — create, list, get, comment edge cases
- [ ] **`agent` command** — invite agent, list agents
- [ ] **Secret** command coverage — create/update/delete secret with masking

#### P2

- [ ] **Profile switching** edge cases in `context.test.ts` — invalid profile names
- [ ] **`http.test.ts`** — retry on 5xx, timeout handling
- [ ] **`doctor.test.ts`** — more environment checks (missing tools, wrong Node version)

---

### `packages/db` — Missing Tests

#### P1

- [ ] **Schema export completeness** — verify all schema tables are exported from `packages/db/src/schema/index.ts`
- [ ] **Migration idempotency** — running `applyPendingMigrations` twice is a no-op

#### P2

- [ ] **`createDb` connection options** — SSL mode, pool size from env
- [ ] **`ensurePostgresDatabase` helper** — creates database when absent

---

### `packages/adapters` — Missing Tests

#### P1

- [ ] **`claude-local` package tests** (unit, not server-level)
  - `parseClaudeCliUsageText` edge cases
  - `readClaudeToken` file-not-found handling

- [ ] **`codex-local` extended**
  - `readCodexAuthInfo` with missing config file
  - `mapCodexRpcQuota` unusual shapes

#### P2

- [ ] **`cursor-local` package tests** — `parseCursorStdout` edge cases
- [ ] **`opencode-local` package tests** — session file parsing edge cases
- [ ] **`adapter-utils` package** — shared utilities that are currently untested

---

## Progress Tracking

Use the checkboxes above. When a test file is created:
1. Check the box in this document.
2. Reference the PR that added the tests in a comment.

### Golang Port — Completed (2026-04-14)

The following Go test files were created as equivalents to the TypeScript backlog items above:

| Go test file | What it covers |
|---|---|
| `backend/server/routes/health_test.go` | Health route: nil DB response, DB-connected response, Content-Type |
| `backend/server/routes/authz_test.go` | `AssertBoard`, `AssertInstanceAdmin`, `AssertCompanyAccess`, `GetActorInfo` ✓ (all pass after context key fix) |
| `backend/server/routes/secrets_test.go` | ListSecretProviders, CreateSecret (company scoping, default provider), ListSecrets (scoping), RotateSecret (increment version, not-found), DeleteSecret, UpdateSecret |
| `backend/server/routes/projects_test.go` | ListProjects (scoping), GetProject (found/not-found), CreateProject (scoping, bad body), UpdateProject (found/not-found), DeleteProject |
| `backend/server/routes/goals_test.go` | ListGoals (scoping, missing companyId→400), GetGoal (found/not-found), CreateGoal (scoping, bad body), UpdateGoal (found/not-found), DeleteGoal (found/not-found) |
| `backend/server/routes/agents_crud_test.go` | ListAgents (scoping, empty), GetAgent (found/not-found), CreateAgent (company scoping, default permissions, bad body), UpdateAgent (name update, not-found, runtimeConfig merge), PauseAgent, ResumeAgent |
| `backend/server/routes/adapters_test.go` | ListAdapters (builtin + plugin), GetAdapterConfiguration (known/unknown), InstallAdapter, OverrideAdapter, ReloadAdapter, DeleteAdapter, GetAdapterConfigSchema |
| `backend/server/routes/llms_test.go` | ListAgentConfiguration, ListAgentIcons, Content-Type checks |
| `backend/server/routes/plugins_test.go` | ListPluginsHandler (empty, with plugins, content type) |
| `backend/server/routes/access_test.go` | InviteAccept (success/not-found/expired/already-accepted), CLIAuthChallenge (create), ResolveCLIAuth (found/not-found), ListJoinRequests (scoping/status filter), ClaimJoinRequest (success/not-found), BoardClaimToken (pending/not-found), ListSkills, ListCompanyMembers, GetCLIAuthMe, RevokeCLIAuthCurrent |
| `backend/shared/crypto_test.go` | `DecryptLocalSecret` round-trip, missing env var, wrong key size, invalid JSON, invalid base64 IV, tampered ciphertext |
| `backend/server/services/secrets_test.go` | `ResolveEnvBindings` (plain, multiple, skip non-map), `ResolveAdapterConfigForRuntime` (passthrough, nested maps), `GenerateOpenBrainToken` (structure, claims) |
| `backend/server/services/issues_test.go` | ListIssues (company scoping, status filter, assignee filter), CreateIssue (default status, identifier generation, activity), TransitionStatus (valid, done, cancelled, same-status no-op, invalid, not-found), Checkout (success, already-locked, same-run idempotent), NormalizeAgentMentionToken |
| `backend/server/services/budgets_test.go` | CheckProjectBudget (no policy, within budget, exceeds, exactly at budget, inactive policy) |
| `backend/server/services/costs_test.go` | CreateEvent (basic, updates agent spend, updates company spend, defaults OccurredAt) |
| `backend/server/services/plugins_test.go` | ListPlugins (empty, installed-only), GetPluginManifest (found, not-found, invalid JSON), UpdatePluginStatus (status change, activity logging) |
| `backend/server/routes/org_chart_svg_test.go` | OrgChartSVG (empty company, single agent, hierarchy with edges, company scoping, nebula style, long name truncation, PNG fallback, htmlEscape) |
| `backend/server/routes/company_skills_test.go` | ListCompanySkills (company scoping, empty), CreateCompanySkill (success, bad body), GetCompanySkill (found, not-found, cross-company block), DeleteCompanySkill, GetCompanySkillUpdateStatus, GetCompanySkillFiles, InstallUpdateCompanySkill |
| `backend/server/routes/mcp_governance_test.go` | MCPTools (company scoping, empty, missing companyId), MCPProfiles (company scoping, content type), AgentMCPTools (no entitlements, missing agentId) |
| `backend/server/routes/heartbeat_test.go` | ListHeartbeatRuns (company scoping, missing companyId, content type, descending order, empty) |
| `backend/server/routes/issues_checkout_wakeup_test.go` | ShouldWakeAssigneeOnCheckout (nil issue, no assignee, same agent, different agent, not in progress, empty actor) |
| `backend/server/routes/context_test.go` | RegisterContextRoutes (run_start, run_complete, checkout triggers), PreRunHydrationHandler, PostRunCaptureHandler |
| `backend/server/services/workspaces_test.go` | deriveRepoNameFromURL (https with/without .git, ssh, bare name, empty, nested path) |
| `backend/server/services/memory_adapter_test.go` | InjectContextIntoPrompt (empty XML, with XML), xmlEscape (all entity types), stringMapToAny (populated, empty) |
| `backend/shared/home_paths_test.go` | ResolvePaperclipHomeDir (default, env override, tilde expansion), ResolvePaperclipInstanceID (default, env), ResolvePaperclipInstanceRoot, SanitizeFriendlyPathSegment (normal, spaces, special chars, empty, all-special, dots, underscores), ResolveManagedProjectWorkspaceDir, ResolveDefaultAgentWorkspaceDir, PathSegmentRegex, expandHomePrefix |
| `backend/server/routes/issues_test.go` | ListIssuesHandler (company scoping, status filter), ListAllIssuesHandler (all companies, status filter), GetIssueHandler (found/not-found), CreateIssueHandler (success/bad body), UpdateIssueHandler (success/not-found), DeleteIssueHandler (soft-delete/not-found), TransitionIssueHandler (success/bad body), Comments (add/list/not-found), WorkProducts (create/list), BulkUpdate (multi/empty), ReleaseIssue (success/not-found), Labels (list/create/delete), Content-Type |
| `backend/server/routes/memory_test.go` | MemoryBindingsHandler (empty array, content type), MemoryOperationsHandler (empty array, content type) |
| `backend/server/routes/chat_test.go` | CeoChatIngestionHandler (bad body, ingested status), RegisterChatRoutes |
| `backend/server/routes/handoff_test.go` | HandoffIssueHandler (success with assignee update, not-found, bad body, handoff markdown stored) |
| `backend/server/routes/assets_test.go` | GetAssetHandler (found/not-found), GetAssetContentHandler (found/not-found), GetAttachmentContentHandler (found/not-found), DeleteAttachmentHandler |

**Bug fix applied:**

- **`routes/authz.go` context key type mismatch** — `WithActor` and `GetActorInfo` each declared a local `type serverActorKey string`; in Go, locally-scoped type declarations are distinct types even with the same name, so the context `Value()` lookup never matched. Fixed by using the package-level `actorContextKey`/`actorKey` already defined at file scope. All 4 previously failing authz tests now pass.

Suggest reviewing this backlog quarterly and re-prioritising based on incident history and code churn in untested areas.
