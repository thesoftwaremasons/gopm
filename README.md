# gopm

`gopm` is a small, cross-platform process manager written in Go. It runs a local daemon, supervises configured processes, restarts crashed apps, captures logs, watches files during development, exposes status over a CLI and web dashboard, and persists desired process state across daemon restarts.

It is designed to feel familiar if you have used tools like PM2, while staying dependency-light and pure Go.

## Features

- CLI plus long-running daemon (`gopm` and `gopmd`)
- Local newline-delimited JSON IPC over `127.0.0.1:6119`
- Start, stop, restart, delete, list, and shutdown commands
- Auto-restart with max restart limits and backoff
- stdout/stderr capture with in-memory ring buffers
- Optional on-disk logs with rotation
- `logs -f`, `logs --all`, `--since`, and `--grep`
- File watching with rebuild-on-change
- `pre_start` hooks, `.env` files, app groups, and dependencies
- HTTP health checks with `ready` and `unhealthy` statuses
- CPU and memory stats where supported
- `exec` command in an app's configured cwd/env
- Live terminal UI, web dashboard, and Prometheus metrics endpoint
- systemd/Windows service template generation

## Install From Source

Requirements:

- Go 1.22 or newer

Build both binaries:

```sh
go mod tidy
go build -o bin/ ./...
```

This creates:

- `bin/gopm`
- `bin/gopmd`

On Windows, the binaries are `gopm.exe` and `gopmd.exe`.

## Quick Start

The repository includes a working example config:

```sh
gopm validate gopm.yaml
gopm start gopm.yaml
gopm list
gopm logs heartbeat -f
```

The first `gopm` command that needs the daemon will start `gopmd` automatically. The daemon keeps running in the background until stopped:

```sh
gopm shutdown
```

## Configuration

`gopm` reads YAML files shaped like this:

```yaml
apps:
  - name: api
    command: ./bin/api
    args:
      - --port
      - "8080"
    cwd: ./services/api
    env_file: .env
    env:
      LOG_LEVEL: debug
    auto_restart: true
    max_restarts: 10
    log_file: ./logs/api.log
    log_max_size_mb: 10
    log_max_backups: 5
    pre_start: go build ./...
    depends_on:
      - db
    groups:
      - backend
    port: 8080
    watch:
      enabled: true
      dirs:
        - ./
      extensions:
        - .go
      ignore:
        - ./vendor
        - ./.git
      build_cmd: go build -o ./bin/api ./cmd/api
      debounce_ms: 300
    health_check:
      url: http://127.0.0.1:8080/health
      interval: 5s
      timeout: 2s
      retries: 3
    on_crash:
      webhook: https://example.com/gopm-crash
```

Important fields:

| Field | Description |
| --- | --- |
| `name` | Unique app name used by CLI commands. |
| `command` | Executable to start. |
| `args` | Command arguments. |
| `cwd` | Working directory. Relative paths resolve from the config file. |
| `env_file` | Optional dotenv-style file. |
| `env` | Inline environment variables. These override `env_file` values. |
| `auto_restart` | Restart after crashes. |
| `max_restarts` | Maximum restart attempts. `0` means unlimited. |
| `log_file` | Optional append-only log file. |
| `log_max_size_mb` | Rotate log file after this size. |
| `log_max_backups` | Number of rotated log files to keep. |
| `pre_start` | Shell command run once before the app supervision loop. |
| `depends_on` | App names that must be `online` or `ready` before this app starts. |
| `groups` | Labels for `gopm start --group <name>`. |
| `port` | Optional port conflict warning before app start. |
| `watch` | File watcher and rebuild settings. |
| `health_check` | HTTP health polling settings. |
| `on_crash.webhook` | Optional one-shot webhook when restart attempts are exhausted. |

## CLI Reference

```text
gopm [-host addr] <command> [flags]

Commands:
  start <gopm.yaml> [--group <name>]        Load config and start apps
  stop <name>                               Stop a process
  restart <name>                            Restart a process
  delete <name>                             Stop and remove a process
  list [--json] [--names]                   Show all managed processes
  logs <name|--all> [-n N] [-f]             Tail or follow process logs
       [--since <duration|RFC3339>]         Filter logs by time
       [--grep <pattern>]                   Filter logs by substring
  exec <name> -- <cmd> [args...]            Run a command in a process env
  validate <gopm.yaml>                      Validate a config file
  web [--port 6120]                         Start web dashboard
  ui                                        Live-refreshing process table
  completion <bash|zsh|powershell>          Print shell completion
  generate-service [--type systemd|windows] Generate service installer text
  daemon                                    Explicitly launch the daemon
  shutdown                                  Gracefully stop the daemon

Global flags:
  -host <addr>                              Daemon address
```

Examples:

```sh
gopm start gopm.yaml
gopm start gopm.yaml --group backend
gopm list --json
gopm logs api -n 200 --grep error
gopm logs --all -f
gopm exec api -- env
gopm web --port 6120
gopm generate-service --type systemd
```

## Web Dashboard And Metrics

Start the dashboard:

```sh
gopm web
```

Then open:

- `http://127.0.0.1:6120`
- `http://127.0.0.1:6120/metrics`

The dashboard runs in the CLI process and talks to the daemon through the same local IPC protocol as the CLI.

## Persistence

The daemon stores desired process state at:

```text
~/.gopm/state.json
```

On daemon restart, persisted app configs are restored and supervised again. Runtime-only values such as PID and current status are not trusted across restarts.

## Architecture

See [docs/SOLUTION_ARCHITECTURE.md](docs/SOLUTION_ARCHITECTURE.md) for the full onboarding architecture guide.

## Development

```sh
go test ./...
go vet ./...
go build ./...
```

Run the example:

```sh
go build -o bin/ ./...
bin/gopm start gopm.yaml
```

## Security

`gopmd` is designed for local loopback control. Do not expose the daemon IPC port to untrusted networks.

See [SECURITY.md](SECURITY.md) for reporting guidance.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).
