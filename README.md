<p align="center">
  <img src="rootly-cli-logo.png" alt="Rootly CLI" />
</p>

A command-line interface for managing Rootly incidents, alerts, services, teams, on-call schedules, and pulses from the terminal.

> **Note**: This is the successor to [`rootlyhq/cli`](https://github.com/rootlyhq/cli) (now archived). All pulse functionality from the original CLI is included here.

![Go Version](https://img.shields.io/github/go-mod/go-version/rootlyhq/rootly-cli)
![License](https://img.shields.io/github/license/rootlyhq/rootly-cli)
![Release](https://img.shields.io/github/v/release/rootlyhq/rootly-cli)

## Features

- Full CRUD for incidents, alerts, services, and teams
- Pulse tracking (`rootly pulse create`, `rootly pulse run`)
- On-call schedule queries (list schedules, view shifts, who's on-call)
- Alert action shortcuts (`rootly alerts ack`, `rootly alerts resolve`)
- Multiple output formats: table, JSON, YAML, markdown
- TTY-aware output (table in terminal, JSON when piped)
- Shell completions for bash, zsh, fish, and PowerShell
- Confirmation prompts for destructive operations
- Pagination and server-side filtering

## Installation

### Homebrew (macOS/Linux)

```bash
brew install rootlyhq/tap/rootly-cli
```

### Go Install

```bash
go install github.com/rootlyhq/rootly-cli/cmd/rootly@latest
```

### Download Binary

Download the latest release from the [Releases](https://github.com/rootlyhq/rootly-cli/releases) page.

Available for Linux (amd64/arm64), macOS (Intel/Apple Silicon), and Windows (amd64).

### Build from Source

```bash
git clone https://github.com/rootlyhq/rootly-cli.git
cd rootly-cli
make build
./bin/rootly
```

## Configuration

Set your API key via environment variable or config file:

```bash
# Environment variable (recommended for CI/scripts)
export ROOTLY_API_KEY="your-api-key"

# Or config file at ~/.rootly-cli/config.yaml
```

Config file format (`~/.rootly-cli/config.yaml`):

```yaml
api_key: "your-api-key"
api_host: "api.rootly.com"  # Optional, defaults to api.rootly.com
```

### Getting an API Key

1. Log in to your Rootly account
2. Navigate to **Settings** > **API Keys**
3. Create a new API key with appropriate permissions

## Usage

```bash
# Incidents
rootly incidents list
rootly incidents list --status=started --severity=critical
rootly incidents get <id>
rootly incidents create --title="Database outage" --severity=critical
rootly incidents update <id> --status=mitigated
rootly incidents delete <id>

# Alerts
rootly alerts list
rootly alerts get <id>
rootly alerts create --summary="High CPU usage" --source=datadog
rootly alerts ack <id>
rootly alerts resolve <id>

# Services
rootly services list
rootly services get <id>
rootly services create --name="api-gateway"
rootly services update <id> --description="Main API gateway"
rootly services delete <id>

# Teams
rootly teams list
rootly teams get <id>
rootly teams create --name="Platform"
rootly teams update <id> --color="#FF5733"
rootly teams delete <id>

# On-Call
rootly oncall schedules                     # List schedules
rootly oncall who                           # Who's on-call right now
rootly oncall who --schedule-id=sched-123   # Filter by schedule
rootly oncall who --service-id=svc-456      # Filter by service
rootly oncall shifts                        # View upcoming shifts (7 days)
rootly oncall shifts --days=14              # Next 14 days
rootly oncall shifts --escalation-policy-id=ep-789

# Pulses
rootly pulse create "Deploy v1.2.3"                      # Send a pulse
rootly pulse create "Deploy v1.2.3" -l "version=1.2.3,team=backend"
rootly pulse create "Deploy v1.2.3" -s api-gateway -e production
rootly pulse create "Deploy v1.2.3" --source=ci -r "commit=abc123,pr=456"

# Pulse Run (wrap a command and send pulse with timing + exit code)
rootly pulse run -- make deploy
rootly pulse run --summary="Deploy to prod" -- make deploy
rootly pulse run -s api-gateway -l "env=prod" -- make deploy
```

### Pulse Flags

| Flag | Short | Env Var | Description |
|------|-------|---------|-------------|
| `--labels` | `-l` | `ROOTLY_LABELS` | Key=value pairs, comma-separated |
| `--services` | `-s` | `ROOTLY_SERVICES` | Service slugs/IDs, comma-separated |
| `--environments` | `-e` | `ROOTLY_ENVIRONMENTS` | Environment slugs/IDs, comma-separated |
| `--source` | | `ROOTLY_SOURCE` | Source identifier (default: `cli`) |
| `--refs` | `-r` | `ROOTLY_REFS` | Reference key=value pairs, comma-separated |
| `--summary` | | `ROOTLY_SUMMARY` | Summary (alternative to positional arg) |

`pulse run` also accepts `--summary` (defaults to the command string) and automatically adds an `exit_status` label with the command's exit code.

### Output Formats

```bash
# Table (default in terminal)
rootly incidents list

# JSON (default when piped, or explicit)
rootly incidents list --format=json
rootly incidents list --format=json | jq '.[] | .title'

# YAML
rootly incidents get <id> --format=yaml

# Markdown (for documentation or LLM consumption)
rootly incidents list --format=markdown
```

### Pagination & Filtering

```bash
# Pagination
rootly incidents list --page-size=50 --page=2

# Filtering
rootly incidents list --status=started --severity=critical
rootly alerts list --source=datadog
rootly services list --name=api

# Sorting (prefix with - for descending)
rootly incidents list --sort=created_at
rootly incidents list --sort=-created_at
```

### Global Flags

| Flag | Short | Env Var | Description |
|------|-------|---------|-------------|
| `--api-key` | `-k` | `ROOTLY_API_KEY` | Rootly API key |
| `--api-host` | | `ROOTLY_API_HOST` | API host (default: `api.rootly.com`) |
| `--format` | | | Output format: `table`, `json`, `yaml`, `markdown` |
| `--debug` | `-d` | `ROOTLY_DEBUG` | Enable debug output |
| `--quiet` | `-q` | `ROOTLY_QUIET` | Suppress non-essential output |
| `--no-color` | | | Disable colored output |

### Command Reference

| Command | Subcommands | Aliases |
|---------|-------------|---------|
| `rootly incidents` | `list`, `get`, `create`, `update`, `delete` | `incident`, `inc` |
| `rootly alerts` | `list`, `get`, `create`, `update`, `ack`, `resolve` | `alert`, `alr` |
| `rootly services` | `list`, `get`, `create`, `update`, `delete` | `service`, `svc` |
| `rootly teams` | `list`, `get`, `create`, `update`, `delete` | `team` |
| `rootly oncall` | `schedules`, `shifts`, `who` | `on-call` |
| `rootly pulse` | `create`, `run` | `pulses` |
| `rootly completion` | `bash`, `zsh`, `fish`, `powershell` | |
| `rootly version` | | |

### Shell Completions

```bash
# Bash
rootly completion bash > /etc/bash_completion.d/rootly

# Zsh
rootly completion zsh > "${fpath[1]}/_rootly"

# Fish
rootly completion fish > ~/.config/fish/completions/rootly.fish
```

## Development

### Prerequisites

- Go 1.24+
- Make

### Build & Test

```bash
make build      # Build binary
make test       # Run tests
make lint       # Run linter
make check      # Format, lint, and test
make coverage   # Tests with coverage report
```

### Release

Releases are automated via GoReleaser when a new tag is pushed:

```bash
make release-patch   # v0.1.0 -> v0.1.1
make release-minor   # v0.1.0 -> v0.2.0
make release-major   # v0.1.0 -> v1.0.0
```

## License

MIT License - see [LICENSE](LICENSE.txt) for details.
