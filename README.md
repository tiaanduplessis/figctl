# figctl

One static Go binary that reads a Figma file and hands a coding agent everything it needs to implement the design.

[![ci](https://github.com/tiaanduplessis/figctl/actions/workflows/ci.yml/badge.svg)](https://github.com/tiaanduplessis/figctl/actions/workflows/ci.yml)
[![go report card](https://goreportcard.com/badge/github.com/tiaanduplessis/figctl)](https://goreportcard.com/report/github.com/tiaanduplessis/figctl)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Quickstart

```sh
brew install tiaanduplessis/tap/figctl     # or: go install github.com/tiaanduplessis/figctl/cmd/figctl@latest
figctl auth login personal                 # paste a Figma personal access token
figctl file tree "https://www.figma.com/design/KEY/Web-App"
```

`file tree` prints the outline of the file. Pick a node id from it and ask for
everything needed to build that node:

```sh
figctl node context KEY --node 2:2
```

Then teach your agent to do the same:

```sh
figctl skill install --agent claude
```

## Why this exists

An agent that can run a shell can implement a Figma design end to end if it can
answer six questions cheaply: what is in this file, what does this node look
like, what are the exact layout and style values, which design token does each
value come from, where are the assets, and what did the designer say about it.
figctl answers all six from the public Figma REST API and prints JSON an agent
can parse.

### CLI or MCP

Figma ships a Dev Mode MCP server, and it is the closest comparison. The
trade-off is real in both directions.

| | figctl | Dev Mode MCP server |
| --- | --- | --- |
| Transport | a binary the agent runs in a shell | a server process the agent connects to |
| Per-turn cost | none; the agent reads `--help` or the skill file when it needs to | tool schemas occupy the context window every turn |
| Selection awareness | node ids passed explicitly | reads the current selection in the Figma desktop app |
| Code generation | none; the agent writes the code with full repo context | generates React and Tailwind from the design |
| Setup | one binary, one token | Figma desktop app, Dev Mode, a running server |
| Works in CI | yes | no |

The CLI gives up bidirectional communication with the Figma app. In exchange
there is no server process, no per-turn tool schema tax, and the same commands
run in CI, in a Dockerfile, and over SSH.

The binary is called `figctl` because `figma` is already taken by Figma's own
Code Connect CLI, which is widely installed in design-system repositories.

## The agent workflow

Five steps, each a real command.

### 1. Outline the file

```
$ figctl file tree KEY -o md
# file.tree

Profile: env

File: Fixture Design System (FixTuReDeSiGnSySt3m01), version 2100123456

- 0:1 CANVAS "Screens" (1 children)
  - 2:1 SECTION "Onboarding" 470x964 @-40,-80 (1 children) [dev=READY_FOR_DEV]
- 1:2 CANVAS "Design System" (3 children)
  - 3:10 COMPONENT_SET "Button" 368x132 @0,0 (4 children) [component]
  - 3:20 COMPONENT "Input/Text" 342x56 @0,160 (3 children) [component, layout=column]
  - 4:0 FRAME "Styles" 520x168 @0,260 (4 children) [layout=row]

Hints:
- Next: figctl file tree KEY --node <id> --depth 3 to go deeper, or figctl node context KEY --node <id> to implement a node.
```

Default depth is 2, so one call shows the pages and their top-level frames.
Narrow with `--node`, `--page`, `--depth`, `--type`, `--name`, `--visible-only`.
Search the whole document with `figctl file find KEY --name "Login*"` or
`--text "sign in"`.

### 2. Get the context for one node

```sh
figctl node context KEY --node 2:2
```

One call returns the normalized model, a PNG screenshot on disk, SVG exports of
the icon-like layers, the raster image fills, only the tokens the subtree uses,
the component and variant definitions of every instance, the comments pinned on
the node, and the dev resource links. Abbreviated `-o md` output:

````
## Node: Login (2:2, FRAME)

- page: Screens
- path: Screens / Onboarding / Login
- size: 390 x 844

#### 2:2 Login (FRAME)

- layout: column; gap 16px [space/4]; padding 24px; justify flex-start; align flex-start; w fixed 390px; h fixed 844px; clips
- fills: #ffffff [bg/surface]
- effects: box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1) {shadow/md}
- tokens: fills[0]=bg/surface, gap=space/4
- styles: effect=shadow/md, grid=grid/12

```css
align-items: flex-start;
background: var(--bg-surface);
box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1);
display: flex;
flex-direction: column;
gap: var(--space-4);
height: 844px;
padding: 24px;
width: 390px;
```
````

Square brackets are variables, braces are styles. A value with `"token": null`
in JSON output is not bound to anything in Figma, so the agent knows it has to
hardcode it or ask.

Trim the payload with `--depth`, `--max-nodes`, `--no-assets`, `--no-comments`,
`--no-screenshot`, `--no-css`.

### 3. Implement the node

The agent writes the code. figctl deliberately does not generate framework code:
the agent knows the repository's conventions and figctl does not.

### 4. Compare the result

```sh
figctl render KEY --node 2:2 --scale 2 --out ./design
```

Renders to PNG, JPG, SVG, or PDF. The manifest lists absolute paths, so the
screenshot can go straight into a vision tool next to a screenshot of the built
component.

### 5. Wire up the design tokens

```sh
figctl tokens export KEY --format css --out ./src/styles
```

```css
/* Design tokens from Fixture Design System (version 2100123456), generated by figctl. Do not edit by hand. */

:root {
  --color-brand-500: #3366ff;
  --font-sans: Inter;
  --neutral-0: #ffffff;
  --neutral-900: #111827;
  --space-4: 16px;
  --bg-surface: var(--neutral-0);
  --text-primary: var(--neutral-900);
  --shadow-md: 0px 4px 12px rgba(0, 0, 0, 0.1);
  --heading-lg-font-size: 28px;
}

[data-theme="dark"] {
  --bg-surface: #111827;
  --text-primary: #ffffff;
}
```

Also `--format dtcg` (the default, DTCG 2025.10), `--format style-dictionary`
(the same bytes), `--format tailwind`, and `--format json`. See
[docs/design-tokens.md](docs/design-tokens.md).

## Multiple accounts

A consultant has one Figma account per client, often on different plans. A
profile is a name plus a token, an optional default team, and an optional API
base URL. Tokens live in the OS keychain; the config file holds no secrets.

```sh
figctl profile add acme --team 555000111 --default
figctl profile add globex
figctl profile list
```

In each client repository, commit a `.figctl.yaml` naming the profile:

```sh
cd ~/work/acme-web
figctl init --profile acme --file https://www.figma.com/design/KEY/Web-App
```

```yaml
# .figctl.yaml
profile: acme
file: KEY
```

Commands walk up from the working directory to find it, so an agent working in
`~/work/acme-web` uses the Acme token without being told, and an agent in
`~/work/globex-app` cannot reach Acme's files. Every JSON envelope carries the
profile it ran as, so the agent can verify the account before doing work.

The active profile is chosen by the first of these that matches:

1. `--profile NAME`
2. `FIGCTL_PROFILE`
3. `FIGMA_TOKEN` (an implicit profile named `env`, for CI)
4. `profile:` in the nearest `.figctl.yaml`
5. the user-level default profile

`figctl auth status` reports which one was picked and why. The cache is keyed by
profile, so one client's data never mixes with another's. See
[docs/profiles.md](docs/profiles.md).

## Command surface

Full generated reference: [docs/commands.md](docs/commands.md). Or run
`figctl <command> --help`.

| Command | What it does |
| --- | --- |
| `file info\|tree\|find\|get` | file metadata, sparse outline, search, raw Figma JSON |
| `node inspect\|context` | normalized model of nodes; everything needed to implement one node |
| `render` | nodes to PNG, JPG, SVG, or PDF files |
| `assets list\|export` | icons, export-marked layers, and raster image fills |
| `tokens resolve\|export` | one variable or style across modes; DTCG, CSS, Tailwind, JSON |
| `variables list\|get` | raw variables and collections per mode (Enterprise) |
| `styles list\|get` | styles with resolved values, for files and team libraries |
| `components list\|get` | components, sets, and variant property definitions |
| `comments list\|add` | designer intent; `add` is the only write |
| `versions list` | saved file versions, for use with `--file-version` |
| `devresources list` | Dev Mode resource links attached to nodes |
| `projects list\|files` | discovery through the v1 projects API |
| `folders list\|files` | discovery through the v2 folders API |
| `profile add\|list\|use\|show\|remove` | named accounts |
| `auth login\|logout\|status\|scopes` | tokens and which scopes to request |
| `skill install\|print\|uninstall` | write the agent skill files for Claude Code, Cursor, Copilot, or Codex |

The skill is also published at `skills/figctl/`, so it can be installed
without figctl: `npx skills add tiaanduplessis/figctl`. See
[docs/agents.md](docs/agents.md).
| `cache status\|clear` | inspect and clear the on-disk cache |
| `me` | the user the active token belongs to |
| `init` | write `.figctl.yaml` in the current repository |
| `schema` | JSON Schema of a command's output |
| `version` | build metadata |
| `completion` | shell completion for bash, zsh, fish, or powershell |

`<ref>` is a file key or any Figma URL. A `node-id=1-2` in the URL is converted
to `1:2` and used as the default `--node`, so an agent can paste the URL from
the browser unchanged.

## Output contract

Output is JSON when stdout is not a terminal (which is what an agent gets) and a
table when it is. `--json` forces JSON, `-o md` produces markdown for agents that
read tool output as text, `-o plain` produces tab-separated rows.

Every success looks like this:

```json
{
  "schemaVersion": 1,
  "command": "file.tree",
  "profile": {"name": "acme", "handle": "Jane"},
  "file": {"key": "KEY", "name": "Web App", "version": "2100123456", "lastModified": "2026-09-01T10:15:00Z"},
  "data": [],
  "truncated": false,
  "nextCursor": null,
  "hints": ["Next: figctl node context KEY --node <id> to implement a node."]
}
```

Every failure looks like this, on stdout, in JSON mode:

```json
{
  "schemaVersion": 1,
  "error": {
    "code": "NOT_FOUND",
    "message": "file NOSUCHFILEKEY000000001 not found",
    "hint": "Check the key or id and that the active profile has access.",
    "httpStatus": 404
  }
}
```

Data goes to stdout; progress, warnings, and the `--verbose` HTTP trace go to
stderr. `schemaVersion` only changes on a breaking change; additions within a
version are additive. `figctl schema <command>` prints the JSON Schema of any
command's `data`, so field names never have to be guessed.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | runtime or API error |
| 2 | usage error |
| 3 | auth error |
| 4 | not found |
| 5 | rate limited |
| 6 | partial success: `data` is usable, `failures` lists what did not work |

### Error codes

| Code | Exit | Meaning |
| --- | --- | --- |
| `USAGE` | 2 | bad flags or arguments |
| `AUTH_MISSING` | 3 | no profile or token selected |
| `AUTH_INVALID` | 3 | the token was rejected |
| `AUTH_SCOPE` | 3 | the token lacks a scope; the message names it |
| `NOT_FOUND` | 4 | file, node, page, or style does not exist |
| `FORBIDDEN` | 1 | the account cannot see this resource |
| `RATE_LIMITED` | 5 | carries `retryAfterSeconds` |
| `PLAN_REQUIRED` | 1 | variables need Enterprise; the hint names the styles fallback |
| `RENDER_FAILED` | 1 | Figma could not render any requested node |
| `PARTIAL` | 6 | some items succeeded, some failed |
| `NETWORK` | 1 | connection or timeout |
| `INTERNAL` | 1 | a bug in figctl |

## Caching and rate limits

Figma's rate limits (in force since November 2025) are tiered per endpoint, and
the tier figctl needs most is the tightest one:

| Tier | Endpoints | Limit |
| --- | --- | --- |
| 1 | get file, get nodes, render images | 10 to 30 per minute on Dev and Full seats depending on plan; 20 per **month** on View and Collab seats |
| 2 | variables, versions, dev resources, comments, image fills, folders | 25 to 150 per minute |
| 3 | components, styles, file meta, users | 50 to 200 per minute |

Ten Tier 1 requests a minute is not enough to walk a file node by node, so
figctl fetches the whole file document once per version, caches it under
`$XDG_CACHE_HOME/figctl/<profile>/<fileKey>/`, and answers `file tree`,
`file find`, `file get`, `node inspect`, and `node context` from the local copy.
Freshness is checked against `last_touched_at` from the Tier 3 file meta
endpoint, at most once a minute.

Files above `--max-file-mb` (200 by default) are not cached whole; those calls
fall back to `depth=2` plus per-node requests, and the hint says so.

`--refresh` re-fetches and rewrites the cache, `--no-cache` bypasses it in both
directions, `--file-version` pins a version so the cache never has to be
validated. A 429 is retried up to three times honouring `Retry-After`, and if it
still fails the error envelope carries `retryAfterSeconds`. See
[docs/caching.md](docs/caching.md).

## Tokens and scopes

figctl uses a Figma personal access token, sent in the `X-Figma-Token` header.
Create one in Figma under Settings, Security, Personal access tokens. Scopes are
chosen when the token is created and cannot be changed afterwards, and tokens
expire after at most 90 days.

Run `figctl auth scopes` for the current list. As of this release:

| Scope | Required | Used by |
| --- | --- | --- |
| `file_content:read` | yes | `file info/tree/find/get`, `node inspect/context`, `render`, `assets export` |
| `file_metadata:read` | yes | `file info`, cache validation |
| `library_content:read` | yes | `components list`, `styles list` for a file |
| `library_assets:read` | no | `components get --key`, `styles get --key` |
| `current_user:read` | yes | `me`, `auth status` |
| `team_library_content:read` | no | `components list`, `styles list` with `--team` |
| `file_variables:read` | no | `variables`, `tokens export/resolve` (Enterprise only) |
| `file_dev_resources:read` | no | `devresources list`, `node context` |
| `file_dev_resources:write` | no | `devresources add/update/remove` |
| `file_comments:read` | no | `comments list`, `node context` |
| `file_comments:write` | no | `comments add` only |
| `file_versions:read` | no | `versions list`, `--file-version` |
| `projects:read` | no | `projects list` |
| `folders:read` | no | `folders list` |

The old blanket `files:read` scope is deprecated; request the granular scopes
instead.

Record the expiry when you log in so figctl can warn you before it lapses; the
API does not expose it:

```sh
figctl auth login --profile acme --expires 2026-12-01
```

Variables need an Enterprise plan and a full seat. On any other plan
`figctl variables list` fails with `PLAN_REQUIRED`, and `tokens export` still
exports the styles and says so in `hints`.

## Installation

| Channel | Command |
| --- | --- |
| Install script | `curl -fsSL https://raw.githubusercontent.com/tiaanduplessis/figctl/main/install.sh \| sh` |
| npm | `npm i -D figctl` then `npx figctl`, or `npm i -g figctl` |
| Homebrew (macOS) | `brew install tiaanduplessis/tap/figctl` |
| Go | `go install github.com/tiaanduplessis/figctl/cmd/figctl@latest` |

The install script covers macOS and Linux on amd64 and arm64, needs no
toolchain, and always verifies the download against the release checksums. It
installs into `/usr/local/bin` when that is writable and `~/.local/bin`
otherwise. It never asks for a password by itself. Pin a version in CI so a
build cannot change under you:

```sh
curl -fsSL https://raw.githubusercontent.com/tiaanduplessis/figctl/main/install.sh | FIGCTL_VERSION=v0.1.0 sh
```

`FIGCTL_INSTALL_DIR` overrides the destination. When [cosign](https://docs.sigstore.dev/)
is on `PATH` the script also verifies the signature on the checksum file.

npm is worth preferring inside a project. `npm i -D figctl` pins the version in
`package.json`, so a repository gets a known figctl rather than whatever the
machine happens to have, which matters when an agent depends on the output
contract. Windows is served by npm or a release archive.

Homebrew ships figctl as a cask, which is macOS only. On Linux use the install
script, npm, or `go install`.

Every release signs `checksums.txt` with cosign keylessly, so a download can be
traced back to the workflow that built it:

```sh
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/tiaanduplessis/figctl/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c checksums.txt --ignore-missing
```

Shell completion:

```sh
figctl completion zsh > "${fpath[1]}/_figctl"
```

## Documentation

- [docs/commands.md](docs/commands.md) generated reference for every command and flag
- [docs/agents.md](docs/agents.md) wiring figctl into Claude Code, Cursor, Copilot, and Codex
- [docs/design-tokens.md](docs/design-tokens.md) the DTCG mapping, modes, CSS and Tailwind output
- [docs/caching.md](docs/caching.md) how the cache works and why it matters
- [docs/profiles.md](docs/profiles.md) profiles, the keychain, and the client-repo pattern
- [CONTRIBUTING.md](CONTRIBUTING.md) development setup and how to add a command
- [SECURITY.md](SECURITY.md) token handling and how to report a vulnerability
- [CHANGELOG.md](CHANGELOG.md)

## Contributing

```sh
git clone https://github.com/tiaanduplessis/figctl
cd figctl
make check
```

`make check` runs formatting, `go vet`, golangci-lint, the race-enabled test
suite, and a build. It is the gate for every change. Read
[CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

## License

MIT. Copyright Tiaan du Plessis. See [LICENSE](LICENSE).
