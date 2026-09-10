# Agent instructions

figctl is a Go CLI. Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing
commands, output schemas, generated documentation, or release configuration.

## Validation

Run the project checks that match the change:

```sh
go test -race ./...
go vet ./...
go build -trimpath -o bin/figctl ./cmd/figctl
```

Install the pinned tools before running the complete gate:

```sh
go install golang.org/x/tools/cmd/goimports@v0.50.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
make check
```

Run `make docs` after changing a command, flag, or example. Run `make gen` after
changing the embedded agent reference. Use `make verify-gen` to check generated
files without rewriting them.

Tests use a fake Figma server and do not need a token. Never place a real token
in source, test data, logs, or command arguments. The opt-in live test needs
`FIGCTL_INTEGRATION=1` and `FIGMA_TOKEN`; read its setup in
`CONTRIBUTING.md` before running it.
