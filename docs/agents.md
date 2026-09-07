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

`--agent` accepts `claude`, `cursor`, `copilot`, `codex`, `generic`, and `all`.
`--file` on `skill print` accepts `SKILL`, `commands`, `schemas`, and
`workflows`.

## Where each agent reads from

| Agent | Location | Shape |
| --- | --- | --- |
| Claude Code | `~/.claude/skills/figctl/` (or `.claude/skills/figctl/` in the repo) | a `SKILL.md` plus generated reference files, all owned by figctl |
| Cursor | `.cursor/rules/figctl.mdc` in the repo | one rule file with front matter, owned by figctl |
| GitHub Copilot | `.github/copilot-instructions.md` in the repo | a delimited block inside a file the project owns |
| Codex | `AGENTS.md` in the repo | a delimited block inside a file the project owns |
| Generic | `AGENTS.md` in the repo | the same block, for any agent that reads `AGENTS.md` |

Every target but `claude` is project-scoped and is written under the working
directory whether or not `--project` is given; `--project` only moves the Claude
Code skill from `~/.claude/skills/figctl/` into `.claude/skills/figctl/`.

Files figctl owns outright carry a marker comment. A re-install replaces a file
that carries the marker and leaves a file of the same name that figctl did not
write alone, unless `--force`. Shared files such as `AGENTS.md` and
`copilot-instructions.md` are edited between `<!-- BEGIN figctl -->` and
`<!-- END figctl -->`, so everything else in them survives. Installs are
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

**Read the exit code.** 0 success, 2 usage, 3 auth, 4 not found, 5 rate limited,
6 partial. Exit 6 means the payload is usable and `data.failures` says what did
not work; do not discard the result.

**Do not guess field names.** `figctl schema node.context` prints the JSON Schema
of that command's `data`. `figctl schema envelope` prints the wrapper.

**Repeated reads are free.** The document is fetched once per file version and
cached, so calling `node inspect` twenty times costs one API request. Adding
`--refresh` or `--no-cache` to every call throws that away. See
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
