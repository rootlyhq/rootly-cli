## Why

Resolving an incident from the CLI does not touch the public status page entries linked to it, and nothing tells you they are still open. Subscribers keep seeing an active incident. See #52. The status page commands themselves already shipped in #57 (v0.6.1) as `status-pages list` and `status-pages events list/create/update/resolve`, so this PR does not add or rename any command.

## What changed

A new best-effort helper package, `internal/statuspagewarn`, prints a warning to **stderr** when a command leaves an incident `resolved` or `closed` and that incident still has open status page events. Each open event is listed with its status page ID, its status and the exact command to close it.

Where it fires (one call per command):
- `incidents update` when the returned incident status is `resolved` or `closed`
- `incidents create` when the created incident is `resolved` or `closed`
- `alerts resolve --resolve-incidents`, for each related incident that is `resolved` or `closed`

Details:
- Events whose status is `resolved` or `completed` are not listed. No events, or an incident that is not left resolved, means no output and no extra request.
- Incident events get `rootly status-pages events resolve <id>`. Open maintenance events (`scheduled`, `in_progress`) get `--status=completed` added, since maintenance is closed with `completed`.
- Best-effort: the whole check shares a 10 second timeout. If the lookup fails or times out, the command still succeeds with the same exit code and the same stdout. The error is only shown as a `debug:` line when `--debug` is set.
- All event pages are inspected, following `total_pages`.
- stdout is unchanged, so `--format json` and `--format yaml` output stays clean.
- **Nothing is auto-resolved.** Resolving a status page entry is a customer-visible write that can notify subscribers, so the CLI only points at the command to run. No flag was added and no existing command or flag was renamed. An opt-in flag to resolve linked events could be a follow-up.
- Help text for `incidents update` and `alerts resolve`, and the README, mention the warning.

Example stderr after `rootly incidents update INC-123 --status=resolved`:

```
Warning: incident INC-123 is resolved but 2 status page event(s) are still open:
  status page 3f1c..., status investigating: rootly status-pages events resolve 9a7b... --message="<public resolution message>"
  status page 8d20..., status in_progress: rootly status-pages events resolve c41e... --status=completed --message="<public resolution message>"
  --message is the public text shown on the status page; resolving may notify subscribers.
```

## How to test

```bash
go test ./internal/statuspagewarn/... ./internal/cmd/incidents/... ./internal/cmd/alerts/...
make coverage && make coverage-check
```

The unit tests use `httptest` servers to cover open and mixed events, pagination, maintenance statuses, silent cases, lookup failures, a stalled endpoint, and the `alerts resolve --resolve-incidents` path.
