---
name: rootly-cli
description: Manage Rootly incidents, alerts, services, teams, and on-call schedules from the terminal using the Rootly CLI
---

# Rootly CLI

Use the `rootly` CLI to manage Rootly resources directly from the terminal. The CLI follows the pattern `rootly <resource> <verb>`.

## Prerequisites

The CLI must be installed and configured before use:

```bash
# Install via Homebrew
brew install rootlyhq/tap/rootly-cli

# Or via Go
go install github.com/rootlyhq/rootly-cli/cmd/rootly@latest
```

Configure authentication with either:
- Environment variable: `export ROOTLY_API_KEY="your-api-key"`
- Config file at `~/.rootly-cli/config.yaml`:
  ```yaml
  api_key: "your-api-key"
  api_host: "api.rootly.com"
  ```

API keys are created in the Rootly dashboard under **Settings > API Keys**.

## Commands Reference

### Incidents

```bash
rootly incidents list                                          # List all incidents
rootly incidents list --status=started --severity=critical     # Filter by status and severity
rootly incidents get <id>                                      # Get incident details
rootly incidents create --title="Database outage" --severity=critical  # Create incident
rootly incidents update <id> --status=mitigated                # Update incident
rootly incidents delete <id>                                   # Delete incident (with confirmation)
```

### Alerts

```bash
rootly alerts list                                             # List all alerts
rootly alerts list --source=datadog                            # Filter by source
rootly alerts get <id>                                         # Get alert details
rootly alerts create --summary="High CPU usage" --source=datadog  # Create alert
rootly alerts ack <id>                                         # Acknowledge alert
rootly alerts resolve <id>                                     # Resolve alert
```

### Services

```bash
rootly services list                                           # List all services
rootly services list --name=api                                # Filter by name
rootly services get <id>                                       # Get service details
rootly services create --name="api-gateway"                    # Create service
rootly services update <id> --description="Main API gateway"   # Update service
rootly services delete <id>                                    # Delete service (with confirmation)
```

### Teams

```bash
rootly teams list                                              # List all teams
rootly teams get <id>                                          # Get team details
rootly teams create --name="Platform"                          # Create team
rootly teams update <id> --color="#FF5733"                     # Update team
rootly teams delete <id>                                       # Delete team (with confirmation)
```

### On-Call

```bash
rootly oncall list                                             # List on-call schedules
rootly oncall shifts                                           # View upcoming shifts
rootly oncall shifts --days=14                                 # Next 14 days of shifts
rootly oncall who                                              # Who's on-call right now
```

## Output Formats

The CLI supports multiple output formats via the `--format` flag:

- **table** (default in terminal) - human-readable table
- **json** (default when piped) - machine-readable JSON
- **yaml** - YAML output
- **markdown** - markdown tables for documentation or LLM consumption

```bash
rootly incidents list --format=json | jq '.[] | .title'
rootly incidents get <id> --format=yaml
rootly incidents list --format=markdown
```

The CLI auto-detects TTY: it uses table format in a terminal and JSON when output is piped.

## Pagination and Filtering

```bash
rootly incidents list --limit=50 --page=2
rootly incidents list --sort=created_at --order=desc
```

## Common Workflows

### Triage an active incident
```bash
rootly incidents list --status=started --severity=critical
rootly incidents get <id>
rootly incidents update <id> --status=mitigated
```

### Acknowledge and resolve alerts
```bash
rootly alerts list
rootly alerts ack <id>
rootly alerts resolve <id>
```

### Check who's on-call
```bash
rootly oncall who
rootly oncall shifts --days=7
```

## Command Aliases

- `incidents` can also be written as `incident`
- `services` can also be written as `service` or `svc`

## Important Notes

- Delete operations prompt for confirmation before executing
- Update commands only send fields that were explicitly changed via flags
- Pagination info is printed to stderr, data to stdout (safe for piping)
