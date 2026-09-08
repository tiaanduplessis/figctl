# figctl workflows

End to end recipes. Every `<url>` is a Figma URL pasted as given; a `node-id` in
it is used automatically.

## Implement a screen from a URL

```sh
# 1. See where the node sits and what is around it.
figctl file tree "<url>" --depth 2 -o md

# 2. Everything needed to build it, in one call.
figctl node context "<url>" -o md --out ./design/login
```

`node context` writes a PNG of the node and the SVG icons under it, and prints
their absolute paths. Read the PNG with a vision tool before writing code.

What to take from the output:

- `nodes[].layout` for flex direction, gap, padding, sizing (`fixed`, `hug`,
  `fill`), and alignment, already expressed in CSS terms.
- `nodes[].visual` and `nodes[].text` for colors, radius, shadows, and type.
- `tokens` for the design system names. A value whose `token` is null is not
  tokenized; hardcode it and mention that in the summary.
- `components` for the variant properties of every instance, so the props of
  the code component match the design.
- `comments` and `devResources` for designer intent and links to existing code.

```sh
# 3. After implementing, render again and compare the two images.
figctl render "<url>" --out ./design/after
```

If the screen is large, cut the output instead of skipping the command:

```sh
figctl node context "<url>" --depth 2 --no-assets --max-nodes 120 -o md
```

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
figctl tokens export "<url>" > tokens.json
figctl tokens export "<url>" --mode-strategy separate --out ./tokens
```

Notes:

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
the plan with no request at all.

If the icons are not named `icon/*`, `Icon*`, or `icons/*`, pass the real
convention: `--icon-pattern "ic/*"`.

## Link a component library to Storybook

Dev resource links show up in Figma's Dev Mode beside the design, so a designer
opening a component sees the story that implements it. This is the one write
worth doing from a design system pipeline.

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
instance sits in are visible without another call. Both calls are served from
the one cached document, so the search is free after the first fetch.

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
`FIGCTL_PROFILE=globex`. `FIGMA_TOKEN` wins over both and creates an implicit
profile named `env`, which is what CI should use.

A `FORBIDDEN` or `NOT_FOUND` on a file that clearly exists usually means the
wrong account: the hint lists the other configured profiles. The cache is keyed
by profile first, so one client's data is never served to another, and
`figctl cache clear --profile acme` clears just one.
