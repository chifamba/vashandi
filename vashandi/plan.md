1. **Port Remaining Missing DB Models (Phase 2 Completion)**
   - Extract the remaining missing models from `packages/db/src/schema/*.ts`.
   - Write corresponding Go struct models in `backend/db/models/*.go`.
   - Based on the previous bash script output, the missing models include:
     - `approval_comments`
     - `company_logos`
     - `company_memberships`
     - `company_secret_versions`
     - `company_skills`
     - `document_revisions`
     - `feedback_exports`
     - `feedback_votes`
     - `issue_approvals` (already found in issue_misc.go but script flagged it? Actually wait, let's verify if `issue_approvals.ts` is just `issue_approvals` or not - ah the script just checked filenames `issue_approvals.go`. It's merged into `issue_misc.go`, so we need to filter carefully).
     - Need to create proper mapping of remaining schemas to Go files, implement them fully.
   - Run tests/compilation to verify GORM models are valid. `cd backend && go build ./...`

2. **Core Server Porting (Phase 3)**
   - Initialize router `github.com/go-chi/chi/v5` and setup base structure.
   - Replicate middleware functionality (Auth, Logging, CORS).
   - Port REST API endpoints currently in `server/src/routes`.

3. **CLI Porting (Phase 6)**
   - Initialize cobra CLI `github.com/spf13/cobra` in `backend/cmd/paperclipai` or similar.
   - Port commands (e.g. `onboard`).

4. **Complete Pre Commit Steps**
   - Ensure proper testing, verification, review, and reflection are done using pre_commit_instructions.

5. **Submit Change**
   - Submit the branch to save the progress of the ported backend components.
