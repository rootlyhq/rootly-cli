# The ask (verbatim; written by the harness)

Solve

https://linear.app/rootly/issue/IR-6824/mcp-claude-desktop-and-claude-code-sdk-cannot-connect-the-mcp-server
https://linear.app/rootly/issue/IR-6586/cli-add-status-page-commands-incidents-update-statusresolved-leaves
https://linear.app/rootly/issue/IR-6584/cli-add-escalate-verb-for-incidents-and-alerts-to-page-a-team
https://linear.app/rootly/issue/IR-5478/rootly-cli-incident-not-found
https://linear.app/rootly/issue/IR-6588/cli-add-workflows-list-and-workflows-run-commands
https://linear.app/rootly/issue/IR-6585/cli-support-services-and-types-on-incidents-createupdate
https://linear.app/rootly/issue/IR-5846/admin-general-organization-settings-checkboxes-toggle-when-clicking

1 pr each

## Additions (beside the ask, never instead of it)
- the operator (operator) relayed it as: Solve IR-6588 — one PR. Add `rootly workflows list` and `rootly workflows run <id-or-slug> --incident=<id>` to the CLI. Ref https://github.com/rootlyhq/rootly-cli/issues/51

## The ticket as the harness read it (quote "cites" from here or from the ask above)

Solve IR-6588 — one PR. Add `rootly workflows list` and `rootly workflows run <id-or-slug> --incident=<id>` to the CLI. Ref https://github.com/rootlyhq/rootly-cli/issues/51
IR-6588
REQUIREMENT (Linear IR-6588, Incident Response team, Todo, Medium, labels Feature/Incident Response Support): "[CLI]: Add workflows list and workflows run commands". Desired: `rootly workflows list` and `rootly workflows run <workflow-id-or-slug> --incident=<incident-id>`. list matters as much as run (ID discovery, same gap as --severity). Reporter @minkimipt, GitHub rootlyhq/rootly-cli#51 (internal vs external unknown). Use case: scripted/agent-driven runbook; e.g. workflow `create-a-google-docs-retrospective`, Slack equivalent `/rootly workflow`.

KEY FINDING: ALREADY SHIPPED. rootly-cli PR #56 "Add workflow list and run commands" (https://github.com/rootlyhq/rootly-cli/pull/56), merged 2026-08-13, author kwent, "Closes #51"; GH issue #51 is closed. PR body: list with same pagination/sort/filter/output conventions; run resolves workflow slugs and incident display IDs to UUIDs; exposes --immediate and --check-conditions; tests cover list, resolution, run payloads, output; `go test ./...` run. A Devin comment on the ticket (2026-11-04) says: in v0.5.0+ (latest tag v0.6.1), internal/cmd/workflows/{list.go,run.go} exist (alias `workflow`; list flags --page --page-size --sort --name --slug; run requires --incident), api/client.go ~1534-1551 has ResolveWorkflowID (slug][eq filter) and RunWorkflowCLI; API route workflows/:id/workflow_runs; runs created triggered_by "user". Devin did not test against a live API. Its recommendation: verify, close ticket as Done linking PR #56, tell reporter to upgrade; no change needed in rootlyhq/rootly. An older Linear triage comment (Aug) claiming no workflow command is outdated.
No docs linked beyond GH issue (no Notion/Granola/Slack followed).

KB: repos/rootly-cli/lint-and-test.md: lint `$HOME/go/bin/golangci-lint run ./... && go vet ./...`; test `go test ./...`. repos/rootly/wiki/workflows-genius.md (stale flag): Genius workflows = automation engine; GeniusWorkflowRun = execution. No other CLI workflow prior art.

Earlier PRs: #56 only (above).

Acceptance hints: list/run work with ID or slug; --incident accepts incident id; tests pass.

PLAN (unverified against code; only if still needed):
1. Check out master, confirm internal/cmd/workflows list/run exist and `go run ./cmd/rootly workflows --help` works.
2. Run `go test ./internal/cmd/workflows/ ./...` plus lint.
3. Look for residual gaps vs ticket (e.g. README/docs mention of workflows, completion, slug/ID edge cases, error messages for missing incident).
4. If no gaps, make no code change; report the ticket should be closed as Done referencing PR #56 (and that no change is needed in rootlyhq/rootly). Do not duplicate the feature.

## Knowledge base entries (in full)

### repos/rootly-cli/lint-and-test.md — Lint and test
- Lint: `$HOME/go/bin/golangci-lint run ./... && go vet ./...`. Test: `go test ./...`.

_Seeded from Devin's rootlyhq/rootly-cli blueprint (lint, test)._

### repos/rootly/wiki/overview.md — Overview
# Overview

_Generated from rootlyhq/rootly at `ce1efe268a86` on 2026-11-04. A page goes stale when the files it cites change._

## What Rootly is

Rootly is an "all-in-one incident response platform for humans" ([README](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/README.md#L1-L7)). The README describes the product: monitor and investigate incidents with automated timelines and retrospectives, track analytics such as MTTM and MTTR, publish public and private status pages, involve stakeholders, and collaborate through integrations such as Slack and Jira ([README](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/README.md#L11-L14)).

This repository (`rootlyhq/rootly`) is the Rails monolith behind that product. Use this page to orient yourself, then follow the links at the bottom to the area you need to change.

## Tech stack at a glance

`AGENTS.md` summarises the stack ([AGENTS.md](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/AGENTS.md#L5-L12)):

| Layer | Technology |
| --- | --- |
| Backend | Rails + Ruby, PostgreSQL with UUID primary keys |
| Frontend | Turbo/Stimulus (Hotwire), Tailwind CSS, ESBuild |
| Background jobs | Sidekiq Pro/Enterprise with specialised queues |
| Tests | RSpec, FactoryBot, WebMock |
| Authorization | CanCanCan abilities |
| UI | ViewComponent + Stimulus controllers |

**Version gotcha:** `AGENTS.md` still says "Rails 8.0 + Ruby 3.4.5". The code pins newer versions:

- The `Gemfile` requires `ruby "4.0.9"` and `rails "~> 8.1"` ([Gemfile](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/Gemfile#L7-L13)).
- `.tool-versions` pins Ruby 4.0.9, Node 24.22.1 and Yarn 1.22.19 ([.tool-versions](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/.tool-versions#L1-L3)).
- The `Dockerfile` builds on Ruby 4.0.9 with Node 24.22.1 ([Dockerfile](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/Dockerfile#L4-L6), [Dockerfile](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/Dockerfile#L33-L35)).

Treat these files as the source of truth.

The `Gemfile` also defines a `next?` helper, which returns true when the file is loaded as `Gemfile.next` ([Gemfile](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/Gemfile#L1-L3)). This is the usual pattern for dual-booting a framework upgrade.

Other infrastructure, from the local `docker-compose.yml`:

- Postgres with pgvector ([docker-compose.yml](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/docker-compose.yml#L2-L14))
- Redis ([docker-compose.yml](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/docker-compose.yml#L16-L21))
- OpenSearch ([docker-compose.yml](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/docker-compose.yml#L23-L41))
- Redpanda (Kafka-compatible) with a console and Kafka Connect ([docker-compose.yml](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/docker-compose.yml#L43-L106))
- An optional ClickHouse profile ([docker-compose.yml](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/docker-compose.yml#L108-L118))

The README points to `rootlyhq/rootly-dev-env` (Tilt) as the preferred way to run these services ([README](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/README.md#L20-L21), [README](https://github.com/rootlyhq/rootly/blob/ce1efe268a86e5f5ab0fcd704808802dc0f06c27/README.md#L93)).

## Product areas and where to read next

The repo is large. By file count it has `app` (10,755), `spec` (8,445), `db` (1,011), `lib`, `config` and an `eval` directory for AI evaluations.

```mermaid
flowchart LR
  "Signals (alert sources, webhooks)" --> "Alerts and routing"
  "Alerts and routing" --> "Escalation policies / On-call"
  "Alerts and routing" --> "Incidents lifecycle"
  "Incidents lifecycle" --> "Workflows (Genius)"
  "Incidents lifecycle" --> "Status pages / Stakeholder comms"
  "Incidents lifecycle" --> "Retrospectives / Action items"
  "Incidents lifecycle" --> "AI SRE investigations"
  "Workflows (Genius)" --> "Integrations (Slack, Teams, Jira...)"
```

The main product areas, and the pages that cover them:

- **Incidents:** [[Incidents lifecycle]], [[Incident details fields and custom forms]], [[Action items and follow-ups]], [[Retrospectives and collaborative editor]] and [[Problem management]].
- **Alerting and on-call:** [[Alerts and routing]], [[Alert sources and incoming webhooks]], [[Escalation policies]], [[On-call schedules and shifts]], and [[Competitor migrations: PagerDuty, Opsgenie and JSM Ops]].
- **Status pages and communications:** [[Status pages]], [[Statuspage.io import and external status pages]] and [[Stakeholder communications]].
- **Automation:** [[Workflows (Geniu
… [cut; kb_get repos/rootly/wiki/overview.md for the rest]
