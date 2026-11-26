
## Antipattern
Implement a feature without `data-testid` attributes, then add them in a follow-up PR or only after test fragility is discovered.

## Why it's bad
Tests couple to CSS classes, sibling positions, or text content. When layout refactors, tests break even though the feature works. Post-hoc selectors are an afterthought, not part of the feature contract.

## Better approach
Add `data-testid` during feature implementation (cheap, clarifies intent). Selector becomes part of the feature contract, not a test implementation detail.

Trigger: CodeFlow review identified: 'Run-page bucket rows lacked a stable test selector.' Fixed by adding `data-testid="sre-evals-bucket-bar"` and updating request spec to use it.
