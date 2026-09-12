# figctl

One static binary that reads a Figma file and hands a coding agent everything it needs to implement the design.

[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/tiaanduplessis/figctl/blob/main/LICENSE)
[![repository](https://img.shields.io/badge/source-github-181717.svg)](https://github.com/tiaanduplessis/figctl)

Create a Figma personal access token with `current_user:read`,
`file_content:read`, `file_metadata:read`, and `library_content:read`.
See [Tokens and scopes](#tokens-and-scopes) for optional features.

```sh
npm install --save-dev figctl
npx figctl version
npx figctl auth login personal --default
npx figctl file tree "https://www.figma.com/design/KEY/Web-App"
```

Paste the token into the hidden login prompt. Replace the URL with a file your
account can access. `KEY` is the segment after `/design/` or `/file/`.
The tree prints node ids and names; choose a frame id and replace `2:2`:

```sh
npx figctl node context KEY --node 2:2
```

The result includes layout and style data and downloaded asset paths.
Read any `hints` for optional data unavailable with your token or plan.

figctl is a Go binary. This package downloads the release build for your
platform and runs it through a shim, so there is no toolchain to install, no
native module to compile, and no native runtime dependency. The npm shim requires Node.js 18 or later.

Installing it as a dev dependency pins the version in `package.json`, so every
machine and CI job in a repository gets the same figctl. That matters more than
usual here: an agent depends on the shape of the output, so a tool that silently
changes underneath it changes the agent's behaviour.

## Contents

- [Why this exists](#why-this-exists)
- [The workflow](#the-workflow)
- [Teaching your agent to use it](#teaching-your-agent-to-use-it)
- [Command surface](#command-surface)
- [Output contract](#output-contract)
- [Exit codes and error codes](#exit-codes-and-error-codes)
- [Tokens and scopes](#tokens-and-scopes)
- [Working across several Figma accounts](#working-across-several-figma-accounts)
- [Caching and rate limits](#caching-and-rate-limits)
- [What this package does on install](#what-this-package-does-on-install)
- [Other ways to install](#other-ways-to-install)
- [Documentation](#documentation)

## Why this exists

An agent that can run a shell can implement a Figma design end to end if it can
answer six questions cheaply: what is in this file, what does this node look
like, what are the exact layout and style values, which design token does each
value come from, where are the assets, and what did the designer say about it.
figctl answers all six from the public Figma REST API and prints JSON an agent
can parse.

### CLI or MCP

figctl exposes explicit shell commands and JSON output for scripts and coding
agents. It uses a personal access token and can run without the Figma desktop
app, including in CI when credentials and network access are available.

[Figma's MCP server](https://developers.figma.com/docs/figma-mcp-server/) connects
supported agents to design context, Code Connect, and canvas-writing tools.
Figma recommends its hosted remote server, which also needs no desktop app;
a desktop server is available for local workflows.

Choose figctl when you want commands you can inspect, pipe, cache, and pin to a
version. Choose Figma MCP when you want its native agent integration and Figma
features. Authentication, available tools, and client support differ; consult
Figma's documentation for current MCP requirements.

## The workflow

### 1. Outline the file

```sh
npx figctl file tree KEY -o md
```

```
- 0:1 CANVAS "Screens" (1 children)
  - 2:1 SECTION "Onboarding" 470x964 @-40,-80 (1 children) [dev=READY_FOR_DEV]
- 1:2 CANVAS "Design System" (3 children)
  - 3:10 COMPONENT_SET "Button" 368x132 @0,0 (4 children) [component]
  - 3:20 COMPONENT "Input/Text" 342x56 @0,160 (3 children) [component, layout=column]
```

Default depth is 2, so one call shows the pages and their top-level frames.
Narrow with `--node`, `--page`, `--depth`, `--type`, `--name`, `--visible-only`.
Search the whole document with `figctl file find KEY --name "Login*"` or
`--text "sign in"`.

### 2. Get everything needed for one node

```sh
npx figctl node context KEY --node 2:2
```

One call returns the normalized model, a PNG screenshot written to disk, SVG
exports of the icon-like layers, the raster image fills, only the design tokens
that subtree actually uses, the component and variant definitions of every
instance, the distances the designer pinned in Dev Mode, the comments pinned on
the node, and the Dev Mode resource links.

````
## Node: Login (2:2, FRAME)

- page: Screens
- path: Screens / Onboarding / Login
- size: 390 x 844

#### 2:2 Login (FRAME)

- layout: column; gap 16px [space/4]; padding 24px; w fixed 390px; h fixed 844px; clips
- fills: #ffffff [bg/surface]
- effects: box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1) {shadow/md}

```css
background: var(--bg-surface);
box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1);
display: flex;
flex-direction: column;
gap: var(--space-4);
padding: 24px;
```
````

Square brackets are variables, braces are styles. The `token` field has three
states: a `name` means it is resolved, a bare `variableId` with
`"unresolved": true` means a variable governs the value but naming it needs an
Enterprise plan, and `null` means the designer typed it by hand. Only the last
is safe to hardcode.

Trim the payload with `--depth`, `--max-nodes`, `--no-assets`, `--no-comments`,
`--no-screenshot`, `--no-css`.

### 3. Implement it

The agent writes the code. figctl deliberately does not generate framework
code: the agent knows the repository's conventions and figctl does not.

### 4. Compare the result

```sh
npx figctl render KEY --node 2:2 --scale 2 --out ./design
```

Renders to PNG, JPG, SVG, or PDF and prints absolute paths, so the image can go
straight into a vision tool beside a screenshot of the built component.

### 5. Wire up the design system once

```sh
npx figctl tokens export KEY --format css --out ./src/styles
```

```css
:root {
  --color-brand-500: #3366ff;
  --space-4: 16px;
  --bg-surface: var(--neutral-0);
  --shadow-md: 0px 4px 12px rgba(0, 0, 0, 0.1);
}

[data-theme="dark"] {
  --bg-surface: #111827;
}
```

Also `--format dtcg` (the default, the W3C Design Tokens Format Module 2025.10),
`--format style-dictionary` (the same bytes, consumed by Style Dictionary v4),
`--format tailwind`, and `--format json`. Variables resolve across every mode,
so light and dark come out of one export.

## Teaching your agent to use it

figctl ships its own instructions, so an agent learns the workflow without you
writing a prompt:

```sh
npx figctl skill install --agent claude    # or: agents, cursor, copilot, codex, all
```

That writes a `SKILL.md` plus a generated command and output-schema reference
into `.agents/skills/figctl/`, the location Codex and other clients read, and
links `.claude/skills/figctl` to it. Add `--project` to install into the
repository instead of your home directory, and `--dry-run` to see what would be
written first.

The skill is also published in the repository, so it can be installed without
figctl being present yet:

```sh
npx skills add tiaanduplessis/figctl
```

## Command surface

Run `npx figctl <command> --help`, or read the
[generated reference](https://github.com/tiaanduplessis/figctl/blob/main/docs/commands.md).

| Command | What it does |
| --- | --- |
| `file info`, `tree`, `find`, `get` | file metadata, sparse outline, search, raw Figma JSON |
| `node inspect`, `context` | normalized model of nodes; everything needed to implement one node |
| `render` | nodes to PNG, JPG, SVG, or PDF files |
| `assets list`, `export` | icons, export-marked layers, and raster image fills |
| `tokens resolve`, `export` | one variable or style across modes; DTCG, CSS, Tailwind, JSON |
| `variables list`, `get` | raw variables and collections per mode (Enterprise) |
| `styles list`, `get` | styles with resolved values, for files and team libraries |
| `components list`, `get` | components, sets, and variant property definitions |
| `comments list`, `add` | designer intent, and leaving implementation notes |
| `devresources list`, `add`, `update`, `remove` | Dev Mode links, for pointing a component at its Storybook story |
| `versions list` | saved file versions, for use with `--file-version` |
| `projects`, `folders` | discovery through the projects and folders APIs |
| `profile add`, `list`, `use`, `show`, `remove` | named accounts |
| `auth login`, `logout`, `status`, `scopes` | tokens and which scopes to request |
| `skill install`, `print`, `uninstall` | write the agent skill files |
| `cache status`, `clear` | inspect and clear the on-disk cache |
| `me`, `init`, `schema`, `version`, `completion` | account, project setup, output schemas, build metadata, shell completion |

`<ref>` is a file key or any Figma URL. A `node-id=1-2` in the URL is converted
to `1:2` and used as the default `--node`, so an agent can paste the URL from
the browser unchanged.

## Output contract

Output is JSON when stdout is not a terminal, which is what an agent gets, and a
table when it is. `--json` forces JSON, `-o md` produces markdown for agents that
read tool output as text, `-o plain` produces tab-separated rows.

Every success is one envelope:

```json
{
  "schemaVersion": 1,
  "command": "file.tree",
  "profile": {"name": "acme", "handle": "Jane"},
  "file": {"key": "KEY", "name": "Web App", "version": "2100123456"},
  "data": [],
  "truncated": false,
  "nextCursor": null,
  "hints": ["Next: figctl node context KEY --node <id> to implement a node."]
}
```

Every failure is one envelope too, on stdout in JSON mode:

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
stderr. Read `hints` on every call: they say what was degraded, what was
truncated, and what to run next. `schemaVersion` changes only on a breaking
change. `npx figctl schema <command>` prints the JSON Schema of any command's
payload, so field names never have to be guessed, and a test in the repository
fails if a payload ever drifts from the schema it advertises.

## Exit codes and error codes

| Exit | Meaning |
| --- | --- |
| 0 | success |
| 1 | runtime or API error |
| 2 | usage error |
| 3 | auth error |
| 4 | not found |
| 5 | rate limited |
| 6 | partial success: `data` is usable, `failures` lists what did not work |

Exit 6 is not a failure to retry. The good results are already on stdout.

| Error code | Exit | Meaning |
| --- | --- | --- |
| `USAGE` | 2 | bad flags or arguments |
| `AUTH_MISSING` | 3 | no profile or token selected |
| `AUTH_INVALID` | 3 | the token was rejected or has expired |
| `AUTH_SCOPE` | 3 | the token lacks a scope; the message names it |
| `NOT_FOUND` | 4 | file, node, page, or style does not exist |
| `FORBIDDEN` | 1 | the account cannot see this resource |
| `RATE_LIMITED` | 5 | carries `retryAfterSeconds` |
| `PLAN_REQUIRED` | 1 | variables need Enterprise; the hint names the styles fallback |
| `RENDER_FAILED` | 1 | Figma could not render any requested node |
| `PARTIAL` | 6 | some items succeeded, some failed |
| `NETWORK` | 1 | connection or timeout |
| `INTERNAL` | 1 | a bug in figctl |

## Tokens and scopes

figctl uses a Figma personal access token, sent in the `X-Figma-Token` header.
Create one in Figma under Settings, Security, Personal access tokens. Scopes are
chosen when the token is created and cannot be changed afterwards, and tokens
expire after at most 90 days.

Run `npx figctl auth scopes` for the current list. The ones marked required
cover the core workflow:

| Scope | Required | Used by |
| --- | --- | --- |
| `file_content:read` | yes | `file`, `node`, `render`, `assets export` |
| `file_metadata:read` | yes | `file info`, cache validation |
| `library_content:read` | yes | `components list`, `styles list` for a file |
| `current_user:read` | yes | `me`, `auth status` |
| `library_assets:read` | no | `components get --key`, `styles get --key` |
| `team_library_content:read` | no | `components`, `styles` with `--team` |
| `file_variables:read` | no | `variables`, `tokens export`. Enterprise plan only |
| `file_dev_resources:read` | no | `devresources list`, `node context` |
| `file_dev_resources:write` | no | `devresources add`, `update`, `remove` |
| `file_comments:read` | no | `comments list`, `node context` |
| `file_comments:write` | no | `comments add` only |
| `file_versions:read` | no | `versions list`, `--file-version` |
| `projects:read`, `folders:read` | no | project and folder discovery |

The old blanket `files:read` scope is deprecated; request the granular scopes.

Variables need an Enterprise plan and a full seat. The `file_variables:read`
scope is not offered on the token screen on other plans, so a token cannot
carry it at all. `tokens export` still exports the styles and says so in
`hints` rather than failing.

In CI, set `FIGMA_TOKEN` and figctl uses it as an implicit profile:

```yaml
- run: npx figctl tokens export "$FIGMA_FILE" --format css --out ./src/styles
  env:
    FIGMA_TOKEN: ${{ secrets.FIGMA_TOKEN }}
```

## Working across several Figma accounts

A consultant has one Figma account per client, often on different plans. A
profile is a name plus a token; tokens live in the OS keychain and the config
file holds no secrets.

```sh
npx figctl profile add acme --team 555000111 --default
npx figctl init --profile acme --file app=KEY1 --file design-system=KEY2 --default app
```

That writes a `.figctl.yaml` naming the profile and the Figma files this
repository uses, with no secret in it. A repository usually refers to more than
one, because a design system lives in its own file, and a configured name then
stands in for a key wherever a command takes a ref:

```sh
npx figctl tokens export design-system --format css --out ./src/styles
npx figctl node context --node 2:2   # the default file
```

Commands
walk up from the working directory to find it, so an agent working in the Acme
repository uses the Acme token without being told, and an agent in another
repository cannot reach Acme's files. Every envelope carries the profile it ran
as, so the agent can check the account before doing work.

The active profile is the first of these that matches: `--profile`,
`FIGCTL_PROFILE`, `FIGMA_TOKEN`, the nearest `.figctl.yaml`, then the user-level
default. `npx figctl auth status` reports which was chosen and why. The cache is
keyed by profile, so one client's data never mixes with another's.

## Caching and rate limits

Figma's rate limits are tiered per endpoint, and the tier figctl needs most is
the tightest:

| Tier | Endpoints | Limit |
| --- | --- | --- |
| 1 | get file, get nodes, render images | 10 to 30 per minute on Dev and Full seats depending on plan; 20 per **month** on View and Collab seats |
| 2 | variables, versions, dev resources, comments, image fills, folders | 25 to 150 per minute |
| 3 | components, styles, file meta, users | 50 to 200 per minute |

Ten Tier 1 requests a minute is not enough to walk a file node by node, so
figctl fetches the whole document once per version, caches it per profile, and
answers `file tree`, `file find`, `node inspect`, and `node context` from the
local copy. Freshness is checked against the cheap Tier 3 metadata endpoint.

Very large files cannot be fetched whole at all: Figma refuses them with a 400.
figctl degrades to depth-limited and per-node requests rather than failing.

`--refresh` re-fetches, `--no-cache` bypasses the cache, and `--file-version`
pins a version so it never has to be validated. A 429 is retried up to three
times honouring `Retry-After`.

## What this package does on install

1. Works out the platform: `darwin`, `linux`, or `win32`, on `x64` or `arm64`.
   Anything else fails with a message naming the supported platforms and the
   other ways to install.
2. Downloads the matching archive and `checksums.txt` from the GitHub release
   for this package's version.
3. Verifies the archive against its `sha256` line in `checksums.txt`. A mismatch
   aborts the install and writes nothing.
4. Unpacks the `figctl` binary next to the shim. No native modules, no
   post-install compilation, no runtime dependencies.

| Variable | Effect |
| --- | --- |
| `FIGCTL_SKIP_DOWNLOAD=1` | skip the download; set `FIGCTL_BINARY` to an existing executable when running figctl |
| `FIGCTL_BINARY=/path/to/figctl` | copy an existing binary during install; also select an executable at runtime |
| `FIGCTL_DOWNLOAD_BASE=URL` | download the assets from somewhere other than GitHub releases |

Behind a proxy or on an air-gapped network, download the release archive
yourself and point `FIGCTL_BINARY` at the extracted binary.

The release workflow signs `checksums.txt` with [cosign](https://docs.sigstore.dev/).
This wrapper checks SHA-256 checksums; it does not verify the cosign signature.
For signature verification, follow the repository release instructions before
installing a downloaded binary with `FIGCTL_BINARY`.

## Other ways to install

```sh
curl -fsSL https://raw.githubusercontent.com/tiaanduplessis/figctl/main/install.sh | sh
go install github.com/tiaanduplessis/figctl/cmd/figctl@latest
```

## Documentation

- [README](https://github.com/tiaanduplessis/figctl#readme)
- [Command reference](https://github.com/tiaanduplessis/figctl/blob/main/docs/commands.md)
- [Wiring figctl into an agent](https://github.com/tiaanduplessis/figctl/blob/main/docs/agents.md)
- [Design tokens](https://github.com/tiaanduplessis/figctl/blob/main/docs/design-tokens.md)
- [Profiles](https://github.com/tiaanduplessis/figctl/blob/main/docs/profiles.md)
- [Caching and rate limits](https://github.com/tiaanduplessis/figctl/blob/main/docs/caching.md)
- [Changelog](https://github.com/tiaanduplessis/figctl/blob/main/CHANGELOG.md)
- [Issues](https://github.com/tiaanduplessis/figctl/issues)

## License

MIT. Copyright Tiaan du Plessis.
