# figctl: an agent-first Figma CLI in Go

Implementation plan. Status: draft for review, 2026-09-07.

## 1. Goal

Build a single static Go binary that lets an AI coding agent (Claude Code, Codex, Cursor, Copilot, Gemini CLI, or any tool with a shell) implement a Figma design fully without any other Figma integration. "Fully" means the agent can, from one Figma URL:

1. Discover the structure of the file cheaply (pages, frames, sections, components).
2. See the design (rendered PNG/SVG of any node, at any scale).
3. Read exact layout and style data (auto layout, padding, sizing, constraints, fills, strokes, effects, corner radius, typography, text content).
4. Resolve every value back to its named design token (variable or style), across modes (light/dark, breakpoints, density).
5. Export assets (icons as SVG, raster image fills, export-marked layers) into the project at the right path.
6. Know which Figma components map to existing code (dev resources, Code Connect where available).
7. Read designer intent: component descriptions, comments, annotations, prototype interactions.

Non-goals for v1: writing to Figma (except comments), FigJam and Slides content, webhooks, enterprise analytics and admin endpoints, a GUI. These may come later as separate command groups and none of them block the core "implement this design" loop.

## 2. Research summary

### 2.1 What agent-friendly CLIs get right

Sources: clig.dev, Anthropic "Writing tools for agents", Rok Garbas "AI agents are your new users" checklist, gibil.dev `--json` pattern, Figma's own MCP server design and mcp-server-guide repo.

Principles adopted:

- **Runs without a human.** Never prompt when stdin or stdout is not a TTY. No interactive confirmations without a `--yes` escape. No spinners on stderr in non-TTY mode.
- **Data on stdout, everything else on stderr.** Progress, warnings, rate limit notices, and hints go to stderr. Agents parse stdout only.
- **Structured output on every command.** Every command supports `--output json` (aliased `--json`). Default output when stdout is not a TTY is JSON, because that is what an agent gets when it runs a command from a tool call. A `--output md` mode exists for agents that read output as text and for humans in a terminal.
- **Exit codes are the primary signal.** 0 success, 1 runtime/API error, 2 usage error, 3 auth error, 4 not found, 5 rate limited, 6 partial success (some items failed). Error JSON always has a stable `code` the agent can switch on, a human `message`, and a `hint` describing the next action.
- **Token efficiency is a feature.** The full Figma file JSON for a real product file is often 10 to 100 MB. Agents cannot consume that. Every read command has a default compact projection, filtering (by type, name, depth, page), pagination, and truncation with a hint on how to narrow the query. This mirrors Figma's MCP guidance: get sparse metadata first, then drill into specific node IDs, then screenshot.
- **Semantic over technical.** Output uses names (`Button/Primary`, `color/brand/500`) alongside IDs, never IDs alone. Colors as hex and rgba, not 0..1 floats. Line height in px and unitless, not Figma's percent variants. Padding as CSS shorthand and as four fields.
- **Progressive disclosure in help.** `figctl --help` is short, lists the recommended workflow in five lines, and points at `figctl <cmd> --help`. Every command has one or two copy-pasteable examples. `figctl schema <command>` prints the JSON Schema of that command's output so the agent never has to guess field names.
- **Idempotent and retry-safe.** Every read is naturally idempotent. Export commands overwrite deterministically named files. Rate limits are honored with `Retry-After` and reported, not swallowed.
- **Predictable output contract.** JSON output shapes are versioned (`"schemaVersion"` in every envelope). Additive changes only within a major version. Golden-file tests guard every shape.
- **Ships its own instructions.** `figctl skill install` writes a SKILL.md and per-topic reference files into the agent's skill directory (Claude Code, Cursor, Copilot, Codex, generic AGENTS.md snippet). Skill files are the agent's onboarding doc, and they are generated from the same source as `--help`, so they never drift.
- **Fewer, higher-level commands beat an endpoint wrapper.** Anthropic's guidance: build tools for workflows, not for endpoints. So in addition to thin endpoint commands, ship composite commands that do what an agent actually needs in one call: `inspect` (node tree with resolved tokens and CSS-like values), `context` (everything needed to implement one node, including screenshot and assets), `tokens` (all variables and styles as DTCG JSON).

### 2.2 The two reference CLIs

`@sahajamit/figma-cli` (Node, v0.2.0):
- Thin wrapper over the REST endpoints, auto JSON when piped, `figma install --skills`, one-step image export with local download, and a `tokens export` that infers DTCG tokens from styles and raw node values.
- Strong ideas to keep: the CLI+skills argument against MCP (no server, fewer schema tokens, direct shell), image export that downloads, skill files, URL parsing guidance, "start shallow then drill down".
- Gaps we will fix: node output is the raw Figma JSON (huge, unresolved colors, no token names); no filtering or pagination; tokens are inferred from node values rather than from real variables with modes; no per-node context command; no caching; no rate limit handling; uses the `figma` binary name which collides with Figma's official Code Connect CLI (`@figma/code-connect` v2 installs a `figma` binary).

`@figma-export/cli` (Node, v6.4.1):
- Build-pipeline tool. Exports components as SVG and styles as CSS/SCSS/Less/Style Dictionary via a config file with pluggable transformers and outputters.
- Strong ideas to keep: config file for repeatable project exports, page filtering, transformers (SVGO), outputs targeting real build systems, version pinning.
- Gaps: not agent-oriented at all (no JSON, no discovery), styles only (no variables), components exported as SVG only.

### 2.3 Figma's own MCP server (what "comprehensive" looks like)

The Dev Mode MCP server's read tools define the bar: `get_metadata` (sparse XML outline: id, name, type, position, size), `get_design_context` (structured code-like representation, React+Tailwind by default), `get_screenshot`, `get_variable_defs` (variables and styles used by a selection), `download_assets` (rendered exports plus raw image fills, up to 20 nodes), `get_code_connect_map`, `search_design_system`, `get_libraries`. Their guidance: metadata first for large designs, then design context on specific nodes, then screenshot, then variables.

The CLI mirrors that workflow with commands that produce equivalent data from the public REST API, with the differences that we do not generate framework code (the agent does that better with full project context) and Code Connect maps are not exposed by the REST API, so the CLI covers that gap with dev resources plus a local mapping file.

### 2.4 Figma REST API facts that shape the design

Source: `figma/rest-api-spec` OpenAPI 3.1, version 0.42.0, plus the developer docs changelog through August 2026.

- **Auth.** Personal access tokens (`X-Figma-Token` header) now expire after at most 90 days (since April 2025). Scopes are chosen at token creation. OAuth2 with refresh at `POST /v1/oauth/token`. Plan access tokens (org-level, GA July 2026) are supported for most read endpoints but not for variables or comment writes.
- **Scopes needed by this CLI.** `file_content:read` (files, nodes, images), `file_metadata:read` (file meta), `library_content:read` (file components/styles), `team_library_content:read` (team components/styles), `file_variables:read` (variables; Enterprise full seats only), `file_dev_resources:read`, `file_comments:read`, `file_comments:write` (optional), `file_versions:read`, `projects:read`, `folders:read` (new v2 folders API, projects API deprecated August 2026), `current_user:read`. The old blanket `files:read` scope is deprecated.
- **Rate limits (in effect since November 2025).** Three tiers by endpoint. Tier 1 (get file, get nodes, render images): 10 to 30 per minute on Dev/Full seats depending on plan, and only 20 per month on View/Collab seats. Tier 2 (variables, versions, dev resources, comments, image fills, folders): 25 to 150 per minute. Tier 3 (components, styles, file meta, users): 50 to 200 per minute. 429 responses carry `Retry-After`, `X-Figma-Plan-Tier`, `X-Figma-Rate-Limit-Type` (low/high), `X-Figma-Upgrade-Link`. Consequence: Tier 1 calls are precious. The CLI must cache file JSON aggressively, batch node IDs into one request, and reuse one file fetch across many local queries.
- **File endpoint parameters.** `ids`, `depth`, `version`, `geometry=paths`, `plugin_data`, `branch_data`. `depth=1` returns pages only, `depth=2` pages plus top-level frames. `GET nodes` has a different `depth` semantic (relative to the requested node).
- **Image rendering.** `GET /v1/images/:key?ids=...&format=png|jpg|svg|pdf&scale=0.01..4` returns temporary URLs (expire in 30 days). Null entries mean render failure for that node. SVG options: `svg_outline_text`, `svg_include_id`, `svg_include_node_id`, `svg_simplify_stroke`. `use_absolute_bounds` and `contents_only` control cropping. Up to 32 megapixels. `GET /v1/files/:key/images` returns URLs for all raster image fills keyed by `imageRef` (expire within 14 days).
- **Variables.** `GET .../variables/local` returns `variables` and `variableCollections` maps with `modes`, `valuesByMode`, `resolvedType` (BOOLEAN, FLOAT, STRING, COLOR), aliases (`{type: VARIABLE_ALIAS, id}`), `scopes`, `codeSyntax` (WEB, ANDROID, iOS), extended collections (`parentVariableCollectionId`, `variableOverrides`). Published variables have `subscribed_id` and omit modes. Node properties carry `boundVariables` referencing variable IDs. Enterprise only; on other plans the CLI must degrade gracefully to styles plus raw values with a clear message.
- **Styles.** File and team style endpoints return metadata only (key, name, type, description, node_id). Actual style values must be read from the style's node via `GET nodes`, and nodes reference styles through the `styles` map (`fill`, `text`, `effect`, `stroke`, `grid` to style ID). The CLI must join these.
- **Components.** File and team component endpoints return key, name, description, `componentSetId`, `documentationLinks`, `containingFrame`. Variant properties live on the component set node (`componentPropertyDefinitions`) and instances carry `componentProperties`, `overrides`, `exposedInstances`.
- **Dev resources.** URLs attached to nodes shown in Dev Mode. Read via `GET /v1/files/:key/dev_resources?node_ids=`. Code Connect mappings are not exposed by the REST API.
- **Node model highlights (for the inspector).** Auto layout: `layoutMode`, `layoutWrap`, `primaryAxisSizingMode`, `counterAxisSizingMode`, `primaryAxisAlignItems`, `counterAxisAlignItems`, `counterAxisAlignContent`, `itemSpacing`, `counterAxisSpacing`, `padding*`, `layoutSizingHorizontal/Vertical`, `layoutGrow`, `layoutAlign`, `layoutPositioning`, `min/maxWidth/Height`, `itemReverseZIndex`, `strokesIncludedInLayout`. Grid layout (July 2025): `gridRowCount`, `gridColumnCount`, `gridRowGap`, `gridColumnGap`, `gridRowsSizing`, `gridColumnsSizing`, `gridRowSpan`, `gridColumnSpan`, `gridChildHorizontalAlign`, `gridChildVerticalAlign`. Visual: `fills`, `strokes`, `strokeWeight`, `individualStrokeWeights`, `strokeAlign`, `strokeDashes`, `cornerRadius`, `rectangleCornerRadii`, `cornerSmoothing`, `effects`, `opacity`, `blendMode`, `clipsContent`, `isMask`. Text: `characters`, `style` (TypeStyle: fontFamily, fontPostScriptName, fontWeight, fontSize, italic, letterSpacing, lineHeightPx, lineHeightPercentFontSize, lineHeightUnit, textCase, textDecoration, textAlignHorizontal/Vertical, textAutoResize, textTruncation, maxLines, paragraphSpacing, listSpacing, hyperlink), `characterStyleOverrides` plus `styleOverrideTable` for mixed runs. Geometry: `absoluteBoundingBox`, `absoluteRenderBounds`, `relativeTransform`, `constraints`, `layoutGrids`. Prototype: `interactions`, `transitionNodeID`, `scrollBehavior`, `overflowDirection`. Dev Mode: `devStatus`. Newer node types: TEXT_PATH, TRANSFORM_GROUP, TABLE, SECTION.
- **Design token format.** The W3C Design Tokens Community Group format reached its first stable version, 2025.10, in October 2025 (`$value`, `$type`, `$description`, `$extensions`, alias references `{group.token}`). Style Dictionary v4+ consumes it. This is the export format.

### 2.5 Naming

The obvious binary name `figma` is taken by Figma's official Code Connect CLI, which is widely installed in design-system repos, and by the sahajamit package. Colliding with the official CLI would break users and confuse agents. Recommended name: **`figctl`** (kubectl convention, short, unambiguous, no known collision). Go module: `github.com/tiaanduplessis/figctl`, published under the `tiaanduplessis` GitHub account.

## 3. Design

### 3.1 Command surface

Noun-verb ordering, consistent flags, every command supports the global flags. Composite commands come first because they are what the skill file will teach.

```
figctl auth login|status|logout|scopes       token management, per profile
figctl profile list|add|use|remove|show     named accounts (one per client or org)
figctl file info <ref>                       name, pages, version, editor type, branches, role
figctl file tree <ref> [--node ID]           sparse outline (id, name, type, size, position), the "metadata" call
figctl file find <ref> --name|--type|--text  search nodes by name glob, type, or text content
figctl file get <ref> [--node ID]            raw Figma JSON, filtered/projected (escape hatch)
figctl node inspect <ref> --node ID ...      resolved layout, style, typography, tokens, text; CSS-like values
figctl node context <ref> --node ID          everything to implement one node: inspect + screenshot + assets + tokens + components + dev resources
figctl render <ref> --node ID ... [-f png|svg|jpg|pdf] [--scale N] [-o DIR]
figctl assets export <ref> [--node ID] [-o DIR]    export-marked layers, icons, and image fills used under a node
figctl tokens export <ref> [--format dtcg|css|tailwind|json] [--mode ...]
figctl tokens resolve <ref> --id VariableID:..  resolve a value or alias chain across modes
figctl variables list|get <ref>              raw variables and collections (Enterprise)
figctl styles list|get <ref|team>            styles with resolved values
figctl components list|get|search <ref|team> components, sets, variant properties, descriptions
figctl comments list|add <ref>               designer intent; add is the one write op
figctl versions list <ref>
figctl devresources list <ref> [--node ID]
figctl projects|folders list, files          discovery via v2 folders API with v1 projects fallback
figctl me
figctl cache status|clear
figctl skill install|print [--agent claude|cursor|copilot|codex|generic]
figctl schema <command>                      JSON Schema for a command's output
figctl completion bash|zsh|fish
```

`<ref>` accepts a file key, a full Figma URL (design, file, board, proto forms), or a branch key. When a URL contains `node-id=1-2`, the CLI converts to `1:2` and uses it as the default `--node`. This removes the most common agent mistake seen in the sahajamit skill docs (manual hyphen to colon conversion).

### 3.2 Global flags and environment

```
--output, -o json|md|table|plain   default: json when stdout is not a TTY, table otherwise
--json                              alias for --output json
--fields a,b,c                      projection on JSON output
--limit N, --cursor C               pagination on list output
--quiet, -q                         no stderr progress
--verbose, -v                       HTTP trace to stderr (tokens redacted)
--no-cache / --refresh              bypass or refresh cache
--version VERSION_ID                pin a file version
--timeout 30s
--yes                               skip confirmations (only comments add and cache clear use them)
--no-color                          plus NO_COLOR and TERM=dumb honored
```

Config precedence: flags > env > project config (`.figctl.yaml` in repo, no secrets) > user config (`$XDG_CONFIG_HOME/figctl/config.yaml`) > defaults.

Token sources, in order: `--token-file`, `FIGMA_TOKEN` env, OS keychain entry for the active profile written by `figctl auth login` (via `zalando/go-keyring`), config file token (discouraged, warn). Never accept the token as a flag value. See 3.8 for how the active profile is chosen.

### 3.3 Output contract

Success envelope (JSON):

```json
{
  "schemaVersion": 1,
  "command": "node.inspect",
  "file": {"key": "ABC", "name": "Web App", "version": "123", "lastModified": "..."},
  "data": {...},
  "truncated": false,
  "nextCursor": null,
  "hints": ["Use --depth 2 to see children of frame 12:34"]
}
```

Error envelope, always on stdout in JSON mode, exit code non-zero:

```json
{
  "schemaVersion": 1,
  "error": {
    "code": "RATE_LIMITED",
    "message": "Figma rate limit hit for tier 1 endpoints (plan: pro, seat: high).",
    "hint": "Retry after 42s. Reuse the cached file with 'figctl file tree ABC' instead of re-fetching.",
    "retryAfterSeconds": 42,
    "httpStatus": 429
  }
}
```

Error codes: `USAGE`, `AUTH_MISSING`, `AUTH_INVALID`, `AUTH_SCOPE` (includes which scope), `NOT_FOUND` (file or node, says which), `FORBIDDEN`, `RATE_LIMITED`, `PLAN_REQUIRED` (variables on non-Enterprise), `RENDER_FAILED`, `PARTIAL`, `NETWORK`, `INTERNAL`.

Markdown output (`--output md`) is for agents that read tool output as text and for humans. It uses headings and compact tables, never ASCII art, and prints the same hints.

Partial success (e.g. 3 of 5 renders failed) returns exit 6 with a full `data` and a `failures` array, so an agent can keep the good results.

### 3.4 Composite commands in detail

**`file tree`** produces a sparse outline like the MCP `get_metadata` tool. Each line or row: id, type, name, x, y, w, h, child count, and flags (component, instance of X, hidden, has auto layout). Defaults: depth 2 from the given node, 200 nodes per page, then `nextCursor`. Filters: `--type FRAME,SECTION`, `--name "Login*"`, `--page "Mobile"`, `--visible-only`, `--depth N`. One Tier 1 fetch, cached; further calls are served locally.

**`node inspect`** is the core value. For each requested node (and children to `--depth`, default 3) it emits a normalized model:

- `layout`: mode (none, row, column, wrap, grid), gap, padding (four values plus shorthand), align (mapped to CSS justify/align equivalents), sizing (fixed/hug/fill for each axis with px), min/max, positioning (auto/absolute with offsets), constraints, clips, overflow.
- `visual`: fills (hex, rgba, gradient stops with angle, image refs with the local asset path once exported), strokes (color, weight per side, align, dashes), radius (uniform or four corners, smoothing), effects (CSS `box-shadow`, `filter: blur`, `backdrop-filter` strings plus raw), opacity, blend mode.
- `text`: characters, font family, weight, style, size, line height (px and unitless), letter spacing (px and em), case, decoration, alignment, truncation, and per-run overrides when text is mixed.
- `tokens`: for every value bound to a variable or style, the token name, collection, mode values, and code syntax (`--web` shows `var(--color-brand-500)` when codeSyntax is set). Alias chains resolved with the path shown.
- `component`: for instances, the main component name, set name, variant properties, overrides vs defaults, exposed nested instances, dev resources, description and documentation links.
- `css`: an optional flat CSS declaration block per node (`--css`), and Tailwind class suggestions (`--tailwind`) as a convenience, clearly marked as approximate. Not framework code generation.
- `prototype`: interactions and transitions when `--interactions` is set.

Everything unresolved (no token bound) is reported as a raw value with `"token": null`, so the agent knows to hardcode or to ask.

**`node context`** bundles for one node: `inspect` at depth 4, a PNG render at 2x written to `--out` (default `./.figctl/<fileKey>/<nodeId>@2x.png`), SVG exports of icon-like vector children, image fills downloaded, the subset of tokens used, component definitions of instances used, comments anchored on the node or its children, dev resources. Output JSON lists file paths so the agent can Read the images with vision. This is the one-call "give me everything for this screen" command and the first thing the skill file teaches.

**`assets export`** finds under a node: layers with `exportSettings`, instances whose name matches icon conventions (`--icon-pattern`, default `icon/*`, `Icon*`), vector-only frames, and raster fills (`imageRef`). Renders via the images endpoint (batched, 1 request per format/scale combination), downloads image fills via the image fills endpoint, writes deterministic slugged filenames (`button-primary.svg`, `hero@2x.png`, `imageref-<hash>.png`), and emits a manifest mapping node id to path. Supports `--svgo`-style minimal SVG cleanup natively (strip ids, dimensions, fill currentColor option) without a Node dependency.

**`tokens export`** converts variables (all collections, all modes) and styles (paint, text, effect, grid) into DTCG 2025.10 JSON with `$type` (color, dimension, fontFamily, fontWeight, number, typography, shadow, string, boolean), aliases as `{collection.group.name}` references, mode handling via `--mode` selection or per-mode files or `$extensions.figma.modes`. Also `--format css` (custom properties per mode with `[data-theme]` selectors), `--format tailwind` (theme object), `--format style-dictionary` (identical to dtcg). Name normalization is configurable (`--name-case kebab|camel|snake`). On plans without variables access it exports styles only and says so in `hints`.

### 3.5 Caching

Tier 1 rate limits (as low as 10 per minute, 20 per month on view seats) make caching mandatory, not an optimization.

- Disk cache at `$XDG_CACHE_HOME/figctl/<fileKey>/`. Entries keyed by endpoint plus query parameters, stored with the file `version` from the response.
- Validation: `GET file meta` (Tier 3, cheap) returns `last_touched_at`; the CLI checks it at most once per `--cache-ttl` (default 60s) and reuses cached file JSON when unchanged. `--refresh` forces a fetch, `--no-cache` bypasses.
- Whole-file strategy: on first touch of a file, fetch the full document once (`GET file` with no depth) unless it exceeds `--max-file-mb` (default 200), then serve `tree`, `find`, `inspect`, `get` from the local copy. For huge files fall back to `depth=2` plus per-node `GET nodes` calls.
- Rendered images and asset downloads are cached by (node, format, scale, version).
- `figctl cache status` shows size and per-file entries; `cache clear` requires `--yes` when not a TTY.

### 3.6 HTTP client

- Hand-written client over `net/http` in `internal/figma`, response types generated from the OpenAPI spec with `oapi-codegen` (types only, not the client) so the node model stays current with Figma's spec releases. Regeneration is a `make generate` target pinned to a spec commit.
- Retries with exponential backoff and jitter on 429 and 5xx, honoring `Retry-After`, max 3 attempts, total time bounded by `--timeout`. Rate limit headers surfaced in verbose mode and in the error envelope.
- Batching helpers: split `ids` lists to keep URLs under 8 KB, and merge concurrent render requests for the same format and scale into one call.
- Concurrency limit for downloads (default 8) with a bounded worker pool.
- User agent `figctl/<version>`.

### 3.7 Skill files

`figctl skill install --agent claude` writes:

- `~/.claude/skills/figctl/SKILL.md`: when to use, URL parsing, the five-step workflow (`file tree` then `node context` then implement then `render` to compare), token guidance, rate limit etiquette, error code table.
- `~/.claude/skills/figctl/reference/commands.md`: generated from the command definitions (same source as `--help`).
- `~/.claude/skills/figctl/reference/output-schemas.md`: generated from the JSON Schemas.

Other agents: Cursor (`.cursor/rules/figctl.mdc`), Copilot (`.github/copilot-instructions.md` snippet), Codex and generic (`AGENTS.md` snippet appended with markers). `figctl skill print` writes to stdout for manual placement. Project-local install (`--project`) writes into the repo so it is shared with teammates.

### 3.8 Profiles (multiple accounts)

A consultant working for several clients has a separate Figma account, token, and often a separate plan tier per client. The CLI treats this as a first-class concept so an agent in client A's repo never uses client B's token.

Model:

- A profile is a name plus a token, an optional default team ID, an optional API base URL (for Figma for Government, `https://api-figma-gov.com`), and cached metadata (user handle, email, detected scopes, plan tier from rate limit headers).
- Profiles live in the user config file (`$XDG_CONFIG_HOME/figctl/config.yaml`) as a map of names to non-secret settings. Tokens live in the OS keychain under `figctl/<profile>`; on headless Linux without a keychain they fall back to a 0600 file under `$XDG_CONFIG_HOME/figctl/credentials`, with a warning.
- One profile is the user-level default.

Selection order for the active profile (first match wins):

1. `--profile NAME` flag.
2. `FIGCTL_PROFILE` env var.
3. `FIGMA_TOKEN` env var, which creates an implicit ad hoc profile named `env` (keeps CI and one-off scripts simple).
4. `profile: NAME` in the project config `.figctl.yaml`, found by walking up from the working directory. This is the mechanism that makes agents pick the right account automatically: each client repo commits a `.figctl.yaml` naming the profile, with no secret in it.
5. The user-level default profile.
6. If no profile exists: `AUTH_MISSING` error whose hint shows `figctl auth login --profile <name>`.

Commands:

```
figctl profile add <name>            prompts for token on a TTY, or reads --token-file / stdin; validates with GET me
figctl profile list                  name, handle, email, default marker, scopes, plan tier; JSON too
figctl profile use <name>            set user-level default
figctl profile show [name]           details, token redacted
figctl profile remove <name> [--yes]
figctl auth login --profile <name>   same as profile add, kept for familiarity
figctl auth status                   reports which profile is active and why (flag, env, project config, default)
figctl init                          writes .figctl.yaml in the current repo with the chosen profile name and optional default file key
```

Behavior details:

- Every JSON envelope includes `"profile": {"name": "acme", "handle": "..."}` so an agent can verify it is on the right account before doing work.
- The cache is keyed by profile first, then file key. Accounts see different `role` and possibly different branches for the same key, and keeping client data separated on disk is a reasonable expectation for consulting work. `cache clear --profile NAME` clears one client's data.
- Rate limit state is tracked per profile, since Figma tracks limits per user and plan.
- On a 403 or 404 for a file, the error hint lists other configured profiles and suggests `--profile`. An optional `--profile auto` tries the cheap Tier 3 file meta call against each profile in order and remembers the file-to-profile mapping in the cache; it is opt-in because it spends requests on every account.
- `skill install --project` and `init` together mean an agent starting in a client repo has both the instructions and the right account with zero manual setup.
- Tokens expire after at most 90 days, so `profile list` and `auth status` show the expiry date when the user recorded it at login (`--expires 2026-12-01`) and warn within 7 days. The API does not expose expiry, so this is best effort.

### 3.9 Project layout

```
cmd/figctl/main.go
internal/cli/            command definitions (one file per noun), flag wiring, output selection
internal/config/         config files, profiles, credential storage (keychain and file fallback)
internal/figma/          HTTP client, auth, rate limiting, generated types (gen/), pagination
internal/cache/          disk cache, validation, image cache
internal/model/          normalized node model (layout, visual, text, tokens, component)
internal/resolve/        variable and style resolution, alias chains, modes, color math
internal/inspect/        raw node to model conversion, CSS/Tailwind formatting
internal/assets/         asset discovery, rendering, downloading, SVG cleanup, filenames
internal/tokens/         DTCG, CSS, Tailwind exporters
internal/output/         JSON envelope, markdown, table renderers, schema generation
internal/skill/          embedded skill templates and installers
internal/ref/            Figma URL and key parsing
testdata/                recorded API fixtures, golden outputs
docs/                    README, command reference (generated), design notes
```

Dependencies (kept minimal): `spf13/cobra` for commands and completion (largest ecosystem, agents have seen the most cobra-style help text), `oapi-codegen` types, `zalando/go-keyring`, `invopop/jsonschema` for `schema`, `charmbracelet/lipgloss` only for the TTY table renderer, `golang.org/x/sync/errgroup`. No viper; config is a small YAML struct.

### 3.10 Distribution

GoReleaser builds darwin/linux/windows for amd64 and arm64, Homebrew tap, `go install`, Scoop, a Docker image, and an npm wrapper package that downloads the binary (so `npx figctl` works in JS-heavy teams). Signed checksums. `figctl version --check` reports newer releases without auto-updating.

## 4. Implementation phases

Each phase ends in a working, tested, releasable binary.

### Phase 0: Decisions and scaffold (0.5 day)

- Name, module path, org, and MIT license are decided. Add LICENSE file.
- `go mod init`, cobra root, global flags, output envelope, error codes, exit codes, TTY detection, config loading, logging to stderr.
- CI: GitHub Actions running `go vet`, `staticcheck`, `golangci-lint`, `go test -race`, GoReleaser dry run.
- Acceptance: `figctl --help`, `figctl version`, `figctl me` against a real token, JSON and table output, error envelope on bad token, exit codes verified by a shell test.

### Phase 1: Client, auth, cache (2 days)

- Generated types from the pinned OpenAPI spec. Client with retries, `Retry-After`, rate limit header capture, batching, concurrency.
- Profiles: config file model, keychain and file fallback storage, selection order, `profile add|list|use|show|remove`, `init`, per-profile cache and rate limit state, profile block in every envelope.
- `auth login` (stdin or `--token-file`, keychain), `auth status` (calls `me`, reports scopes by probing endpoints once and caching the result, and explains which profile was chosen and why), `auth logout`, `auth scopes` (prints the scope list to request).
- Disk cache with file-version validation via file meta.
- `file info`, `file get` with `--node`, `--depth`, `--fields`, `--version`.
- Ref parser for all Figma URL forms and `node-id` extraction, with unit tests.
- Tests: `httptest` recorded fixtures for every endpoint, retry and 429 behavior, cache hit/miss/refresh, profile selection order (flag, env, project config walk-up, default), cache isolation between profiles.

### Phase 2: Discovery (1.5 days)

- `file tree` with filters, pagination, sparse rows, markdown and table renderers.
- `file find` by name glob, type, and text content, returning the ancestor path for each hit.
- `components list|get|search` and `styles list|get` for files and teams, with pagination. `styles get` joins style metadata with the style node to give actual values.
- `versions list`, `projects` and `folders` (v2 with v1 fallback), `comments list|add`, `devresources list`.
- Golden tests on a checked-in synthetic file fixture covering all node types.

### Phase 3: Rendering and assets (1.5 days)

- `render` with all format and SVG options, batched, cached, downloaded to disk, manifest output, null-render handling with per-node failures and exit 6.
- `assets export`: discovery rules, icon detection, image fills, deterministic slugs, SVG cleanup options, manifest.
- Tests: batching splits, filename collisions, failed render reporting, fixture-served images.

### Phase 4: Resolution and inspection (3 days)

- `variables list|get` with plan gating and a clear `PLAN_REQUIRED` error including the styles-only fallback hint.
- Resolver: variable alias chains across modes, extended collections and overrides, style lookup, color conversion (Figma 0..1 floats to hex/rgba, gradient stops and angle from `gradientHandlePositions`), typography normalization (line height variants, letter spacing units, weight from `fontWeight` and `fontPostScriptName`).
- Normalized node model and `node inspect` with layout mapping (auto layout and grid to CSS semantics), visual, text with mixed runs, tokens, component info, `--css` and `--tailwind` formatters, depth control, truncation with hints.
- `tokens resolve`.
- Tests: table-driven per property family, golden outputs for the synthetic fixture, property-based tests on color math.

### Phase 5: Tokens export (1.5 days)

- DTCG 2025.10 exporter for variables (all types, aliases, modes) and styles (paint, text, effect, grid).
- CSS custom properties and Tailwind theme outputs. Name normalization options.
- Validate output against the DTCG JSON Schema and by round-tripping through Style Dictionary in an integration test (Node used only in CI for this check).

### Phase 6: Context command and skills (1.5 days)

- `node context`: orchestrates inspect, render, assets, tokens subset, components, comments, dev resources into one manifest with local paths. Output budget flags (`--max-nodes`, `--no-screenshot`, `--no-assets`).
- `schema <command>` from Go types.
- Skill templates, generated command reference, `skill install|print` for each agent target, idempotent installs with markers.
- Agent evaluation harness (see section 5).

### Phase 7: Release (1 day)

- README with the agent workflow first, human usage second; generated `docs/commands.md`; CONTRIBUTING; CHANGELOG; security policy (token handling).
- GoReleaser config, Homebrew tap, npm wrapper, Docker image, checksums.
- v0.1.0 tag.

Total: about 12.5 engineering days for a complete v0.1.0.

### Later (not in v0.1.0)

- OAuth device-style flow for teams that forbid personal tokens.
- Local Code Connect mapping file (`.figctl/codeconnect.json`, or reading `figma.config.json` from the official Code Connect CLI) so `inspect` can name the code component for an instance.
- `watch` mode using file version polling to re-export tokens and assets when the file changes.
- `diff <ref> --from VERSION --to VERSION` for design change review.
- Optional MCP server mode (`figctl mcp serve`) exposing the same commands for agent frameworks without shell access, sharing the CLI's code paths.
- Config-driven repeatable exports (`figctl run` with `.figctl.yaml` jobs) covering the figma-export use case.
- FigJam and Slides read support.

## 5. Quality gates

- Unit coverage target 85 percent on `internal/resolve`, `internal/inspect`, `internal/tokens`, `internal/ref`, `internal/output`.
- Golden tests: every command's JSON and markdown output for the synthetic fixture file, checked in and reviewed on change. Schema version bumps require a changelog entry.
- Contract test: `figctl schema` output validates every golden file.
- Integration test (opt-in with `FIGMA_TOKEN` and a public community file key) runs nightly, not on every PR, to respect rate limits.
- Agent evaluation harness (`eval/`): a set of tasks ("implement frame X as a React component", "list all colors used in screen Y with token names", "export the icons in Z") run through Claude Code with the skill installed, scoring tool-call count, tokens consumed, wall time, and a verifiable check (assets exist, token names present, no hardcoded hex where a token exists). Results recorded per release to catch regressions in agent ergonomics, per Anthropic's tool-evaluation guidance.
- Static checks: `golangci-lint` with `errcheck`, `gosec`, `revive`; `go vet`; `govulncheck` in CI.
- Security: token never logged, never in URLs, redacted in `--verbose`; cache directory created with 0700; no telemetry.

## 6. Open questions to confirm before Phase 0

Decided: binary `figctl`, module `github.com/tiaanduplessis/figctl`, repository `tiaanduplessis/figctl`, MIT license.

1. Whether to include the `--tailwind` class suggestions in v0.1.0 or defer (they are approximate by nature and may mislead agents; recommendation is to ship `--css` only in v0.1.0 and add Tailwind behind a flag in v0.2.0).
2. Whether comment creation should be included in v0.1.0 (it is the only write and needs `file_comments:write`; recommendation is yes, it is cheap and useful for agents to leave implementation notes).

## 7. Sources

- Figma REST API OpenAPI spec 0.42.0: https://github.com/figma/rest-api-spec
- Figma REST API docs and changelog: https://developers.figma.com/docs/rest-api/ and https://developers.figma.com/docs/rest-api/changelog
- Figma rate limits: https://developers.figma.com/docs/rest-api/rate-limits
- Figma MCP server tools: https://developers.figma.com/docs/figma-mcp-server/tools-and-prompts
- Figma mcp-server-guide: https://github.com/figma/mcp-server-guide
- @sahajamit/figma-cli: https://github.com/sahajamit/figma-cli
- @figma-export/cli: https://github.com/marcomontalbano/figma-export
- Command Line Interface Guidelines: https://clig.dev/
- Anthropic, Writing tools for agents: https://www.anthropic.com/engineering/writing-tools-for-agents
- Rok Garbas, AI agents are your new users, CLI checklist: https://garbas.si/posts/ai-agents-are-your-new-users-cli/
- Gibil, the --json pattern: https://www.gibil.dev/blog/cli-json-pattern
- DTCG Design Tokens Format Module 2025.10: https://www.designtokens.org/tr/drafts/format/
