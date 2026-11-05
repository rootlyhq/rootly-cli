# E2E Testing Guidelines for QA Engineers

## Overview

This document provides QA engineers with comprehensive guidelines for implementing end-to-end (E2E) testing using Playwright at Rootly. These standards ensure stable, maintainable tests that are resilient to UI changes.

## Custom Selectors and Data Attributes

### Use `data-testid` for Test Selectors When Semantic Selectors Are Unavailable

**Why avoid standard selectors:**
- CSS classes frequently change during styling updates
- IDs are often tied to application logic or styling and may be volatile
- XPath selectors break with DOM structure changes
- Auto-generated attributes change between builds

**Preferred approach:** Use dedicated test attributes that decouple tests from styling and structure:

```html
<!-- ✅ Good: Stable test selector -->
<button data-testid="submit-form-button">Submit</button>

<!-- ❌ Avoid: Volatile CSS classes -->
<button class="btn btn-primary mt-4">Submit</button>

<!-- ❌ Avoid: Logic-tied IDs -->
<button id="user-123-submit">Submit</button>
```

### Playwright Implementation

```javascript
// ✅ Preferred: Use getByTestId()
await page.getByTestId('submit-form-button').click();

// ✅ Alternative: Direct attribute selector
await page.locator('[data-testid="submit-form-button"]').click();

// ❌ Avoid: Brittle selectors
await page.locator('.btn.btn-primary').click();
await page.locator('#dynamic-id-123').click();
```

### Naming Conventions

Use descriptive, hierarchical naming patterns:

```html
<!-- Pattern: {scope}-{component}-{element}-{action} -->
<input data-testid="login-form-email-input" />
<button data-testid="login-form-submit-button">Login</button>
<div data-testid="dashboard-metrics-chart">...</div>
<button data-testid="user-profile-edit-button">Edit</button>
```

**Guidelines:**
- Use kebab-case for consistency
- Include scope/context for clarity
- Be specific enough to avoid collisions
- Keep names readable and meaningful

### Selector Hierarchy

Follow Playwright's recommended selector priority:

1. **Semantic locators** (when possible):
   ```javascript
   await page.getByRole('button', { name: 'Login' });
   await page.getByText('Welcome');
   await page.getByLabel('Email address');
   ```

2. **Test ID locators** (for UI elements without clear semantics):
   ```javascript
   await page.getByTestId('submit-form-button');
   await page.getByTestId('complex-widget-toggle');
   ```

3. **CSS selectors** (as last resort):
   ```javascript
   await page.locator('[data-stable-id="legacy-component"]');
   ```

## Best Practices

### Timeouts and Waiting

**Use built-in waiting mechanisms:**
```javascript
// ✅ Good: Rely on auto-waiting (default 30s timeout)
await page.getByTestId('submit-button').click();

// ✅ Good: Custom timeout when needed
await page.getByTestId('slow-loading-chart').click({ timeout: 60000 });

// ✅ Good: Wait for specific conditions
await page.waitForSelector('[data-testid="success-message"]');

// ⚠️ When necessary
await page.waitForLoadState('networkidle');

// ❌ Avoid: Hard-coded delays
await page.waitForTimeout(3000);
```

**Configure reasonable timeouts:**
```typescript
// playwright.config.ts
export default defineConfig({
  use: {
    actionTimeout: 40000, // 40 seconds for actions
    navigationTimeout: 40000, // 40 seconds for navigation
  },
  expect: {
    timeout: 15000, // 15 seconds for assertions
  },
});
```

### Page Navigation

**Best practices for page visits and navigation:**
```javascript
// ✅ Good: Use goto for initial navigation
await page.goto('/dashboard');

// ✅ Good: Wait for navigation to complete
await Promise.all([
  page.waitForNavigation(),
  page.getByTestId('logout-button').click()
]);

// ✅ Good: Use baseURL in config
// playwright.config.js: use: { baseURL: 'http://localhost:3000' }
await page.goto('/users/new');

// ✅ Good: Handle dynamic routes
await page.goto(`/users/${userId}/edit`);

// ❌ Avoid: Navigation without waiting
await page.getByTestId('nav-link').click();
// Missing: await page.waitForURL('/expected-page');
```

### Self-Cleaning Fixtures

**Design tests to clean up after themselves:**

```javascript
// ✅ Good: Use test hooks for cleanup
test.describe('User Management', () => {
  let createdUserId;

  test.afterEach(async ({ request }) => {
    // Clean up created data via API
    if (createdUserId) {
      await request.delete(`/api/users/${createdUserId}`);
      createdUserId = null;
    }
  });

  test('should create new user', async ({ page, request }) => {
    // Create user via API for faster setup
    const response = await request.post('/api/users', {
      data: { name: 'Test User', email: 'test@example.com' }
    });
    createdUserId = (await response.json()).id;

    // Test the UI interaction
    await page.goto(`/users/${createdUserId}`);
    await expect(page.getByTestId('user-name')).toHaveText('Test User');
  });
});

// ✅ Good: Use the run ID system for unique test data (see Run ID System below)
import { createResourceName } from '../utility/runID';

test('Verify new user sign up', async ({ page }) => {
  const userName = createResourceName('User');

  await page.getByTestId('name-input').fill(userName);
  // Test continues...
});

// ❌ Avoid: Raw timestamps or random strings without the run ID system
test('Verify new user sign up', async ({ page }) => {
  const timestamp = Date.now();
  const testEmail = `test-${timestamp}@example.com`;
  // No way to identify and clean up resources from this specific run
});
```

### Run ID System

Every test run generates a shared alphanumeric run ID. All resources created during the run embed this ID in their name using `createResourceName()`:

```
e2e_<timestamp>_<runID>_<resource-type>_<suffix>
```

During teardown, the cleanup process deletes only resources whose names contain the current run ID. This ensures that test-created resources are reliably cleaned up while leaving permanent resources (e.g., alert sources, shared integrations) untouched.

```typescript
import { createResourceName, getRunId } from '../utility/runID';

// All resources from this run share the same run ID
const alertName = createResourceName('Alert');       // e2e_1710000000_abc123_Alert
const scheduleName = createResourceName('Schedule'); // e2e_1710000000_abc123_Schedule
const userName = createResourceName('User');          // e2e_1710000000_abc123_User

// Teardown deletes everything matching the run ID — nothing else
```

### Organization Isolation

For tests that may conflict even with unique naming (e.g., global settings, integrations, webhooks), run them on separate Rootly organizations:

```typescript
// Tests modifying org-wide Slack settings → org A
// Tests modifying org-wide paging settings → org B
```

This prevents race conditions where one test's org-level changes affect another test's assertions. Use environment variables and role-based auth to target the correct organization per test suite.

**Fixture management for Rails apps:**
```javascript
// Create reusable fixture helpers
class TestDataManager {
  constructor(request) {
    this.request = request;
    this.createdResources = [];
  }

  async createUser(userData) {
    const response = await this.request.post('/users', {
      data: userData,
      headers: { 'Content-Type': 'application/json' }
    });
    const user = await response.json();
    this.createdResources.push({ type: 'user', id: user.id });
    return user;
  }

  async createIncident(incidentData) {
    const response = await this.request.post('/incidents', {
      data: incidentData,
      headers: { 'Content-Type': 'application/json' }
    });
    const incident = await response.json();
    this.createdResources.push({ type: 'incident', id: incident.id });
    return incident;
  }

  async cleanup() {
    // Clean up in reverse order to handle dependencies
    for (const resource of this.createdResources.reverse()) {
      await this.request.delete(`/${resource.type}s/${resource.id}`);
    }
    this.createdResources = [];
  }
}
```

**Use environment-aware API helpers for setup and teardown:**

`getEndpoint(route)` builds the correct API URL for the current environment (production uses `/v1/<route>`, staging uses `/api/v1/<route>`). `getHeaders(prodToken, stageToken)` returns the appropriate auth and content-type headers, automatically including Cloudflare Access headers for staging.

```typescript
import { getEndpoint, getHeaders } from '../utility/helperWeb';

const endpoint = getEndpoint('incidents');
const headers = getHeaders(prodToken, stageToken);

// Create a resource
const response = await request.post(endpoint, {
  headers,
  data: { data: { type: 'incidents', attributes: incidentData } },
});
const incident = await response.json();

// Clean up in afterEach
await request.delete(getEndpoint(`incidents/${incident.data.id}`), { headers });
```

### Linting with eslint-plugin-playwright

The test suite uses [`eslint-plugin-playwright`](https://github.com/playwright-community/eslint-plugin-playwright) to catch common mistakes — missing assertions, hardcoded waits, fragile patterns, and more. The recommended ruleset is enabled by default.

**Respect lint rules rather than disabling them.** If a rule flags your code, fix the underlying issue instead of reaching for `eslint-disable`. Disable comments should be rare and always include a justification:

```typescript
// ❌ Before: Lint flags playwright/expect-expect — test has no assertion
test('check status', async ({ page }) => {
  await page.goto('/dashboard');
  await page.getByTestId('status').click();
});

// ✅ After: Add the missing assertion instead of disabling the rule
test('check status', async ({ page }) => {
  await page.goto('/dashboard');
  await expect(page.getByTestId('status')).toBeVisible();

// ❌ Avoid: Suppressing without reason
// eslint-disable-next-line playwright/expect-expect
test('check status', async ({ page }) => { ... });

// ⚠️ Acceptable only with justification
// eslint-disable-next-line playwright/no-wait-for-timeout -- polling for third-party webhook delivery
await page.waitForTimeout(2000);
```

If you find yourself repeatedly disabling the same rule, raise it with the team — the rule config may need updating rather than scattering disable comments across test files.

### Stability
- Never use selectors with timestamps, session tokens, or random strings
- Avoid selectors that depend on specific text that might be localized
- Don't rely on element positioning (nth-child, first, last)

### Auto-waiting
Leverage Playwright's built-in waiting:

```javascript
// ✅ Good: Automatic waiting for element to be ready
await page.getByTestId('submit-button').click();

// ❌ Avoid: Manual waits
await page.waitForTimeout(1000);
await page.getByTestId('submit-button').click();
```

## Getting Test IDs Added to Templates

### Collaborative Approach with Developers (Preferred)

**During feature development (Recommended):**
Collaborate with developers during feature planning and development to include `data-testid` attributes as they build. This is the most efficient approach:

**Planning phase collaboration:**
- Review feature requirements and identify testable elements
- Discuss which UI components will need automation
- Provide developers with specific test ID requirements
- Include test ID requirements in acceptance criteria

**During development:**
- Work with developers to add test attributes as features are built
- Review PRs to ensure test IDs are included and follow conventions
- Provide immediate feedback on test ID naming and placement

```erb
<%# Developers add test IDs during feature development %>
<%= form.submit 'Login',
    data: { testid: 'login-form-submit-button' },
    class: 'btn btn-primary' %>
```

**Benefits of this approach:**
- Test IDs are included from day one
- No separate PRs needed for test infrastructure
- Developers become familiar with testing needs
- Features ship ready for automation
- Reduces technical debt and retrofit work

### Configuration

Configure Playwright to use custom test ID attributes if needed:

```javascript
// playwright.config.js
module.exports = {
  use: {
    // Configure custom test ID attribute
    testIdAttribute: 'data-testid'
  }
};
```

## Authentication State Management

Avoid logging in through UI for every test. Use Playwright's `storageState` to persist and reuse authentication.

### Global Setup Pattern

Authentication is handled once in `global-setup.ts`, which authenticates all user roles and saves their storage state files to `.auth/`:

```typescript
// playwright.config.ts
export default defineConfig({
  globalSetup: resolve(__dirname, './global-setup.ts'),
  globalTeardown: resolve(__dirname, './utility/global-teardown.ts'),
  projects: [
    {
      name: 'chromium-rootly',
      use: {
        storageState: 'auth.json',
      },
    },
  ],
});
```

### Role-Based Authentication

Use `test.use()` with `getStorageStatePath()` to run tests as different user roles:

```typescript
import { test, expect } from '@playwright/test';
import { getStorageStatePath } from '../utility/authHelpers';

test.describe('Feature user tests', () => {
  test.use({ storageState: getStorageStatePath('feature-user') });

  test('can access feature', async ({ page }) => {
    await page.goto('/dashboard');
    // Test runs with feature-user's authentication
  });
});
```

Available roles are defined per environment in `utility/userRoles.ts`. Each role gets its own storage state file (e.g., `.auth/auth-kate.json`).

### External OAuth (Teams, Slack)

For OAuth flows that can't be automated, generate auth state locally with a headed browser and store as a CI secret:

```bash
# Generate locally (headed browser for OAuth prompts)
npx playwright test setup-ms-teams-auth.spec.ts --headed

# Upload .auth/ms-teams-state.json to CI secrets
```

```typescript
// Use in tests
test.use({ storageState: '.auth/ms-teams-state.json' });
```

### Staging Environment

Staging requires Cloudflare Access headers (`cf-access-client-id` and `cf-access-client-secret`) in all requests. These are injected via route patterns in `global-setup.ts` — do not manually add them in test files.

## Resources

- [Playwright Locators Documentation](https://playwright.dev/docs/locators)
- [Playwright Best Practices](https://playwright.dev/docs/best-practices)