package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tiaanduplessis/figctl/internal/cli"
	"github.com/tiaanduplessis/figctl/internal/docs"
)

// repoRoot is the module root relative to this package.
const repoRoot = "../.."

// TestCommandReferenceUpToDate regenerates the reference from the live
// command tree and compares it with the committed file, so a new command,
// flag, or example cannot ship without the documentation that describes it.
func TestCommandReferenceUpToDate(t *testing.T) {
	root := cli.Root()
	root.InitDefaultCompletionCmd()
	got := docs.Markdown(root)

	path := filepath.Join(repoRoot, "docs", "commands.md")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(want) != got {
		t.Fatalf("docs/commands.md is out of date with the command tree; run: make docs\n%s", firstDifference(string(want), got))
	}
}

// firstDifference reports the first line where two documents diverge, which
// is far more useful than dumping both.
func firstDifference(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return "first difference at line " + itoa(i+1) + ":\n  committed: " + wantLines[i] + "\n  generated: " + gotLines[i]
		}
	}
	return "committed has " + itoa(len(wantLines)) + " lines, generated has " + itoa(len(gotLines))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// nodePlatform maps a Go GOOS to the process.platform value Node reports.
var nodePlatform = map[string]string{
	"darwin":  "darwin",
	"linux":   "linux",
	"windows": "win32",
}

// nodeArch maps a Go GOARCH to the process.arch value Node reports.
var nodeArch = map[string]string{
	"amd64": "x64",
	"arm64": "arm64",
}

// jsPlatformEntry matches one entry of the PLATFORMS table in
// npm/postinstall.js: the Node key, then the Go os and arch it downloads.
var jsPlatformEntry = regexp.MustCompile(`"([a-z0-9]+)-([a-z0-9]+)":\s*\{\s*os:\s*"([a-z0-9]+)",\s*arch:\s*"([a-z0-9]+)"`)

// TestNPMWrapperCoversEveryReleaseTarget asserts that the platform table in
// the npm wrapper matches the GoReleaser build matrix exactly, so adding a
// release target cannot silently leave npm users without a binary.
func TestNPMWrapperCoversEveryReleaseTarget(t *testing.T) {
	var config struct {
		Builds []struct {
			Goos   []string `yaml:"goos"`
			Goarch []string `yaml:"goarch"`
		} `yaml:"builds"`
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yml: %v", err)
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parsing .goreleaser.yml: %v", err)
	}
	if len(config.Builds) == 0 {
		t.Fatal(".goreleaser.yml declares no builds")
	}

	want := map[string]string{}
	for _, build := range config.Builds {
		for _, goos := range build.Goos {
			for _, goarch := range build.Goarch {
				platform, ok := nodePlatform[goos]
				if !ok {
					t.Fatalf("GOOS %q has no Node process.platform mapping in this test", goos)
				}
				arch, ok := nodeArch[goarch]
				if !ok {
					t.Fatalf("GOARCH %q has no Node process.arch mapping in this test", goarch)
				}
				want[platform+"-"+arch] = goos + "/" + goarch
			}
		}
	}

	script, err := os.ReadFile(filepath.Join(repoRoot, "npm", "postinstall.js"))
	if err != nil {
		t.Fatalf("reading npm/postinstall.js: %v", err)
	}
	got := map[string]string{}
	for _, m := range jsPlatformEntry.FindAllStringSubmatch(string(script), -1) {
		got[m[1]+"-"+m[2]] = m[3] + "/" + m[4]
	}
	if len(got) == 0 {
		t.Fatal("no PLATFORMS entries found in npm/postinstall.js; the table's shape changed and this test needs updating")
	}

	for key, target := range want {
		actual, ok := got[key]
		if !ok {
			t.Errorf("npm/postinstall.js has no entry for %q (GoReleaser builds %s)", key, target)
			continue
		}
		if actual != target {
			t.Errorf("npm/postinstall.js maps %q to %s, but GoReleaser builds %s for it", key, actual, target)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("npm/postinstall.js has an entry for %q that GoReleaser does not build", key)
		}
	}

	if t.Failed() {
		keys := make([]string, 0, len(want))
		for key := range want {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		t.Logf("GoReleaser targets, as Node platform keys: %s", strings.Join(keys, ", "))
	}
}

// unameToGo maps what uname reports onto GOOS and GOARCH, mirroring the case
// arms of install.sh.
var unameToGoos = map[string]string{"Darwin": "darwin", "Linux": "linux"}

var unameToGoarch = map[string]string{
	"x86_64": "amd64", "amd64": "amd64",
	"arm64": "arm64", "aarch64": "arm64",
}

// TestInstallScriptCoversItsPlatforms keeps install.sh in step with the
// release matrix. The script serves macOS and Linux only, so Windows is
// expected to be absent and must be named in the refusal rather than silently
// unsupported.
func TestInstallScriptCoversItsPlatforms(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "install.sh"))
	if err != nil {
		t.Fatalf("reading install.sh: %v", err)
	}
	script := string(raw)

	var config struct {
		Builds []struct {
			Goos   []string `yaml:"goos"`
			Goarch []string `yaml:"goarch"`
		} `yaml:"builds"`
	}
	cfg, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yml: %v", err)
	}
	if err := yaml.Unmarshal(cfg, &config); err != nil {
		t.Fatalf("parsing .goreleaser.yml: %v", err)
	}

	built := map[string]bool{}
	for _, build := range config.Builds {
		for _, goos := range build.Goos {
			for _, goarch := range build.Goarch {
				built[goos+"/"+goarch] = true
			}
		}
	}

	// Every platform the script claims to handle must actually be built.
	for uname, goos := range unameToGoos {
		if !strings.Contains(script, uname+")") {
			t.Errorf("install.sh does not handle uname -s %q", uname)
		}
		for _, goarch := range unameToGoarch {
			if !built[goos+"/"+goarch] {
				t.Errorf("install.sh offers %s/%s but no release is built for it", goos, goarch)
			}
		}
	}
	for uname := range unameToGoarch {
		if !strings.Contains(script, uname) {
			t.Errorf("install.sh does not handle uname -m %q", uname)
		}
	}

	// Windows is built and shipped, so the script must say where to get it
	// rather than leaving the user at a bare refusal.
	if !built["windows/amd64"] {
		t.Fatal("expected a windows build; adjust this test if that changed")
	}
	if !strings.Contains(script, "Windows is served by npm") {
		t.Error("install.sh should name the supported route for Windows in its refusal")
	}

	// The asset name must match the archive template in .goreleaser.yml.
	if !strings.Contains(script, `${BINARY}_${version#v}_${plat}.tar.gz`) {
		t.Error("install.sh builds an asset name that no longer matches the archive template")
	}
	// The download must always be checked.
	for _, want := range []string{"checksums.txt", "verify_checksum", "sha256sum", "shasum"} {
		if !strings.Contains(script, want) {
			t.Errorf("install.sh should verify downloads; missing %q", want)
		}
	}
}
