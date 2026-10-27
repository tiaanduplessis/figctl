# Wiring figctl into a coding agent

figctl is a shell command, so any agent that can run a shell can use it. What
each agent needs is a document telling it when to reach for figctl and how the
workflow goes. `figctl skill install` writes that document in the shape the
agent expects.

```sh
figctl skill install                       # --agent claude by default
figctl skill install --agent all --project # claude, cursor, copilot, and codex
figctl skill install --agent all --dry-run # list what would be written
figctl skill print --file workflows -o md  # stdout instead of a file
figctl skill uninstall --yes               # remove what was installed
```

`--agent` accepts `agents`, `claude`, `cursor`, `copilot`, `codex`, `generic`,
and `all`. `--file` on `skill print` accepts `SKILL`, `commands`, `schemas`,
and `workflows`.

## Install through skills.sh

The [skills.sh](https://skills.sh) CLI discovers `skills/figctl/SKILL.md` in
this public repository. With Node.js 22.20 or later, run from your project:

```sh
npx skills add tiaanduplessis/figctl --skill figctl
```

Choose your coding agents when prompted. The default scope is the current
project; add `-g` for all projects. To select agents explicitly:

```sh
npx skills add tiaanduplessis/figctl --skill figctl -a claude-code -a codex
npx skills add tiaanduplessis/figctl --skill figctl -g
npx skills add tiaanduplessis/figctl --list # discover without installing
```

The skills CLI uses `claude-code`; the built-in figctl installer uses `claude`.
It installs `SKILL.md` and the three reference documents in the selected agents'
skill locations, using links to a shared copy by default. These are skills CLI
installs, managed with `npx skills check`, `npx skills update`, and
`npx skills remove figctl`. Avoid installing over the same location with both
installers: figctl refuses to replace files owned by another installer.

The skill does not install the binary. Follow the [CLI installation options](../README.md#installation)
to put `figctl` on `PATH`. Local PNG comparison with `figctl diff` needs no
Figma token; Figma API commands require authentication.

### Publication and maintenance

There is no separate upload or registry manifest. According to the
[skills.sh FAQ](https://skills.sh/docs/faq), public skills appear in its
directory through installation telemetry from `npx skills add`. Installability
can be verified before a directory listing appears; ranking is managed by
skills.sh. See its [skill format and discovery rules](https://github.com/vercel-labs/skills/blob/main/docs/creating-skills.md).

Edit `internal/skill/templates/SKILL.md` and
`internal/skill/templates/reference/workflows.md` for hand-written guidance.
Run `make gen` to regenerate command and schema references and mirror the whole
skill into `skills/figctl/`; do not edit the published copy directly. Tests and
`make verify-gen` check both generated references and the published mirror.
These checks detect structural drift; review the hand-written workflows when
adding behavior or changing recommended usage.

To check discovery and installation from a checkout without changing your own
agent setup, replace the source path below and use a temporary project:

```sh
npx skills add /absolute/path/to/figctl --list
mkdir -p /tmp/figctl-skill-check
cd /tmp/figctl-skill-check
DISABLE_TELEMETRY=1 npx skills add /absolute/path/to/figctl --skill figctl -a codex -a claude-code -y
```

## Where each agent reads from

`.agents/skills` is the canonical location that Codex and most other clients
read. The skill is written there once and every other client is pointed at it,
so the two locations cannot drift apart.

| Agent | Location | Shape |
| --- | --- | --- |
| Codex, generic | `~/.agents/skills/figctl/` (or `.agents/skills/figctl/` with `--project`) | a `SKILL.md` plus generated reference files, all owned by figctl |
| Claude Code | `.claude/skills/figctl` | a relative symlink to the canonical directory |
| Cursor | `.cursor/rules/figctl.mdc` in the repo | one rule file with front matter, owned by figctl |
| GitHub Copilot | `.github/copilot-instructions.md` in the repo | a delimited block inside a file the project owns |

The skill directory goes under the home directory by default and under the
working directory with `--project`. Cursor and Copilot read their files from
the repository, so those two are always project-scoped.

The link is relative, so it keeps working when the project is moved or cloned.
Where symlinks are unavailable, notably Windows without developer mode, the
files are copied instead and the result says so.

### A skill is not written into AGENTS.md

`AGENTS.md` holds the always-on rules for a repository. A skill is a directory
loaded on demand when a task matches its description, which is the reason to
ship one rather than a block of instructions that occupies every turn. figctl
therefore never edits `AGENTS.md`. If you want a one-line pointer there, add it
by hand.

Files figctl owns outright carry a marker comment. A re-install replaces a file
that carries the marker and leaves a file of the same name that figctl did not
write alone, unless `--force`; the same applies to a link that points somewhere
else. `copilot-instructions.md` is edited between `<!-- BEGIN figctl -->` and
`<!-- END figctl -->`, so everything else in it survives. Installs are
idempotent: run `skill install` again after upgrading figctl to refresh the
generated reference. `--dry-run` shows what would change, with byte counts.

The reference files are generated from the same cobra command tree and JSON
Schema registry as `--help` and `docs/commands.md`, so they cannot describe a
flag that does not exist.

## Project setup for a client repository

Two commands make a repository ready for an agent with no further explanation:

```sh
figctl init --profile acme --file https://www.figma.com/design/KEY/Web-App
figctl skill install --agent claude --project
```

`init` writes `.figctl.yaml`, which names the profile and the default file and
contains no secret, so it is safe to commit. `skill install --project` writes the
skill into the repository rather than the home directory, so teammates and CI
agents get the same instructions. See [profiles.md](profiles.md) for how the
profile is picked up.

## What the agent should know without reading a skill file

If you would rather write your own instructions, these are the parts that
matter.

**Paste the URL, do not convert node ids.** `<ref>` accepts a file key or any
Figma URL. A `node-id=1-2` in the URL is converted to `1:2` and used as the
default `--node`. Manual hyphen-to-colon conversion is the single most common
mistake in hand-written Figma agent instructions.

**Start shallow.** `figctl file tree KEY` returns pages and top-level frames, not
the whole document. Go deeper with `--node` and `--depth` only where needed.
`figctl file find KEY --name "Login*"` and `--text "sign in"` locate a node
without walking.

**One call per node to implement.** `figctl node context KEY --node 2:2` bundles
the model, screenshot, assets, tokens, component definitions, comments, and dev
resources. Prefer it over composing four commands by hand.

**Read the exit code.** 0 success, 1 runtime error, 2 usage, 3 auth, 4 not found,
5 rate limited, 6 partial, 7 image mismatch. Exit 6 retains usable data and
`data.failures`. Exit 7 retains comparison metrics with `data.passed: false`.
Inspect the diff image and fix the implementation; do not treat it as an API
failure. Match screenshot dimensions and scale before comparing.

**Do not guess field names.** `figctl schema node.context` prints the JSON Schema
of that command's `data`. `figctl schema envelope` prints the wrapper.

**Reuse cached reads.** The document is cached per file version. Freshness
validation, large-file fallbacks, optional context data, and uncached renders
can still need API requests. Reuse `node context --screenshot-scale 1` output
for a 1x visual comparison. Adding `--refresh` or `--no-cache` to every call
throws the cache away. See
[caching.md](caching.md).

**A null token means nothing is bound.** In `node inspect` and `node context`
output, a value with `"token": null` is not bound to a Figma variable or style.
Hardcode it or ask the designer; do not invent a token name.

## Using figctl in CI

Set `FIGMA_TOKEN` and figctl uses an implicit profile named `env` with no
config file and no keychain:

```yaml
- run: figctl tokens export "$FIGMA_FILE" --format css --out ./src/styles
  env:
    FIGMA_TOKEN: ${{ secrets.FIGMA_TOKEN }}
```

Remember the Tier 1 monthly cap on View and Collab seats: a CI job that renders
images needs a Dev or Full seat token. See [caching.md](caching.md) for the
limits.
