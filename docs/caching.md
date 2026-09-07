# Caching and rate limits

Caching in figctl is not an optimization. Figma's rate limits make it the only
way the tool can work at all.

## The limits

Figma has enforced tiered rate limits since November 2025. The tier figctl needs
most is the tightest.

| Tier | Endpoints | Limit |
| --- | --- | --- |
| 1 | get file, get nodes, render images | 10 to 30 per minute on Dev and Full seats depending on plan; 20 per **month** on View and Collab seats |
| 2 | variables, versions, dev resources, comments, image fills, folders | 25 to 150 per minute |
| 3 | components, styles, file meta, users | 50 to 200 per minute |

A 429 response carries `Retry-After`, plus `X-Figma-Plan-Tier`,
`X-Figma-Rate-Limit-Type`, and `X-Figma-Upgrade-Link`. figctl records all four.

Ten Tier 1 requests a minute is roughly one file fetch every six seconds. An
agent inspecting a screen frame by frame would exhaust that in the first minute.
On a View or Collab seat, twenty Tier 1 requests is the entire month.

## The whole-file strategy

On the first Tier 1 touch of a file, figctl fetches the complete document once
and stores it. Everything that can be answered from a document tree is then
answered locally:

- `file tree`, `file find`, `file get`
- `node inspect`, `node context`
- the component and component-set maps used to name instances
- the style nodes referenced by the document

So a session that runs `file tree`, then `file find`, then `node inspect` on
four nodes, then `node context` on one of them costs one Tier 1 file request
rather than seven. Repeating any of those on an unchanged file costs zero.

Renders and downloaded assets are cached separately, keyed by node, format,
scale, and file version, so re-running `render` or `assets export` on an
unchanged file costs no image requests either.

### Large files

A file whose document exceeds `--max-file-mb` (200 by default) is not fetched
whole. Those commands fall back to `depth=2` for an overview, or one `GET nodes`
request for the subtree under `--node`, and the response carries a hint saying
which fallback was used and what is missing. Raise `--max-file-mb` if you have
the memory and would rather pay one big request than many small ones.

## Where the cache lives

```
$XDG_CACHE_HOME/figctl/<profile>/<fileKey>/
```

falling back to `~/.cache/figctl` when `XDG_CACHE_HOME` is unset. Directories are
created 0700 and files 0600. Entries are gzip-compressed JSON keyed by endpoint
and query parameters; downloaded blobs live alongside them.

The profile comes first in the path on purpose. Two accounts see different
roles, different branches, and sometimes different content for the same file
key, and a consultant should not have one client's design data interleaved with
another's on disk. `figctl cache clear --profile acme` removes exactly one
client's data.

## Validation

A cached entry is stamped with the file version it came from. Before serving it,
figctl needs to know whether the file has changed, and asking that question must
not cost a Tier 1 request.

It uses the file meta endpoint, which is Tier 3 (50 to 200 per minute) and
returns `last_touched_at`. That check is made at most once per TTL, 60 seconds by
default, per file per command run. If `last_touched_at` is unchanged, the cached
document is served with no further request.

With `--file-version` the version is pinned, so there is nothing to validate and
even the meta call is skipped.

## Controlling the cache

| Flag | Effect |
| --- | --- |
| (none) | read from the cache when valid, write what is fetched |
| `--refresh` | ignore what is cached, fetch, and rewrite the entry |
| `--no-cache` | ignore what is cached, fetch, and write nothing |
| `--file-version ID` | pin a version; the entry is keyed by it and never revalidated |
| `--max-file-mb N` | largest document fetched whole; bigger files use the fallbacks |

Use `--refresh` after a designer tells you they changed something and you do not
want to wait out the 60-second TTL. Use `--no-cache` when reproducing a bug.
Neither belongs in an agent's default command line: adding `--refresh` to every
call converts a one-request session back into a seven-request one.

## Inspecting the cache

```sh
figctl cache status          # size and entries per profile and file
figctl cache status --json
figctl cache clear --yes                              # everything
figctl cache clear --profile acme --yes               # one account
figctl cache clear --profile acme --file KEY --yes    # one file
```

`cache clear` asks for confirmation on a terminal and requires `--yes`
otherwise, so an agent cannot delete a cache by accident.

## When a rate limit is hit anyway

A 429 or a 5xx is retried up to three times. `Retry-After` is honoured when the
response carries one, otherwise the wait is exponential backoff with jitter,
capped at 8 seconds, and the whole chain is bounded by `--timeout`.

If the retries do not clear it, the error envelope carries the wait and the
next thing to try:

```json
{
  "schemaVersion": 1,
  "error": {
    "code": "RATE_LIMITED",
    "message": "Figma rate limit hit (plan: pro, seat limit: high)",
    "hint": "Retry after 42s. Reuse cached data instead of re-fetching; the file cache serves file tree, find, inspect and get without new requests.",
    "retryAfterSeconds": 42,
    "httpStatus": 429,
    "details": {"planTier": "pro", "rateLimitType": "high"}
  }
}
```

The exit code is 5. An agent should wait `retryAfterSeconds` and retry, not
switch to `--no-cache` and try harder.

Figma applies the limits per user, so a second client's profile has its own
budget: running against one account does not consume another's. Use `--verbose`
to see the rate limit headers of every response on stderr; tokens are redacted
from that trace.
