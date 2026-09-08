---
name: figctl
description: Read Figma designs from the shell with figctl - outline a file, get one node's layout, styles and design tokens, render screenshots, compare local PNGs, export icons, and export the design system. Use when a task mentions Figma, a figma.com URL, a design file, design tokens, or implementing a screen from a design.
allowed-tools: Bash(figctl:*)
---

# figctl

figctl reads a Figma file and returns what is needed to implement the design in
code. It is a single binary run from the shell: no server to start, no tool
schemas in every turn.

For Figma API commands, confirm it is installed and pointed at the right account:

```
figctl auth status
```

## Refs: a name, a URL, or nothing at all

Every command that reads a file takes a `<ref>`, and there are three forms.

Read `.figctl.yaml` first. A project usually names its files, because a design
system lives in a file of its own, and a name is a ref:

```
figctl file tree design-system          # a configured name
figctl node context --node 2:2          # nothing: the repository's default
figctl file tree "https://www.figma.com/design/KEY/App?node-id=2-2"
```

An unknown name fails and lists the configured ones. A key or URL always works
whether or not the repository is configured. When a URL carries `node-id=1-2`,
figctl converts it to `1:2` and uses it as the default `--node`, so paste the
URL as given: converting it by hand is the most common mistake and is never
needed. `--node` accepts `1:2`, `1-2`, or a URL.

## Workflow

1. Orient. One cheap outline of the file; find the node id to work on.
   ```
   figctl file tree "<url>" -o md
   figctl file find "<url>" --name "Login*"
   ```
2. Get the screen. Layout, styles, tokens, screenshot, icons, comments, and dev
   links for one node in a single call.
   ```
   figctl node context "<url>" --node 2:2 -o md
   ```
   The output lists absolute file paths. Read the PNG it wrote with a vision
   tool to see the design.
3. Implement in the codebase, using the token names from step 2 rather than the
   raw values.
4. Compare. Render at the browser screenshot scale (1x in this example).
   ```
   figctl render "<url>" --node 2:2 --scale 1 --out ./design
   figctl diff design.png implementation.png --out diff.png --max-diff-ratio 0.01
   ```
   Use the render manifest path and a browser capture at the same viewport and crop.
   Read the diff PNG, fix the highlighted differences, and repeat.
5. Wire up the design system once per project.
   ```
   figctl tokens export "<url>" --format css --out ./src/styles
   ```

## Output

Output is JSON when stdout is piped, which is what a tool call gets, and a table
on a terminal. `--json` forces JSON. `--output md` gives markdown.

Prefer `-o md` for `node context`, `node inspect`, and `file tree` when reading
the output as text: it is the same data with far fewer tokens. Use JSON when
piping into `jq` or a script.

Every success response is one envelope:

```json
{"schemaVersion":1,"command":"node.context","profile":{"name":"acme"},
 "file":{"key":"KEY","name":"App","version":"123"},"data":{},
 "truncated":false,"nextCursor":null,"hints":[]}
```

Read `hints` on every call: they say what was degraded, truncated, or what to
run next. `--fields a,b` keeps only the named top level keys of `data`.
`--limit` and `--cursor` page list output; `nextCursor` is non-null when more
rows exist.

## Exit codes and errors

| Exit | Meaning |
| --- | --- |
| 0 | success |
| 1 | runtime or API error |
| 2 | usage error |
| 3 | auth error |
| 4 | file or node not found |
| 5 | rate limited |
| 6 | partial success: `data` is on stdout, check `data.failures` |
| 7 | image mismatch: comparison completed, `data.passed` is false |

Exit 6 is not a failure to retry. The good results are already on stdout; the
items that failed are listed in `data.failures`.

Errors are an envelope on stdout with a stable `code` and a `hint`. Switch on
the code and follow the hint.

| Code | Do this |
| --- | --- |
| `USAGE` | Fix the arguments; the hint names the valid values. |
| `AUTH_MISSING` | No token. Run `figctl auth login --profile <name>`. |
| `AUTH_INVALID` | Token rejected or expired (Figma tokens last 90 days at most). Ask for a new one. |
| `AUTH_SCOPE` | The token lacks the scope named in the hint. Ask for a token with it. |
| `NOT_FOUND` | The hint says whether the file or the node is missing. Re-check the id with `figctl file tree`. |
| `FORBIDDEN` | Wrong account for this file. The hint lists the other profiles; retry with `--profile <name>`. |
| `RATE_LIMITED` | Wait `retryAfterSeconds`, then reuse the cache instead of re-fetching. |
| `PLAN_REQUIRED` | Variables need Enterprise. Use `figctl styles list` and the raw values. |
| `RENDER_FAILED` | The node is hidden or empty. Pick a different node. |
| `NETWORK` | Retry once. |
| `INTERNAL` | Report it; do not retry in a loop. |

## Rate limits

This is the real constraint. `file`, `nodes`, and image endpoints are Tier 1 at
Figma: as low as 10 to 30 requests per minute on paid seats, and 20 per month on
view seats. Burning them ends the session.

figctl caches the whole file per version, so after the first fetch repeated
`file tree`, `file find`, `node inspect`, and `node context` calls on the same
file cost no requests.

- Do not loop over nodes with one call each. Pass `--node` several times, or
  widen `--depth`, and get them all in one call.
- Do not pass `--no-cache` or `--refresh` unless the file changed during the
  session.
- Start shallow (`file tree`, default depth 2), then drill into the node ids
  that matter.
- `figctl cache status` shows what is already local and therefore free.

## Design tokens

Every value reports a `token`, and its three states mean different things:

- `{"name": "color/brand/500"}`: use that design token in code.
- `{"variableId": "...", "unresolved": true}`: a variable governs this value,
  but reading its name needs an Enterprise plan. Do not hardcode it as if it
  were ad hoc; use the nearest token in the codebase and say where it came
  from. `figctl variables infer` recovers what these govern.
- `null`: the designer typed the value by hand. Hardcode it, and say so.

## Variables without an Enterprise plan

When `variables list` fails with `AUTH_SCOPE` or `PLAN_REQUIRED`, use
`figctl variables infer <ref>` instead. It walks the file and reports which
variables govern which values, how widely each is used, and what each resolves
to, none of which needs the Enterprise endpoint.

The names it reports are derived from the values, not the designer's names.
Treat them as identifiers for mapping onto the tokens already in the codebase,
and do not present them to a person as the design system's own names.

## Measurements

`node context` reports `measurements`: distances the designer pinned in Dev
Mode, resolved to pixels. They are a spacing decision stated outright, so use
them in preference to a gap inferred from layout or estimated from the
screenshot. When `freeText` is set the designer overrode the measured value,
and that override is what the design specifies.

Variables need an Enterprise plan and the `file_variables:read` scope. On any
other plan the variables commands degrade to styles plus raw values and say so
in `hints`. That is expected, not an error to work around.

## Profiles

A profile selects which Figma account is used. The active one is picked, in
order, from `--profile`, `FIGCTL_PROFILE`, `FIGMA_TOKEN`, a `.figctl.yaml` in
the repository, then the user default.

Every response echoes `profile` in the envelope. In a client repository, check
it before doing work:

```
figctl auth status
```

If a file returns `FORBIDDEN` or `NOT_FOUND`, the hint lists the other
configured profiles; the file probably belongs to one of them.

## Field names

Never guess a field name. Print the JSON Schema of a command's `data` payload:

```
figctl schema node.context
figctl schema envelope
figctl schema
```

## Reference

| Question | File |
| --- | --- |
| Which command, which flag, what does it do? | `reference/commands.md` |
| What fields does the output have? | `reference/output-schemas.md` |
| How do I do a whole task end to end? | `reference/workflows.md` |

`figctl <cmd> --help` carries the same flags and examples as
`reference/commands.md`, and `figctl skill print --file workflows` prints the
recipes without a file on disk.
