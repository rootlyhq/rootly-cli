
## Trigger
Two correct findings were auto-suppressed on the premise 'testing/README.md does not exist at this commit' and 'rails/testing.md does not exist,' despite sound code-level evidence.

## The Mistake
Finding 1 (correct, suppressed): Test IDs should be stable (data-testid attributes should follow pattern sre-runs-*). Evidence: data-testid appears 129 times under app/views/admin in sibling partials all following sre-runs-* pattern. Suppressed because: testing/README.md not found.

Finding 2 (correct, suppressed): Zero-delta parity test is tautological. Evidence: EntityRefs#call is defined as `report.refs`, so assertion compares report.refs to itself. Suppressed because: rails/testing.md not found.

Both findings were technically correct and code-grounded, not dependent on documentation existing.

## Why It Happened
Suppression logic treated 'missing documentation' as grounds to disbelieve the finding itself, rather than as grounds to note that the pattern is not formally documented but still real in the codebase.

## Why It's Dangerous
Conventions can be real and locally enforced even without a written spec. Discarding a finding because the spec is missing throws away true positives. Second-order cost: teaches reviewers to dismiss findings for procedural reasons (missing doc) rather than technical ones (is the code correct?).

## The Fix
Separate two questions: (1) Is the finding technically correct? (Answer: consult the code, not the docs) (2) Is there a written convention this violates? (Answer: could be yes or no, document accordingly). If the answer to (1) is yes, take the finding even if (2) is no. Note 'this convention is observed in N places but not formally documented' rather than suppressing the finding. Missing documentation is a documentation bug, not evidence that the finding is wrong.

Trigger: Two correct findings (test IDs, tautological assertion) were auto-suppressed on 'missing documentation' grounds despite sound code evidence; both taken after manual escalation and re-verification
