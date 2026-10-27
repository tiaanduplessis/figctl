# figctl workflows

End to end recipes. Every `<url>` is a Figma URL pasted as given; a `node-id` in
it is used automatically.

## Implement a screen from a URL

```sh
# 1. See where the node sits and what is around it.
figctl file tree "<url>" --depth 2 -o md

# 2. Everything needed to build it, in one call.
figctl node context "<url>" --preset full --screenshot-scale 1 -o md --out ./design/login
```

`node context` writes a PNG of the node and the SVG icons under it, and prints
their absolute paths. Read the PNG with a vision tool before writing code.

What to take from the output:

- `nodes[].inspect.layout` for flex direction, gap, padding, sizing (`fixed`, `hug`,
  `fill`), and alignment, already expressed in CSS terms.
- `nodes[].inspect.visual` and `nodes[].inspect.text` for colors, radius, shadows, and type.
- `tokens` for the design system names. A value whose `token` is null is not
  tokenized; hardcode it and mention that in the summary.
- `components` for the variant properties of every instance, so the props of
  the code component match the design.
- `comments` and `devResources` for designer intent and links to existing code.

```sh
# 3. Capture implementation.png, then compare it with the context PNG.
# Replace design.png with the screenshot path printed by node context.
figctl diff design.png implementation.png --out diff.png --max-diff-ratio 0.01 --json
```

Match viewport, crop, and device scale before comparing. Read the diff image
and handle exit 7 as described in [visual comparison](#compare-an-implementation-with-a-design).

For an agreed frame that needs no new assets or discussion, use
`--preset implement`: depth 2, at most 120 nodes, a 1x screenshot, and skipped
assets/comments/dev resources. Explicit flags override these defaults, such as
`--preset implement --no-comments=false`. `sections` reports complete, skipped,
or failed; skipped discussion does not establish designer approval. Full
context bounds comments/dev resources with `--extras-timeout` (default 5s)
and reuses collaboration responses for one minute. Use `--refresh` for current
intent, even when the design version has not changed.

If the screen is large, cut the output instead of skipping the command:

```sh
figctl node context "<url>" --depth 2 --no-assets --max-nodes 120 -o md
```

## Read structured results without guessing

Read `figctl schema file.find -o md` or `figctl schema comments.list -o md`
before parsing unfamiliar output. These commands return arrays directly in
`data`. Context wraps its normalized model in `nodes[].inspect`.

```sh
figctl file find "<url>" --name "Login*" --json > frames.json
jq -r '.data[] | [.id, .name, .path] | @tsv' frames.json
figctl comments list "<url>" --thread 9001 --limit 20 --json > discussion.json
jq -r '.data[] | [.id, .user, .createdAt, .message] | @tsv' discussion.json
```

`user` is already the person's display name. Keep stderr separate from JSON;
never pipe `2>&1` into `jq`. Pass the same filters with `--cursor` when
`nextCursor` is present. `--thread` accepts either a parent or reply id;
`--resolved true|false` selects the thread state, and `--since 2026-09-01`
selects individual comments created on or after that UTC date. Upstream Figma
still sends the comment file; local filtering and pagination bound agent output.

Under `set -e`, keep usable partial context results and inspect their failures:

```sh
status=0
figctl node context "<url>" --screenshot-scale 1 --json > context.json || status=$?
case "$status" in
  0|6) cat context.json ;;
  *) exit "$status" ;;
esac
```

Run separate simple shell calls if the host cannot verify compound commands.
Use repeated `--node` for supported batches rather than a loop.

## Inspect cache availability

```sh
figctl cache status -o table
figctl cache status --profile acme --file KEY --details --limit 20
```

Default JSON contains file summaries, empty `entries` arrays, and the full
`entryCount`. `--details` pages entries; otherwise `--limit` pages files.
`--profile` and `--file` filter the inventory without contacting Figma.
`--fields profiles` does not trim nested arrays. A recent whole-file network
failure selects targeted requests for two minutes at the same file stamp;
expiry, a changed stamp, or `--refresh` retries the whole document.

## Extract the design system

CSS custom properties, one `:root` block plus a block per extra mode:

```sh
figctl tokens export "<url>" --format css --out ./src/styles
figctl tokens export "<url>" --format css --mode Dark --out ./src/styles
```

Tailwind theme module:

```sh
figctl tokens export "<url>" --format tailwind --tailwind-vars --out ./src
```

Portable DTCG (Style Dictionary v4 reads it unchanged):

```sh
figctl tokens export "<url>" --out tokens.json
figctl tokens export "<url>" --mode-strategy separate --out ./tokens
```

Notes:

- Stdout is the figctl envelope. `--out` writes the token document itself;
  redirecting stdout saves the wrapper instead of a consumable DTCG file.
- Without an Enterprise plan the export contains styles only and says so in
  `hints`. That is a plan limit, not a broken token: the `file_variables:read`
  scope is offered only inside an Enterprise organization and does not appear
  on the token screen otherwise. Ship the styles; they are still real.
- On that plan, recover the variables from how they are used:

  ```sh
  figctl variables infer "<ref>" --min-usages 20
  ```

  The bindings are readable even when the names are not, so this reports which
  values a variable governs, how widely, and what each resolves to. The names
  it prints are derived from the values; use them to map onto the tokens
  already in the codebase, and never present them as the design system's own
  names.
- A style defined twice with the same value is merged. Defined twice with
  different values, the first wins and `hints` names the conflict, which is
  worth passing on to the designer rather than silently picking one.
- `--name-case kebab` (the default) makes `Color/Brand/500` into
  `--color-brand-500`. Use `--name-case none` to keep the Figma names.
- Filter noise with `--collection Semantic --exclude-remote --exclude-hidden`.
- Resolve one token, with its alias chain, without exporting everything:

  ```sh
  figctl tokens resolve "<url>" --name color/text/primary --mode Dark
  ```

## Compare an implementation with a design

```sh
figctl render "<url>" --scale 1 --out ./design --json > render.json
figctl diff --manifest render.json implementation.png --out diff.png \
  --threshold 0.1 --max-diff-ratio 0.01 --json
```

Reuse the screenshot from `node context --screenshot-scale 1` if already
available; render separately only when another capture is needed. Replace
`design.png` with that screenshot path or the PNG path from the render manifest.
`--manifest` accepts the JSON envelope from either `render` or `node context`,
selects the exact successful PNG path, and retains reference provenance in the
diff result. For several captures, pass `--node <id>`. A manifest with no
successful PNG or an ambiguous selection is an input error.

The capture reports actual `pixelWidth`/`pixelHeight` for PNG/JPG alongside
Figma `bounds`, `renderBounds`, and the authored `background` color when
available. Bounds are design coordinates, not a promise about raster crop.
The authored background is not an inferred/composited pixel background. Use
actual pixel dimensions for the browser capture; never resize to hide a mismatch.

Capture `implementation.png` with
browser tools at the same viewport, crop, and device scale. Wait for fonts and
images to load; keep content and animation state consistent. `render` defaults
to 2x, so explicitly use 1x when the browser screenshot is 1x.

`diff` runs offline and accepts equal-sized PNGs up to 16 million pixels each.
It never resizes inputs. `--threshold` is color tolerance per pixel;
`--max-diff-ratio 0.01` accepts up to 1% mismatched pixels. The default accepted
ratio is zero. Detected anti-aliasing is ignored; use `--include-aa` to count it.
Transparency is compared over a checkerboard.

Exit 0 means the ratio is within the limit; exit 7 means it exceeds the limit.
Both print comparison data (`passed`, `mismatchedPixels`, `totalPixels`,
`mismatchRatio`, dimensions, settings, and optional `diffPath`). Under `set -e`,
handle exit 7 explicitly so the agent can inspect the result and continue.
For example, this preserves the result while stopping on real errors:

```sh
status=0
figctl diff design.png implementation.png --out diff.png --json > comparison.json || status=$?
case "$status" in
  0|7) cat comparison.json ;;
  *) exit "$status" ;;
esac
```

Other nonzero exits are input or I/O errors, not completed comparisons.

`--out` writes the diff on pass or fail, replacing an existing output file.
Its parent directory must exist. Input paths, including links to inputs, are
rejected as output destinations. Omit it when only the metrics are needed.
Read the diff with a vision tool: red marks mismatches, yellow marks ignored
anti-aliasing, and unchanged regions are faded. Fix the highlighted differences
and repeat; the mismatch ratio alone is not a design-quality score.

## Export the icons of a page

```sh
# What is there, without spending an image request.
figctl assets list "<url>" --node 3:1 -o table

# Export them.
figctl assets export "<url>" --node 3:1 --icons-only \
  --out ./src/assets/icons \
  --svg-current-color --svg-strip-dimensions --svg-strip-ids
```

`--svg-current-color` makes single-color icons inherit the text color, which is
almost always what a component library wants. The command writes a
`manifest.json` next to the files mapping node ids to paths, and prints the same
manifest. Renders are batched per format and scale and cached by file version,
so re-running on an unchanged file costs no image requests. `--dry-run` prints
the plan without image requests or downloads; discovering the nodes can still
need file data from Figma.

If the icons are not named `icon/*`, `Icon*`, or `icons/*`, pass the real
convention: `--icon-pattern "ic/*"`.

## Link a component library to Storybook

Dev resource links show up in Figma's Dev Mode beside the design, so a designer
opening a component sees the story that implements it. Use this workflow when
the user has requested publishing those links; implementing a design alone
does not authorize Figma writes.

Link one component:

```
figctl devresources add "<url>" --node 3:11 \
  --url "https://storybook.example.com/?path=/story/button--primary" \
  --name "Storybook"
```

Link a whole library in one call. Build a JSON array of `{nodeId, name, url}`
from your story index and pipe it in:

```
figctl devresources add KEY --from-file stories.json --yes
```

Check the plan before writing to a shared file:

```
figctl devresources add KEY --from-file stories.json --dry-run
```

Figma accepts at most 10 links per node and rejects a URL a node already
carries. A bulk write can therefore partly succeed: the command exits 6, the
links that landed are in `data.written`, and the rejected ones are in
`data.failures` with the reason. That is not a reason to retry the whole batch.

To find the node ids to link, list the components first:

```
figctl components list KEY --json
```

Existing links are read with `figctl devresources list KEY`, changed with
`figctl devresources update KEY --id <id> --url <new>`, and deleted with
`figctl devresources remove KEY --id <id>`. Writes need `file_dev_resources:write`
on the token and edit access to the file, and they prompt unless `--yes` is
passed.

## Find every screen that uses a component

```sh
# 1. Find the component and its node id.
figctl components list "<url>" --query "Button" -o table

# 2. Find its instances by name; instance names default to the component name.
figctl file find "<url>" --name "Button*" --type INSTANCE -o md
```

`file find` prints the ancestor path of each hit, so the page and frame each
instance sits in are visible without another call. The search reuses the cached
document; freshness checks can still need a metadata request.

To see how one instance is configured, inspect it: `component.componentProperties`
holds the variant values.

```sh
figctl node inspect "<url>" --node 4:11 --depth 0 -o md
```

## Check what changed between two file versions

```sh
# 1. List the saved versions, newest first.
figctl versions list "<url>" -o table

# 2. Read the same node at each version and diff the two.
figctl node inspect "<url>" --node 2:2 --file-version 2100120000 --json > /tmp/before.json
figctl node inspect "<url>" --node 2:2 --file-version 2100999000 --json > /tmp/after.json
diff <(jq -S . /tmp/before.json) <(jq -S . /tmp/after.json)
```

`--file-version` works on every read command, and each version is cached
separately, so the second read of a version is free. The same trick compares
`file tree` output to find nodes that were added or removed, and
`tokens export` output to find design system changes.

## Work across two client accounts

One profile per account, one `.figctl.yaml` per repository:

```sh
figctl auth login --profile acme
figctl auth login --profile globex

cd ~/code/acme-web && figctl init --profile acme
cd ~/code/globex-app && figctl init --profile globex
```

`.figctl.yaml` holds no secret and is meant to be committed. After that every
command in the repository uses the right account with no flag, and the envelope
echoes which one:

```sh
figctl auth status
figctl file info "<url>" --json | jq .profile
```

Override for one call with `--profile globex`, or for a shell with
`FIGCTL_PROFILE=globex`. `--profile` wins over `FIGCTL_PROFILE`, and both win
over `FIGMA_TOKEN`. With neither set, `FIGMA_TOKEN` selects the implicit
profile named `env`, which CI can use without a stored account. `--token-file`
overrides the credential source for the selected profile.

A `FORBIDDEN` or `NOT_FOUND` on a file that clearly exists usually means the
wrong account: the hint lists the other configured profiles. The cache is keyed
by profile first, so one client's data is never served to another, and
`figctl cache clear --profile acme` clears just one.
