#!/bin/bash

# Some sed commands missed updating the parent checkbox or were looking for strings that were already updated
# This script forces the checkboxes to be checked for the services we just ported

sed -i 's/- \[ \] \*\*`access` service\*\* (`server\/src\/services\/access.ts`)/- [x] \*\*`access` service\*\* (`server\/src\/services\/access.ts`) — Go: `backend\/server\/services\/access_test.go`/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`agents` service\*\* (`server\/src\/services\/agents.ts`)/- [x] \*\*`agents` service\*\* (`server\/src\/services\/agents.ts`) — Go: `backend\/server\/services\/agents_test.go`/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`goals` service\*\*/- [x] \*\*`goals` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`projects` service\*\*/- [x] \*\*`projects` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`finance` service\*\*/- [x] \*\*`finance` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`issue-approvals` service\*\*/- [x] \*\*`issue-approvals` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`issue-assignment-wakeup` service\*\*/- [x] \*\*`issue-assignment-wakeup` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`workspace-runtime-read-model` service\*\*/- [x] \*\*`workspace-runtime-read-model` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`cron` service\*\*/- [x] \*\*`cron` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`live-events` service\*\*/- [x] \*\*`live-events` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`openbrain-client` service\*\*/- [x] \*\*`openbrain-client` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-lifecycle` service\*\*/- [x] \*\*`plugin-lifecycle` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-manifest-validator` service\*\*/- [x] \*\*`plugin-manifest-validator` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-config-validator` service\*\*/- [x] \*\*`plugin-config-validator` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-capability-validator` service\*\*/- [x] \*\*`plugin-capability-validator` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-host-services` service\*\*/- [x] \*\*`plugin-host-services` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-job-coordinator` service\*\*/- [x] \*\*`plugin-job-coordinator` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-registry` service\*\*/- [x] \*\*`plugin-registry` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
sed -i 's/- \[ \] \*\*`plugin-loader` service\*\*/- [x] \*\*`plugin-loader` service\*\*/g' ./vashandi/docs/plans/2026-04-14-typescript-test-backlog.md
