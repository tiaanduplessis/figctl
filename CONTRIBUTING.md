# Contributing to figctl

Thanks for helping. This document covers the development setup, the quality
gate, and the conventions a new command has to follow.

## Setup

figctl needs Go 1.27 and these tools on `PATH`:

```sh
go install golang.org/x/tools/cmd/goimports@v0.50.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0    # for make vuln
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
target on Linux and macOS. Linting is platform-sensitive because the analyser
loads the target platform's standard library, so `make lint-cross` is worth a
run before pushing from macOS. CI also runs `make vuln` (`govulncheck`) and a
documentation drift check.

Other targets:

| Target | What it does |
| --- | --- |
| `make docs` | regenerate `docs/commands.md` from the cobra tree |
| `make gen` | regenerate the skill reference files embedded in the binary |
| `make verify-gen` | fail when those generated files are stale |
| `make vuln` | `govulncheck ./...` |
| `make lint-cross` | the linter as Linux and Windows see it. Analysis loads the standard library of the target platform, so a lint result can differ per platform even when the code does not |
| `make spec-check` | fail when Figma publishes a different OpenAPI spec version than the one pinned in `internal/figma/spec.go` (needs network) |
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
| `internal/imagediff` | local PNG comparison and visual diff output |
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

### The schema contract

`figctl schema <command>` is how an agent learns the field names of a payload,
so a payload that drifts from its schema is a defect. `internal/cli/contract_test.go`
runs each command against the fake server, takes the `data` payload out of the
envelope, and validates it against the schema `figctl schema` prints for that
command, plus the envelope against `figctl schema envelope` and a real error
against the error schema. Adding a command to the contract is one line:

```go
{command: "styles.list", args: []string{"styles", "list", figmatest.FileKey}},
```

Use `outDirToken` where the command needs a writable directory. A failure names
the command, the JSON path, and what the schema expected. When it fires, decide
which side is wrong: a payload that grew a field the schema does not declare is
usually a missing schema type, and a schema that promises a field the payload
omits is usually a wrong struct tag.

Because the schema is reflected from Go types, `omitempty` is what separates
"absent" from "present and null". A pointer field without `omitempty` is
published as nullable, which is what the CLI actually prints.

### The live integration test

`integration/` holds one opt-in test that runs the built binary against the real
Figma API. It exists because the defects that only show up live are invisible to
the fake server: a file Figma refuses to return whole, an optional request that
is slow enough to fail the command around it, an endpoint that needs a different
scope than expected, a 404 that does not mean the file is missing.

Run it with:

```sh
make build
FIGCTL_INTEGRATION=1 FIGMA_TOKEN=... go test -tags integration ./integration/...
```

It drives `bin/figctl` rather than the package APIs, so it tests the shipped
artefact. Run `make build` first, or point `FIGCTL_INTEGRATION_BIN` at another
binary.

It is guarded twice. The `//go:build integration` tag keeps it out of
`go test ./...`, so `make check` cannot compile it, and the test skips with an
explanatory message unless `FIGCTL_INTEGRATION=1` and `FIGMA_TOKEN` are both
set. Add `-v` to see the skip reason and, on a real run, the request accounting.

| Variable | Default | Meaning |
| --- | --- | --- |
| `FIGCTL_INTEGRATION` | unset | must be `1` for the test to run |
| `FIGMA_TOKEN` | unset | the personal access token to run as |
| `FIGCTL_INTEGRATION_FILE` | `fzYhvQpqwhZDUImRz431Qo` | target file key: the public figma-export demo file, readable by any token, with components, styles with real values, and vector icons |
| `FIGCTL_INTEGRATION_NODE` | `54:22` | target node: a component with export settings in that file |
| `FIGCTL_INTEGRATION_BIN` | `bin/figctl` | binary under test |

A run costs about eight Tier 1 requests (get file, get nodes, render images) and
never more than ten, which the test asserts on itself. Tier 1 allows as few as
10 requests per minute, and only 20 per month on view seats, so this is the whole
reason the test is not in `make check` and not on pull requests: running it on
every push would exhaust the rate limit of the token behind the shared file. The
test stays inside the budget by sharing one cache directory across every step
and never passing `--refresh` or `--no-cache`. Keep it that way when you add an
assertion, and put an expensive call before the ones that reuse its result.

CI runs it from `.github/workflows/nightly.yml`, scheduled nightly and available
through `workflow_dispatch`. The workflow needs a `FIGMA_TOKEN` repository
secret; without one it says so and passes, so a fork is never permanently red.
The same job round trips a DTCG export through Style Dictionary v4, which is the
only place Node is used in this repository.

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
6. Add a line to `contractCases` in `internal/cli/contract_test.go` so the
   payload is checked against that schema.
7. Add tests: at minimum one success path, one error path with the expected
   code and exit code, and a request-count assertion if the command should be
   cached.
8. Run `make docs` and `make check`.

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

## Tracking the Figma API

The response types, endpoints, and node properties in `internal/figma` are
written by hand. The plan called for generating them from Figma's OpenAPI
document, but the Go generators handle that 3.1 spec poorly and produce a model
that is harder to work with than the subset figctl needs. The cost of writing
them by hand is that nothing notices when Figma changes the API, so the spec
version is pinned instead:

```go
// internal/figma/spec.go
const SpecVersion = "0.42.0"
```

`figma.SpecVersion` is the `info.version` of the spec the types were written
against. It is a record of what was reviewed, not something the code reads at
runtime.

```sh
make spec-check
```

downloads `openapi.yaml` from [figma/rest-api-spec][spec] and exits non-zero
when `info.version` differs from the pin, naming both versions. It needs
network access, so it is not part of `make check` and no test calls it. Run it
when a Figma response looks wrong, before a release, or on a schedule.

When it reports a new version:

1. Read the [API changelog][changelog] and the [spec releases][releases] for
   everything between the pinned version and the new one.
2. Check each change against `internal/figma`: new or renamed fields on the
   node model in `node.go`, new response fields in `types.go`, changed query
   parameters or paths in `endpoints.go`, and changed scopes or rate limit
   tiers.
3. Apply what figctl needs by hand. Nothing is generated, so a field Figma adds
   stays invisible to figctl until someone adds it, and a field Figma removes
   keeps decoding as a zero value rather than failing.
4. Bump `figma.SpecVersion` in the same change as the code, and add a
   `CHANGELOG.md` entry when the update changes output.

A version bump with no code change is fine when nothing figctl uses moved; say
so in the commit message so the next reader knows the spec was reviewed rather
than skipped.

[spec]: https://github.com/figma/rest-api-spec
[changelog]: https://www.figma.com/developers/api#changelog
[releases]: https://github.com/figma/rest-api-spec/releases

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
archives, the source archive, `checksums.txt` and its cosign signature, and the
Homebrew cask in `tiaanduplessis/homebrew-tap`. Publishing to the tap needs the
`HOMEBREW_TAP_TOKEN` repository secret, because the default token cannot push
across repositories. The npm wrapper is published separately from `npm/` once
the release assets exist, because its `postinstall` downloads them.

`install.sh` needs nothing at release time: it reads the release feed and the
published assets. It is served from `main`, so a change to it takes effect for
everyone immediately and is worth treating with the same care as a release.
