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

- [x]**Real JWT/session verification in auth middleware** — Replace stub with actual bearer token parsing, board API key lookup and verification (hashed at rest), agent API key verification
- [x] **Board API key routes** — `GET /board-claim/:token`, `POST /board-claim/:token/claim`
- [x]**Member management routes** — `GET /companies/:companyId/members`, `PATCH /companies/:companyId/members/:userId`, `POST /companies/:companyId/members/:userId/remove`
- [x] **Admin user access** — `GET /admin/users/:userId/company-access`, `PUT /admin/users/:userId/company-access`
- [x]**Board mutation guard middleware** — Prevent agents from hitting board-only write endpoints
- [x]**Error handler middleware** — Consistent `{ error, code }` JSON error response format
- [x]**Validation middleware** — Request body schema validation

## 2. DB Models (Missing Tables)

- [x]`approval_comments` — comments on approvals
- [x]`company_memberships` — company member records
- [x]`company_skills` — company-scoped skill registry
- [x]`document_revisions` — versioned document history
- [x]`feedback_votes` — user feedback votes on issues
- [x]`inbox_dismissals` (functional) — already exists but needs proper persistence
- [x]`issue_approvals` — join table: issue ↔ approval
- [x]`issue_attachments` — file attachments on issues
- [x]`issue_comments` — comments on issues
- [x]`issue_documents` — documents linked to issues
- [x] `issue_execution_decisions` — execution stage decisions
- [x]`issue_inbox_archives` — per-user inbox archive state
- [x]`issue_labels` — join table: issue ↔ label
- [x]`issue_read_states` — per-user issue read state
- [x] `issue_relations` — issue blocking relationships
- [x]`issue_work_products` — PR/branch/artifact links
- [x]`labels` — company-scoped labels
- [x] `principal_permission_grants` — RBAC grants
- [x]`project_goals` — join table: project ↔ goal
- [x] `workspace_runtime_services` — running workspace services

## 3. Agent Management

- [x]`PATCH /agents/:id` — update agent fields
- [x]`POST /agents/:id/pause`, `/resume`, `/terminate`
- [x]`GET /agents/:id/runtime-state`, `POST /agents/:id/runtime-state/reset-session`
- [x]`GET /agents/:id/task-sessions`
- [x]`GET /agents/:id/config-revisions`, `GET /agents/:id/config-revisions/:revId`, `POST .../rollback`
- [x]`GET /agents/:id/configuration`
- [x]`GET /agents/:id/keys`, `POST /agents/:id/keys`, `DELETE /agents/:id/keys/:keyId`
- [x]`POST /agents/:id/wakeup`
- [x]`GET /agents/:id/skills`, `POST /agents/:id/skills`
- [x]`GET /agents/:id/instructions-bundle`, `PATCH ...`, `GET .../file`, `PUT .../file`, `DELETE .../file`
- [x]`PATCH /agents/:id/instructions-path`
- [x]`GET /companies/:companyId/adapters/:type/models`, `/detect-model`
- [x]`GET /companies/:companyId/org`, `/org.svg`, `/org.png`
- [x]`GET /companies/:companyId/agent-configurations`
- [x]`GET /instance/scheduler-heartbeats`
- [x]`GET /companies/:companyId/live-runs`, `/heartbeat-runs`
- [x]`GET /heartbeat-runs/:runId`, `POST .../cancel`
- [x]`GET /heartbeat-runs/:runId/events`, `/log`, `/workspace-operations`
- [x]`GET /workspace-operations/:operationId/log`

## 4. Issues

- [x]`POST /issues/:id/release` — release checkout lock
- [x]`GET /issues/:id/documents`, CRUD for documents and revisions
- [x]`PATCH /work-products/:id`, `DELETE /work-products/:id`
- [x]`GET /issues/:id/comments/:commentId`
- [x]`POST /issues/:id/read`, `DELETE /issues/:id/read`
- [x]`POST /issues/:id/inbox-archive`, `DELETE /issues/:id/inbox-archive`
- [x]`GET /issues/:id/approvals`, `POST /issues/:id/approvals`, `DELETE /issues/:id/approvals/:approvalId`
- [x]`GET /issues/:id/attachments`, `POST /companies/:companyId/issues/:issueId/attachments`, `GET /attachments/:id/content`, `DELETE /attachments/:id`
- [x]`GET /issues/:id/feedback-votes`, `POST /issues/:id/feedback-votes`
- [x]`GET /issues/:id/heartbeat-context`
- [x]`GET /companies/:companyId/labels`, `POST /companies/:companyId/labels`, `DELETE /labels/:labelId`

## 5. Companies

- [x]`PATCH /companies/:companyId` — update company
- [x]`DELETE /companies/:companyId`
- [x]`GET /companies/stats`
- [x]`PATCH /companies/:companyId/branding`
- [x] `POST /companies/:companyId/exports`, `POST /companies/:companyId/imports/apply`

## 6. Costs & Budgets

- [x]`POST /companies/:companyId/finance-events`
- [x]`GET /companies/:companyId/costs/by-agent-model`, `/by-provider`, `/by-biller`, `/by-project`
- [x]`GET /companies/:companyId/costs/finance-summary`, `/finance-by-biller`, `/finance-by-kind`, `/finance-events`
- [x]`GET /companies/:companyId/costs/window-spend`, `/quota-windows`
- [x]`GET /companies/:companyId/budgets/overview`
- [x]`PATCH /companies/:companyId/budgets`, `PATCH /agents/:agentId/budgets`

## 7. Routines

- [x]`POST /routines/:id/triggers` — create trigger
- [x]`PATCH /routine-triggers/:id`, `DELETE /routine-triggers/:id`
- [x]`POST /routine-triggers/public/:publicId/fire`
- [x]`POST /routines/:id/run` — manual run

## 8. Company Skills

- [x]`GET /companies/:companyId/skills/:skillId`
- [x]`DELETE /companies/:companyId/skills/:skillId`
- [x]`GET /companies/:companyId/skills/:skillId/update-status`
- [x]`GET /companies/:companyId/skills/:skillId/files`
- [x]`POST /companies/:companyId/skills/:skillId/install-update`

## 9. Execution Workspaces

- [x]`GET /execution-workspaces/:id/close-readiness`
- [x]`GET /execution-workspaces/:id/workspace-operations`
- [x]`POST /execution-workspaces/:id/runtime-services/:action`

## 10. Projects

- [x]`DELETE /projects/:id`
- [x]`GET /projects/:id/workspaces`, `POST /projects/:id/workspaces`
- [x] `PATCH /projects/:id/workspaces/:workspaceId`
- [x] `POST /projects/:id/workspaces/:workspaceId/runtime-services/:action`
- [x]`DELETE /projects/:id/workspaces/:workspaceId`

## 11. Adapters

- [x]`POST /adapters/install`
- [x]`PATCH /adapters/:type`, `PATCH /adapters/:type/override`
- [x]`DELETE /adapters/:type`
- [x]`POST /adapters/:type/reload`, `/reinstall`
- [x]`GET /adapters/:type/config-schema`

## 12. Approvals

- [x]`GET /approvals/:id`
- [x]`GET /approvals/:id/issues`
- [x]`POST /approvals/:id/resubmit`
- [x]`GET /approvals/:id/comments`

## 13. Assets

- [x]`POST /companies/:companyId/assets/images`
- [x]`POST /companies/:companyId/logo`
- [x]`GET /assets/:assetId/content` — stream asset content

## 14. Activity

- [x]`GET /issues/:id/activity`
- [x]`GET /issues/:id/runs`
- [x]`GET /heartbeat-runs/:runId/issues`

## 15. Sidebar Badges (Functional)

- [x]Compute real-time badge counts (unread inbox, pending approvals, open issues)

## 16. Inbox Dismissals (Functional)

- [x]Proper persistence and queries against `inbox_dismissals` table

## 17. LLMs / Agent Configuration

- [x]`GET /llms/agent-configuration.txt`
- [x]`GET /llms/agent-icons.txt`
- [x]`GET /llms/agent-configuration/:adapterType.txt`

## 18. Realtime / SSE

- [x] SSE hub wired to routes
- [x]`GET /heartbeat-runs/:runId/events` — live run event stream
- [x] `GET /companies/:companyId/sidebar-badges/stream` — live badge updates

## 19. Middleware

- [x]Real auth middleware (JWT + board API key + agent API key)
- [x]Board mutation guard
- [x]Error handler (consistent JSON errors)
- [x]Request validation middleware

## 20. Invites & Join Requests

- [x] `GET /invites/:token/onboarding.txt` — plain-text onboarding document for agents
- [x] `POST /companies/:companyId/invites` — create a company invite with token
- [x] `POST /companies/:companyId/openclaw/invite-prompt` — create agent-only OpenClaw invite
- [x] `POST /companies/:companyId/join-requests/:requestId/approve` — approve a pending join request
- [x] `POST /companies/:companyId/join-requests/:requestId/reject` — reject a pending join request
- [x] `POST /join-requests/:requestId/claim-api-key` — claim initial agent API key after approval

## 21. CLI Auth

- [x] `POST /cli-auth/challenges` — canonical plural path (singular `/cli-auth/challenge` kept as alias)
- [x] `POST /cli-auth/challenges/:id/approve` — approve CLI auth challenge
- [x] `POST /cli-auth/challenges/:id/cancel` — cancel CLI auth challenge

## 22. Instance Admin

- [x] `POST /admin/users/:userId/promote-instance-admin` — promote user to instance admin
- [x] `POST /admin/users/:userId/demote-instance-admin` — demote user from instance admin

---

## Not Ported (Out of Scope)

These items are too large and complex for this parity cycle and are deferred:

- **Plugin system** — **ported**
- **Company portability** (export/import as zip) — 4,417-line service; **ported**
- **Full heartbeat orchestration** — workspace assignment, session compaction, billing ledger; **ported**
- **Workspace runtime management** — Local process lifecycle implemented natively in Go; Docker isolation is not used. **ported**
- **Feedback system** — **ported**
- **CLI porting** — `backend/cmd/paperclipai`; tracked in `PORT_TO_GOLANG_PLAN.md`
