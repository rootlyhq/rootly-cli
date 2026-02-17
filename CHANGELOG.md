# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/rootlyhq/rootly-cli/commits/master
