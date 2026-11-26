
## Problem
Acceptance criteria and designs may describe hypothetical bugs that don't actually repro. Writing tests after the fix risks testing the fix rather than the bug.

## Solution
Write the test against the reported repro *before* implementing any fix. Run it on master and verify it fails (2 failures: expected {"summary" => "..."}; got {}). Then implement the fix and watch the same test pass (0 failures). This proves the bug existed and the fix addresses it.

## Pitfalls Avoided
- Shipping a fix for a hypothetical bug while the reported one still lives.
- Over-testing the fix's happy path without proving the bug actually reproduced.
- Accepting a design that sounds good but doesn't address the actual failure mode.
