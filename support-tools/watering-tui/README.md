# HerbHub365 Watering TUI

A terminal UI and command-line client for controlling the HerbHub365 watering relay API. It can display relay status, toggle channels, run timed pulses and execute administrative actions.

The Go module is published under:

```text
github.com/barbera01/HerbHub365/wateringtui
```

## Requirements

- Go 1.25 or later
- Network access to the watering API
- A terminal with ANSI colour support for the interactive UI

## Build

From this directory:

```bash
go mod download
go build -o watering ./cmd
```

## Configuration

The application reads `config.json` from the current working directory. Create it from the provided example:

```bash
cp config.json.example config.json
```

Example configuration:

```json
{
  "default": {
    "host": "http://hh-02:8181",
    "pollInterval": 2
  },
  "channels": {
    "enabled": true,
    "entries": [
      { "name": "BASIL", "pin": 1 },
      { "name": "CHILLI", "pin": 2 },
      { "name": "OREGANO", "pin": 3 }
    ]
  },
  "add": {
    "enabled": true
  },
  "admin": {
    "enabled": true,
    "actions": ["reset_all", "ping", "version"]
  }
}
```

If the file is missing or invalid, the application uses its built-in defaults: `http://hh-02:8181`, a two-second polling interval, and the three channels shown above.

The API host can be overridden with `WATERING_HOST` or `-host`. Precedence is:

```text
-host flag > WATERING_HOST > config.json > built-in default
```

## Interactive TUI

Start the application without a command:

```bash
./watering
```

Or run it directly from source:

```bash
go run ./cmd
```

### Keyboard controls

| Key | Action |
| --- | --- |
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` / `Space` | Toggle the selected channel |
| `p` | Pulse the selected channel |
| `a` | Open the Add Channel screen |
| `A` | Open the Admin screen |
| `r` | Refresh channel status |
| `Esc` / `Back` | Return to the channel screen |
| `q` / `Ctrl+C` | Quit |

## Command-line usage

```text
watering [-host URL] <command>
```

| Command | Description |
| --- | --- |
| `status` | Show relay states |
| `on <channel>` | Switch a channel on |
| `off <channel>` | Switch a channel off |
| `pulse <channel> <seconds>` | Pulse one channel |
| `combo <ch1,ch2,...> <seconds>` | Pulse multiple channels |
| `cycle <totalSeconds> <onSeconds>` | Rotate channels for a total duration |
| `all on\|off\|reset` | Control or reset all channels |

Examples:

```bash
./watering status
./watering on BASIL
./watering off BASIL
./watering pulse CHILLI 10
./watering combo BASIL,OREGANO 15
./watering cycle 120 10
./watering all off
./watering -host http://localhost:8181 status
```

CLI responses are printed as formatted JSON when the API returns valid JSON.

## Expected API endpoints

The client uses the following watering API endpoints:

```text
GET  /status
POST /relay/{channel}/on
POST /relay/{channel}/off
POST /relay/{channel}/pulse?seconds={seconds}
POST /pulsecombo?channels={comma-separated-channels}&seconds={seconds}
POST /cycle?total={seconds}&on={seconds}
POST /all/{on|off|reset}
POST /admin/ping
POST /admin/version
```

## Project structure

```text
watering-tui/
├── cmd/                 # CLI entry point
├── internal/config/     # Configuration types and loading
├── internal/tui/        # Bubble Tea terminal interface
├── config.json.example  # Example runtime configuration
├── go.mod
└── go.sum
```

## Development

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
```

The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).
