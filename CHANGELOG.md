# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Migrated GoReleaser config from deprecated `brews` to `homebrew_casks`

## [0.1.0] - 2024-12-31

### Added
- Terminal UI for viewing Rootly incidents and alerts
- Setup screen for API key, endpoint, timezone, and language configuration
- Tabbed navigation between Incidents and Alerts views
- Split-pane layout with list and detail panels
- Keyboard-driven navigation (j/k, Tab, arrows, g/G)
- Pagination support with h/l or [ ] keys
- Detail view with Enter key to fetch extended information
- Open incidents/alerts in browser with 'o' key
- Clickable URLs and emails using OSC 8 hyperlinks
- Markdown rendering for descriptions and summaries
- Scrollable detail panes with focus mode (f key)
- In-app debug logs viewer (l key)
- Help overlay with keyboard shortcuts (? key)
- About screen with version and documentation links (a key)
- Internationalization support for 12 languages
  - English (US/UK), Spanish, French, German, Chinese, Hindi
  - Arabic, Bengali, Portuguese, Russian, Japanese
- Auto-detect system locale from environment variables
- Debug logging with --debug flag or --log file option
- Persistent cache using BoltDB (~/.rootly-tui/cache.db)
- In-memory cache with configurable TTL (5 minutes default)
- Manual refresh with 'r' key to clear cache
- Emoji icons for alert sources (Datadog, PagerDuty, etc.)
- Severity signal bars in incident list
- Pastel color scheme for status indicators
- Arrow indicator for selected rows in table views
- Clipboard support for copying from logs viewer

### Technical
- Built with Bubble Tea TUI framework (Elm Architecture)
- Uses Bubbles for UI components and Lip Gloss for styling
- Uses rootly-go SDK for API communication
- GitHub Actions CI/CD with multi-platform testing
- GoReleaser for binary releases and Homebrew tap
- golangci-lint for code quality
- 80%+ test coverage

[Unreleased]: https://github.com/rootlyhq/rootly-tui/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rootlyhq/rootly-tui/releases/tag/v0.1.0
