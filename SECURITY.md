# Security policy

## Reporting a vulnerability

Report security issues privately. Do not open a public issue.

Use GitHub's private reporting form:
<https://github.com/tiaanduplessis/figctl/security/advisories/new>

Include the figctl version (`figctl version`), your operating system, what you
did, what happened, and what you expected. A proof of concept helps. Please do
not include a real Figma token in the report; redact it.

You should get an acknowledgement within seven days and an assessment within
fourteen. Fixes ship in a patch release with an advisory crediting you, unless
you would rather stay anonymous. Please give the fix a chance to ship before
disclosing publicly.

## How figctl handles your token

figctl authenticates with a Figma personal access token sent in the
`X-Figma-Token` request header.

**Storage.** `figctl auth login` writes the token to the OS keychain: Keychain
on macOS, the Secret Service on Linux, the Credential Manager on Windows, under
the service name `figctl` and the profile name as the account. Where no keychain
is available, typically a headless Linux box or a container, figctl falls back
to `$XDG_CONFIG_HOME/figctl/credentials`, a 0600 file inside a 0700 directory,
and warns on stderr that it did. `FIGCTL_CREDENTIAL_STORE=keyring` disables that
fallback; `FIGCTL_CREDENTIAL_STORE=file` skips the keychain entirely.

**Never a flag.** figctl does not accept the token as a command-line flag value,
because flags land in shell history, in `ps` output, and in CI logs. It is read
from `--token-file`, from stdin when stdin is not a terminal, or from a hidden
terminal prompt.

**Never in a URL.** The token goes in a request header only. It is never a query
parameter, so it cannot end up in a proxy log or a `Referer`.

**Never logged.** The `--verbose` HTTP trace prints methods, paths, status
codes, and rate limit headers, with the token header redacted. There is a test
asserting the redaction. `figctl profile show` never includes the token at
all.

**Config holds no secrets.** `$XDG_CONFIG_HOME/figctl/config.yaml` holds profile
names, team ids, base URLs, handles, and recorded expiry dates. It is written
0600 in a 0700 directory. The project file `.figctl.yaml` holds a profile name
and an optional file key and is meant to be committed.

**Cache.** Cached API responses and downloaded assets live under
`$XDG_CACHE_HOME/figctl/<profile>/<fileKey>/`, with 0700 directories and 0600
files, keyed by profile so one account's design data is not interleaved with
another's. No token is stored in the cache. Clear it with `figctl cache clear`.

**No telemetry.** figctl makes no network request other than to the Figma API
host for the active profile (`https://api.figma.com` by default) and to the
image CDN hosts Figma returns for renders and image fills. Nothing is reported
anywhere. There is no update check that phones home.

**One write.** The only command that writes to Figma is `comments add`, which
needs the `file_comments:write` scope. It asks for confirmation on a terminal
and requires `--yes` otherwise. Every other command is read-only.

## Reducing your exposure

- Request only the scopes you need. `figctl auth scopes` lists them and says
  which command needs each. Leave out `file_comments:write` if you never post
  comments.
- Figma tokens expire after at most 90 days. Record the date at login
  (`figctl auth login acme --expires 2026-12-01`) so figctl warns you within a
  week of expiry, and rotate rather than extending indefinitely.
- Use one profile per client. The cache and the token are both scoped to the
  profile, and `.figctl.yaml` in the client's repository selects it
  automatically, so a mistake in one repository cannot read another client's
  files.
- In CI, pass the token through `FIGMA_TOKEN` from the secret store and set
  `FIGCTL_CREDENTIAL_STORE=file` or nothing at all; do not run `auth login` in a
  pipeline.
- Add `.figctl/` to `.gitignore`. Rendered screenshots and exported assets go
  there by default and may contain unreleased design work.

## Supported versions

figctl is pre-1.0. Security fixes go onto the latest minor release only.
