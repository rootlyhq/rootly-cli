# Deployment Risk Assessment: Tiered Change Classification

Every PR must be classified as **High / Medium / Low Risk** before merging.
The classification determines **validation requirements, rollout plan, and client communications.**

## High Risk

**Definition:**
Changes where failure could cause:
- Widespread downtime
- Issues in business critical functionality (see Core Features below)
- Issues specifically for Yahoo (e.g., modifications to a feature we know is critical for them)
- Anything where if we mess it up, it's a Sev0

**Examples:**
- **Core Features**: Anything that alters the **behavior** of Paging (Alert Sources, Heartbeats, LCR, non-Alert Sources), routing alerts, alert suppression (deduplication, grouping), Escalation Policy Execution, Scheduler, Propagating data to Notifications Service, Creating Incidents, Workflow Execution, Slack Auth logic, Authentication/Login (including SSO), Permissions (RBAC), changes in any Lambda involved in the paging pipeline, Shift Calculator.
- **Networking, Edge, Infra**: DNS, ELB config, CDN, Cloudflare, TLS, Rate Limiting, API Gateway Config (routing, retries, etc), Caching, any Infra config.
- **Data Layer**: DB Changes (scaling, sharding, replication), Index Modification on high traffic tables, high risk Data Migrations.
- **Caching Policy**: Changes to caching strategies (e.g., TTLs, cache keys, eviction policies) that could cause a thundering herd — such as modifying how we cache user profile images. If all caches expire simultaneously, the resulting spike in origin requests could cause widespread degradation or downtime.

**Important:** Performance optimizations (e.g., eager loading, N+1 fixes) and display-only changes in core feature areas do not automatically qualify as high risk — classify based on what breaks if the change has a bug, not which files are touched.

**Process:**
1. Mandatory Testing Plan created with our QA Team
2. Mandatory Staging Validation
3. PR Requires 1 standard approval + 1 extra from your EM
4. Safe Rollout Plan Documented
5. Feature Flagged where possible, progressive rollout (bottom 80% of customers first, top 20 last)
6. Dedicated Deploy Huddle during rollout (Eng + a PR Reviewer + QA monitoring live)
7. Mandatory Production Validation (with Yahoo-specific checks - impersonation + checking in a test org that has Yahoo config)
8. Validation Receipt in Linear with Loom, metrics screenshots, Yahoo checks
9. Client Notification: If Yahoo-impacting, notify proactively. Talk to Product to make the call on whether notification is required.

## Medium Risk

**Definition:**
Changes to non-core functionality where failure would not impact all users OR would not impact business critical features OR would not cause downtime.

**Examples:**
- Most changes likely fall in the Medium Risk bucket
- Changes required for an in-progress feature, where all changes are behind a Flipper (need to regression test in staging with the flipper off)
- Feature-work outside of the Core Features defined above

**Process:**
1. Mandatory Staging Validation
2. PR Requires 1 approval, no constraints on who
3. Safe Rollout Plan Documented
4. Feature Flagged preferred where possible, progressive rollout (bottom 80% of customers first, top 20 last) if feature flag is used
5. Mandatory Production Validation (with Yahoo-specific checks - impersonation + checking in a test org that has Yahoo config)
6. Validation Receipt in Linear with Loom of prod test + observability screenshots

## Low Risk

**Definition:**
Changes that are purely cosmetic, internal-only, or low-blast radius. If broken, impact is isolated, quickly reversible, or not customer-visible.

**Examples:**
- Copy / text updates in UI
- Small bug fixes in non-critical features
- Small UI changes
- Observability improvements (adding a metric or log line)
- Internal tooling / dev experience improvements
- Minor dependency or Ruby version bumps

**Process:**
1. Staging testing not a requirement - up to Engineer's discretion
2. PR requires 1 approval, no constraints on who
3. Validation in Production required
4. Validation Receipt in Linear ticket (short Loom or screenshot)
5. Can be rolled out immediately, no feature flag required

## Important Notes

- Validation Receipt is required for **all** risk categories.
- It is a requirement that a PR has a Risk Category on it the moment it is put up for review.
- It's the reviewer's job to validate the Risk Categorization as well as the PR itself.
- Monitor the [Production Alerts Sentry Dashboard](https://rootly.sentry.io/issues/views/198378/?environment=production) after deploying.
