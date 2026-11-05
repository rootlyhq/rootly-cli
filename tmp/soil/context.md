# IR-5478 context sweep

## Verdict
Already done. Master contains the fix from PR #32 (v0.3.2; latest release v0.6.1). The ticket (customer FRAYT, GH issue rootlyhq/rootly-cli#28, Pylon #26340) was most likely filed against a CLI older than v0.3.2.

## Requirement
`rootly incidents get INC-31` returned "failed to get incident: incident not found: INC-31". The API's /v1/incidents/{id} takes a UUID, slug or bare sequential number, not the INC- display ID. Help text advertises INC-123.

## What the code does now (verified in this checkout)
- `internal/api/client.go:1151-1160`: `NormalizeIncidentID` strips a case-insensitive `INC-<digits>` prefix. UUIDs, slugs, bare numbers, `INC-` and `INC-abc` pass through unchanged.
- Callers that normalize: `internal/cmd/incidents/get.go:41`, `update.go:62`, `delete.go:61`, `internal/cmd/workflows/run.go:39`, `internal/cmd/statuspages/events_list.go:36`, `events_mutations.go:75`.
- No other command takes an incident ID. The alerts commands only print related incidents or use `--resolve-incidents`, a boolean. Alert related-incident output uses `SequentialID` (alerts/get.go:124-132).
- Errors keep the ID the user typed: get.go:52 `failed to get incident %s` (displayID) wraps `incident not found: <normalized id>` from client.go:1194. A 404 for `INC-999` therefore reads "failed to get incident INC-999: incident not found: 999". This is clear and not a bug.
- Tests on master: `TestNormalizeIncidentID` (internal/api/client_test.go:62), `TestRunGetNormalizesSequentialID` and `TestRunGetAcceptsUUID` (incidents/cmd_test.go:750, 774), runUpdate/runDelete with INC-42, `TestRunEventsListNormalizesIncidentID` (statuspages/cmd_test.go:145), workflows cmd_test with `INC-42`. `run_tests` on the incidents and api test files ran; `go` is not on the VM PATH, so it was run through the harness.
- Help and README: get/update/delete/workflows/status-pages examples all use INC-123 (README:118 for status-pages). root.go:35,50 also show it.

## History
`git log` on get.go and client.go shows PR #32 first, then later PRs (#55, #56, #57, #68) that kept the normalization. Nothing since has regressed it.

## Monolith side (not changed here)
Per Devin's comment, cancan.rb:55-67 in rootly splits the id on "-": a numeric first part means a sequential_id lookup, otherwise a slug lookup. An API-side `INC-` prefix would be a separate enhancement ticket and does not belong in this repo.

## Knowledge base
- repos/rootly-cli/lint-and-test.md: lint `golangci-lint run ./... && go vet ./...`, test `go test ./...`. Used for commands only.
- Nothing else applies: no standard or anti-pattern entry covers CLI ID normalization.

## Not read
Slack thread, GH issue #28 body, Pylon #26340 (not linked as readable). The Linear ticket and Devin's comment already carry their conclusion.

## Bug evidence
- Linear: https://linear.app/rootly/issue/IR-5478/rootly-cli-incident-not-found (Devin comment 2026-10-26 concludes it is already fixed)
- Fix PR: https://github.com/rootlyhq/rootly-cli/pull/32 (merged 2026-06-04, v0.3.2)
- GH issue: https://github.com/rootlyhq/rootly-cli/issues/28
- missing: Sentry/Datadog (a CLI client bug, no server-side telemetry applies)
Suspected root cause: the CLI sent the INC-<n> display ID straight to /v1/incidents/{id} (internal/cmd/incidents/get.go) before NormalizeIncidentID was added in #32; fixed on master.

## Plan if the owner wants a PR anyway
No code change is needed. Recommend: ask FRAYT to upgrade to v0.6.1, then close IR-5478. Optional extra, only if wanted: a table test that all six callers normalize, plus a separate ticket for the API accepting an `INC-` prefix.

## Risks
Low. Making no change risks nothing. Any new test touches only test files; coverage floor (COVERAGE_MIN 80) is unaffected.
