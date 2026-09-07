# Agent evaluation harness

This harness measures whether an agent with the figctl skill installed can do
real design-to-code work, and whether it does so efficiently. It is the check
behind the claim that a CLI plus a skill file beats an MCP server: if the agent
needs eight calls where two would do, the skill is wrong, not the agent.

## Why it is not in CI

It needs three things CI does not have:

1. A real Figma personal access token, and therefore a real Figma account.
2. A real Figma file whose structure the tasks assume.
3. A real agent (`claude -p`), which costs money and is not deterministic.

It also spends Tier 1 Figma requests, which are rate limited to as few as 10 per
minute. Running it on every pull request would exhaust the limit and make the
build flaky for reasons unrelated to the change under review.

So it is opt-in, run by hand before a release, and its results are recorded in
the repository rather than enforced by a gate.

## What it measures

Per task:

- **figctl calls.** How many times the agent invoked figctl. This is the number
  that matters: it maps directly to Figma API requests and to how much the skill
  taught the agent to batch. A task that should take two calls and takes eight
  is a regression in the skill file.
- **Total tool calls.** Everything the agent did, including reads, writes, and
  greps. A proxy for the total work.
- **Wall time.** Seconds from launch to exit.
- **Verifiable checks.** Assertions about the files the agent produced: the file
  exists, a token name appears in the generated code, no hardcoded hex where the
  design binds a token, an SVG uses `currentColor`.

It does not score the quality of the code. A human reads the diff for that. The
harness only catches the regressions a machine can see.

## Requirements

- `figctl` on `PATH` (`make build`, then put `bin/figctl` on `PATH`).
- A configured figctl profile with access to the evaluation file
  (`figctl auth login --profile eval`).
- The figctl skill installed for the agent (`figctl skill install`).
- `claude` on `PATH` (Claude Code), authenticated.

## Running it

```sh
export FIGCTL_EVAL_URL="https://www.figma.com/design/YOURKEY/Your-File?node-id=2-2"
export FIGCTL_PROFILE=eval

go run ./eval
go run ./eval -task list-colors -keep
```

Each task runs in its own temporary directory, so nothing touches the
repository. `-keep` leaves those directories in place so the generated code can
be read. Transcripts and a `results.json` are written to
`eval/results/<timestamp>/`.

Flags:

| Flag | Default | Purpose |
| --- | --- | --- |
| `-tasks` | `eval/tasks` | directory of task definitions |
| `-out` | `eval/results` | where transcripts and results are written |
| `-task` | all | run one task by name |
| `-agent` | `claude` | the agent binary |
| `-agent-args` | `-p --output-format stream-json --verbose --permission-mode acceptEdits` | arguments before the prompt |
| `-keep` | false | keep the working directory of every task |

`-agent-args` exists because agent CLIs change their flags. If the transcript
comes out empty, the flags are wrong: the harness needs JSON lines on stdout,
one per event, with `tool_use` blocks in the assistant messages.

## Task definitions

`eval/tasks/*.yaml`. One task per file:

```yaml
name: list-colors
description: Report every color used in a screen with its design token name.
prompt: |
  Write colors.md listing every colour used in ${FIGCTL_EVAL_URL} ...
timeoutSeconds: 600
checks:
  - type: file_exists
    path: colors.md
  - type: max_figctl_calls
    value: 4
```

`${VAR}` in a prompt is substituted from the environment.

Check types:

| Type | Fields | Passes when |
| --- | --- | --- |
| `file_exists` | `path` | the file exists in the working directory |
| `glob_exists` | `path` | at least one file matches the glob |
| `contains_any` | `path`, `values` | one of the values appears in a matching file |
| `matches` | `path`, `pattern` | the regular expression matches a file |
| `not_matches` | `path`, `pattern` | the regular expression matches nothing |
| `max_figctl_calls` | `value` | the agent invoked figctl at most that many times |
| `max_tool_calls` | `value` | the agent made at most that many tool calls |

`reason` on any check is printed when it fails.

The tasks assume the evaluation file has a screen frame with bound colour
tokens and a page of icons. Point `FIGCTL_EVAL_URL` at a file that does, and
adjust the paths in the checks if your file is laid out differently.

## Recording results per release

Before tagging a release:

1. Run the harness three times against the same file. Agents are not
   deterministic, so one run is noise.
2. Record the median figctl call count and wall time per task in the release
   entry of `CHANGELOG.md`, together with the figctl version and the agent
   version.
3. If a figctl call count went up against the previous release, find out why
   before tagging. The usual cause is a change to `SKILL.md` that removed a
   piece of batching guidance, or a command whose defaults got narrower so the
   agent has to call it twice.

Record what the runs actually did. A number nobody produced is worse than no
number.
