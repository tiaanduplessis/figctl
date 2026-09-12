package docs_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallBehavior(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell installer supports macOS and Linux")
	}
	for _, tc := range []struct {
		name, failure          string
		noHash, cosign, latest bool
	}{
		{name: "pinned release"},
		{name: "latest release", latest: true},
		{name: "signed release", cosign: true},
		{name: "missing hashing tool", noHash: true, failure: "required to verify"},
		{name: "corrupt archive", failure: "checksum mismatch"},
		{name: "missing checksum", failure: "not listed in checksums.txt"},
		{name: "missing signature", cosign: true, failure: "could not download checksums.txt.sig"},
		{name: "missing certificate", cosign: true, failure: "could not download checksums.txt.pem"},
		{name: "invalid signature", cosign: true, failure: "signature did not verify"},
		{name: "unusable binary", failure: "downloaded binary did not run"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			commands := filepath.Join(root, "commands")
			destination := filepath.Join(root, "destination")
			for _, dir := range []string{commands, destination} {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, content string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"tar", "gzip", "mktemp", "rm", "sed", "head", "awk", "mkdir", "cp", "chmod", "mv"} {
				path, err := exec.LookPath(name)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(commands, name)); err != nil {
					t.Fatal(err)
				}
			}
			if !tc.noHash {
				hasher := "sha256sum"
				path, err := exec.LookPath(hasher)
				if err != nil {
					hasher = "shasum"
					path, err = exec.LookPath(hasher)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(commands, hasher)); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(commands, "uname"), "#!/bin/sh\ncase \"$1\" in -s) echo Linux;; -m) echo x86_64;; esac\n")
			write(filepath.Join(commands, "curl"), `#!/bin/sh
url="$2"
out="$4"
case "$url" in
 */releases/latest) printf '{"tag_name":"v0.1.0"}' > "$out" ;;
 */figctl_0.1.0_linux_amd64.tar.gz) cp "$FIXTURE/archive" "$out" ;;
 */checksums.txt) cp "$FIXTURE/checksums" "$out" ;;
 */checksums.txt.sig) [ "$CASE" != 'missing signature' ] || exit 22; echo sig > "$out" ;;
 */checksums.txt.pem) [ "$CASE" != 'missing certificate' ] || exit 22; echo cert > "$out" ;;
 *) exit 22 ;;
esac
`)
			if tc.cosign {
				write(filepath.Join(commands, "cosign"), `#!/bin/sh
[ "$CASE" != 'invalid signature' ] || exit 1
while [ "$#" -gt 0 ]; do
 if [ "$1" = --certificate-identity ]; then
  shift
  [ "$1" = 'https://github.com/tiaanduplessis/figctl/.github/workflows/release.yml@refs/tags/v0.1.0' ] || exit 1
  exit 0
 fi
 shift
done
exit 1
`)
			}
			binary := "#!/bin/sh\n[ \"$1\" = version ] || exit 1\necho 'figctl 0.1.0'\n"
			if tc.name == "unusable binary" {
				binary = "#!/bin/sh\nexit 1\n"
			}
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: "figctl", Mode: 0o755, Size: int64(len(binary))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(binary)); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(root, "archive"), archive.String())
			digest := sha256.Sum256(archive.Bytes())
			if tc.name == "corrupt archive" {
				digest[0] ^= 0xff
			}
			asset := "figctl_0.1.0_linux_amd64.tar.gz"
			if tc.name == "missing checksum" {
				asset = "another.tar.gz"
			}
			write(filepath.Join(root, "checksums"), fmt.Sprintf("%x  %s\n", digest, asset))
			installed := filepath.Join(destination, "figctl")
			write(installed, "existing installation\n")
			cmd := exec.Command("/bin/sh", filepath.Join(repoRoot, "install.sh"))
			version := "0.1.0"
			if tc.latest {
				version = ""
			}
			cmd.Env = []string{"PATH=" + commands, "HOME=" + root, "TMPDIR=" + root, "FIXTURE=" + root, "CASE=" + tc.name, "FIGCTL_VERSION=" + version, "FIGCTL_INSTALL_DIR=" + destination}
			output, err := cmd.CombinedOutput()
			if tc.failure == "" {
				if err != nil {
					t.Fatalf("install failed: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.failure) {
				t.Fatalf("expected failure %q, got %v\n%s", tc.failure, err, output)
			}
			got, err := os.ReadFile(installed)
			if err != nil {
				t.Fatal(err)
			}
			want := binary
			if tc.failure != "" {
				want = "existing installation\n"
			}
			if string(got) != want {
				t.Fatalf("installed binary changed unexpectedly: %q", got)
			}
			entries, err := os.ReadDir(destination)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("staging files were not cleaned up: %v", entries)
			}
		})
	}
}
