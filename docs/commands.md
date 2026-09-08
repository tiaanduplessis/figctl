# figctl command reference

Generated from the command tree by `make docs`. Do not edit by hand.

Every command accepts the [global flags](#global-flags). Output is JSON when
stdout is not a terminal and a table when it is; `--json` forces JSON. See
[the output contract](../README.md#output-contract) for the envelope, exit
codes, and error codes.

## Commands

| Command | Description |
| --- | --- |
| [`figctl assets`](#figctl-assets) | Find and export icons, export-marked layers, and image fills |
| [`figctl assets export`](#figctl-assets-export) | Export icons as SVG, export-marked layers, and image fills to a directory |
| [`figctl assets list`](#figctl-assets-list) | List export candidates under a node without rendering anything |
| [`figctl auth`](#figctl-auth) | Log in, log out, check status, and list required scopes |
| [`figctl auth login`](#figctl-auth-login) | Store a token for a profile (same as profile add) |
| [`figctl auth logout`](#figctl-auth-logout) | Remove the stored token for a profile, keeping its settings |
| [`figctl auth scopes`](#figctl-auth-scopes) | List the token scopes figctl uses and which commands need them |
| [`figctl auth status`](#figctl-auth-status) | Show the active profile and how it was selected |
| [`figctl cache`](#figctl-cache) | Inspect and clear the on-disk response cache |
| [`figctl cache clear`](#figctl-cache-clear) | Delete cached data for every profile, one profile, or one file |
| [`figctl cache status`](#figctl-cache-status) | Show cache size and entries per profile and file |
| [`figctl comments`](#figctl-comments) | Read designer comments and leave implementation notes |
| [`figctl comments add`](#figctl-comments-add) | Add a comment, pinned on a node or at canvas coordinates |
| [`figctl comments list`](#figctl-comments-list) | List comments, optionally only those pinned on a node |
| [`figctl completion`](#figctl-completion) | Generate the autocompletion script for the specified shell |
| [`figctl completion bash`](#figctl-completion-bash) | Generate the autocompletion script for bash |
| [`figctl completion fish`](#figctl-completion-fish) | Generate the autocompletion script for fish |
| [`figctl completion powershell`](#figctl-completion-powershell) | Generate the autocompletion script for powershell |
| [`figctl completion zsh`](#figctl-completion-zsh) | Generate the autocompletion script for zsh |
| [`figctl components`](#figctl-components) | List and inspect components, component sets, and variant properties |
| [`figctl components get`](#figctl-components-get) | Show one component or component set with its property definitions and variants |
| [`figctl components list`](#figctl-components-list) | List the components of a file or a team library |
| [`figctl devresources`](#figctl-devresources) | Dev Mode resource links attached to nodes |
| [`figctl devresources add`](#figctl-devresources-add) | Attach a link to a node, for example a Storybook story or a pull request |
| [`figctl devresources list`](#figctl-devresources-list) | List dev resources, optionally for specific nodes |
| [`figctl devresources remove`](#figctl-devresources-remove) | Delete links from a file |
| [`figctl devresources update`](#figctl-devresources-update) | Change the name or URL of existing links |
| [`figctl file`](#figctl-file) | Read a Figma file: info, tree, find, and raw JSON |
| [`figctl file find`](#figctl-file-find) | Search nodes by name glob, type, or text content |
| [`figctl file get`](#figctl-file-get) | Print raw Figma API JSON for the document or specific nodes (escape hatch) |
| [`figctl file info`](#figctl-file-info) | Show file name, pages, version, editor type, role, and branches |
| [`figctl file tree`](#figctl-file-tree) | Sparse outline of pages, frames, and layers with ids, sizes, and flags |
| [`figctl folders`](#figctl-folders) | Discover folders and files (v2 API) |
| [`figctl folders files`](#figctl-folders-files) | List the files in a folder |
| [`figctl folders list`](#figctl-folders-list) | List the top level folders of a team, or the subfolders of a folder |
| [`figctl init`](#figctl-init) | Write .figctl.yaml in the current directory with the profile to use |
| [`figctl me`](#figctl-me) | Show the user the active token belongs to |
| [`figctl node`](#figctl-node) | Inspect nodes: resolved layout, styles, typography, tokens, and CSS |
| [`figctl node context`](#figctl-node-context) | Everything needed to implement one node: inspect, screenshot, assets, tokens, components, comments |
| [`figctl node inspect`](#figctl-node-inspect) | Resolved layout, visual, text, tokens, and component info for nodes |
| [`figctl profile`](#figctl-profile) | Manage named accounts (one per client or organization) |
| [`figctl profile add`](#figctl-profile-add) | Add or update a profile and store its token |
| [`figctl profile list`](#figctl-profile-list) | List profiles |
| [`figctl profile remove`](#figctl-profile-remove) | Remove a profile and its stored token |
| [`figctl profile show`](#figctl-profile-show) | Show a profile (the active one when no name is given), token redacted |
| [`figctl profile use`](#figctl-profile-use) | Set the default profile |
| [`figctl projects`](#figctl-projects) | Discover projects and files (v1 API, deprecated in favor of folders) |
| [`figctl projects files`](#figctl-projects-files) | List the files in a project |
| [`figctl projects list`](#figctl-projects-list) | List the projects of a team |
| [`figctl render`](#figctl-render) | Render nodes to PNG, JPG, SVG, or PDF files |
| [`figctl schema`](#figctl-schema) | JSON Schema of a command's data payload |
| [`figctl skill`](#figctl-skill) | Install the figctl agent skill into a coding agent |
| [`figctl skill install`](#figctl-skill-install) | Write the skill files for one or more agents |
| [`figctl skill print`](#figctl-skill-print) | Print a skill document to stdout |
| [`figctl skill uninstall`](#figctl-skill-uninstall) | Remove the skill files or marked blocks |
| [`figctl styles`](#figctl-styles) | Styles with resolved values: fills, typography, effects, and grids |
| [`figctl styles get`](#figctl-styles-get) | Show one style with its full resolved value |
| [`figctl styles list`](#figctl-styles-list) | List styles with metadata and resolved values |
| [`figctl tokens`](#figctl-tokens) | Design tokens: resolve variables and styles, export as DTCG, CSS, or Tailwind |
| [`figctl tokens export`](#figctl-tokens-export) | Export variables and styles as DTCG, CSS custom properties, or a Tailwind theme |
| [`figctl tokens resolve`](#figctl-tokens-resolve) | Resolve one variable or style to concrete values across modes |
| [`figctl variables`](#figctl-variables) | Raw variables and collections with values resolved per mode (Enterprise) |
| [`figctl variables get`](#figctl-variables-get) | Show one variable with its alias chain per mode |
| [`figctl variables infer`](#figctl-variables-infer) | Reconstruct the variables a file uses from how they are used |
| [`figctl variables list`](#figctl-variables-list) | List variables with values per mode, code syntax, scopes, and descriptions |
| [`figctl version`](#figctl-version) | Print the figctl version |
| [`figctl versions`](#figctl-versions) | List saved versions of a file |
| [`figctl versions list`](#figctl-versions-list) | List versions, newest first |

## Global flags

These persistent flags are accepted by every command and are not repeated below.

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--cursor` | `string` | - | pagination cursor taken from a previous nextCursor |
| `--fields` | `string` | - | comma separated top level fields to keep in data |
| `--file-version` | `string` | - | pin a file version id |
| `--json` | `bool` | `false` | alias for --output json |
| `--limit` | `int` | `0` | maximum items in list output (0 uses the command default) |
| `--max-file-mb` | `int` | `200` | largest file document (MB) fetched whole and cached; bigger files use per-node requests |
| `--no-cache` | `bool` | `false` | bypass the disk cache |
| `--no-color` | `bool` | `false` | disable color (NO_COLOR and TERM=dumb are honored too) |
| `--profile` | `string` | - | profile name (overrides FIGCTL_PROFILE and .figctl.yaml) |
| `--refresh` | `bool` | `false` | refresh the disk cache |
| `--timeout` | `duration` | `1m0s` | request timeout |
| `--token-file` | `string` | - | read the token from this file instead of the credential store |
| `--yes` | `bool` | `false` | skip confirmations |
| `-o`, `--output` | `string` | - | output format: json, md, table, or plain (default: json when piped, table on a terminal) |
| `-q`, `--quiet` | `bool` | `false` | no progress or warnings on stderr |
| `-v`, `--verbose` | `bool` | `false` | HTTP trace on stderr (tokens redacted) |

Configuration precedence is flags, then environment (`FIGMA_TOKEN`, `FIGCTL_PROFILE`, `NO_COLOR`),
then the project config `.figctl.yaml`, then the user config, then the built-in defaults.

## figctl assets

Find and export icons, export-marked layers, and image fills

```
figctl assets [command]
```

Subcommands:

- [`figctl assets export`](#figctl-assets-export) Export icons as SVG, export-marked layers, and image fills to a directory
- [`figctl assets list`](#figctl-assets-list) List export candidates under a node without rendering anything

Plus the [global flags](#global-flags).

## figctl assets export

Export icons as SVG, export-marked layers, and image fills to a directory

```
figctl assets export <ref> [flags]
```

Discover assets under the given nodes (the whole file when no --node is
given) and write them to the output directory: icons as SVG (or --format
png), export-marked layers at the format, scale, and suffix stored in Figma,
and raster image fills as imageref-<ref>.<ext>. A manifest.json mapping
node ids and image refs to paths is written next to the files, and the same
manifest is printed.

Renders are batched per format and scale and cached by file version, so a
repeated export of an unchanged file costs no image requests. --dry-run
prints the plan without any image request.

Examples:

```sh
figctl assets export KEY --node 2:2 --out ./src/assets
figctl assets export KEY --icons-only --svg-current-color --svg-strip-dimensions
figctl assets export KEY --dry-run
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--concurrency` | `int` | `8` | maximum simultaneous downloads |
| `--dry-run` | `bool` | `false` | print what would be exported without any image request |
| `--exports-only` | `bool` | `false` | export layers with export settings only |
| `--fills-only` | `bool` | `false` | download image fills only |
| `--format` | `string` | `svg` | icon format: svg or png |
| `--icon-pattern` | `string (repeatable)` | - | name glob that marks an icon (default icon/*, Icon*, icons/*); repeatable |
| `--icons-only` | `bool` | `false` | export icons only |
| `--include-hidden` | `bool` | `false` | include hidden nodes |
| `--node` | `string (repeatable)` | - | node id (1:2, 1-2, or a URL with node-id) to search under; repeatable |
| `--out` | `string` | - | output directory (default ./.figctl/<fileKey>/assets) |
| `--scale` | `float` | `0` | icon scale (default 1 for svg, 2 for png) |
| `--svg-current-color` | `bool` | `false` | svg: replace a single-color icon's fill and stroke with currentColor |
| `--svg-strip-dimensions` | `bool` | `false` | svg: remove root width and height when a viewBox exists |
| `--svg-strip-ids` | `bool` | `false` | svg: remove unreferenced id attributes |

Plus the [global flags](#global-flags).

## figctl assets list

List export candidates under a node without rendering anything

```
figctl assets list <ref> [flags]
```

List the assets found under the given nodes (the whole file when no --node is
given): layers with export settings, icon-like nodes (name matches an icon
pattern, or a frame made only of vector shapes), and raster image fills.
Nothing is rendered or downloaded.

Examples:

```sh
figctl assets list KEY
figctl assets list KEY --node 2:2 --icon-pattern "ic/*" -o table
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--icon-pattern` | `string (repeatable)` | - | name glob that marks an icon (default icon/*, Icon*, icons/*); repeatable |
| `--include-hidden` | `bool` | `false` | include hidden nodes |
| `--node` | `string (repeatable)` | - | node id (1:2, 1-2, or a URL with node-id) to search under; repeatable |

Plus the [global flags](#global-flags).

## figctl auth

Log in, log out, check status, and list required scopes

```
figctl auth [command]
```

Subcommands:

- [`figctl auth login`](#figctl-auth-login) Store a token for a profile (same as profile add)
- [`figctl auth logout`](#figctl-auth-logout) Remove the stored token for a profile, keeping its settings
- [`figctl auth scopes`](#figctl-auth-scopes) List the token scopes figctl uses and which commands need them
- [`figctl auth status`](#figctl-auth-status) Show the active profile and how it was selected

Plus the [global flags](#global-flags).

## figctl auth login

Store a token for a profile (same as profile add)

```
figctl auth login [name] [flags]
```

Store a token for a profile. The profile name comes from the argument or
--profile. The token is read from --token-file, from stdin when stdin is not
a terminal, or from a hidden prompt on a terminal.

Examples:

```sh
figctl auth login --profile acme --expires 2026-12-01
printf '%s' "$TOKEN" | figctl auth login acme --default
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--base-url` | `string` | - | API base URL, for example https://api-figma-gov.com |
| `--default` | `bool` | `false` | make this the default profile |
| `--expires` | `string` | - | token expiry date as YYYY-MM-DD (Figma tokens expire after at most 90 days) |
| `--team` | `string` | - | default team id for this profile |

Plus the [global flags](#global-flags).

## figctl auth logout

Remove the stored token for a profile, keeping its settings

```
figctl auth logout [name]
```

Examples:

```sh
figctl auth logout
figctl auth logout acme
```

Plus the [global flags](#global-flags).

## figctl auth scopes

List the token scopes figctl uses and which commands need them

```
figctl auth scopes
```

Examples:

```sh
figctl auth scopes
```

Plus the [global flags](#global-flags).

## figctl auth status

Show the active profile and how it was selected

```
figctl auth status
```

Examples:

```sh
figctl auth status
figctl auth status --profile acme --json
```

Plus the [global flags](#global-flags).

## figctl cache

Inspect and clear the on-disk response cache

```
figctl cache [command]
```

Subcommands:

- [`figctl cache clear`](#figctl-cache-clear) Delete cached data for every profile, one profile, or one file
- [`figctl cache status`](#figctl-cache-status) Show cache size and entries per profile and file

Plus the [global flags](#global-flags).

## figctl cache clear

Delete cached data for every profile, one profile, or one file

```
figctl cache clear [flags]
```

Delete cached data. Without flags every profile's cache is removed. Use
--profile to clear one account and --file to clear one file. Requires
confirmation on a terminal and --yes otherwise.

Examples:

```sh
figctl cache clear --yes
figctl cache clear --profile acme --file KEY --yes
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--file` | `string` | - | only clear this file key or Figma URL |

Plus the [global flags](#global-flags).

## figctl cache status

Show cache size and entries per profile and file

```
figctl cache status
```

Examples:

```sh
figctl cache status
figctl cache status --json
```

Plus the [global flags](#global-flags).

## figctl comments

Read designer comments and leave implementation notes

```
figctl comments [command]
```

Subcommands:

- [`figctl comments add`](#figctl-comments-add) Add a comment, pinned on a node or at canvas coordinates
- [`figctl comments list`](#figctl-comments-list) List comments, optionally only those pinned on a node

Plus the [global flags](#global-flags).

## figctl comments add

Add a comment, pinned on a node or at canvas coordinates

```
figctl comments add <ref> <message|-> [flags]
```

Add a comment to a file. Pass - as the message to read it from stdin. With
--node the comment is pinned on that node at the offset given by --x and
--y; without --node, --x and --y are absolute canvas coordinates. This is
the only write figctl performs, so it asks for confirmation on a terminal
and requires --yes otherwise.

Examples:

```sh
figctl comments add KEY "Implemented in src/Login.tsx" --node 1:2 --yes
echo "Note" | figctl comments add KEY - --x 120 --y 320 --yes
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--node` | `string` | - | pin the comment on this node id |
| `--x` | `float` | `0` | x offset inside the node, or canvas x without --node |
| `--y` | `float` | `0` | y offset inside the node, or canvas y without --node |

Plus the [global flags](#global-flags).

## figctl comments list

List comments, optionally only those pinned on a node

```
figctl comments list <ref> [flags]
```

Examples:

```sh
figctl comments list KEY
figctl comments list KEY --node 1:2 --md
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--md` | `bool` | `false` | return messages as markdown (as_md) |
| `--node` | `string` | - | only comments pinned on this node id |

Plus the [global flags](#global-flags).

## figctl completion

Generate the autocompletion script for the specified shell

```
figctl completion [command]
```

Generate the autocompletion script for figctl for the specified shell.
See each sub-command's help for details on how to use the generated script.

Subcommands:

- [`figctl completion bash`](#figctl-completion-bash) Generate the autocompletion script for bash
- [`figctl completion fish`](#figctl-completion-fish) Generate the autocompletion script for fish
- [`figctl completion powershell`](#figctl-completion-powershell) Generate the autocompletion script for powershell
- [`figctl completion zsh`](#figctl-completion-zsh) Generate the autocompletion script for zsh

Plus the [global flags](#global-flags).

## figctl completion bash

Generate the autocompletion script for bash

```
figctl completion bash
```

```
Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(figctl completion bash)

To load completions for every new session, execute once:

#### Linux:

	figctl completion bash > /etc/bash_completion.d/figctl

#### macOS:

	figctl completion bash > $(brew --prefix)/etc/bash_completion.d/figctl

You will need to start a new shell for this setup to take effect.
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--no-descriptions` | `bool` | `false` | disable completion descriptions |

Plus the [global flags](#global-flags).

## figctl completion fish

Generate the autocompletion script for fish

```
figctl completion fish [flags]
```

```
Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	figctl completion fish | source

To load completions for every new session, execute once:

	figctl completion fish > ~/.config/fish/completions/figctl.fish

You will need to start a new shell for this setup to take effect.
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--no-descriptions` | `bool` | `false` | disable completion descriptions |

Plus the [global flags](#global-flags).

## figctl completion powershell

Generate the autocompletion script for powershell

```
figctl completion powershell [flags]
```

```
Generate the autocompletion script for powershell.

To load completions in your current shell session:

	figctl completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--no-descriptions` | `bool` | `false` | disable completion descriptions |

Plus the [global flags](#global-flags).

## figctl completion zsh

Generate the autocompletion script for zsh

```
figctl completion zsh [flags]
```

```
Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(figctl completion zsh)

To load completions for every new session, execute once:

#### Linux:

	figctl completion zsh > "${fpath[1]}/_figctl"

#### macOS:

	figctl completion zsh > $(brew --prefix)/share/zsh/site-functions/_figctl

You will need to start a new shell for this setup to take effect.
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--no-descriptions` | `bool` | `false` | disable completion descriptions |

Plus the [global flags](#global-flags).

## figctl components

List and inspect components, component sets, and variant properties

```
figctl components [command]
```

Subcommands:

- [`figctl components get`](#figctl-components-get) Show one component or component set with its property definitions and variants
- [`figctl components list`](#figctl-components-list) List the components of a file or a team library

Plus the [global flags](#global-flags).

## figctl components get

Show one component or component set with its property definitions and variants

```
figctl components get <ref> [flags]
```

Show a component or component set by node id or by published key. A
component set lists its componentPropertyDefinitions (type, default,
variant options, preferred values) and its variant components. A component
shows its set, its size, and its own definitions when it has any.

Examples:

```sh
figctl components get KEY --node 3:10
figctl components get KEY --key a1b2c3d4e5f60718293a4b5c6d7e8f9012345311
figctl components get "https://www.figma.com/design/KEY/App?node-id=3-20" -o md
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--key` | `string` | - | published component or component set key |
| `--node` | `string` | - | component or component set node id (1:2, 1-2, or a URL with node-id) |

Plus the [global flags](#global-flags).

## figctl components list

List the components of a file or a team library

```
figctl components list [ref] [flags]
```

List components with their keys, node ids, component set, variant
properties parsed from the name, page, documentation links, and whether
they are remote. For a file the published components are merged with the
components of the cached document, so unpublished ones appear too with
"published": false. With --team the team library endpoint is paged with
--limit and --cursor. --query keeps components whose name, description, or
set name contains the text. "components search" is an alias of
"components list".

Aliases: `search`

Examples:

```sh
figctl components list KEY
figctl components search KEY --query button
figctl components list --team 555000111 --limit 50
figctl components list --team 555000111 --cursor 50
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--query` | `string` | - | keep components whose name, description, or set contains this text (case-insensitive) |
| `--team` | `string` | - | list a team library instead of a file |

Plus the [global flags](#global-flags).

## figctl devresources

Dev Mode resource links attached to nodes

```
figctl devresources [command]
```

Subcommands:

- [`figctl devresources add`](#figctl-devresources-add) Attach a link to a node, for example a Storybook story or a pull request
- [`figctl devresources list`](#figctl-devresources-list) List dev resources, optionally for specific nodes
- [`figctl devresources remove`](#figctl-devresources-remove) Delete links from a file
- [`figctl devresources update`](#figctl-devresources-update) Change the name or URL of existing links

Plus the [global flags](#global-flags).

## figctl devresources add

Attach a link to a node, for example a Storybook story or a pull request

```
figctl devresources add <ref> [flags]
```

Attach dev resource links to nodes. The links appear in Figma's Dev Mode
next to the design, which is how a design system points a component at the code
that implements it.

Pass one link with --node and --url, or many with --from-file for a whole
component library at once. Figma allows at most 10 links per node and rejects a
URL a node already carries.

Examples:

```sh
figctl devresources add KEY --node 3:11 --url https://storybook.example.com/?path=/story/button --name Storybook
figctl devresources add KEY --from-file stories.json
storybook-index --json | figctl devresources add KEY --from-file - --yes
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--dry-run` | `bool` | `false` | show what would be written without writing |
| `--from-file` | `string` | - | JSON array of {nodeId, name, url}, or - for stdin |
| `--name` | `string` | - | link name shown in Dev Mode (defaults to the URL host) |
| `--node` | `string` | - | node id to attach the link to |
| `--url` | `string` | - | link URL |

Plus the [global flags](#global-flags).

## figctl devresources list

List dev resources, optionally for specific nodes

```
figctl devresources list <ref> [flags]
```

Examples:

```sh
figctl devresources list KEY
figctl devresources list KEY --node 3:11 --node 3:20
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--node` | `string (repeatable)` | - | node id to filter by; repeatable |

Plus the [global flags](#global-flags).

## figctl devresources remove

Delete links from a file

```
figctl devresources remove <ref> [flags]
```

Examples:

```sh
figctl devresources remove KEY --id abc123 --id def456
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--id` | `string (repeatable)` | - | dev resource id to delete; repeatable |

Plus the [global flags](#global-flags).

## figctl devresources update

Change the name or URL of existing links

```
figctl devresources update <ref> [flags]
```

Examples:

```sh
figctl devresources update KEY --id abc123 --url https://storybook.example.com/?path=/story/button--primary
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--from-file` | `string` | - | JSON array of {id, name, url}, or - for stdin |
| `--id` | `string` | - | dev resource id to update |
| `--name` | `string` | - | new name |
| `--url` | `string` | - | new URL |

Plus the [global flags](#global-flags).

## figctl file

Read a Figma file: info, tree, find, and raw JSON

```
figctl file [command]
```

Subcommands:

- [`figctl file find`](#figctl-file-find) Search nodes by name glob, type, or text content
- [`figctl file get`](#figctl-file-get) Print raw Figma API JSON for the document or specific nodes (escape hatch)
- [`figctl file info`](#figctl-file-info) Show file name, pages, version, editor type, role, and branches
- [`figctl file tree`](#figctl-file-tree) Sparse outline of pages, frames, and layers with ids, sizes, and flags

Plus the [global flags](#global-flags).

## figctl file find

Search nodes by name glob, type, or text content

```
figctl file find <ref> [flags]
```

Search every node of the document (or of the subtree given with --node or
--page) and return the matches with the path of ancestor names that leads
to them. At least one of --name, --type, or --text is required; when
several are given a node must match all of them.

--name is a case-insensitive glob (* and ?). --type is a comma separated
list of node types. --text is a case-insensitive substring searched in the
characters of TEXT nodes; matching text is echoed, cut at 120 characters.
Results are paged 200 at a time. The document is fetched once per file
version and cached, so searching is free after the first call.

Examples:

```sh
figctl file find KEY --name "Login*"
figctl file find KEY --type COMPONENT,COMPONENT_SET --page "Design System"
figctl file find KEY --text "sign in"
figctl file find KEY --name "icon/*" --type VECTOR --node 2:2
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--name` | `string` | - | name glob to match (* and ?, case-insensitive) |
| `--node` | `string` | - | search only inside this node id (1:2, 1-2, or a URL with node-id) |
| `--page` | `string` | - | search only this page, by name or id |
| `--text` | `string` | - | substring to find in TEXT node characters (case-insensitive) |
| `--type` | `string` | - | node types to match, comma separated (FRAME,TEXT,INSTANCE,...) |

Plus the [global flags](#global-flags).

## figctl file get

Print raw Figma API JSON for the document or specific nodes (escape hatch)

```
figctl file get <ref> [flags]
```

Print the JSON of the whole document, or of the nodes given with --node,
exactly as the Figma REST API returns it. Nothing is re-modelled, so every
property Figma sends is present, including ones figctl does not understand.
The document is fetched once per file version and cached, so repeated
calls with --node cost no extra API requests. A node-id in the ref URL is
used when no --node is given. --depth, --geometry, and --plugin-data are
passed to the API; --fields keeps only the named top level keys.

Examples:

```sh
figctl file get KEY --node 1:2 --depth 2
figctl file get "https://www.figma.com/design/KEY/App?node-id=1-2" --fields document
figctl file get KEY --node 1:2 --geometry paths
figctl file get KEY --depth 1 --fields document
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--depth` | `int` | `0` | levels of children to include (0 for all) |
| `--geometry` | `string` | - | set to paths to include vector geometry |
| `--node` | `string (repeatable)` | - | node id (1:2, 1-2, or a URL with node-id); repeatable |
| `--plugin-data` | `string` | - | plugin ids (comma separated, or shared) whose data to include |

Plus the [global flags](#global-flags).

## figctl file info

Show file name, pages, version, editor type, role, and branches

```
figctl file info <ref>
```

Examples:

```sh
figctl file info https://www.figma.com/design/KEY/Web-App
figctl file info KEY --json --fields name,pages
```

Plus the [global flags](#global-flags).

## figctl file tree

Sparse outline of pages, frames, and layers with ids, sizes, and flags

```
figctl file tree <ref> [flags]
```

Print a sparse outline of the document: one row per node with its id, type,
name, position and size (from absoluteBoundingBox, rounded), depth, child
count, and flags (component, instance of a component, hidden, auto layout
mode, export settings, dev status). This is the cheapest way to find the
node ids to pass to figctl node context.

The walk starts at --node (or the node-id in the ref URL, or the page given
with --page) and goes --depth levels down, 2 by default, so a plain call
shows the pages and their top level frames. Rows are in document order and
paged 200 at a time; pass nextCursor back with --cursor. Filters keep
matching rows but still walk through their parents. The document is
fetched once per file version and cached.

Examples:

```sh
figctl file tree KEY
figctl file tree KEY --node 2:2 --depth 3 -o md
figctl file tree KEY --type FRAME,SECTION --page Screens
figctl file tree KEY --name "icon/*" --depth 0 --limit 50
figctl file tree "https://www.figma.com/design/KEY/App?node-id=2-2" --visible-only
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--depth` | `int` | `2` | levels below the start node to walk (0 for all) |
| `--name` | `string` | - | keep only nodes whose name matches this glob (* and ?, case-insensitive) |
| `--node` | `string` | - | start node id (1:2, 1-2, or a URL with node-id); default is the document |
| `--page` | `string` | - | start at this page, by name or id |
| `--type` | `string` | - | keep only these node types, comma separated (FRAME,SECTION,TEXT,...) |
| `--visible-only` | `bool` | `false` | skip hidden nodes and their children |

Plus the [global flags](#global-flags).

## figctl folders

Discover folders and files (v2 API)

```
figctl folders [command]
```

Subcommands:

- [`figctl folders files`](#figctl-folders-files) List the files in a folder
- [`figctl folders list`](#figctl-folders-list) List the top level folders of a team, or the subfolders of a folder

Plus the [global flags](#global-flags).

## figctl folders files

List the files in a folder

```
figctl folders files <folder-id> [flags]
```

Examples:

```sh
figctl folders files 2001
figctl folders files 2001 --branches
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--branches` | `bool` | `false` | include branch data for each file |

Plus the [global flags](#global-flags).

## figctl folders list

List the top level folders of a team, or the subfolders of a folder

```
figctl folders list [team-id|folder-id] [flags]
```

List folders. By default the id is a team id and the team's top level
folders are listed; with --folder the id is a folder id and its subfolders
are listed. Without an id the profile's default team is used.

Examples:

```sh
figctl folders list 123456789
figctl folders list 2001 --folder
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--folder` | `bool` | `false` | treat the id as a folder id and list its subfolders |

Plus the [global flags](#global-flags).

## figctl init

Write .figctl.yaml in the current directory with the profile to use

```
figctl init [flags]
```

```
Write .figctl.yaml in the current directory. The file names the profile
and the Figma files this repository works with, and contains no secrets, so it
can be committed. Commands walk up from the working directory to find it.

Name each file with --file <name>=<key or URL>. A repository usually refers to
more than one, because a design system lives in its own file, and a name can
then be used wherever a command takes a ref:

  figctl file tree design-system
  figctl tokens export design-system --format css

--default picks the file used when a command is given no ref at all. With a
single configured file that is implied.
```

Examples:

```sh
figctl init --profile acme --file web=https://www.figma.com/design/KEY/Web-App
figctl init --profile acme --file app=KEY1 --file design-system=KEY2 --default app
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--default` | `string` | - | name of the file used when a command is given no ref |
| `--file` | `string (repeatable)` | - | a named Figma file as <name>=<key or URL>; repeatable |

Plus the [global flags](#global-flags).

## figctl me

Show the user the active token belongs to

```
figctl me
```

Examples:

```sh
figctl me
figctl me --profile acme --json
```

Plus the [global flags](#global-flags).

## figctl node

Inspect nodes: resolved layout, styles, typography, tokens, and CSS

```
figctl node [command]
```

Subcommands:

- [`figctl node context`](#figctl-node-context) Everything needed to implement one node: inspect, screenshot, assets, tokens, components, comments
- [`figctl node inspect`](#figctl-node-inspect) Resolved layout, visual, text, tokens, and component info for nodes

Plus the [global flags](#global-flags).

## figctl node context

Everything needed to implement one node: inspect, screenshot, assets, tokens, components, comments

```
figctl node context <ref> --node ID [flags]
```

Bundle in one call everything an agent needs to implement a screen or a
component: the normalized model at --depth (default 4) with CSS and
prototype interactions, a PNG screenshot of each node written to disk, SVG
exports of the icon-like layers and the raster image fills used in the
subtree, only the design tokens and styles the subtree actually uses, the
component and variant definitions of every instance, the comments pinned
on the node or its children, and the dev resource links attached to them.

File paths in the output are absolute, so a screenshot can be read
straight away with a vision tool. Values with a null token are not bound
to a variable and have to be hardcoded. Screenshot and asset failures do
not fail the command: they are listed in data.failures and the exit code
is 6 (PARTIAL) with the rest of the bundle intact.

Trim the work with --no-screenshot, --no-assets, --no-comments, --no-css,
--depth, and --max-nodes. A node-id in the ref URL is used when no --node
is given.

Examples:

```sh
figctl node context KEY --node 2:2
figctl node context "https://www.figma.com/design/KEY/App?node-id=2-2" -o md
figctl node context KEY --node 2:6 --depth 2 --no-assets
figctl node context KEY --node 2:2 --out ./design/login --screenshot-scale 1
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--depth` | `int` | `4` | levels of children to inspect (-1 for all) |
| `--icon-pattern` | `string (repeatable)` | - | name glob that marks an icon (default icon/*, Icon*, icons/*); repeatable |
| `--max-nodes` | `int` | `500` | stop inspecting after this many nodes and report truncation |
| `--no-assets` | `bool` | `false` | skip icon exports and image fill downloads |
| `--no-comments` | `bool` | `false` | skip comments and dev resources |
| `--no-css` | `bool` | `false` | omit the CSS declaration map from the inspected nodes |
| `--no-screenshot` | `bool` | `false` | skip the PNG render |
| `--node` | `string (repeatable)` | - | node id (1:2, 1-2, or a URL with node-id); repeatable |
| `--out` | `string` | - | directory for the screenshot and assets (default ./.figctl/<fileKey>/context) |
| `--screenshot-scale` | `float` | `2` | screenshot render scale between 0.01 and 4 |

Plus the [global flags](#global-flags).

## figctl node inspect

Resolved layout, visual, text, tokens, and component info for nodes

```
figctl node inspect <ref> --node ID [flags]
```

Inspect nodes and their children (to --depth, default 3) as a normalized
model: layout in CSS terms (row, column, wrap, grid; gap, padding,
justify, align, sizing fixed/hug/fill), visual (fills as hex and rgba,
gradients as CSS, strokes, radius, effects as box-shadow and filter),
text (font, size, line height in px and unitless, letter spacing, case,
mixed runs), component info for instances, and every bound variable and
style as a token reference. Values without a token are reported with
"token": null so it is clear what is not tokenized.

The document is fetched once per file version and cached, so repeated
calls cost no extra Tier 1 requests. Variables need an Enterprise plan;
on other plans the command falls back to styles and raw values with a
hint. A node-id in the ref URL is used when no --node is given.

Examples:

```sh
figctl node inspect KEY --node 2:2
figctl node inspect "https://www.figma.com/design/KEY/App?node-id=2-2" --depth 1
figctl node inspect KEY --node 2:3 --css --web
figctl node inspect KEY --node 2:6 --interactions -o md
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--css` | `bool` | `false` | add a flat map of CSS declarations per node |
| `--depth` | `int` | `3` | levels of children to include (-1 for all) |
| `--include-hidden` | `bool` | `false` | include hidden nodes |
| `--interactions` | `bool` | `false` | include prototype interactions |
| `--max-nodes` | `int` | `500` | stop after this many nodes and report truncation |
| `--node` | `string (repeatable)` | - | node id (1:2, 1-2, or a URL with node-id); repeatable |
| `--web` | `bool` | `false` | add the var(--name) form to tokens with WEB code syntax |

Plus the [global flags](#global-flags).

## figctl profile

Manage named accounts (one per client or organization)

```
figctl profile [command]
```

Subcommands:

- [`figctl profile add`](#figctl-profile-add) Add or update a profile and store its token
- [`figctl profile list`](#figctl-profile-list) List profiles
- [`figctl profile remove`](#figctl-profile-remove) Remove a profile and its stored token
- [`figctl profile show`](#figctl-profile-show) Show a profile (the active one when no name is given), token redacted
- [`figctl profile use`](#figctl-profile-use) Set the default profile

Plus the [global flags](#global-flags).

## figctl profile add

Add or update a profile and store its token

```
figctl profile add <name> [flags]
```

Add or update a profile. The token is read from --token-file, from stdin
when stdin is not a terminal, or from a hidden prompt on a terminal. It is
never accepted as a flag value.

Examples:

```sh
figctl profile add acme --expires 2026-12-01
printf '%s' "$FIGMA_TOKEN" | figctl profile add acme --team 123456 --default
figctl profile add gov --base-url https://api-figma-gov.com --token-file ./token.txt
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--base-url` | `string` | - | API base URL, for example https://api-figma-gov.com |
| `--default` | `bool` | `false` | make this the default profile |
| `--expires` | `string` | - | token expiry date as YYYY-MM-DD (Figma tokens expire after at most 90 days) |
| `--team` | `string` | - | default team id for this profile |

Plus the [global flags](#global-flags).

## figctl profile list

List profiles

```
figctl profile list
```

Examples:

```sh
figctl profile list
figctl profile list --json --fields name,expires
```

Plus the [global flags](#global-flags).

## figctl profile remove

Remove a profile and its stored token

```
figctl profile remove <name>
```

Examples:

```sh
figctl profile remove acme --yes
```

Plus the [global flags](#global-flags).

## figctl profile show

Show a profile (the active one when no name is given), token redacted

```
figctl profile show [name]
```

Examples:

```sh
figctl profile show
figctl profile show acme
```

Plus the [global flags](#global-flags).

## figctl profile use

Set the default profile

```
figctl profile use <name>
```

Examples:

```sh
figctl profile use acme
```

Plus the [global flags](#global-flags).

## figctl projects

Discover projects and files (v1 API, deprecated in favor of folders)

```
figctl projects [command]
```

Subcommands:

- [`figctl projects files`](#figctl-projects-files) List the files in a project
- [`figctl projects list`](#figctl-projects-list) List the projects of a team

Plus the [global flags](#global-flags).

## figctl projects files

List the files in a project

```
figctl projects files <project-id> [flags]
```

Examples:

```sh
figctl projects files 1001
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--branches` | `bool` | `false` | include branch data for each file |

Plus the [global flags](#global-flags).

## figctl projects list

List the projects of a team

```
figctl projects list [team-id]
```

Examples:

```sh
figctl projects list 123456789
figctl projects list
```

Plus the [global flags](#global-flags).

## figctl render

Render nodes to PNG, JPG, SVG, or PDF files

```
figctl render <ref> [flags]
```

Render one or more nodes to image files and print a manifest of the paths.
Nodes sharing a format and scale are rendered in one API request, and every
render is cached by node, format, scale, and file version, so repeating a
render of an unchanged file costs no image requests.

A node-id in the ref URL is used when no --node is given. Nodes Figma cannot
render (hidden, empty) are reported per node; the good files are kept and
the command exits 6 (PARTIAL).

Examples:

```sh
figctl render KEY --node 2:2
figctl render "https://www.figma.com/design/KEY/App?node-id=2-2" --scale 1 --out ./design
figctl render KEY --node 2:9 -f svg --svg-outline-text --name-by id
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--absolute-bounds` | `bool` | `false` | use the full node dimensions regardless of cropping |
| `--concurrency` | `int` | `8` | maximum simultaneous downloads |
| `--name-by` | `string` | `name` | file naming: name (slug of the node name) or id |
| `--no-contents-only` | `bool` | `false` | include overlapping content from other nodes |
| `--no-svg-simplify-stroke` | `bool` | `false` | svg: keep strokes as strokes instead of simplifying them |
| `--node` | `string (repeatable)` | - | node id (1:2, 1-2, or a URL with node-id); repeatable |
| `--out` | `string` | - | output directory (default ./.figctl/<fileKey>/renders) |
| `--scale` | `float` | `0` | render scale between 0.01 and 4 (default 2 for png and jpg, 1 for svg and pdf) |
| `--svg-include-id` | `bool` | `false` | svg: include id attributes for every element |
| `--svg-include-node-id` | `bool` | `false` | svg: include data-node-id attributes |
| `--svg-outline-text` | `bool` | `false` | svg: render text as outlines |
| `-f`, `--format` | `string` | `png` | png, jpg, svg, or pdf |

Plus the [global flags](#global-flags).

## figctl schema

JSON Schema of a command's data payload

```
figctl schema [command]
```

Print the JSON Schema of the data payload of a command, so field names
never have to be guessed. Command names are the dotted names that appear
in the "command" field of every envelope, for example node.context or
file.tree. Run without an argument to list the commands that have a
schema.

"figctl schema envelope" prints the shape of the envelope itself: the
schemaVersion, command, file, profile, data, truncated, nextCursor, and
hints fields, plus the error envelope.

JSON output is the schema itself. Markdown and table output flatten it
into a field list (path, type, required, description).

Examples:

```sh
figctl schema
figctl schema node.context
figctl schema envelope
figctl schema file.tree -o table
```

Plus the [global flags](#global-flags).

## figctl skill

Install the figctl agent skill into a coding agent

```
figctl skill [command]
```

Write the figctl skill (an onboarding document plus a command, output
schema, and workflow reference) into the place a coding agent reads it
from. The reference files are generated from the same command tree and
JSON Schemas the CLI uses, so they cannot drift from the binary.

Subcommands:

- [`figctl skill install`](#figctl-skill-install) Write the skill files for one or more agents
- [`figctl skill print`](#figctl-skill-print) Print a skill document to stdout
- [`figctl skill uninstall`](#figctl-skill-uninstall) Remove the skill files or marked blocks

Examples:

```sh
figctl skill install
figctl skill install --agent all --project
figctl skill print --file workflows -o md
```

Plus the [global flags](#global-flags).

## figctl skill install

Write the skill files for one or more agents

```
figctl skill install [flags]
```

```
Write the skill for the selected agent.

.agents/skills is the canonical location that Codex and most other clients
read, so the skill is written there once and other clients are pointed at it.

  agents   .agents/skills/figctl/ (SKILL.md and reference/)
  codex    the same directory; Codex reads .agents/skills directly
  generic  the same directory, for any client that reads .agents/skills
  claude   .claude/skills/figctl symlinked to the canonical directory
  cursor   .cursor/rules/figctl.mdc
  copilot  a marked block in .github/copilot-instructions.md
  all      agents, claude, cursor, and copilot

The skill directory goes under the home directory by default and under the
working directory with --project. Cursor and Copilot read their files from
the repository, so those two are always project scoped.

A skill is not written into AGENTS.md. That file holds the always-on rules
for a repository, while a skill is a directory loaded on demand, which is
the reason to ship one.

Installs are idempotent. Files figctl owns carry a marker comment and are
replaced in place; a file of the same name that figctl did not write is
left alone unless --force, as is a link that points somewhere else. Shared
instruction files keep everything outside the <!-- BEGIN figctl --> and
<!-- END figctl --> markers. Run it again after upgrading figctl to refresh
the generated reference.
```

Examples:

```sh
figctl skill install
figctl skill install --agent claude --project
figctl skill install --agent all --dry-run
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--agent` | `string` | `claude` | target agent: agents, claude, cursor, copilot, codex, generic, all |
| `--dry-run` | `bool` | `false` | list what would be written, with byte counts, without writing |
| `--force` | `bool` | `false` | replace a file of the same name that figctl did not write |
| `--project` | `bool` | `false` | install the claude skill into this repository instead of the home directory |

Plus the [global flags](#global-flags).

## figctl skill print

Print a skill document to stdout

```
figctl skill print [flags]
```

Print one skill document instead of installing it, for manual
placement or for reading the reference without a file on disk.

--file selects the document: SKILL, commands, schemas, or workflows.
--agent selects the shape of SKILL: the skill file for agents, codex,
generic, and claude, the .mdc rule for cursor, and the marked block for
copilot. The reference documents are the same for every agent.

In JSON mode the document is a string field of the envelope; in every
other mode the document itself is written to stdout, so "-o md" can be
redirected into a file.

Examples:

```sh
figctl skill print --file workflows -o md
figctl skill print --agent copilot -o md >> .github/copilot-instructions.md
figctl skill print --file commands --json
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--agent` | `string` | `claude` | shape of the SKILL document: agents, claude, cursor, copilot, codex, generic, all |
| `--file` | `string` | `SKILL` | document to print: SKILL, commands, schemas, workflows |

Plus the [global flags](#global-flags).

## figctl skill uninstall

Remove the skill files or marked blocks

```
figctl skill uninstall [flags]
```

Remove what skill install wrote: the files figctl owns, and the
<!-- BEGIN figctl --> block from shared instruction files, leaving the
rest of those files untouched. A file figctl did not write is left alone
unless --force.

Removing files needs a confirmation, so pass --yes when stdin is not a
terminal.

Examples:

```sh
figctl skill uninstall --yes
figctl skill uninstall --agent all --project --yes
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--agent` | `string` | `claude` | target agent: agents, claude, cursor, copilot, codex, generic, all |
| `--dry-run` | `bool` | `false` | list what would be removed without removing it |
| `--force` | `bool` | `false` | remove a file of the same name that figctl did not write |
| `--project` | `bool` | `false` | remove the claude skill from this repository instead of the home directory |

Plus the [global flags](#global-flags).

## figctl styles

Styles with resolved values: fills, typography, effects, and grids

```
figctl styles [command]
```

Subcommands:

- [`figctl styles get`](#figctl-styles-get) Show one style with its full resolved value
- [`figctl styles list`](#figctl-styles-list) List styles with metadata and resolved values

Plus the [global flags](#global-flags).

## figctl styles get

Show one style with its full resolved value

```
figctl styles get <ref> --node ID | --key KEY [flags]
```

Examples:

```sh
figctl styles get KEY --node 5:2
figctl styles get KEY --key 5f4e3d2c1b0a9f8e7d6c5b4a39281706f5e4d502
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--key` | `string` | - | style key (from a library listing) |
| `--node` | `string` | - | style node id (from styles list, or the styles map of a node) |

Plus the [global flags](#global-flags).

## figctl styles list

List styles with metadata and resolved values

```
figctl styles list <ref> | --team ID [flags]
```

List the styles of a file (the styles used by the document plus its
published styles) with their resolved values, read from the style nodes
in one batched request. With --team, list the team's published styles;
values are not available there because they live in the owning file.

Examples:

```sh
figctl styles list KEY
figctl styles list KEY --type TEXT -o table
figctl styles list --team 555000111 --limit 50
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--team` | `string` | - | list a team's published styles instead of a file's |
| `--type` | `string` | - | only this style type: FILL, TEXT, EFFECT, or GRID |

Plus the [global flags](#global-flags).

## figctl tokens

Design tokens: resolve variables and styles, export as DTCG, CSS, or Tailwind

```
figctl tokens [command]
```

Subcommands:

- [`figctl tokens export`](#figctl-tokens-export) Export variables and styles as DTCG, CSS custom properties, or a Tailwind theme
- [`figctl tokens resolve`](#figctl-tokens-resolve) Resolve one variable or style to concrete values across modes

Plus the [global flags](#global-flags).

## figctl tokens export

Export variables and styles as DTCG, CSS custom properties, or a Tailwind theme

```
figctl tokens export <ref> [flags]
```

Export the variables and styles of a file as design tokens.

The default format is DTCG (Design Tokens Format Module 2025.10): groups
nest on the "/" in Figma names, aliases become {group.token} references,
and everything Figma expresses but DTCG does not (modes, scopes, layout
grids, gradient angles) is kept under $extensions.com.figma.
--format style-dictionary is the same bytes: Style Dictionary v4 reads
DTCG unchanged.

--format css writes custom properties, a :root block for the selected
modes and one block per remaining mode. --format tailwind writes a theme
module. --format json writes figctl's flat token list with the Figma ids.

Collections with several modes cannot be flattened once, so pick a
strategy: default keeps the default mode and records the rest under
$extensions, separate writes one document per mode, and select takes the
modes named by --mode (which also switches the strategy on its own).

Without variables access (any plan below Enterprise, or a token without
the file_variables:read scope) the command still exports the styles and
says so in hints.

Examples:

```sh
figctl tokens export KEY > tokens.json
figctl tokens export KEY --format css --out ./src/styles
figctl tokens export KEY --mode Dark --format css
figctl tokens export KEY --mode-strategy separate --out ./tokens
figctl tokens export KEY --format tailwind --tailwind-vars --tailwind-format cjs
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--collection` | `string (repeatable)` | - | only this variable collection (name or id); repeatable |
| `--exclude-hidden` | `bool` | `false` | drop variables marked hiddenFromPublishing |
| `--exclude-remote` | `bool` | `false` | drop variables and styles from remote libraries |
| `--include` | `string` | `variables,styles` | sources to export: variables, styles, or both |
| `--mode-selector` | `string` | `[data-theme="<mode>"]` | css: selector for a mode block; <mode> is replaced with the kebab cased mode name |
| `--mode-strategy` | `string` | `default` | how to flatten collections with several modes: default, separate, or select |
| `--mode` | `string (repeatable)` | - | mode name to select per collection; repeatable, implies --mode-strategy select |
| `--name-case` | `string` | `kebab` | normalize name segments: kebab, camel, snake, or none |
| `--out` | `string` | - | write files to this directory (or file) instead of the envelope |
| `--tailwind-format` | `string` | `esm` | tailwind: esm, cjs, or json |
| `--tailwind-vars` | `bool` | `false` | tailwind: emit var(--name) values instead of literals |
| `-f`, `--format` | `string` | `dtcg` | dtcg, css, tailwind, style-dictionary, or json |

Plus the [global flags](#global-flags).

## figctl tokens resolve

Resolve one variable or style to concrete values across modes

```
figctl tokens resolve <ref> --id ID | --name NAME [flags]
```

Resolve a variable (by id or name) or a style (by node id, key, or name) to
its concrete values. Variables show the value per mode with the alias
chain that produced it; --mode narrows to one mode. Styles show the
resolved fills, typography, effects, or layout grids. Without variables
access (non Enterprise plans) only styles resolve, with a hint.

Examples:

```sh
figctl tokens resolve KEY --id VariableID:1:201
figctl tokens resolve KEY --name text/primary --mode Dark
figctl tokens resolve KEY --id 5:2
figctl tokens resolve KEY --name shadow/md
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--id` | `string` | - | variable id (VariableID:1:2), style node id (5:1), or style key |
| `--mode` | `string` | - | only this mode (name or id) for variables |
| `--name` | `string` | - | variable or style name |

Plus the [global flags](#global-flags).

## figctl variables

Raw variables and collections with values resolved per mode (Enterprise)

```
figctl variables [command]
```

Subcommands:

- [`figctl variables get`](#figctl-variables-get) Show one variable with its alias chain per mode
- [`figctl variables infer`](#figctl-variables-infer) Reconstruct the variables a file uses from how they are used
- [`figctl variables list`](#figctl-variables-list) List variables with values per mode, code syntax, scopes, and descriptions

Plus the [global flags](#global-flags).

## figctl variables get

Show one variable with its alias chain per mode

```
figctl variables get <ref> --id ID | --name NAME [flags]
```

Examples:

```sh
figctl variables get KEY --id VariableID:1:201
figctl variables get KEY --name bg/surface
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--id` | `string` | - | variable id (VariableID:1:2) or subscribed id |
| `--name` | `string` | - | variable name (for example color/brand/500) |

Plus the [global flags](#global-flags).

## figctl variables infer

Reconstruct the variables a file uses from how they are used

```
figctl variables infer [ref] [flags]
```

Report the variables a file uses by walking its nodes, for accounts that
cannot read the variables endpoint.

Reading variables needs an Enterprise plan, but the bindings do not: every node
says which variable governs each of its properties, and the value that variable
resolved to sits on the same node. Walking the file therefore recovers which
values are governed, which places share one, and what each resolves to.

What it cannot recover is the designer's name for a variable and the collection
it belongs to. Names here are derived from the category and the most common
observed value, and every entry says so, because a plausible invented name
reads as authoritative and cannot be checked.

A variable seen resolving to more than one value is reported as likely having
modes, which is what light and dark look like from the outside.

On a plan that can read variables, prefer figctl variables list: it has the
real names, collections, and every mode.

Examples:

```sh
figctl variables infer design-system
figctl variables infer KEY --node 2:2
figctl variables infer KEY --json > variables.json
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--include-hidden` | `bool` | `false` | walk nodes the designer turned off |
| `--min-usages` | `int` | `0` | drop variables bound fewer times than this |
| `--node` | `string (repeatable)` | - | limit the walk to these nodes; repeatable |
| `--samples` | `int` | `3` | node ids kept per observed value |

Plus the [global flags](#global-flags).

## figctl variables list

List variables with values per mode, code syntax, scopes, and descriptions

```
figctl variables list <ref> [flags]
```

List the local variables of a file. Aliases are resolved to concrete
values per mode and colors are shown as hex. Filter by collection, type,
or mode. The variables API needs an Enterprise plan with a full seat and
the file_variables:read scope; on other plans the command fails with
PLAN_REQUIRED and figctl styles list is the fallback.

Examples:

```sh
figctl variables list KEY
figctl variables list KEY --collection Semantic --mode Dark
figctl variables list KEY --type COLOR -o table
figctl variables list KEY --published
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--collection` | `string` | - | only variables in this collection (name or id) |
| `--mode` | `string` | - | only values in this mode (name or id) |
| `--published` | `bool` | `false` | list published variables (metadata only, with subscribed ids) |
| `--type` | `string` | - | only this resolved type: COLOR, FLOAT, STRING, or BOOLEAN |

Plus the [global flags](#global-flags).

## figctl version

Print the figctl version

```
figctl version [flags]
```

Examples:

```sh
figctl version
figctl version --json
```

Flags:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--check` | `bool` | `false` | check for a newer release |

Plus the [global flags](#global-flags).

## figctl versions

List saved versions of a file

```
figctl versions [command]
```

Subcommands:

- [`figctl versions list`](#figctl-versions-list) List versions, newest first

Plus the [global flags](#global-flags).

## figctl versions list

List versions, newest first

```
figctl versions list <ref>
```

List the saved versions of a file, newest first. Use a version id with
--file-version on any other command to read the file as it was at that
version. Pass --cursor from nextCursor to page further back.

Examples:

```sh
figctl versions list KEY
figctl versions list KEY --limit 5 --cursor 2100120000
```

Plus the [global flags](#global-flags).
