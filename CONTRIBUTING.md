# Contributing to adit-code

## Development Setup

```bash
git clone https://github.com/justinstimatze/adit-code.git
cd adit-code
go build ./cmd/adit
go test ./... -race
```

Requires Go 1.25+ (for modelcontextprotocol/go-sdk).

## Running Checks

```bash
go test ./... -race -count=1                     # Tests
go vet ./...                                     # Vet
golangci-lint run ./cmd/adit/... ./internal/...  # Lint
./adit enforce cmd/adit/ internal/               # Self-enforce (must pass)
./adit score --pretty cmd/ internal/             # Self-test
```

All five must pass before submitting a PR. We squash-merge all PRs.

## Adding a Language Frontend

1. Create `internal/lang/newlang.go` implementing the `Frontend` interface
2. Register in `cmd/adit/main.go` and `internal/mcp/server.go`
3. Add fixtures in `testdata/` and tests in `internal/lang/`

## Adding a Metric

1. Add scoring function in `internal/score/`
2. Add fields to types in `internal/score/types.go`
3. Wire into `pipeline.go`, `internal/output/`, and `internal/mcp/server.go`

See [CLAUDE.md](CLAUDE.md) for detailed architecture and conventions.

## Conventions

- All dependencies must be MIT or BSD-2 licensed
- JSON output is the default; `--pretty` is opt-in
- No composite scores — each metric stands alone
- `internal/score/` must not import tree-sitter (language-agnostic)

## License

By contributing, you agree that your contributions will be licensed under MIT.
