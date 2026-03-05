# Rootly CLI

A command-line interface for managing Rootly incidents, alerts, services, teams, and on-call schedules from the terminal.

![Go Version](https://img.shields.io/github/go-mod/go-version/rootlyhq/rootly-cli)
![License](https://img.shields.io/github/license/rootlyhq/rootly-cli)
![Release](https://img.shields.io/github/v/release/rootlyhq/rootly-cli)

## Features

- Full CRUD for incidents, alerts, services, and teams
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
rootly oncall list              # List schedules
rootly oncall shifts            # View upcoming shifts
rootly oncall shifts --days=14  # Next 14 days
rootly oncall who               # Who's on-call right now
```

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
rootly incidents list --limit=50 --page=2

# Filtering
rootly incidents list --status=started --severity=critical
rootly alerts list --source=datadog
rootly services list --name=api

# Sorting
rootly incidents list --sort=created_at --order=desc
```

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
