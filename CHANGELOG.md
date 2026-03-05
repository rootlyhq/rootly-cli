# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Switch `oncall who` and `oncall shifts` to unified `/v1/oncalls` endpoint with richer data (escalation policy, level, user email)
- Add new filter flags: `--schedule-id`, `--service-id`, `--escalation-policy-id`, `--user-id`, `--time-zone`, `--earliest`
- Table output now includes Escalation Policy, Level, and Email columns

### Removed
- Removed legacy `/v1/shifts` endpoint usage and associated `Shift`/`ShiftsResult` types

## [0.1.5] - 2026-03-03

### Fixed
- Use `application/vnd.api+json` Content-Type for all API requests (fixes 415 errors for orgs created after 2026-01-01)
- Handle API returning labels as both array and object formats (fixes `incidents get` JSON parsing error)

## [0.1.4] - 2026-03-02

### Added
- `--debug` / `-d` flag to dump full HTTP requests and responses to stderr for troubleshooting

## [0.1.3] - 2026-02-17

### Added
- Pulse commands (`rootly pulse list`, `rootly pulse create`)

### Changed
- Upgraded rootly-go from v0.6.0 to v0.8.0

## [0.1.2] - 2026-02-17

### Added
- Cobra CLI with `rootly <resource> <verb>` command structure
- Full CRUD for incidents, alerts, services, and teams
- On-call read-only commands (list schedules, shifts, who's on-call)
- Alert action shortcuts (ack, resolve)
- Multiple output formats: table, JSON, YAML, markdown
- TTY-aware output (table in terminal, JSON when piped)
- Shell completions for bash, zsh, fish, and PowerShell
- Confirmation prompts for destructive operations with --yes bypass
- Pagination and server-side filtering
- GoReleaser for cross-platform binary releases
- Homebrew tap distribution
- GitHub Actions CI (lint, test, build) and release workflows

[Unreleased]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.4...HEAD
[0.1.4]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/rootlyhq/rootly-cli/releases/tag/v0.1.2
