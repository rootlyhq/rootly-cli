
## Problem
Test selectors added retroactively or inconsistently lead to fragile tests that select by CSS class, position, or text content—brittle to refactor, hard to maintain.

## Solution
When implementing a new UI component, add `data-testid` in the same commit. This PR added `data-testid="sre-evals-bucket-bar"` to `.bar-row` in `_bucket_bars.html.erb` during implementation; request spec immediately uses it: `page.find('[data-testid="sre-evals-bucket-bar"]')`. Selector becomes part of the feature contract.

## Pitfalls Avoided
- Test flakiness from incidental CSS or markup refactors.
- Clear intent in specs: readers see exactly which element is tested.
- Aligns with Rootly's `testing/README.md` (lines 9–33) to add selectors during feature work.
