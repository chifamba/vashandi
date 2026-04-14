# Go Backend Parity Tracking

**Date:** 2026-04-14  
**Branch:** copilot/parity-analysis-node-services  
**Goal:** Bring the Go backend (`backend/`) to full endpoint and service parity with the Node.js server (`server/`).

---

## Summary

The Go backend currently covers ~40% of Node.js endpoints with mostly thin CRUD wrappers that lack the business logic present in the Node.js layer. This document tracks every identified gap and marks items as done when implemented.

Total Go routes at analysis time: ~90 | Total Node.js routes: ~220+

---

## 1. Authentication & Authorization

- [ ] **Real JWT/session verification in auth middleware** — Replace stub with actual bearer token parsing, board API key lookup and verification (hashed at rest), agent API key verification
- [ ] **Board API key routes** — `GET /board-claim/:token`, `POST /board-claim/:token/claim`
- [ ] **Member management routes** — `GET /companies/:companyId/members`, `PATCH /companies/:companyId/members/:userId`, `POST /companies/:companyId/members/:userId/remove`
- [ ] **Admin user access** — `GET /admin/users/:userId/company-access`, `PUT /admin/users/:userId/company-access`
- [ ] **Board mutation guard middleware** — Prevent agents from hitting board-only write endpoints
- [ ] **Error handler middleware** — Consistent `{ error, code }` JSON error response format
- [ ] **Validation middleware** — Request body schema validation

## 2. DB Models (Missing Tables)

- [ ] `approval_comments` — comments on approvals
- [ ] `company_memberships` — company member records
- [ ] `company_skills` — company-scoped skill registry
- [ ] `document_revisions` — versioned document history
- [ ] `feedback_votes` — user feedback votes on issues
- [ ] `inbox_dismissals` (functional) — already exists but needs proper persistence
- [ ] `issue_approvals` — join table: issue ↔ approval
- [ ] `issue_attachments` — file attachments on issues
- [ ] `issue_comments` — comments on issues
- [ ] `issue_documents` — documents linked to issues
- [ ] `issue_execution_decisions` — execution stage decisions
- [ ] `issue_inbox_archives` — per-user inbox archive state
- [ ] `issue_labels` — join table: issue ↔ label
- [ ] `issue_read_states` — per-user issue read state
- [ ] `issue_relations` — issue blocking relationships
- [ ] `issue_work_products` — PR/branch/artifact links
- [ ] `labels` — company-scoped labels
- [ ] `principal_permission_grants` — RBAC grants
- [ ] `project_goals` — join table: project ↔ goal
- [ ] `workspace_runtime_services` — running workspace services

## 3. Agent Management

- [ ] `PATCH /agents/:id` — update agent fields
- [ ] `POST /agents/:id/pause`, `/resume`, `/terminate`
- [ ] `GET /agents/:id/runtime-state`, `POST /agents/:id/runtime-state/reset-session`
- [ ] `GET /agents/:id/task-sessions`
- [ ] `GET /agents/:id/config-revisions`, `GET /agents/:id/config-revisions/:revId`, `POST .../rollback`
- [ ] `GET /agents/:id/configuration`
- [ ] `GET /agents/:id/keys`, `POST /agents/:id/keys`, `DELETE /agents/:id/keys/:keyId`
- [ ] `POST /agents/:id/wakeup`
- [ ] `GET /agents/:id/skills`, `POST /agents/:id/skills`
- [ ] `GET /agents/:id/instructions-bundle`, `PATCH ...`, `GET .../file`, `PUT .../file`, `DELETE .../file`
- [ ] `PATCH /agents/:id/instructions-path`
- [ ] `GET /companies/:companyId/adapters/:type/models`, `/detect-model`
- [ ] `GET /companies/:companyId/org`, `/org.svg`, `/org.png`
- [ ] `GET /companies/:companyId/agent-configurations`
- [ ] `GET /instance/scheduler-heartbeats`
- [ ] `GET /companies/:companyId/live-runs`, `/heartbeat-runs`
- [ ] `GET /heartbeat-runs/:runId`, `POST .../cancel`
- [ ] `GET /heartbeat-runs/:runId/events`, `/log`, `/workspace-operations`
- [ ] `GET /workspace-operations/:operationId/log`

## 4. Issues

- [ ] `POST /issues/:id/release` — release checkout lock
- [ ] `GET /issues/:id/documents`, CRUD for documents and revisions
- [ ] `PATCH /work-products/:id`, `DELETE /work-products/:id`
- [ ] `GET /issues/:id/comments/:commentId`
- [ ] `POST /issues/:id/read`, `DELETE /issues/:id/read`
- [ ] `POST /issues/:id/inbox-archive`, `DELETE /issues/:id/inbox-archive`
- [ ] `GET /issues/:id/approvals`, `POST /issues/:id/approvals`, `DELETE /issues/:id/approvals/:approvalId`
- [ ] `GET /issues/:id/attachments`, `POST /companies/:companyId/issues/:issueId/attachments`, `GET /attachments/:id/content`, `DELETE /attachments/:id`
- [ ] `GET /issues/:id/feedback-votes`, `POST /issues/:id/feedback-votes`
- [ ] `GET /issues/:id/heartbeat-context`
- [ ] `GET /companies/:companyId/labels`, `POST /companies/:companyId/labels`, `DELETE /labels/:labelId`

## 5. Companies

- [ ] `PATCH /companies/:companyId` — update company
- [ ] `DELETE /companies/:companyId`
- [ ] `GET /companies/stats`
- [ ] `PATCH /companies/:companyId/branding`
- [ ] `POST /companies/:companyId/exports`, `POST /companies/:companyId/imports/apply`

## 6. Costs & Budgets

- [ ] `POST /companies/:companyId/finance-events`
- [ ] `GET /companies/:companyId/costs/by-agent-model`, `/by-provider`, `/by-biller`, `/by-project`
- [ ] `GET /companies/:companyId/costs/finance-summary`, `/finance-by-biller`, `/finance-by-kind`, `/finance-events`
- [ ] `GET /companies/:companyId/costs/window-spend`, `/quota-windows`
- [ ] `GET /companies/:companyId/budgets/overview`
- [ ] `PATCH /companies/:companyId/budgets`, `PATCH /agents/:agentId/budgets`

## 7. Routines

- [ ] `POST /routines/:id/triggers` — create trigger
- [ ] `PATCH /routine-triggers/:id`, `DELETE /routine-triggers/:id`
- [ ] `POST /routine-triggers/public/:publicId/fire`
- [ ] `POST /routines/:id/run` — manual run

## 8. Company Skills

- [ ] `GET /companies/:companyId/skills/:skillId`
- [ ] `DELETE /companies/:companyId/skills/:skillId`
- [ ] `GET /companies/:companyId/skills/:skillId/update-status`
- [ ] `GET /companies/:companyId/skills/:skillId/files`
- [ ] `POST /companies/:companyId/skills/:skillId/install-update`

## 9. Execution Workspaces

- [ ] `GET /execution-workspaces/:id/close-readiness`
- [ ] `GET /execution-workspaces/:id/workspace-operations`
- [ ] `POST /execution-workspaces/:id/runtime-services/:action`

## 10. Projects

- [ ] `DELETE /projects/:id`
- [ ] `GET /projects/:id/workspaces`, `POST /projects/:id/workspaces`
- [ ] `PATCH /projects/:id/workspaces/:workspaceId`
- [ ] `POST /projects/:id/workspaces/:workspaceId/runtime-services/:action`
- [ ] `DELETE /projects/:id/workspaces/:workspaceId`

## 11. Adapters

- [ ] `POST /adapters/install`
- [ ] `PATCH /adapters/:type`, `PATCH /adapters/:type/override`
- [ ] `DELETE /adapters/:type`
- [ ] `POST /adapters/:type/reload`, `/reinstall`
- [ ] `GET /adapters/:type/config-schema`

## 12. Approvals

- [ ] `GET /approvals/:id`
- [ ] `GET /approvals/:id/issues`
- [ ] `POST /approvals/:id/resubmit`
- [ ] `GET /approvals/:id/comments`

## 13. Assets

- [ ] `POST /companies/:companyId/assets/images`
- [ ] `POST /companies/:companyId/logo`
- [ ] `GET /assets/:assetId/content` — stream asset content

## 14. Activity

- [ ] `GET /issues/:id/activity`
- [ ] `GET /issues/:id/runs`
- [ ] `GET /heartbeat-runs/:runId/issues`

## 15. Sidebar Badges (Functional)

- [ ] Compute real-time badge counts (unread inbox, pending approvals, open issues)

## 16. Inbox Dismissals (Functional)

- [ ] Proper persistence and queries against `inbox_dismissals` table

## 17. LLMs / Agent Configuration

- [ ] `GET /llms/agent-configuration.txt`
- [ ] `GET /llms/agent-icons.txt`
- [ ] `GET /llms/agent-configuration/:adapterType.txt`

## 18. Realtime / SSE

- [ ] SSE hub wired to routes
- [ ] `GET /heartbeat-runs/:runId/events` — live run event stream
- [ ] `GET /companies/:companyId/sidebar-badges/stream` — live badge updates

## 19. Middleware

- [ ] Real auth middleware (JWT + board API key + agent API key)
- [ ] Board mutation guard
- [ ] Error handler (consistent JSON errors)
- [ ] Request validation middleware

---

## Not Ported (Out of Scope)

These items are too large and complex for this parity cycle and are deferred:

- **Plugin system** (25+ routes, ~8,000 lines) — plugin loader, sandbox, worker manager, job scheduler, event bus, stream bus; retained in Node.js only
- **Company portability** (export/import as zip) — 4,417-line service; deferred
- **Full heartbeat orchestration** — workspace assignment, session compaction, billing ledger; partial Go implementation retained, full parity deferred
- **Workspace runtime management** — Docker/process lifecycle for execution workspaces; deferred
- **Feedback system** (shares, exports, redaction) — deferred
- **CLI porting** — `backend/cmd/paperclipai`; tracked in `PORT_TO_GOLANG_PLAN.md`
