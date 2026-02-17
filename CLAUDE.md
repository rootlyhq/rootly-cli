# Rootly CLI

A command-line interface for managing Rootly incidents, alerts, services, teams, and on-call schedules.

## Tech Stack

- **Go 1.24+**
- **Cobra** - CLI framework
- **Viper** - Configuration management
- **go-pretty** - Table output formatting
- **rootly-go** - Rootly API client (raw HTTP, JSON:API format)

## Project Structure

```
cmd/rootly/main.go           # CLI entry point
internal/
  api/
    client.go                # Rootly API wrapper (stateless CLI methods)
  cmd/
    root.go                  # Cobra root command, Viper config, PersistentPreRunE
    version.go               # Version command with build info
    completion.go            # Shell completions (bash, zsh, fish, powershell)
    helpers.go               # Shared helpers (confirm, dry-run)
    incidents/               # Incidents CRUD subcommands
    alerts/                  # Alerts CRUD + ack/resolve subcommands
    services/                # Services CRUD subcommands
    teams/                   # Teams CRUD subcommands
    oncall/                  # On-call read-only subcommands (list, shifts, who)
    *_register.go            # Wire subcommands to rootCmd
  config/config.go           # Config management (~/.rootly-cli/config.yaml)
  printer/
    printer.go               # Printer factory (table/json/yaml/markdown) with TTY detection
```

## Build Commands

```bash
make build      # Build binary to bin/rootly
make build-all  # Build with CGO_ENABLED=0 for cross-compilation
make run        # Build and run
make dev        # Run without building (go run)
make test       # Run tests
make lint       # Run golangci-lint
make coverage   # Run tests with coverage report
make check      # Format, lint, and test
```

## Architecture

### Command Pattern
- `rootly <resource> <verb>` (e.g., `rootly incidents list`, `rootly alerts ack <id>`)
- Each resource is a subpackage under `internal/cmd/`
- Registration files (`*_register.go`) in package `cmd` wire subcommands to rootCmd
- This avoids circular imports between subpackages and root

### API Client
- `NewClientWithoutCache()` — stateless constructor for CLI (no cache)
- Methods follow pattern: `List*CLI`, `Get*ByID`, `Create*`, `Update*`, `Delete*`
- Raw HTTP with JSON:API format

### Output
- `printer.NewPrinter(format)` — factory for table/json/yaml/markdown
- TTY detection: table for terminal, JSON when piped
- go-pretty with `FormatDefault` to preserve header casing
- Pagination info printed to stderr, data to stdout

### Config

Config stored at `~/.rootly-cli/config.yaml` or via `ROOTLY_API_TOKEN` env var:

```yaml
api_key: "your-api-key"
api_host: "api.rootly.com"
```

## Key Patterns

- **Confirmation prompts**: Inline `confirmDelete()` in each delete.go (avoids circular imports)
- **Change detection**: `cmd.Flags().Changed()` for update commands (send only modified fields)
- **Required flags**: `MarkFlagRequired()` for create commands
- **Subcommand aliases**: `incidents` → `incident`, `services` → `service`/`svc`

## Releasing New Versions

Releases are automated via GitHub Actions and GoReleaser. Pushing a tag triggers the release workflow.

```bash
make version         # Check current version and recent tags
make release-patch   # v0.2.0 -> v0.2.1
make release-minor   # v0.2.0 -> v0.3.0
make release-major   # v0.2.0 -> v1.0.0
```

The release workflow:
1. Builds binaries for Linux, macOS (Intel + Apple Silicon), and Windows
2. Creates GitHub release with changelog
3. Updates Homebrew tap at `rootlyhq/homebrew-tap`

Monitor releases at: https://github.com/rootlyhq/rootly-cli/actions
