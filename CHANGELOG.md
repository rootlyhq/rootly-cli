# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Fix plural filter param names for `/v1/oncalls` endpoint (`filter[schedule_ids]`, `filter[service_ids]`, etc.)
- URL-encode filter values across all list endpoints

### Added
- Name-based filtering for `oncall shifts` and `oncall who` (`--schedule`, `--service`, `--user`, `--team` flags)
- Team/group filtering via `--team-id` / `--team` flags

## [0.3.3] - 2026-06-04

### Added
- AI agent quick-start section in help output

## [0.3.2] - 2026-06-04

### Fixed
- Fix `incidents get/update/delete` with sequential IDs (INC-xxx format)

## [0.3.1] - 2026-05-18

### Changed
- Dependency updates

## [0.3.0] - 2026-05-14

### Added
- `rootly login` — browser-based OAuth2 authentication with PKCE (no API key needed)
- `rootly logout` — clear stored OAuth tokens
- OAuth2 auto-refresh transport using `golang.org/x/oauth2`
- Auto-append `/api` for localhost endpoints (no need to pass `--api-host=localhost:22166/api`)
- `http://` scheme auto-detection for localhost/127.0.0.1 endpoints

### Changed
- OAuth tokens stored in `~/.rootly-cli/config.yaml` under `oauth` key (single config file)
- API client uses OAuth Bearer tokens when available, falls back to API key
- Auth-exempt commands use `Annotations["skipAuth"]` instead of hardcoded name list
- Pin GitHub Actions SHAs for supply-chain security

## [0.2.1] - 2026-03-05

### Changed
- Rename `oncall list` to `oncall schedules`

### Fixed
- Fix `oncall schedules` 404 error (use correct `/v1/schedules` endpoint)

## [0.2.0] - 2026-03-05

### Changed
- Switch `oncall who` and `oncall shifts` to unified `/v1/oncalls` endpoint with richer data (escalation policy, level, user email)
- Add new filter flags: `--schedule-id`, `--service-id`, `--escalation-policy-id`, `--user-id`, `--time-zone`, `--earliest`
- Table output now includes Escalation Policy, Level, and Email columns

### Fixed
- Correct env var name in docs from `ROOTLY_API_TOKEN` to `ROOTLY_API_KEY`

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

[Unreleased]: https://github.com/rootlyhq/rootly-cli/compare/v0.3.3...HEAD
[0.3.3]: https://github.com/rootlyhq/rootly-cli/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/rootlyhq/rootly-cli/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/rootlyhq/rootly-cli/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/rootlyhq/rootly-cli/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/rootlyhq/rootly-cli/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.5...v0.2.0
[0.1.5]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/rootlyhq/rootly-cli/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/rootlyhq/rootly-cli/releases/tag/v0.1.2
