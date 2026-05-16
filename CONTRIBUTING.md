# Contributing

Thanks for helping improve gopm.

## Development Setup

Requirements:

- Go 1.22 or newer

Useful commands:

```sh
go mod tidy
go test ./...
go vet ./...
go build ./...
```

## Pull Requests

Before opening a pull request:

1. Run `gofmt` on changed Go files.
2. Run `go test ./...`.
3. Run `go vet ./...`.
4. Update README or `docs/SOLUTION_ARCHITECTURE.md` when behavior changes.
5. Add or update tests for user-visible behavior and concurrency-sensitive code.

## Code Style

- Keep packages focused and small.
- Keep OS-specific behavior behind build-tagged files.
- Prefer standard library APIs unless a dependency meaningfully reduces complexity.
- Avoid adding daemon responsibilities to the CLI; the daemon should remain the owner of process state.

