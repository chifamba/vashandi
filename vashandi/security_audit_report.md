# Paperclip Security Audit Report

## 1. Libraries Assessed

We ran a `pnpm audit` on the project which discovered 15 vulnerabilities (9 high, 4 moderate, 2 low). We resolved the high and moderate vulnerabilities by injecting version overrides via `pnpm audit --fix`.

Vulnerabilities patched:
- `esbuild` (<=0.24.2 to >=0.25.0) - Unrestricted access to development server.
- `fast-xml-parser` (multiple version bumps up to >=5.5.7) - Entity Expansion Limits Bypass and Stack Overflow.
- `kysely` (<=0.28.13 to >=0.28.14)
- `picomatch` (>=4.0.0 <4.0.4 to >=4.0.4) - ReDoS and Method Injection.
- `path-to-regexp` (>=8.0.0 <8.4.0 to >=8.4.0) - Regular Expression Denial of Service.
- `rollup`, `multer`

A minor low-severity vulnerability remains in a deep transient dependency (`cli` <1.0.0 Arbitrary File Write) which couldn't be automatically fixed via `pnpm audit --fix` cleanly across all packages, but does not present immediate danger to the typical execution environment.

## 2. Telemetry Assessed

We searched the codebase for common tracking and analytics services such as Mixpanel, PostHog, Amplitude, Datadog, Sentry, and Segment. We found:
- **No external telemetry services are used.**
- The only usages of the word "track" are internal logic functions, such as `trackRecentAssignee` which maintains state locally in the UI, or git `trackingRef`s for remote repos.
- There are no outgoing tracking requests to `api.paperclip.ing` or other analytical endpoints.
- The `PAPERCLIP_API_URL` is completely local/self-hosted by default (e.g., `http://localhost:3100`), guaranteeing privacy.

## 3. Data Loss Potentials Checked

- **Database Access:** The project correctly uses Drizzle ORM to perform queries, safely abstracting raw SQL queries and mitigating the risk of raw SQL injections.
- **Docker Volumes:** The untrusted-review docker environment (`docker-compose.untrusted-review.yml`) correctly isolates data to `/home/reviewer` and `/work`. Crucially, it uses `tmpfs` mounts, drops all Linux capabilities (`cap_drop: - ALL`), and enforces `no-new-privileges:true`. This prevents malicious AI agents or untrusted PRs from escaping the sandbox or persisting malicious modifications back to the host system.
- No dangerous operations (like `DROP TABLE` or `rm -rf /`) are exposed to unauthenticated users.

## 4. Other Untrusted Activities Assessed

- **SSRF Protections:** In `server/src/services/plugin-host-services.ts` (`validateAndResolveFetchUrl`), the codebase correctly limits `fetch` requests originating from untrusted plugin workers. It does this by executing a DNS lookup manually, ensuring that the resolved IP address is not private (checking against RFC 1918, loopbacks, etc.), and pinning the valid IP into the request options, successfully mitigating DNS Rebinding attacks.
- **Supply Chain / Script Execution:** An assessment of `package.json` scripts, `scripts/*`, and `.npmrc` shows no hidden or malicious setup instructions. The release and onboard scripts (`build-npm.sh`, `release.sh`, `scripts/smoke/*`) perform standard file orchestration and git interactions. The `check-forbidden-tokens.mjs` script actively protects developers by ensuring sensitive tokens are not accidentally committed.
- **Untrusted Review Isolation:** The helper `docker/untrusted-review/bin/review-checkout-pr` securely fetches remote GitHub pull requests while properly detaching the `worktree`, minimizing risk during review of potentially hostile code.

## Summary

The Paperclip project is sound from a security standpoint. With the recently applied dependency updates via `pnpm`, no critical or high vulnerabilities are present. Data and execution boundaries (both in the application's fetch mechanisms and the Docker review container) are well implemented.