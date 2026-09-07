# Profiles: working with several Figma accounts

A consultant has one Figma account per client, often on different plans with
different seats. An agent running in a client's repository must use that
client's token and no other. figctl treats that as a first-class concept rather
than an environment-variable problem.

A profile is a name plus:

- a token, stored in the OS keychain
- an optional default team id, used by `projects list` and `folders list`
- an optional API base URL, for Figma for Government (`https://api-figma-gov.com`)
- cached metadata: the handle and email confirmed at login, and the expiry date
  you recorded

```sh
figctl profile add acme --team 555000111 --default
figctl profile add globex
figctl profile add gov --base-url https://api-figma-gov.com --token-file ./token.txt
figctl profile list
figctl profile show acme        # token redacted
figctl profile use globex       # change the user-level default
figctl profile remove acme --yes
```

`figctl auth login <name>` is the same thing as `figctl profile add <name>`,
kept because it is what people type.

## How the token is read

The token is never accepted as a flag value, because flags end up in shell
history and process listings. `profile add` and `auth login` read it from, in
order:

1. `--token-file PATH`
2. stdin, when stdin is not a terminal
3. a hidden prompt, on a terminal

```sh
printf '%s' "$FIGMA_TOKEN" | figctl auth login acme --default
figctl auth login gov --token-file ./token.txt
```

The token is validated with a `GET me` call before it is stored, so a typo fails
immediately rather than on the next command.

## Selection order

The active profile is the first of these that matches:

| | Source | Reported as |
| --- | --- | --- |
| 1 | `--profile NAME` | `flag` |
| 2 | `FIGCTL_PROFILE` | `env` |
| 3 | `FIGMA_TOKEN`, which creates an implicit profile named `env` | `token-env` |
| 4 | `profile:` in the nearest `.figctl.yaml`, found by walking up from the working directory | `project` |
| 5 | the user-level default profile | `default` |
| 6 | `--token-file` alone, which creates an implicit profile named `file` | `token-file` |

If nothing matches, the command fails with `AUTH_MISSING` and a hint showing
`figctl auth login --profile <name>`.

`figctl auth status` reports the profile and the source, so there is never a
question about which account is in play:

```json
{
  "profile": "env",
  "source": "token-env",
  "tokenSource": "FIGMA_TOKEN",
  "hasToken": true,
  "validated": true,
  "handle": "Fixture Designer",
  "email": "designer@example.com",
  "userId": "1234567890"
}
```

Every success envelope also carries `"profile": {"name": ..., "handle": ...}`,
so an agent can verify the account before acting on the data.

## The client-repository pattern

This is the mechanism that makes multi-account work automatic.

```sh
cd ~/work/acme-web
figctl init --profile acme --file https://www.figma.com/design/KEY/Web-App
```

writes:

```yaml
# .figctl.yaml
profile: acme
file: KEY
```

Commit it. It names an account but contains no secret. From then on, any figctl
command run anywhere inside that repository uses the Acme token, and an agent
that has never been told which client it is working for gets the right one. An
agent in `~/work/globex-app` gets Globex's token and cannot reach Acme's files.

Pair it with a project-local skill install so the instructions travel with the
repository too:

```sh
figctl skill install --agent claude --project
```

## Where things are stored

| What | Where | Permissions |
| --- | --- | --- |
| Profiles, default profile, non-secret settings | `$XDG_CONFIG_HOME/figctl/config.yaml` | 0600 file in a 0700 directory |
| Tokens | OS keychain, service `figctl`, account `<profile>` | as the keychain provides |
| Tokens, fallback | `$XDG_CONFIG_HOME/figctl/credentials` | 0600 file in a 0700 directory |
| Project config | `.figctl.yaml` in the repository | committed, no secrets |
| Cache | `$XDG_CACHE_HOME/figctl/<profile>/<fileKey>/` | 0700 directories, 0600 files |

`$XDG_CONFIG_HOME` falls back to `~/.config` and `$XDG_CACHE_HOME` to
`~/.cache`.

### The keychain and the file fallback

By default figctl uses the OS keychain: Keychain on macOS, the Secret Service on
Linux, the Credential Manager on Windows. On a headless Linux box with no Secret
Service the keychain call fails, and figctl falls back to the 0600 credentials
file, warning on stderr when it does.

`FIGCTL_CREDENTIAL_STORE` forces the choice:

| Value | Behaviour |
| --- | --- |
| unset | keychain, falling back to the file with a warning |
| `keyring` | keychain only; fail if it is unavailable |
| `file` | the 0600 file only, no keychain call |

Set it to `file` in containers and CI images where the keychain is absent and
the warning is noise. Better still in CI: set `FIGMA_TOKEN` and skip the
credential store entirely.

## Cache isolation

The cache path starts with the profile name, so each account's data sits in its
own subtree. Two accounts can hold the same file key and see different roles,
different branches, and sometimes different content; keeping them apart also
means one client's design data is never mixed with another's on disk.

```sh
figctl cache status                                   # per profile and file
figctl cache clear --profile acme --yes               # one client
figctl cache clear --profile acme --file KEY --yes    # one file
```

## Token expiry

Figma personal access tokens expire after at most 90 days and the API does not
expose the expiry date. Record it at login and figctl will warn you within seven
days of it:

```sh
figctl auth login acme --expires 2026-12-01
figctl profile list
```

This is best effort: if you did not record a date, nothing can be warned about.

## When a file is not found

A 403 or 404 on a file is usually the wrong account rather than a missing file.
The error hint lists the other configured profiles and suggests `--profile`, so
the next command is obvious:

```sh
figctl file info KEY --profile globex
```
