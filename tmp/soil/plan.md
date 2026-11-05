# IR-5478 plan

## Decision
No production code changes. PR #32 (v0.3.2, merged 2026-06-04) added `api.NormalizeIncidentID` (`internal/api/client.go:1153-1160`) and wired it into every command that takes an incident ID. Master still behaves as the ticket asks. GitHub rootlyhq/rootly-cli#28 is the same report, and PR #32 closed it.

The lane makes only two test changes, so that every criterion is checked by an assertion rather than inferred:
- the delete test will assert the request path;
- the 404 test will assert the exact error text.

Both changes pass at HEAD, because the fix is already there.

The owner asked to mark the ticket as a duplicate ("mark tix as dup of the other one"). IR-5478 now has a comment and links pointing at #28 and PR #32. Linear refused the Duplicate status because no Linear target issue exists, so the status stays In Review until the owner names a target or picks another state. That is a ticket-hygiene decision for a person and none of the criteria depend on it.

## Ordered work (each piece committable on its own)

### 1. Pin the delete path (ac-2)
- **File:** `internal/cmd/incidents/cmd_test.go`, `TestRunDeleteWithYes` (line 835).
- **Change:** besides the DELETE method, assert that `r.URL.Path == "/v1/incidents/42"` when the user types `INC-42`.
- **Why:** today this test would still pass if `delete.go:61` stopped normalizing. It is the only incident-ID caller whose test does not check the path.

### 2. Pin the 404 message (ac-3)
- **File:** `internal/cmd/incidents/cmd_test.go`, `TestRunGetAPIError` (line 795).
- **Change:** assert the error contains `failed to get incident INC-999` (the ID as typed, from `get.go:52`) and `incident not found` (from `client.go:1194`).
- **Why:** today the test only checks the prefix `failed to get incident`.

### 3. Verify (no diff)
Run `run_tests` on these files, plus `go vet` and golangci-lint:
- `internal/api/client_test.go`
- `internal/cmd/incidents/cmd_test.go`
- `internal/cmd/workflows/cmd_test.go`
- `internal/cmd/statuspages/cmd_test.go`

Coverage cannot drop, because only tests change.

## Criterion → change → proving test

### ac-1: `incidents get` normalization
- **Change:** none. The code is already correct.
  - `NormalizeIncidentID`: `internal/api/client.go:1153-1160`.
  - Call site: `internal/cmd/incidents/get.go:41`.
- **Proved by:**
  - `TestNormalizeIncidentID` (`internal/api/client_test.go:62`). Cases: `INC-42`, `inc-99`, `Inc-123` and bare `42` become the number; a UUID, a slug, `INC-` and `INC-abc` pass through unchanged.
  - `TestRunGetNormalizesSequentialID` (`internal/cmd/incidents/cmd_test.go:750`): asserts `/v1/incidents/42`.
  - `TestRunGetAcceptsUUID` (`internal/cmd/incidents/cmd_test.go:774`).

### ac-2: every incident-ID command normalizes
- **Change:** piece 1 (test only).
- **Callers already normalizing:**
  - `incidents/update.go:62`
  - `incidents/delete.go:61`
  - `workflows/run.go:39`
  - `statuspages/events_list.go:36`
  - `statuspages/events_mutations.go:75`
- **Proved by:**

| Command | Test | Assertion |
|---|---|---|
| update | `TestRunUpdate*` (`cmd_test.go:246`, `:344`) | PUT `/v1/incidents/42` |
| delete | `TestRunDeleteWithYes` (after piece 1) | DELETE `/v1/incidents/42` |
| workflows run `--incident` | `internal/cmd/workflows/cmd_test.go:115` | `INC-42` resolves to the incident before the run request |
| status-pages events list | `TestRunEventsListNormalizesIncidentID` (`statuspages/cmd_test.go:145`) | `/v1/incidents/42/status-page-events` |
| status-pages events create | `TestRunEventsCreate` (`statuspages/cmd_test.go:173`) | POST `/v1/incidents/42/status-page-events` |

### ac-3: help text, README and the 404 message
- **Change:** piece 2 (test only).
- **Help text and README already show INC-123:**
  - `incidents/get.go:21,27,30`
  - `update.go:19-30`
  - `delete.go:20,23`
  - `incidents.go:24-33`
  - `workflows/run.go:17-25`
  - `statuspages/events_list.go:18`
  - `events_mutations.go:18`
  - `root.go:35,50`
  - README status-pages example (line 118)
- **404 message:** `get.go:52` wraps the error with the ID as typed, giving `failed to get incident INC-999: incident not found: 999`.
- **Proved by:** `TestRunGetAPIError` (after piece 2). The help and README examples are checked by reading them in verify; they are documentation, not executable.

## Human actions
None. No criterion needs a flag, config, infra or access change.

## Reusable helper (operator guidance)
`api.NormalizeIncidentID` is already the shared helper. IR-6585 (`incidents update --services/--types`) and IR-6584 (`incidents escalate`) should call it on their incident argument. Nothing new to extract, and nothing blocks those lanes.

## Out of scope
- The six other tickets, each in its own lane (cut approved by the operator).
- Accepting an `INC-` prefix on the API side (rootly `config/initializers/cancan.rb:55-67`) would be a separate enhancement in another repo.
