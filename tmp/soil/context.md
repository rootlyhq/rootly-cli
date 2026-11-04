# Context: IR-6588 — CLI workflows list / run

## Ask
Linear IR-6588 (GitHub rootlyhq/rootly-cli#51): add `rootly workflows list` and `rootly workflows run <workflow-id-or-slug> --incident=<incident-id>` so triggering a workflow (e.g. `create-a-google-docs-retrospective`) is scriptable from a terminal or agent, not Slack-only. The requirement lives in the ticket body; the only comments are an August triage note (outdated) and a Devin note of 2026-11-04 saying the feature shipped.

## Does the ask still stand? No: already done
- rootly-cli PR #56 "Add workflow list and run commands" (commit 14c8081, merged 2026-08-13, closes #51): https://github.com/rootlyhq/rootly-cli/pull/56
- Code on master:
  - `internal/cmd/workflows/workflows.go:17` — alias `workflow`
  - `internal/cmd/workflows/list.go:33` — `runList` (flags --page, --page-size, --sort, --name, --slug)
  - `internal/cmd/workflows/run.go:28` — `--incident` marked required; `run.go:32` `runWorkflow`; also --immediate (default true) and --check-conditions
  - `internal/cmd/workflows_register.go:6` — wires `WorkflowsCmd` to the root command
  - `internal/api/client.go:1426` `ListWorkflowsCLI`, `:1534` `ResolveWorkflowID` (exact `slug][eq` filter), `:1551` `RunWorkflowCLI`
  - Tests: `internal/cmd/workflows/cmd_test.go` (TestRunList, TestRunWorkflowResolvesSlugAndNormalizesIncident), `internal/api/client_workflows_test.go`
- Verified in this lane (go 1.24 via mise): built the binary; `workflows --help` and `workflows run --help` show list/run, the required `--incident`, and an example using `create-a-google-docs-retrospective`. `go test ./internal/cmd/workflows/ ./internal/api/` passes. Not tested against a live API.
- Released in v0.5.0 and later (latest tag v0.6.1), per the Devin comment.

## Decisions from the operator
- No code change. The `--severity` ID discovery gap is out of scope.
- Do not close the ticket or comment on Linear or GitHub from this lane. The owner declined the proposed close comment when asked, so the ticket stays open.

## Risks / unknowns
- Live-API behaviour (permissions, `triggered_by`) is untested here.
- The reporter's runbook may still say workflows are Slack-only; the fix there is to upgrade the CLI.

## Knowledge base
- `repos/rootly-cli/lint-and-test.md` — lint `$HOME/go/bin/golangci-lint run ./... && go vet ./...`, test `go test ./...`. Applies only if code changes; none planned.
- `repos/rootly/wiki/overview.md` — product background (Genius workflows are the automation engine). No CLI prior art found.
- Searched "rootly-cli workflows CLI commands standards anti-patterns": no entry beyond these applies.

## Scope narrowing (owner)
The owner narrowed IR-6588 to: approved cut, no code change, since the workflows list and run commands already shipped in rootly-cli#56. The lane does not close the ticket or comment on it; the close-as-done proposal (link PR #56, upgrade to v0.5.0+) goes to the human as an ask. intent.json now has criteria for only that, with ask_stands.verdict "partly-done" and "remaining" set to the narrowed scope.
