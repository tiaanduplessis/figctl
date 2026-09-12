# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The JSON output contract is versioned separately by the `schemaVersion` field in
every envelope. Within one `schemaVersion` only additive changes are made; a
breaking change to any command's output bumps it and is listed here as a
breaking change.

## [Unreleased]

## [0.1.0] - 2026-09-12

First public release. Output contract `schemaVersion` 1.

### Added

- `figctl diff` compares two local PNG screenshots without credentials, reports
  mismatch counts and ratios, and optionally writes a visual diff. Color
  tolerance, allowed mismatch ratio, and anti-aliasing inclusion are configurable.
  Completed comparisons above the limit exit 7 and retain the result envelope
  with `data.passed: false`. Includes a command schema and agent workflow guidance.

- `make spec-check` compares the Figma OpenAPI version pinned in
  `internal/figma/spec.go` with the one Figma publishes, and fails when they
  differ. The response types are hand-written, so the pin records which spec
  version they were reviewed against.

- **Discovery.** `file info` (name, pages, version, editor type, role,
  branches), `file tree` (sparse outline with ids, types, positions, sizes,
  child counts, and component, instance, hidden, auto-layout, export, and dev
  status flags; filters by node, page, depth, type, name, and visibility),
  `file find` (name glob, node type, and text search with the ancestor path of
  each hit), `file get` (raw Figma JSON, projected with `--fields`, `--depth`,
  `--geometry`, and `--plugin-data`).
- **Inspection.** `node inspect` returns a normalized model: auto layout and
  grid layout in CSS terms (row, column, wrap, grid; gap, padding, justify,
  align, fixed/hug/fill sizing), fills as hex and rgba with gradients as CSS,
  strokes, corner radius, effects as `box-shadow` and `filter`, typography with
  line height in px and unitless and per-run overrides for mixed text, component
  and variant information for instances, and every bound variable or style as a
  named token. Unbound values report `"token": null`. `--css` adds a flat CSS
  declaration map, `--web` adds the `var(--name)` form, `--interactions` adds
  prototype interactions.
- **`node context`**, the one-call bundle for implementing a node: the model at
  depth 4, a PNG screenshot on disk, SVG exports of icon-like layers, downloaded
  raster image fills, only the tokens the subtree uses, component and variant
  definitions of every instance, comments pinned on the node or its children,
  and dev resource links. Budget flags `--depth`, `--max-nodes`,
  `--no-screenshot`, `--no-assets`, `--no-comments`, `--no-css`.
- **Rendering and assets.** `render` to PNG, JPG, SVG, or PDF with scale, SVG
  options, absolute bounds, and contents-only control; requests batched per
  format and scale. `assets list` and `assets export` discover export-marked
  layers, icon-like nodes, and raster image fills, write deterministic file
  names and a `manifest.json`, and offer SVG cleanup (`--svg-strip-ids`,
  `--svg-strip-dimensions`, `--svg-current-color`) with no Node dependency.
- **Design tokens.** `tokens export` in DTCG 2025.10, Style Dictionary (the same
  bytes), CSS custom properties, a Tailwind theme module, and figctl's flat
  JSON. Mode strategies `default`, `separate`, and `select`; name cases `kebab`,
  `camel`, `snake`, and `none`; filtering by collection, source, hidden, and
  remote. Figma concepts DTCG cannot express (radial and angular gradients,
  image paints, blur effects, layout grids, mode sets, scopes, code syntax) are
  recorded under `$extensions.com.figma` with an explanatory `$description`
  rather than dropped. `tokens resolve` resolves one variable or style across
  modes and shows the alias chain.
- **Library data.** `variables list|get` (Enterprise), `styles list|get` for
  files and team libraries with values joined from the style nodes,
  `components list|get` with `componentPropertyDefinitions` and variants,
  `comments list|add`, `versions list`, `devresources list|add|update|remove`,
  `projects list|files`, `folders list|files`.
- **Profiles.** One named account per client, with the token in the OS keychain
  and a 0600 file fallback. `profile add|list|use|show|remove`,
  `auth login|logout|status|scopes`, and `init` to write a committable
  `.figctl.yaml`. Selection order: `--profile`, `FIGCTL_PROFILE`, `FIGMA_TOKEN`,
  the nearest `.figctl.yaml`, the user default. `auth status` reports which was
  chosen and why; every envelope carries the profile it ran as. Token expiry
  dates recorded at login are warned about within seven days.
- **Caching.** The whole file document is fetched once per version and cached
  under `$XDG_CACHE_HOME/figctl/<profile>/<fileKey>/`, so `tree`, `find`,
  `get`, `inspect`, and `context` cost one Tier 1 request between them. Cache
  validity is checked against `last_touched_at` from the Tier 3 file meta
  endpoint at most once per TTL. Renders and downloads are cached by node,
  format, scale, and version. `--refresh`, `--no-cache`, `--file-version`,
  `--max-file-mb`, and `cache status|clear`.
- **Rate limit handling.** 429 and 5xx retried up to three times honouring
  `Retry-After`, otherwise exponential backoff with jitter capped at 8 seconds.
  `X-Figma-Plan-Tier`, `X-Figma-Rate-Limit-Type`, and `X-Figma-Upgrade-Link`
  captured and surfaced in the error envelope and in `--verbose`.
- **Output contract.** JSON when stdout is not a terminal, a table when it is;
  `--json`, `-o md`, `-o plain`. A stable envelope with `schemaVersion`,
  `command`, `profile`, `file`, `data`, `truncated`, `nextCursor`, and `hints`.
  A stable error envelope with `code`, `message`, `hint`, `retryAfterSeconds`,
  `httpStatus`, and `details`. Exit codes 0, 1, 2, 3, 4, 5, and 6 (partial
  success). Error codes `USAGE`, `AUTH_MISSING`, `AUTH_INVALID`, `AUTH_SCOPE`,
  `NOT_FOUND`, `FORBIDDEN`, `RATE_LIMITED`, `PLAN_REQUIRED`, `RENDER_FAILED`,
  `PARTIAL`, `NETWORK`, `INTERNAL`. `--fields` projection, `--limit` and
  `--cursor` pagination.
- **`schema`** prints the JSON Schema of any command's `data`, and
  `schema envelope` the wrapper, so field names never have to be guessed.
- **Reference parsing.** `<ref>` accepts a file key or any Figma URL (design,
  file, board, proto, branch). A `node-id=1-2` in a URL is converted to `1:2`
  and used as the default `--node`.
- **Agent skills.** `skill install|print|uninstall` writes the figctl skill for
  Claude Code, Cursor, GitHub Copilot, Codex, or a generic `AGENTS.md`, with
  ownership markers so re-installing is idempotent and hand-written files are
  never clobbered. Reference files are generated from the command tree.
- **Distribution.** GoReleaser builds static binaries for macOS, Linux, and
  Windows on amd64 and arm64. Planned install channels: release archives, an
  install script that verifies checksums, an npm wrapper (`npx figctl`) that
  pins a version per project, and `go install`. The release workflow signs
  checksums with cosign.
- **Documentation.** README, generated `docs/commands.md`, and guides for
  agents, design tokens, caching, and profiles.
- `completion` for bash, zsh, fish, and powershell.

### Fixed

- Installation fails when checksums cannot be verified or available signature
  verification cannot complete, and preserves the existing binary if its
  replacement cannot run.
- The npm package includes the MIT license and supports an explicit
  `FIGCTL_BINARY` override after skipping the download.

- `figctl schema` now declares the fields that are printed as `null` as
  nullable. Reflection mapped a pointer to its element type alone, so the
  published schema promised an object or a string where the CLI prints `null`:
  `profile`, `file`, and `nextCursor` on every envelope, and the `token` of a
  fill, stroke, gradient stop, effect, and radius, and the entries of the
  layout and text `tokens` maps, all of which are documented as null when the
  value is not tokenized. The payloads are unchanged; only the schema was
  wrong.

### Known limitations

- Variables require an Enterprise plan with a full seat and the
  `file_variables:read` scope. Elsewhere `variables list` fails with
  `PLAN_REQUIRED` and `tokens export` exports styles only, reporting it in
  `hints`.
- `version --check` queries GitHub for the latest release and reports a
  `checkError` when none is published or the feed is unavailable. figctl never
  checks for updates on its own.
- Code Connect mappings are not exposed by the Figma REST API, so `node inspect`
  cannot name the code component of an instance. Dev resources and component
  documentation links are the available substitute.
- Writing to Figma is limited to `comments add` and
  `devresources add|update|remove`. FigJam and Slides content, webhooks, and
  admin endpoints are out of scope.
- Files whose document exceeds `--max-file-mb` are not cached whole; those
  commands fall back to `depth=2` or per-node requests and say so in `hints`.

[Unreleased]: https://github.com/tiaanduplessis/figctl/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tiaanduplessis/figctl/releases/tag/v0.1.0
