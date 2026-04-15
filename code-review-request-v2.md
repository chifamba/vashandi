I have updated the port based on feedback.
Fixes include:
1. Created `plugin_lifecycle_test.go` and implemented thorough test coverage for the plugin lifecycle transitions.
2. Completed the logic inside `GetConfig`, `SetConfig`, and `DeleteConfig` in `plugin_registry.go` using GORM correctly rather than returning mock/empty structs. Tested correctly.
3. Updated the flakiness in `plugin_job_coordinator_test.go` by utilizing channels instead of `time.Sleep`.
4. Translated `plugin-host-services.ts` logic into `plugin_host_services.go` and provided comprehensive tests for SSRF fetch URL resolution and routing.
5. The tracker script explicitly only marks the completed services done.

Tests are passing and no regressions were detected.
