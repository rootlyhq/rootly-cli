
## Problem
Test state (e.g., the `queuedSelectResults` array mocking DB responses) is reset manually at the END of each test body instead of in a shared `afterEach` or `beforeEach` hook. This is easy to forget when adding new tests, and left-over state from one test leaks into the next test, causing flaky failures.

## Example
```typescript
describe('readChecksForCommits', () => {
  it('test 1', async () => {
    queuedSelectResults = [/* test 1 data */];
    // ... assertions ...
    queuedSelectResults = []; // Manual reset at the end
  });

  it('test 2', async () => {
    queuedSelectResults = [/* test 2 data */];
    // If test 1 forgot to reset, queuedSelectResults still has test 1's data
    // and this test fails unexpectedly
  });
});
```

## Solution
Centralize cleanup in a shared `beforeEach` (or `afterEach`) hook.

```typescript
describe('readChecksForCommits', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    queuedSelectResults = [];
  });

  it('test 1', async () => {
    queuedSelectResults = [/* test 1 data */];
    // beforeEach handles the reset for test 2 automatically
  });

  it('test 2', async () => {
    queuedSelectResults = [/* test 2 data */];
    // beforeEach resets again before test 3
  });
});
```

## Why it Matters
Test flakiness from state leaks is hard to debug (test passes alone, fails when run after another). Centralizing setup/teardown makes the contract explicit: every test starts clean. It also reduces boilerplate and makes it harder to forget the reset.

Trigger: Council hygiene-3: queuedSelectResults reset repeated at the end of each test instead of in beforeEach/afterEach
