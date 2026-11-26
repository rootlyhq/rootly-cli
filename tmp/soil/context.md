# Context: IR-6586 status page commands

## Ask
Add status page commands to the CLI (`status-pages list`, create/update/resolve of status-page incident entries), because `rootly incidents update <id> --status=resolved` leaves the public status page entry open ("Investigating"). A read-only check was named an acceptable fast partial fix. Source: https://github.com/rootlyhq/rootly-cli/issues/52 (closed). Ticket is unassigned, Todo.

## Does the ask stand? partly-done (0.85)
Verified in this checkout (HEAD 57534c3):
- `internal/cmd/statuspages/list.go`: `status-pages list`.
- `internal/cmd/statuspages/events_list.go:16`: `events list <incident-id>` (the read-only open-entry check).
- `internal/cmd/statuspages/events_mutations.go:16,24,32`: `events create/update/resolve`; `--notify-subscribers` is create-only.
- All shipped by rootly-cli#57 (merged 2026-08-13, v0.6.1); #68 added templates and maintenance statuses.
- The ticket says `status-pages incidents ...`; shipped group is `status-pages events ...`. Not renamed (would break v0.6.1 users). The PR body must say so (operator instruction).

Remaining: `internal/cmd/incidents/update.go:161` calls `UpdateIncident` and prints "Updated incident X", nothing about status pages. Same gap in `incidents create --status=resolved` (create.go:110) and `alerts resolve --resolve-incidents` (alerts/resolve.go:48).

## Operator constraints (relayed 2026-11-04)
1. Best-effort: lookup failure never fails the command (at most a debug note).
2. Warning to stderr only.
3. Fire whenever the CLI leaves an incident resolved, in any command that resolves one.
4. List each open event with its status page and the exact `rootly status-pages events resolve <id>` command.
5. No auto-resolve, no renames.
6. Lane IR-6585 edits `internal/cmd/incidents/update.go` concurrently: put the logic in its own file/package and add only a one-line call per command.

## Design
- New package `internal/statuspagewarn` (no import cycle: it takes the `*api.Client`, imports only `api`). Roughly `WarnOpenEvents(ctx, client, incidentID string, w io.Writer)`: calls `ListStatusPageEventsCLI(ctx, api.NormalizeIncidentID(id), 1, 100)`, filters events with status not in {resolved, completed}, prints to `w` (os.Stderr) one line per event: status page ID, event status, and `rootly status-pages events resolve <event-id> --message="..."`. Errors are swallowed (debug note only if the existing debug flag is cheap to read).
- Callers: `incidents/update.go` after the success message, only when `--status` is resolved/closed (one line); `incidents/create.go` when status is resolved/closed; `alerts/resolve.go` when `--resolve-incidents`: re-fetch with `GetAlertByID` and check `RelatedIncidents` whose `Status` is resolved/closed (client.go:2399-2410 parses `attributes.incidents` with status).
- Events pagination: first page of 100 is enough for a warning; note the limit.
- Tests: mock HTTP server style of `internal/api/client_status_pages_test.go` and `internal/cmd/statuspages/cmd_test.go`; assert stderr only, stdout unchanged, lookup error ignored.
- Docs: a line in README / command help that incident resolution does not resolve status-page events and pointing to `status-pages events resolve`.

## Key files
internal/cmd/incidents/update.go, create.go; internal/cmd/alerts/resolve.go; internal/api/client.go (`ListStatusPageEventsCLI` ~1843, `StatusPageEvent` 347, `GetAlertByID` 2211); internal/cmd/statuspages/*; Makefile (COVERAGE_MIN).

## Risks / unknowns
- IR-6585 touches update.go (and likely create.go): rebase conflicts; keep one-liners.
- `alerts resolve --resolve-incidents` is server-side; whether related incident statuses are already resolved when the follow-up GET returns is untested against the live API; the warning is skipped when status is not resolved/closed.
- Extra GET adds latency to resolve commands; acceptable, best-effort, bounded by context.
- Auto-resolve: settled by the operator. Answer: "No: warn only, default behaviour unchanged. Resolving a public status page entry is a customer-visible write that can notify subscribers." No opt-in flag in this PR; the PR body mentions one as a possible follow-up.

## Knowledge base
- repos/rootly-cli/lint-and-test.md: lint `$HOME/go/bin/golangci-lint run ./... && go vet ./...`, test `go test ./...` (relied on).
- repos/rootly/wiki/status-pages.md: background on status pages/events (read as background; server-side, not changed here).
- Searched "status page incident events resolve" and "CLI warning stderr best-effort standards anti-patterns": no CLI-specific standard or anti-pattern applies.
