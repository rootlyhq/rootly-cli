
## Problem
A config feature (e.g., a Dependabot registry) can accidentally be referenced by multiple subsystems over time. Without an automated check, a later editor might widen its scope (e.g., apply the private-gem credential to npm) and the reviewer misses it, causing auth failures or unintended behavior in unrelated ecosystems.

## Solution
1. Write a spec that asserts the feature is referenced only by its intended component(s).
2. Use `.reject` or similar to enumerate the non-intended components and assert they do NOT reference the feature.
3. Run the spec against both the pre-fix state (should fail if the feature doesn't exist yet) and the fixed state (should pass).
4. Name the spec clearly: `it "keeps the private-gem credential scoped to bundler"` tells future editors exactly what invariant to maintain.

## Pitfalls Avoided
- A spec that only asserts the registry exists would pass even if npm later started using it (false negative).
- A code comment in the YAML (per repo convention, not done here) is less maintainable than a failing spec, because comments can go stale or be removed without consequence.
