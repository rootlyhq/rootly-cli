# Handoff: IR-5478, step 4/8 "verify" (verifying: run the checks, capture evidence)

This is a FRESH session for this step. The harness wrote this brief from the lane's records; the earlier steps' conversation does not carry over. Open the files under "Read on demand" only when this step needs their detail.

- Workflow: ticket, lane "Accept sequential incident IDs in incidents get"
- Steps: context (done) → plan (done) → implement (done) → verify (NOW) → council (next) → ship (next) → babysit (next) → learn (next)
- The run advanced here from "implement".

## Intent: Rootly CLI - Incident Not Found (0/3 criteria proved at HEAD)
`rootly incidents get INC-31` returned "incident not found: INC-31" because the CLI sent the display ID to /v1/incidents/{id}, which accepts a UUID, slug or bare sequential number. PR #32 (v0.3.2, merged 2026-06-04) added NormalizeIncidentID and wired it into incidents get/update/delete, workflows run and status-page events; master already behaves as the ticket asks, so the likely outcome is no code change and a recommendation that the customer upgrade.
- [open] ac-1: `incidents get` with INC-31, inc-31 and bare 31 requests /v1/incidents/31, and a UUID is passed through unchanged (TestNormalizeIncidentID, TestRunGetNormalizesSequentialID, TestRunGetAcceptsUUID pass)
- [open] ac-2: Every command that takes an incident ID (incidents update, incidents delete, workflows run --incident, status-pages events list/create) calls api.NormalizeIncidentID before the API call, each covered by a test
- [open] ac-3: Help text and README examples show INC-123 and match the behaviour; a real 404 reports "failed to get incident <id-as-typed>: incident not found"

## Original ask (verbatim; the intent restates it, this is what was asked)
> Solve
>
> https://linear.app/rootly/issue/IR-6824/mcp-claude-desktop-and-claude-code-sdk-cannot-connect-the-mcp-server
> https://linear.app/rootly/issue/IR-6586/cli-add-status-page-commands-incidents-update-statusresolved-leaves
> https://linear.app/rootly/issue/IR-6584/cli-add-escalate-verb-for-incidents-and-alerts-to-page-a-team
> https://linear.app/rootly/issue/IR-5478/rootly-cli-incident-not-found
> https://linear.app/rootly/issue/IR-6588/cli-add-workflows-list-and-workflows-run-commands
> https://linear.app/rootly/issue/IR-6585/cli-support-services-and-types-on-incidents-createupdate
> https://linear.app/rootly/issue/IR-5846/admin-general-organization-settings-checkboxes-toggle-when-clicking
>
> 1 pr each
- the operator (operator) relayed it as: Solve IR-5478 — one PR. `rootly incidents get INC-31` fails with "incident not found" though help says INC-123 format is accepted; resolve sequential IDs. Ref https://github.com/rootlyhq/rootly-cli/issues/28

## Operator and owner guidance so far (still binding)
- [context] answer from operator:op-3104c931d936: IR-5478: the lane proposes leaving 6 parts of the ask out of this run. Approve the cut? Approve with a reason ("approve: <why>"); the cut and the reason are printed on the PR. → approve: the human asked for one PR per ticket. Each of the other six tickets has its own lane (IR-6824 → ir-6824-7cf140, IR-5846 → ir-5846-67d8cb, IR-6586 → ir-6586-cb8ca4, IR-6588 → ir-6588-c99a1c; IR-6585 → ir-6585-694b70 and IR-6584 → ir-6584-304657 are held until your plan lands). Scope this PR to IR-5478 only. Keep the INC-123 → incident lookup in a reusable helper, because IR-6585 (incidents update) and IR-6584
- [plan] quentin@rootly.com: mark tix as dup of the other one

## Gate results by head
- context: passed at 57534c36ca (2026-11-05T04:30:20.316Z): IR-5478 may no longer stand (already-done) and nobody answered: carrying on as asked rather than skipping or closing it
- plan: passed at 57534c36ca (2026-11-05T05:02:43.301Z)
- implement: passed at 6f400af470 (2026-11-05T05:03:23.288Z)
- go-vet passed on 6f400af470 (2026-11-05T05:03:18.443Z) → tmp/soil/proofs/harness-go-vet-6f400af470.txt
- gofmt passed on 6f400af470 (2026-11-05T05:03:14.417Z) → tmp/soil/proofs/harness-gofmt-6f400af470.txt
- go-vet passed on 57534c36ca (2026-11-04T20:46:42.124Z) → tmp/soil/proofs/harness-go-vet-57534c36ca.txt

## Files changed (57534c36cac6a220d788256f8d627b31784c9a68...HEAD)
1 file changed, 8 insertions(+), 2 deletions(-)
- M internal/cmd/incidents/cmd_test.go

## Commits on the branch
- 6f400af [ac-2] [ac-3] pin delete path and 404 message in incidents tests

## Plan (tmp/soil/plan.md)
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

## Environment
environment: your organization's environment, every included repo's services available (rootly-cli is not in it: its checkout and toolchain only); nothing is started: mcp__environment__env_start brings up only the services the task needs (env_services lists them).

## Read on demand
- tmp/soil/ask.md
- tmp/soil/contract.json
- tmp/soil/intent.json
- tmp/soil/context.md
- tmp/soil/plan.md
- tmp/soil/risk.json
- tmp/soil/repro.json
- git diff 57534c36cac6a220d788256f8d627b31784c9a68...HEAD (the full change)
- tmp/soil/step-handoff.md (this brief)

## Working set from earlier steps

The files earlier steps read or changed, most-used first, with the lines they read. Read only the ranges this step needs, with offset and limit; the Edit tool needs one Read of a file before you change it.
- internal/cmd/incidents/cmd_test.go (lines 748-917; read 4x, edited 2x, changed on the branch; context, plan, implement)
- internal/api/client_test.go (lines 58-87; read 2x; context, plan)
- internal/api/client.go (lines 1145-1197; read 2x; context, plan)
- internal/cmd/incidents/get.go (lines 14-58; read 2x; context, plan)
- internal/cmd/statuspages/cmd_test.go (lines 145-204; read 1x; plan)
- internal/cmd/workflows/cmd_test.go (lines 95-129; read 1x; plan)

### What the previous step reported
I think the review is right about one gap. I'd only run one of the four planned test files, and I hadn't checked the help text or README. I've now done both.

- **Tests:** `go vet` is clean on all four files from the plan. `run_tests` reported only the vet result, with no per-test pass or fail lines. So the `go vet` pass is solid. A pass for the tests themselves is not shown.
- **[ac-3] help text and README:** every incident-ID command's help example uses `INC-123`, and so do the README status-pages example and the examples in `root.go`. These match the behavior: all of those commands call `NormalizeIncidentID`.
- **[ac-2] callers:** the update, workflows run and status-page events list/create tests all use `INC-42`. Delete is covered by the new path check.

Nothing else is outstanding, so I've made no further changes. IR-5478's status in Linear is still waiting on a human, as I reported before.