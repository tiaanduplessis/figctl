#!/bin/sh
# Install figctl, the agent-first Figma CLI.
#
#   curl -fsSL https://raw.githubusercontent.com/tiaanduplessis/figctl/main/install.sh | sh
#
# Environment:
#   FIGCTL_VERSION      release tag to install, for example v0.1.0. Defaults to
#                       the latest release. Pin this in CI so a build does not
#                       change under you.
#   FIGCTL_INSTALL_DIR  where to put the binary. Defaults to /usr/local/bin
#                       when it is writable, otherwise ~/.local/bin.
#
# The download is always verified against the release checksums. When cosign is
# on PATH the checksum file's signature is verified too.
set -eu

REPO="tiaanduplessis/figctl"
BINARY="figctl"

log() { printf '%s\n' "$*" >&2; }

fail() {
	printf 'install: %s\n' "$*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required but was not found on PATH"
}

# fetch writes a URL to a file, using whichever downloader is present.
fetch() {
	url="$1"
	out="$2"
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$out" || return 1
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$out" "$url" || return 1
	else
		fail "curl or wget is required"
	fi
}

# target maps uname output onto a release asset name. The pairs here must stay
# in step with the GoReleaser build matrix; a test cross-checks them.
target() {
	os=$(uname -s)
	arch=$(uname -m)
	case "$os" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*)
		fail "unsupported operating system: $os. Windows is served by npm (npm i -g figctl) or a release archive."
		;;
	esac
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*)
		fail "unsupported architecture: $arch. Build from source with: go install github.com/$REPO/cmd/$BINARY@latest"
		;;
	esac
	printf '%s_%s' "$os" "$arch"
}

# latest_version resolves the newest published release tag.
latest_version() {
	body=$(mktemp)
	if ! fetch "https://api.github.com/repos/$REPO/releases/latest" "$body"; then
		rm -f "$body"
		fail "could not reach the release feed. Set FIGCTL_VERSION to install a specific tag."
	fi
	tag=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$body" | head -n 1)
	rm -f "$body"
	[ -n "$tag" ] || fail "no release has been published yet. Install from source with: go install github.com/$REPO/cmd/$BINARY@latest"
	printf '%s' "$tag"
}

# install_dir picks a writable destination without ever escalating privileges
# on its own. Asking for a password from a piped script is not something a
# reader can inspect first.
install_dir() {
	if [ -n "${FIGCTL_INSTALL_DIR:-}" ]; then
		printf '%s' "$FIGCTL_INSTALL_DIR"
		return
	fi
	if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		printf '%s' /usr/local/bin
		return
	fi
	printf '%s' "$HOME/.local/bin"
}

# verify_checksum compares the archive against its line in checksums.txt.
verify_checksum() {
	archive="$1"
	sums="$2"
	name="$3"
	expected=$(awk -v name="$name" '$2 == name || $2 == "*" name { print $1; exit }' "$sums")
	[ -n "$expected" ] || fail "$name is not listed in checksums.txt"
	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum "$archive" | awk '{print $1}')
	elif command -v shasum >/dev/null 2>&1; then
		actual=$(shasum -a 256 "$archive" | awk '{print $1}')
	else
		fail "sha256sum or shasum is required to verify the download"
	fi
	[ "$actual" = "$expected" ] || fail "checksum mismatch for $name: expected $expected, got $actual"
	log "install: checksum verified"
}

# verify_signature checks the cosign signature when cosign is available. It is
# not required, because most machines do not have cosign, but when it is there
# the extra proof is free.
verify_signature() {
	sums="$1"
	base="$2"
	command -v cosign >/dev/null 2>&1 || return 0
	sig="$sums.sig"
	cert="$sums.pem"
	fetch "$base/checksums.txt.sig" "$sig" || fail "could not download checksums.txt.sig"
	fetch "$base/checksums.txt.pem" "$cert" || fail "could not download checksums.txt.pem"
	if cosign verify-blob "$sums" \
		--signature "$sig" --certificate "$cert" \
		--certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$version" \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com >/dev/null 2>&1; then
		log "install: signature verified"
	else
		fail "the checksum signature did not verify. Do not use this download."
	fi
}

main() {
	need uname
	need tar

	version="${FIGCTL_VERSION:-}"
	[ -n "$version" ] || version=$(latest_version)
	case "$version" in
	v*) ;;
	*) version="v$version" ;;
	esac

	plat=$(target)
	name="${BINARY}_${version#v}_${plat}.tar.gz"
	base="https://github.com/$REPO/releases/download/$version"

	tmp=$(mktemp -d)
	staged=""
	trap 'rm -rf "$tmp"; if [ -n "$staged" ]; then rm -f "$staged"; fi' EXIT
	trap 'exit 1' INT TERM

	log "install: downloading $BINARY $version for $plat"
	fetch "$base/$name" "$tmp/$name" ||
		fail "could not download $name. Check that $version exists at https://github.com/$REPO/releases"
	fetch "$base/checksums.txt" "$tmp/checksums.txt" ||
		fail "could not download checksums.txt for $version"

	verify_signature "$tmp/checksums.txt" "$base"
	verify_checksum "$tmp/$name" "$tmp/checksums.txt" "$name"

	tar -xzf "$tmp/$name" -C "$tmp" ||
		fail "could not unpack $name"
	[ -f "$tmp/$BINARY" ] || fail "$name did not contain a $BINARY binary"

	dir=$(install_dir)
	mkdir -p "$dir" || fail "could not create $dir"
	if [ ! -w "$dir" ]; then
		fail "$dir is not writable. Set FIGCTL_INSTALL_DIR to a directory you own, or re-run with sudo."
	fi
	# Validate on the destination filesystem before replacing an existing binary.
	staged=$(mktemp "$dir/.figctl.XXXXXX") || fail "could not stage the binary in $dir"
	cp "$tmp/$BINARY" "$staged" || fail "could not copy the binary into $dir"
	chmod 0755 "$staged"
	"$staged" version >/dev/null 2>&1 ||
		fail "the downloaded binary did not run; check the architecture and executable permissions on $dir"
	mv -f "$staged" "$dir/$BINARY" || fail "could not install into $dir"
	staged=""

	log "install: installed $dir/$BINARY"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) log "install: $dir is not on PATH; add it with: export PATH=\"$dir:\$PATH\"" ;;
	esac
}

main "$@"
