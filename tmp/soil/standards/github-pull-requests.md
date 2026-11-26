
# Pull Request Standards

## Risk Labels (Required)

Every PR must have exactly one risk label. Add it immediately after creating the PR.

| Label | When to use |
|-------|------------|
| `risk:low` | No migrations, behind feature flag, config-only, docs, tests |
| `risk:medium` | New API endpoints, service changes, non-destructive migrations |
| `risk:high` | Destructive migrations, auth changes, billing, data deletion, infra |

```bash
gh pr edit <PR_NUMBER> --add-label "risk:low"
```

## PR Template

Always use the template at `.github/pull_request_template.md`. Fill in:
- **Why**: Business/technical motivation
- **What**: Scope, risk, highlights
- Leave **Demo** empty for engineers to fill
- Keep all checklist items — only check off completed ones

## Title Format

When a Linear ticket is associated: `[TICKET-ID] Description`
Example: `[IR-1234] Add error logging to payment service`
