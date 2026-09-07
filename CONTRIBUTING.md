# Contributing to figctl

Thanks for helping. This document covers the development setup, the quality
gate, and the conventions a new command has to follow.

## Setup

figctl needs Go 1.27 and these tools on `PATH`:

```sh
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
go install golang.org/x/vuln/cmd/govulncheck@latest    # for make vuln
export PATH="$HOME/go/bin:$PATH"
```

```sh
git clone https://github.com/tiaanduplessis/figctl
cd figctl
make build      # ./bin/figctl
make check
```

No Figma token is needed to develop or to run the tests. Every test runs against
a fake API server.

## The gate

```sh
make check
```

runs, in order:

| Target | What it does |
| --- | --- |
| `fmt` | `gofmt -l` and `goimports -l -local github.com/tiaanduplessis/figctl` must both be empty |
| `vet` | `go vet ./...` |
| `lint` | `golangci-lint run` with errcheck, govet, staticcheck, revive, gosec, unused, ineffassign, misspell |
| `test` | `go test -race ./...` |
| `build` | `go build` into `bin/figctl` |

`make check` must be green before a pull request is opened, and CI runs the same
target on Linux and macOS plus `make vuln` (`govulncheck`) and a documentation
drift check.

Other targets:

| Target | What it does |
| --- | --- |
| `make docs` | regenerate `docs/commands.md` from the cobra tree |
| `make gen` | regenerate the skill reference files embedded in the binary |
| `make verify-gen` | fail when those generated files are stale |
| `make vuln` | `govulncheck ./...` |
| `make clean` | remove `bin`, `dist`, `coverage.out` |

## Layout

| Package | Responsibility |
| --- | --- |
| `cmd/figctl` | the binary |
| `cmd/gen-docs` | writes `docs/commands.md` |
| `cmd/gen-skill-docs` | writes the generated reference files of the agent skill |
| `internal/cli` | command definitions, one file per noun, flag wiring, output selection |
| `internal/config` | config files, profiles, credential storage |
| `internal/figma` | HTTP client, retries, rate limits, API types |
| `internal/cache` | disk cache and validation |
| `internal/model` | the normalized node model |
| `internal/resolve` | variables, styles, alias chains, colour maths |
| `internal/inspect` | raw node to model conversion, CSS formatting |
| `internal/assets` | asset discovery, rendering, downloading, SVG cleanup |
| `internal/tokens` | DTCG, CSS, and Tailwind exporters |
| `internal/output` | envelope, renderers, JSON Schema |
| `internal/docs` | the command reference generator |
| `internal/skill` | embedded skill templates and the installers |
| `internal/skilldoc` | the agent skill reference generator |
| `internal/ref` | Figma URL and key parsing |
| `internal/figctl` | error codes, typed errors, exit codes |
| `eval` | the agent evaluation harness |

## Testing

### The fake Figma server

`internal/figma/figmatest` is an `httptest` server that speaks the Figma REST
API and serves a synthetic file, the Fixture Design System, from
`internal/figma/figmatest/testdata`. Read
[`internal/figma/figmatest/testdata/README.md`](internal/figma/figmatest/testdata/README.md)
first: it documents the whole fixture tree, every node id, every variable,
style, component, and the exact behaviour of the fake server. Tests reference
those ids directly, so knowing the tree saves a lot of time.

In `internal/cli`, tests use two helpers:

```go
api := setup(t)                                     // isolate + fake server + FIGMA_TOKEN
r := execute(t, "", "file", "tree", figmatest.FileKey)
ok(t, r)
```

`setup` points `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `HOME`, and the working
directory at temporary directories, starts the fake server, and sets
`FIGMA_TOKEN`, so a test never touches the real filesystem, the real keychain,
or the network.

The server can be steered:

| Method | Use |
| --- | --- |
| `Enqueue` | inject a one-shot response (429, 500, 401, 403) ahead of the default |
| `Respond`, `Handle` | override a path permanently |
| `Count`, `Requests` | assert on how many requests a command made, and with what query |

Asserting request counts is how caching is tested. If your command should serve
a second call from the cache, assert it.

### Golden files

Golden outputs live in `internal/cli/testdata`. Compare with `checkGolden`:

```go
r := execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2")
ok(t, r)
checkGolden(t, "node_inspect_2-2.json", r.stdout)
```

Regenerate them after an intentional change:

```sh
go test ./internal/cli -update
git diff internal/cli/testdata
```

Always read the diff. A golden file is the output contract; an unexplained
change in one is a breaking change for every agent parsing it.

If a change to a golden file changes the shape of `data` and not just its
content, it needs a `CHANGELOG.md` entry, and a breaking change needs a
`schemaVersion` bump in `internal/output`.

### Documentation drift

`internal/docs` has a test that regenerates the command reference into a buffer
and compares it with the committed `docs/commands.md`. If you change a command,
a flag, a short description, or an example, run:

```sh
make docs
```

and commit the result. The same test guards the npm wrapper's platform mapping
against the GoReleaser build matrix, so adding a target to `.goreleaser.yml`
means adding it to `npm/postinstall.js` too.

The skill reference files embedded in the binary are generated the same way and
guarded by their own test. Run `make gen` after the same kinds of change, and
`make verify-gen` to check without writing.

## Adding a command

1. Put it in `internal/cli/<noun>.go`. One file per noun; subcommands of the
   same noun share the file.
2. Define the cobra command as a package-level var and register it from an
   `init` function with `rootCmd.AddCommand` or `<noun>Cmd.AddCommand`.
3. Wrap the body in `run(...)` so it receives a `*Context` and so usage errors
   are separated from runtime errors:

   ```go
   func init() {
       node := myCmd.Flags().String("node", "", "node id (1:2, 1-2, or a URL with node-id)")
       myCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
           s, err := ctx.Session()
           if err != nil {
               return err
           }
           ...
           return ctx.Print(data)
       })
       fileCmd.AddCommand(myCmd)
   }
   ```

4. Write `Short`, `Long`, and `Example`. `Short` is one line and is what the
   command table and the generated reference show. `Example` holds one or two
   copy-pasteable commands using `KEY` as the file key. All three end up in
   `docs/commands.md` and in the agent skill, so write them for an agent.
5. Register the data type in the schema registry so `figctl schema <name>`
   works. Look at how a neighbouring command does it.
6. Add tests: at minimum one success path, one error path with the expected
   code and exit code, and a request-count assertion if the command should be
   cached.
7. Run `make docs` and `make check`.

## The output contract a new command must follow

These are not style preferences; agents depend on them.

- **Data on stdout, everything else on stderr.** Progress, warnings, and the
  `--verbose` trace go through `ctx.Printer.Log`. Never print data with `fmt`.
- **Return the envelope, do not build it by hand.** `ctx.Print` and
  `ctx.Envelope` fill in `schemaVersion`, `command`, `profile`, and `file`.
- **Never prompt when stdin or stdout is not a terminal.** A confirmation must
  accept `--yes` and must require it off a terminal.
- **Every error is a `*figctl.Error`** with one of the existing codes, a message
  that says what went wrong, and a hint that says what to do next. The hint is
  the most useful field in the envelope; write it for someone who cannot see
  your screen.
- **Add a code, do not overload one.** If nothing fits, add a code to
  `internal/figctl/errors.go`, map its exit code, and document it in the README.
- **Support the global flags.** List output must honour `--limit` and
  `--cursor` (use `paginate`), set `nextCursor`, and set `truncated` when it
  cuts something short, with a hint saying how to narrow the query.
- **Implement `Tabular`** (`Columns()` and `Rows()`) so `-o table` and
  `-o plain` work, and a markdown renderer where the data warrants one.
- **Partial success is exit 6**, not exit 1. Keep the good results in `data`,
  list the rest in `failures`, and return the `PARTIAL` error.
- **Prefer names over ids.** Colours as hex, sizes in px, layout in CSS terms.
  Include the id as well, never only the id.
- **Be cheap.** Use `Session.File` and the cache rather than a fresh API call.
  If your command has to make a Tier 1 request, say so in the help text.

## Commit messages

Conventional commits. The release notes are generated from them, so the type
prefix matters:

```
feat(tokens): add --mode-strategy select
fix(cache): validate before serving a pinned version
docs: describe the DTCG gradient fallback
```

`feat`, `fix`, and `perf` appear in the changelog; `docs`, `test`, `chore`,
`refactor`, `ci`, and `build` do not.

## Pull requests

- One logical change per pull request.
- `make check` green, and `make docs` run if you touched a command.
- A `CHANGELOG.md` entry under `Unreleased` for anything a user would notice.
- No `TODO` or `FIXME` comments. If something is unfinished, say so in the pull
  request description instead.
- Exported identifiers need doc comments; `revive` enforces it.

## Releasing

Maintainers only.

1. Move the `Unreleased` entries in `CHANGELOG.md` under the new version.
2. Bump `version` in `npm/package.json` to match; the wrapper downloads the
   release matching its own version.
3. Validate the release config: `goreleaser check`, and
   `goreleaser release --snapshot --clean` for a full dry run into `dist/`.
4. Tag `vX.Y.Z` on `main` and push the tag.

The release workflow runs the gate, then GoReleaser, which publishes the
archives, the source archive, `checksums.txt`, the Homebrew cask in
`tiaanduplessis/homebrew-tap`, and the Scoop manifest in
`tiaanduplessis/scoop-bucket`. Cross-repository publishing needs the
`HOMEBREW_TAP_TOKEN` and `SCOOP_BUCKET_TOKEN` repository secrets. A parallel job
builds and pushes the multi-architecture image to `ghcr.io`. The npm wrapper is
published separately from `npm/` once the release assets exist, because its
`postinstall` downloads them.
