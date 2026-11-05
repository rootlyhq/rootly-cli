> Migrated from Devin playbook `playbook-fc6857a3e32840c2acc68a482181c2be` (macro `!e2e_ticket`), org rootly-development, on 2026-10-24 (devin-migration).

# E2E Ticket → Playwright Spec (approval before PR)

## Overview
Turn one E2E Team Linear ticket into Playwright coverage in `rootlyhq/playwright-e2e`. Explore the live feature with the Playwright MCP first so every selector and flow comes from the real UI, never from guesses. Keep all work uncommitted and local, and get the requester's approval before committing, opening a PR or changing the Linear ticket.

## What's Needed From User
- Linear ticket ID (e.g. `E2E-306`). Small related tickets can be paired (e.g. `E2E-385 + E2E-420`).
- The intended test org (e.g. `orgs.default`, `orgs.feature`, `orgs.filter`, `orgs.retro`, `orgs.membersRbac`; see `utility/orgConfig.ts`). If it's not given, ask before exploring.
- Target env: defaults to prod (`ENV=smoke_prod`), and new tests must also pass on staging (`ENV=staging`). Say so if the tests should be prod-only or staging-only.
- The gitignored `.env` (1Password item "E2E .env config") if it isn't already on the machine.
- The Playwright MCP (or a browser the session can drive) available to the session.

## Procedure
1. Read the ticket with Linear `get_issue` (include relations), plus linked feature tickets/PRs. List every case and acceptance criterion to cover. If the ticket is too vague to test (e.g. a one-line description), stop and ask for scope.
2. In `playwright-e2e`, read `CLAUDE.md`, then find the closest existing specs, POMs (`pageObjects/`, `fixtures.ts`) and helpers (`utility/helperAPI.ts`, `helperWeb.ts`, `runID.ts`, `orgConfig.ts`, `userRoles.ts`) for the feature. Reuse them rather than writing new ones.
3. Explore the feature with the Playwright MCP on prod, in the intended test org, logged in as the role the spec will use. Walk each case end to end and record locators (`data-testid` > role/accessible name > CSS), the network calls each action makes, toasts/validation text, and any flag gating.
   - Never write a locator you haven't seen in the live DOM.
   - Check staging as well if the flow or DOM may differ there (flags, integrations, org data).
   - Where no `data-testid` exists, use the best stable locator and list it in the summary as a candidate companion ticket.
4. Plan the spec: target file (`smokes/tests/smokes.<feature>.spec.ts`, or extend an existing one), how cases map to tests (see step 5), the test org's `orgs.*` entry and matching role (`test.use({ storageState: getStorageStatePath(...) })`), API setup/teardown, and whether a POM method should be added or extended.
5. Write the spec. Everything in the ticket must be covered, but balance test isolation against runtime. Group several cases/ACs into one `test` when they share setup and flow naturally. Split them into separate tests when isolation matters more. Name resources with `createResourceName()`, set up data through `helperAPI`, take IDs from the test org's `orgs.*` entry (add any missing prod/staging IDs to `orgConfig.ts`), and import `isProd`/`url` from `helperConfig`. Use web-first assertions and `waitForResponse` for 2xx checks where the ticket asks. Tag and skip flag-gated tests as `@flagged`. Clean up everything the spec creates (alerts excepted).
6. Run lint: `npm run lint && npm run typecheck && npm run format:check` (run `npm run format` first if needed). Fix any errors and don't add new warnings.
7. Run the new spec once on prod: `ENV=smoke_prod npx playwright test smokes/tests/<file>.spec.ts --project chromium-rootly --reporter=line`. Once it passes, run it once on staging with `ENV=staging`. If the tests are prod-only or staging-only, skip them cleanly in the other env with a reason, following existing specs.
   - If it fails, fix it. Failures are usually bad selectors, so re-check the live DOM with the Playwright MCP and the trace. Don't fix by deviating from existing spec patterns, guessing selectors, or adding fixed delays.
   - After 3 failed fix attempts, stop and flag it to the user with the failure output and what was tried.
   - Once it passes in both envs, run it with `--repeat-each=3` in each env to check for flakiness, and treat any flaky run as a failure (same 3-attempt cap).
8. If a product bug is found, don't write a test that just accepts the broken behavior. Note it, with repro steps, for the user.
9. Leave the changes uncommitted, then send the user the following and wait for approval:
    - a short summary of the cases covered, how they're grouped into tests, and any deviations from the ticket
    - the diff (`git diff`, including new files) as a `.patch` file (attached, or pasted when attachments aren't available)
    - test results per env (prod and staging: single run, then repeat-each outcome)
    - a suggested risk label (`Risk:low|medium|high`)
    - follow-ups (missing `data-testid`s, product bugs, cases left out and why)
10. Apply any requested changes, re-run steps 6–7, and resend the diff until approved.
11. Only after explicit approval: create branch `<ticket-id>-<slug>` (e.g. `e2e-306-escalation-path-is-one-of`, no `feature/` prefix), commit, push, open the PR with the risk label and the Linear ticket ID in the title, and move the ticket to In Review.

## Specifications
- Every ticket case/AC is covered (alone or grouped in a test), or explicitly called out as not covered with a reason.
- Every locator was seen in the live DOM via the Playwright MCP.
- Lint, typecheck and format pass with no new warnings.
- New/changed specs pass a single run, then `--repeat-each=3`, on prod and on staging (unless explicitly scoped to one env).
- Tests run in the intended test org, with IDs from `orgConfig.ts`.
- No commit, push, PR or Linear status change happens before user approval.

## Advice and Pointers
- Follow `CLAUDE.md` for roles, POMs and run-ID conventions. Its POM notes record UI quirks (Choices.js hidden selects, Turbo-streamed timelines, autofocused inputs) that are easy to trip on.
- Prefer API setup over UI setup. Exercise in the UI only the flow under test.
- Update `CLAUDE.md` if you add a POM, role or helper.
- The repo's `.claude/skills/playwright-best-practices` and `playwright-skill` are useful references for locators and flake fixes.

## Forbidden Actions
- Guessing selectors, or copying them from tickets or screenshots without checking the live DOM.
- `waitForTimeout` / fixed `delay()` waits; `test.only` / `describe.only`.
- Committing, pushing, opening a PR, or changing Linear before approval.
- Running against customer orgs, or mutating shared org config without restoring it.
