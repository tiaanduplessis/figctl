# figctl (npm wrapper)

npm distribution of [figctl](https://github.com/tiaanduplessis/figctl), an
agent-first Figma CLI: one binary that reads a Figma file and hands a coding
agent everything it needs to implement the design.

```sh
npx figctl --help
npm install -g figctl
```

figctl itself is a static Go binary. This package downloads the release build
for your platform on install and runs it through a shim.

## What install does

1. Works out the platform: `darwin`, `linux`, or `win32`, on `x64` or `arm64`.
   Anything else fails with a message naming the supported platforms and the
   other ways to install.
2. Downloads the matching archive and `checksums.txt` from the GitHub release
   for this package's version.
3. Verifies the archive against its `sha256` line in `checksums.txt`. A mismatch
   aborts the install and writes nothing.
4. Unpacks the `figctl` binary next to the shim. No native modules, no
   post-install compilation, no runtime dependencies.

## Environment variables

| Variable | Effect |
| --- | --- |
| `FIGCTL_SKIP_DOWNLOAD=1` | skip the download; the shim then needs `figctl` on `PATH` |
| `FIGCTL_BINARY=/path/to/figctl` | install a binary you already have instead of downloading |
| `FIGCTL_DOWNLOAD_BASE=URL` | download the assets from somewhere other than GitHub releases |

Behind a proxy or an air-gapped network, download the release archive yourself
and point `FIGCTL_BINARY` at the extracted binary.

## Usage

Everything is documented in the
[main README](https://github.com/tiaanduplessis/figctl#readme) and the
[command reference](https://github.com/tiaanduplessis/figctl/blob/main/docs/commands.md).

```sh
npx figctl auth login personal
npx figctl file tree "https://www.figma.com/design/KEY/Web-App"
npx figctl node context KEY --node 2:2
npx figctl tokens export KEY --format css --out ./src/styles
```

## Alternatives

```sh
curl -fsSL https://raw.githubusercontent.com/tiaanduplessis/figctl/main/install.sh | sh
brew install tiaanduplessis/tap/figctl
go install github.com/tiaanduplessis/figctl/cmd/figctl@latest
```

## License

MIT. Copyright Tiaan du Plessis.
