# How PRs merge in rootlyhq/rootly-cli

_Discovered from rootlyhq/rootly-cli's `master` at `57534c36cac6` on 2026-11-04, read-only, as the Soil bot. Regenerated whenever the files it cites change and daily; no part of it is model-written. Version `3eu729quq7vv6`._

## Required checks

- `Check Tests Passed` (app 15368) — API `GET /repos/rootlyhq/rootly-cli/rules/branches/master`
- `Lint` (app 15368) — API `GET /repos/rootlyhq/rootly-cli/rules/branches/master`
- `Build` (app 15368) — API `GET /repos/rootlyhq/rootly-cli/rules/branches/master`

## Approvals and merge rules

- Approvals required: 1
- Code owner review required: unknown (not readable with the bot's access)
- Branch must be up to date with `master`: yes
- Signed commits required: unknown (not readable with the bot's access)
- Linear history required: unknown (not readable with the bot's access)
- Merge queue: no
- Source: API `GET /repos/rootlyhq/rootly-cli/rules/branches/master`, API `GET /repos/rootlyhq/rootly-cli/branches/master`

## PR template

- The repo has no PR template.

## Label and body gates

- No workflow gates a PR on its labels or body.

Jobs that run on pull requests:

- `lint` / `Lint` (required) — [.github/workflows/ci.yml:13](https://github.com/rootlyhq/rootly-cli/blob/57534c36cac6a220d788256f8d627b31784c9a68/.github/workflows/ci.yml#L13)
- `test` / `Test` — [.github/workflows/ci.yml:31](https://github.com/rootlyhq/rootly-cli/blob/57534c36cac6a220d788256f8d627b31784c9a68/.github/workflows/ci.yml#L31)
- `validate-tests-passed` / `Check Tests Passed` (required) — [.github/workflows/ci.yml:74](https://github.com/rootlyhq/rootly-cli/blob/57534c36cac6a220d788256f8d627b31784c9a68/.github/workflows/ci.yml#L74)
- `build` / `Build` (required) — [.github/workflows/ci.yml:84](https://github.com/rootlyhq/rootly-cli/blob/57534c36cac6a220d788256f8d627b31784c9a68/.github/workflows/ci.yml#L84)

## Risk scale

- `risk`, lowest to highest: `risk:low` < `risk:medium`
- Source: API `GET /repos/rootlyhq/rootly-cli/labels`

## Review bots

- `devin-ai-integration[bot]`: 4 reviews, 17 comments on recent PRs (API `pulls/{n}/reviews`, `issues/{n}/comments`)
- `greptile-apps[bot]`: 9 reviews, 4 comments on recent PRs (API `pulls/{n}/reviews`, `issues/{n}/comments`)
- `soil-cloud[bot]`: 0 reviews, 2 comments on recent PRs (API `pulls/{n}/reviews`, `issues/{n}/comments`)

## Code owners and guidance

- No CODEOWNERS file.
