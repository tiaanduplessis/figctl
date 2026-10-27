---
name: figctl
description: Use figctl to inspect Figma designs, implement screens from Figma URLs, export assets and design tokens, or compare local PNG screenshots with a visual diff. Use for Figma design-to-code tasks and figctl commands; local image comparisons need no Figma account.
allowed-tools: Bash(figctl:*)
---

# figctl

figctl is a shell CLI for reading Figma designs and comparing local screenshots.
It returns design data and files for implementing a design in the codebase.

## Setup

Check `figctl version`. If missing, use the project's installation method or
`npm install -g figctl` (macOS, Linux, Windows). Installing this
skill does not install the CLI.

For Figma API commands, run `figctl auth status` and check the selected account.
If authentication is missing, have the user run `figctl auth login <name>` with
its hidden prompt, or supply credentials through their secret manager using
`FIGMA_TOKEN` or `--token-file`. Never put a token in chat, logs, or arguments.
`figctl diff`, `schema`, and `skill print` need no credentials or network.

## File and node references

Read `.figctl.yaml` if present. `<ref>` accepts a configured file name, a key,
a Figma URL, or no argument when a default file is configured:

```sh
figctl file tree design-system
figctl node context --node 2:2
figctl file tree "https://www.figma.com/design/KEY/App?node-id=2-2"
```

Paste URLs as given: `node-id=1-2` becomes the default `--node 1:2` automatically.
`--node` also accepts `1:2`, `1-2`, or a URL. An unknown configured name lists
valid names in the error.

## Implement a design

1. Orient with a shallow outline or search to find the relevant frame.
   ```sh
   figctl file tree "<url>" -o md
   figctl file find "<url>" --name "Login*"
   ```
2. Get layout, styles, tokens, screenshot, assets, components, measurements,
   comments, and dev links in one call. Match the planned capture scale (1x here).
   ```sh
   figctl node context "<url>" --node 2:2 --screenshot-scale 1 -o md
   ```
   Read the PNG at the reported absolute path with an image tool before coding.
   Use `--depth`, `--max-nodes`, `--no-assets`, `--no-comments`, `--no-screenshot`,
   or `--no-css` to reduce work when those results are unnecessary.
3. Implement using existing codebase conventions and the reported tokens.
4. Capture the implementation at the same viewport, crop, and device scale.
   Reuse the context screenshot as `design.png` below; use its actual path.
   ```sh
   figctl diff design.png implementation.png --out diff.png --max-diff-ratio 0.01 --json
   ```
   Exit 7 is a completed comparison with `data.passed: false`, not an API error.
   Read the diff image, fix visible differences, and compare again. Do not
   increase tolerances just to pass. For capture preparation, settings, and
   shell exit handling, run `figctl skill print --file workflows -o md`.
5. Export design tokens when the project needs them.
   ```sh
   figctl tokens export design-system --format css --out ./src/styles
   ```

To render separately, use `figctl render "<url>" --scale 1 --out ./design`
and take the PNG path from its manifest. `render` defaults to 2x for PNGs.
`diff` accepts equal-sized PNGs up to 16 million pixels each; it never resizes
inputs. The example accepts 1% mismatched pixels; the default accepts none.
The mismatch ratio measures pixel differences, not overall design quality.

## Output and errors

Output is JSON when piped and a table on a terminal. Prefer `-o md` for reading
`node context`, `node inspect`, and `file tree`; use `--json` for scripts.
Success responses wrap the payload in `data`:

```json
{"schemaVersion":1,"command":"node.context","profile":{"name":"acme"},
 "file":{"key":"KEY","name":"App","version":"123"},"data":{},
 "truncated":false,"nextCursor":null,"hints":[]}
```

Read `hints`: they explain degradation, truncation, and useful next steps.
`--fields a,b` selects top-level keys of `data`. `--limit` and `--cursor` page
lists; continue when `nextCursor` is non-null.

`tokens export` also prints an envelope. Use `--out tokens.json` for a DTCG file;
redirecting stdout saves the wrapper, not a file a token consumer can read.

| Exit | Meaning |
| --- | --- |
| 0 | success |
| 1 | runtime or API error |
| 2 | usage or invalid image input |
| 3 | auth error |
| 4 | file, node, or local path not found |
| 5 | rate limited |
| 6 | partial success: keep `data`, inspect `data.failures` |
| 7 | image mismatch: keep `data`, `data.passed` is false |

In JSON mode, errors carry `error.code` and `error.hint` on stdout. Exit 6 and
7 retain their usable result envelope; do not discard it or retry whole batches.

| Code | Response |
| --- | --- |
| `USAGE` | Correct arguments or image inputs; follow the hint. |
| `AUTH_MISSING`, `AUTH_INVALID`, `AUTH_SCOPE` | Have the user log in with a valid token and the required scopes. |
| `NOT_FOUND`, `FORBIDDEN` | Check the file or node id, local path, and account; follow profile hints. |
| `RATE_LIMITED` | Wait `retryAfterSeconds`, then reuse cached results. |
| `PLAN_REQUIRED` | Use styles and `variables infer` instead of the Enterprise endpoint. |
| `RENDER_FAILED` | Check for a hidden or empty node; choose a renderable frame. |
| `IMAGE_MISMATCH` | Inspect the comparison data and diff image; fix visible differences. |
| `IMAGE_IO` | Fix local file access or output directory; no Figma login is needed. |
| `NETWORK` | Retry once. |
| `INTERNAL` | Report the failure; do not retry in a loop. |

## Request budget

Figma file, node, and render endpoints are Tier 1: as few as 10 requests per
minute on paid seats and 20 per month on view seats. Reuse the cached document
and screenshots. Document reads can still validate freshness; context may need
optional endpoints and uncached assets. Large files can fall back to per-node
requests. Inspect `hints` rather than assuming every repeated call is free.

- Start shallow (`file tree` defaults to depth 2), then narrow the subtree.
- Batch node ids with repeated `--node` where supported instead of calling once
  per node. `--depth` controls inspection detail.
- Avoid `--refresh` and `--no-cache` unless fresh data is needed.
- `figctl cache status` shows cached data. Local `diff` spends no API requests.

## Tokens and measurements

A value's `token` has three distinct states:

- `{"name":"color/brand/500"}`: use that token in code.
- `{"variableId":"...","unresolved":true}`: a binding exists, but its name
  is unavailable. Map to an existing codebase token and explain the mapping;
  `figctl variables infer <ref>` can recover usage and resolved values.
- `null`: the value is unbound. Use the literal value; do not invent a token.

`variables list/get` need Enterprise and `file_variables:read`. On `AUTH_SCOPE`
or `PLAN_REQUIRED`, use `variables infer`; its names are derived from values,
not the designer's names. Token export degrades to styles and reports hints.

`node context` includes Dev Mode `measurements`. Prefer these explicit spacing
decisions over estimated gaps. `freeText` is the designer's override when set.

## Accounts and schemas

Profile selection: `--profile`, `FIGCTL_PROFILE`, `FIGMA_TOKEN` (implicit `env`
profile), `.figctl.yaml`, then the user default. `--token-file` overrides the
credential source. Check `auth status` before reading a client's file.

Never guess payload fields. `figctl schema node.context`, `figctl schema diff`,
and `figctl schema envelope` print their JSON Schemas; `figctl schema` lists them.

Figma writes such as comments and dev resource links require the user's request
or approval; implementing a design alone does not authorize them.

## Reference

Read only the reference needed for the task:

| Need | Reference |
| --- | --- |
| Command flags and examples | [reference/commands.md](reference/commands.md) |
| Output field shapes | [reference/output-schemas.md](reference/output-schemas.md) |
| Token exports, visual comparisons, version comparisons, accounts, or linking Storybook | [reference/workflows.md](reference/workflows.md) |

`figctl <cmd> --help` describes flags and examples. `figctl skill print --file
workflows -o md` prints recipes when reference files are unavailable.
