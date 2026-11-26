# Breaking Changes

## Overview

Breaking changes are modifications that alter existing API contracts, or behaviors in ways that can break client integrations or existing functionality. These changes require careful planning and communication to minimize disruption.

## Why This Matters

Recent breaking changes (maintenance statuses, nested schedules API, alert routes) were handled poorly, causing significant pain for clients and GTM. **It is not acceptable to ship breaking changes without proper planning.**

## What Constitutes a Breaking Change

- Removing or renaming API endpoints
- Changing API request/response formats
- Changing function signatures or behavior that clients depend on

## Required Process

### 1. Planning

- **Document in Engineering Doc** — Explicitly call out breaking changes
- **Include in PRD** — Breaking changes must be identified in the PRD
- **Create Migration Plan** — Document how existing clients will transition
- **Consider Deprecation Timeline**

### 2. Communication

- **GTM Team** — Must be informed early for customer coordination
- **Clients** — Provide advance notice through deprecation warnings, changelog, and direct outreach for major changes

### 3. Implementation

- **Use API Versioning** — Maintain old and new versions simultaneously when possible
- **Provide Migration Guides** — Clear documentation with migration steps
- **Monitor Usage** — Track usage of deprecated features

## API Object Backwards Compatibility

When modifying API objects (request/response shapes, field types, nesting), you **must** ensure the change is backwards compatible at the API level. This means:

- **Additive fields are safe** — Adding a new optional field to a response or request does not break existing clients.
- **Changing field types is breaking** — e.g., changing a field from `string` to `object`, or from a flat map to a nested structure.
- **Changing nesting is breaking** — e.g., moving `conditions` from a JSON-encoded string inside a flat map to a first-class nested object.
- **Removing or renaming fields is breaking** — Even if the new name is "better", existing clients depend on the old one.

If your API change **is not** backwards compatible, you must:

1. **Version the endpoint or field** — Support both the old and new shape simultaneously during a deprecation window.
2. **Update all downstream consumers** — This includes the Terraform provider, SDKs, and any internal services. See [Terraform Provider](./terraform-provider/README.md) for provider-specific requirements.
3. **Provide migration tooling** — Clients should not need to write manual migration scripts.

## Best Practices

- **Avoid When Possible** — Consider additive changes instead
- **Maintain Backwards Compatibility** — Use [feature flags](../feature-flags) or dual-write patterns during transitions
- **Monitor and Support** — Be available to help clients during migration

## Questions?

If you're unsure whether a change is breaking or how to handle it, ask in #engineering before proceeding.
