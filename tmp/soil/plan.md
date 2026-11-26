# Plan: IR-6586, warn when resolving an incident leaves status-page events open

**Already done (out of scope):** the status-page commands (`status-pages list` and `status-pages events list/create/update/resolve`) shipped in rootly-cli#57 (v0.6.1). This PR does not rename or re-add them.

**Left to do:** resolving an incident from the CLI (`incidents update --status=resolved`, `incidents create --status=resolved`, `alerts resolve --resolve-incidents`) still prints nothing about public status-page events that are still open. This PR adds a stderr-only, best-effort warning. It does not auto-resolve anything and adds no flag.

No criterion needs a person (no flag, config, infra or access change). ac-7 is delivered at the ship step.

## Pieces of work (in order, each committable on its own)

### 1. New package `internal/statuspagewarn` (ac-1, ac-3, ac-4, ac-5)
File: `internal/statuspagewarn/statuspagewarn.go`. It imports only `internal/api` and `viper`, so there is no import cycle.

- `IsResolved(status string) bool`: true for `resolved` and `closed`.
- `Incident(ctx, client *api.Client, incidentID, status string, w io.Writer)`:
  - Does nothing unless `IsResolved(status)` is true (ac-3: the incident is not left resolved).
  - Calls `client.ListStatusPageEventsCLI(ctx, incidentID, 1, 100)`. One page of 100 is enough for a warning; this limit is noted in a comment.
  - Keeps only events whose status is not `resolved` or `completed`. If none are left, prints nothing (ac-3).
  - Otherwise writes to `w` a header line, `Warning: incident <id> is resolved but N status page event(s) are still open:`, then one line per event with the status page ID, the event status and the exact command `rootly status-pages events resolve <event-id>` (ac-1).
  - On a lookup error it returns quietly. It prints `debug: status page check skipped: <err>` to `w` only when `viper.GetBool("debug")` is set (ac-4). It never returns an error, so the caller's exit code cannot change.
- `AlertIncidents(ctx, client, alertID string, w io.Writer)`: calls `client.GetAlertByID`. For each `RelatedIncidents` entry whose `Status` passes `IsResolved`, it calls `Incident(...)` with that incident's ID. If the GET fails, it skips the warning the same best-effort way (ac-2, ac-4).
- Tests: `internal/statuspagewarn/statuspagewarn_test.go` uses `httptest` and a real `api.Client` pointed at the server. The cases:
  - Open events are listed with the page ID, status and resolve command. Mixed open and resolved/completed events list only the open ones (ac-1).
  - No events, all resolved/completed, and status `started`/`mitigated` all print nothing. The non-resolved case also checks that no HTTP request is made (ac-3).
  - A 500 or 403 from the events endpoint prints nothing and does not panic. With debug on, a debug note is printed (ac-4).
  - `AlertIncidents` warns only for related incidents that are resolved or closed, and skips quietly when the alert GET fails (ac-2, ac-4).
  - The package targets 100% statement coverage (ac-6).

### 2. Wire `incidents update` and `incidents create` (ac-1, ac-2, ac-3, ac-4, ac-5)
- `internal/cmd/incidents/update.go`: add one line after `Updated incident %s`: `statuspagewarn.Incident(cmd.Context(), apiClient, incident.ID, incident.Status, os.Stderr)`. It uses the status the API returned, so it fires only when the incident really is resolved or closed. The change is kept to one line plus the import so that IR-6585's concurrent edits to this file rebase cleanly.
- `internal/cmd/incidents/create.go`: the same one line after `Created incident %s`.
- Tests in `internal/cmd/incidents/cmd_test.go`:
  - `update --status=resolved` against a mock server that returns `status: resolved` and one open event. Stderr contains the warning and the `status-pages events resolve` command. Stdout (`--format=json`) is byte-identical to the incident's raw body (ac-1, ac-4).
  - `update --status=mitigated`: no warning and no events request (ac-3).
  - `update --status=resolved` with the events endpoint returning 500: the run still returns nil and stdout is unchanged (ac-4).
  - `create --status=resolved` with an open event prints the warning (ac-2).
  - A stderr capture helper is added next to the existing stdout one if none exists.

### 3. Wire `alerts resolve --resolve-incidents` (ac-2, ac-4, ac-5)
- `internal/cmd/alerts/resolve.go`: after a successful resolve, one line: `if resolveIncidents { statuspagewarn.AlertIncidents(cmd.Context(), apiClient, alertID, os.Stderr) }`. Without the flag nothing extra runs and no extra request is made.
- Tests in `internal/cmd/alerts/cmd_test.go`:
  - With the flag, the mock alert GET returns one related incident `resolved` and one `started`, and the resolved one has an open event. The warning names only the resolved incident's event, and stdout still has only `Resolved alert ALR-42` (ac-2, ac-4).
  - Without the flag, no GET request is made (ac-3).
  - The existing `TestRunResolveWithIncidents` keeps passing: its mock gives an unparseable GET, which is skipped as best-effort.

### 4. Docs (ac-5)
- Add one sentence to the `Long` help of `incidents update` and `alerts resolve`, and to the README status-pages section: resolving an incident does not resolve status-page events, and the CLI warns and points to `rootly status-pages events resolve`. No command or flag is renamed or added.

### 5. Gates (ac-6)
- Run `go test ./...`, `golangci-lint run ./...`, `go vet ./...` and `make coverage && make coverage-check`. If total coverage rises past the next whole number, raise `COVERAGE_MIN` in the Makefile, as CLAUDE.md requires.

### 6. Ship (ac-7)
- The PR body links rootly-cli#57 and issue #52. It says the `status-pages events` commands are not renamed to `status-pages incidents`, lists the approved cuts, and names an opt-in `--resolve-status-page-events` style flag as a possible follow-up, not part of this PR.

## Criterion → change → proof

| id | Change | Proof |
|----|--------|-------|
| ac-1 | statuspagewarn.Incident; update.go one-liner | statuspagewarn_test (open events listed), incidents update test |
| ac-2 | create.go one-liner; statuspagewarn.AlertIncidents; resolve.go one-liner | incidents create test, alerts resolve test, AlertIncidents unit test |
| ac-3 | `IsResolved` guard and open-status filter | no-events, all-closed and non-resolved tests (no request made) |
| ac-4 | errors swallowed, writer is os.Stderr | events 500/403 tests, stdout byte-equality in the json test |
| ac-5 | new package, one call per command, no flag or rename | diff review; package exists; `git diff` shows no flag/Use changes |
| ac-6 | — | go test, golangci-lint, go vet, make coverage-check |
| ac-7 | PR body | PR body at ship |
