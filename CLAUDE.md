# Rootly TUI

A terminal user interface for viewing Rootly incidents and alerts.

## Tech Stack

- **Go 1.24+**
- **Bubble Tea** - TUI framework (Elm Architecture)
- **Bubbles** - UI components (text input, spinner, list)
- **Lipgloss** - Terminal styling
- **Log** - Structured logging (charmbracelet/log)
- **rootly-go** - Rootly API client
- **go-i18n** - Internationalization
- **BoltDB** - Persistent cache storage

## Project Structure

```
cmd/rootly-tui/main.go      # Entry point
internal/
  api/
    client.go               # Rootly API wrapper
    cache.go                # In-memory cache with TTL
    persistent_cache.go     # BoltDB persistent cache
  app/                      # Main application model
    app.go                  # tea.Model implementation
    keymap.go               # Keybindings
    messages.go             # Message types
  config/config.go          # Config management (~/.rootly-tui/config.yaml)
  debug/debug.go            # Debug logging utilities
  i18n/
    i18n.go                 # Internationalization setup
    messages.go             # Message ID constants
    locales/*.yaml          # Translation files (12 languages)
  styles/styles.go          # Lipgloss styles and helpers
  views/                    # UI views
    setup.go                # API key/language/timezone setup screen
    incidents.go            # Incidents split view
    alerts.go               # Alerts split view
    help.go                 # Help overlay and help bar
    logs.go                 # In-app debug logs viewer
```

## Build Commands

```bash
make build      # Build binary to bin/rootly-tui
make run        # Build and run
make dev        # Run without building (go run)
make test       # Run tests
make lint       # Run golangci-lint
make coverage   # Run tests with coverage report
```

## Architecture

The app follows the Elm Architecture pattern:
- **Model**: Application state in `internal/app/app.go`
- **Update**: Message handling in `Update()` method
- **View**: Rendering in `View()` method

### Screens
1. **Setup Screen**: First-run configuration (API key, endpoint, timezone, language)
2. **Main Screen**: Tabbed view with Incidents and Alerts
3. **Logs Overlay**: Debug log viewer (press `l`)
4. **Help Overlay**: Keyboard shortcuts (press `?`)

### Message Flow
- `IncidentsLoadedMsg` / `AlertsLoadedMsg` - API list responses
- `IncidentDetailLoadedMsg` / `AlertDetailLoadedMsg` - API detail responses
- `APIKeyValidatedMsg` / `ConfigSavedMsg` - Setup flow
- `tea.KeyMsg` - Keyboard input
- `tea.WindowSizeMsg` - Terminal resize

## Config

Config stored at `~/.rootly-tui/config.yaml`:
```yaml
api_key: "your-api-key"
endpoint: "api.rootly.com"
timezone: "America/Los_Angeles"
language: "en_US"
```

## Internationalization (i18n)

The app supports 12 languages via go-i18n:
- English (US/UK), Spanish, French, German, Chinese, Hindi, Arabic, Bengali, Portuguese, Russian, Japanese

Translation files are in `internal/i18n/locales/`. Use `i18n.T("key")` or `i18n.Tf("key", data)` for translations.

## Caching

- **In-memory cache**: Parameter-based keys with 30s TTL
- **Persistent cache**: BoltDB at `~/.rootly-tui/cache.db`
- Press `r` to refresh and clear cache
- Detail views use `updated_at` for cache invalidation

## Key Bindings

- `j/k` or arrows: Navigate list
- `g/G`: Go to first/last item
- `h/l` or `←/→`: Previous/next page
- `Tab`: Switch tabs (Incidents/Alerts)
- `Enter`: Load detailed view for selected item
- `o`: Open item URL in browser
- `r`: Refresh data (clears cache)
- `l`: View debug logs
- `s`: Open setup screen
- `?`: Toggle help overlay
- `q/Esc`: Quit (or return from setup/overlay)

## Debug Mode

Enable debug logging to troubleshoot API issues:

```bash
# Debug to stderr
rootly-tui --debug

# Debug to file
rootly-tui --log debug.log

# Debug to stderr, redirect to file
rootly-tui --debug 2> debug.log
```

Debug logs include:
- API requests (method, URL)
- Response status and body length
- JSON parsing errors with prettified response body
- Configuration details

The debug package (`internal/debug/debug.go`) provides:
- `debug.Logger` - Global structured logger
- `debug.Enable()` - Turn on debug logging
- `debug.PrettyJSON()` - Format JSON for readable output
- `debug.SetLogFile()` - Write logs to file
- Ring buffer captures logs in memory (1000 entries) for in-app viewer

## Releasing New Versions

Releases are automated via GitHub Actions and GoReleaser. Pushing a tag triggers the release workflow.

```bash
# Check current version and recent tags
make version

# Release options (creates tag and pushes to trigger release):
make release-patch   # v0.2.0 -> v0.2.1 (bug fixes)
make release-minor   # v0.2.0 -> v0.3.0 (new features)
make release-major   # v0.2.0 -> v1.0.0 (breaking changes)

# Or do it in two steps:
make bump-minor      # Create tag locally
make push-tag        # Push tag to trigger release
```

The release workflow:
1. Builds binaries for Linux, macOS (Intel + Apple Silicon), and Windows
2. Creates GitHub release with changelog
3. Updates Homebrew tap at `rootlyhq/homebrew-tap`

Monitor releases at: https://github.com/rootlyhq/rootly-tui/actions
