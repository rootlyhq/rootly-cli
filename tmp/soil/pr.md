# Warn when resolving an incident from the CLI leaves status page events open

## Why

The status page commands in the ask already shipped in rootly-cli#57 (v0.6.1) as `status-pages list` and `status-pages events list/create/update/resolve`. What remains is the footgun in the title: resolving an incident from the CLI says nothing about public status-page entries still open. Add a best-effort, stderr-only warning, in its own helper package, that lists each open event with its status page and the exact `rootly status-pages events resolve <id>` command. No auto-resolve, no renames.

## What changed

- Warn about open status page events when an incident is resolved
- Document the status page warning in help and README
- Follow event pagination and print --message in resolve command
- Inspect every events page, no page cap
- Use total_pages and drop ignored-page duplicates

## Proof
