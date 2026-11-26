> Migrated from Devin knowledge note `note-3266c0ad10024864854dc5ca53e50cc8` (macro `qa-test-plan`), org rootly-development, on 2026-10-24 (devin-migration).

QA Checklist Generator

Turn a pull request plus its Linear ticket into a QA checklist a developer can self-apply and QA can trust to sign off. The goal is consistency: every change, big or small, gets the same three-tier treatment — happy path, regression (scoped to what actually changed), and edge cases — with each item justified by evidence from the diff or the ticket, not guesswork.

A checklist QA trusts has three properties, and every step below serves them:

Grounded — each regression item points to a real file/module in the diff, so QA can see why it's on the list.
Actionable — each item is a concrete, observable check with a clear pass/fail, not a vague "test the feature".
Honest about gaps — if something can't be verified from the PR (e.g. requires prod data, a paid integration, or a device farm), say so rather than inventing a check.
Guardrail: never post without explicit approval

This skill reads freely but writes only after the human approves. Always present the draft checklist in chat first and ask for a go-ahead before posting anything to GitHub or Linear. Offer the person a chance to edit tiers, add or remove items, or change wording. Posting a wrong checklist erodes the trust the whole workflow depends on, so treat the approval gate as mandatory even when the change looks trivial.

Workflow
Step 1 — Identify the PR and the linked Linear ticket

You need both inputs. Resolve them from whatever the user gave you:

A GitHub PR URL or owner/repo#123 → the PR.
A Linear ticket ID (e.g. IR-1234, OC-88) or URL → the ticket.
Just one of the two → find the other. PRs at Rootly usually name the ticket in the branch (kate/IR-1234-fix-escalation), title, or description; Linear tickets link the PR under attachments/dev links. Use GitHub search or the Linear ticket's git branch/PR links to bridge the gap.

If you genuinely can't find the counterpart after looking, don't stall — tell the person what you found, note that the checklist will be weaker without the missing half (ticket gives intent/acceptance criteria; PR gives blast radius), and ask if they want to proceed with one side or supply the other.

Step 2 — Gather the evidence

From GitHub (`gh pr view --json ...`, `gh pr diff`, or the GitHub tools the session has):

Title, description, author, target branch, draft status.
The list of changed files with additions/deletions per file.
The actual diff for non-trivial files. Read it — the specific functions, conditionals, and data shapes that changed are what make regression items concrete. Don't skim; the diff is your primary source of truth.
Linked/related PRs, labels, and any migration or feature-flag files.
Feature flags (Flipper). Scan the diff for flag gating — Rootly uses Flipper, so look for Flipper.enabled?(:flag_name), flipper_enabled?, Flipper[:flag_name], feature_enabled?(:...), flag constants, or flags added in config/initializers/seed files. For every flag the changed code reads, record the flag name and, from the diff, whether the new behavior is behind it (ON reveals the change) or the flag guards a fallback (OFF is the old path). QA cannot test what they can't turn on, so a checklist that omits the flag is broken before it starts — treat flag capture as required whenever gating is present.

From Linear (get_issue / get_document):

Description and, crucially, acceptance criteria — these seed the happy path.
Labels (bug vs feature vs chore), team (IR/OC/BIN/AIL/RAI/MOB), and any linked customer needs or parent project that hint at what matters to users.

If Linear (or another connector) isn't authorized in this session, say so plainly and continue from the GitHub side; the person can connect it in their Soil connector settings.

Step 3 — Assess risk from the diff

This is the analytical core and what separates a trusted checklist from a generic one. Classify the change and map each changed area to what it could break. Read the diff for risk signals (shared modules, auth/permissions, migrations, feature flags, async/jobs, external APIs, money/quotas, notifications/paging), the regressions each signal implies, and the edge-case families to sweep. (The Devin original pointed at a references/risk-heuristics.md file that was never part of the note.)

The output of this step is an internal map: "file X changed function Y, which is also used by Z → regression check on Z; the change branches on empty input → edge case for empty/null." You'll turn that map into checklist items next.

Step 4 — Build the checklist

Use this exact structure so every checklist looks the same and QA knows where to look. Keep items concrete and observable. Aim for coverage, not volume — a tight list of high-signal checks beats an exhaustive one nobody runs. Right-size to the change: a one-line copy fix might be 3–5 items; a new feature touching jobs and permissions might be 15–20.

markdown
## QA Checklist — [PR title] ([PR link])
**Ticket:** [LINEAR-ID] · **Author:** @author · **Risk:** Low / Medium / High
**Scope:** one-line summary of what changed and the main areas it touches.

### 🚩 Setup — feature flags to enable
_Turn these on in the test environment before running the checks below. If none, say "No feature flags — nothing to enable."_
| Flipper flag | Enable for testing | Default in prod | Notes |
|--------------|--------------------|-----------------|-------|
| `flag_name` | ON | OFF | New behavior is gated behind this; test both ON and OFF (see R#) |

### ✅ Happy path
_The change does what the ticket asked._
| # | Check | Why it matters | Result |
|---|-------|----------------|--------|
| H1 | [observable action → expected result] | [ties to acceptance criterion] | ☐ Pass ☐ Fail |

### 🔁 Regression — scoped to this diff
_Existing behavior that could break because of what changed._
| # | Check | Why it matters (file/module) | Result |
|---|-------|------------------------------|--------|
| R1 | [verify existing behavior still works] | `path/to/file.rb` — [what in the diff creates the risk] | ☐ Pass ☐ Fail |

### ⚠️ Edge cases
_Boundaries, bad input, and failure modes._
| # | Check | Why it matters | Result |
|---|-------|----------------|--------|
| E1 | [boundary / error / concurrency case] | [failure mode it guards against] | ☐ Pass ☐ Fail |

### 🚫 Not covered here
_Called out honestly so QA knows the gaps._
- [check that needs prod data / paid integration / device / manual setup, and why it's out of scope for this checklist]

Rules that keep the list trustworthy:

Every regression row must name the file or module from the diff in its "why" column. If you can't tie it to the diff, it belongs in edge cases or not at all.
Every happy-path row should trace to an acceptance criterion or the ticket's stated intent. If the ticket had explicit acceptance criteria, cover each one.
Prefer checks a human can run against a staging/preview environment. Where the repo has E2E coverage (e.g. Playwright), note if an existing test already covers an item so QA doesn't duplicate effort.
Set Risk from the heuristics: High if it touches auth/permissions, migrations, money/quota, or incident paging/notifications; Medium for shared modules or async jobs; Low for isolated/cosmetic changes.
If the change is behind a feature flag, the Setup table must list it, and there must be a regression row checking the flag-OFF path still works (the old behavior) — a flagged feature that breaks when the flag is off is a common, easily-missed failure.
Step 5 — Review with the human

Show the full draft in chat. Briefly note the risk rating and why, and point out anything you flagged as "not covered". Explicitly invite edits: tiers, individual items, wording, risk level. Wait for approval before Step 6.

Step 6 — Post (only after approval)

On approval, post the approved markdown:

GitHub: as a PR comment (`gh pr comment <number>`).
Linear: as a comment on the ticket (save_comment / create_comment).

Post to both by default so the checklist lives with the code and with the ticket. If the person wants only one destination, honor that. After posting, give them the comment link(s). If a write fails (permissions/auth), report which destination failed and hand them the markdown so nothing is lost.

A note on tone and trust

QA adopts this only if it's right more than it's wrong. When you're unsure whether something changed behavior, phrase the check as a verification ("confirm X still …") rather than asserting a bug exists, and lean on the "Not covered" section instead of padding with low-confidence guesses. Under-claiming beats crying wolf.
