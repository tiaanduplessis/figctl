package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// writeProject puts a .figctl.yaml in the working directory.
func writeProject(t *testing.T, body string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".figctl.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRefResolvesAConfiguredName covers the reason named files exist: a
// design system lives in its own file, so a repository refers to several and
// a command should name one rather than carry a key.
func TestRefResolvesAConfiguredName(t *testing.T) {
	setup(t)
	writeProject(t, "profile: env\nfiles:\n  app: "+figmatest.FileKey+"\n  design-system: "+figmatest.FileKey+"\n")

	r := execute(t, "", "file", "info", "design-system")
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	if env["file"].(map[string]any)["key"] != figmatest.FileKey {
		t.Fatalf("the name should resolve to its key: %v", env["file"])
	}
}

// TestRefFallsBackToTheDefault covers omitting the ref entirely.
func TestRefFallsBackToTheDefault(t *testing.T) {
	setup(t)
	writeProject(t, "profile: env\ndefault: app\nfiles:\n  app: "+figmatest.FileKey+"\n  website: OtHeRkEy000000000000x\n")

	r := execute(t, "", "file", "info")
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	if env["file"].(map[string]any)["key"] != figmatest.FileKey {
		t.Fatalf("the default should have been used: %v", env["file"])
	}
}

// TestSingleFileIsTheDefault: with one configured file there is nothing else
// a bare command could mean, so naming a default should not be required.
func TestSingleFileIsTheDefault(t *testing.T) {
	setup(t)
	writeProject(t, "profile: env\nfiles:\n  app: "+figmatest.FileKey+"\n")

	r := execute(t, "", "file", "info")
	ok(t, r)
	if decodeEnvelope(t, r.stdout)["file"].(map[string]any)["key"] != figmatest.FileKey {
		t.Fatal("a single configured file should be the default")
	}
}

// TestUnknownNameListsTheConfiguredOnes keeps a typo actionable.
func TestUnknownNameListsTheConfiguredOnes(t *testing.T) {
	setup(t)
	writeProject(t, "profile: env\nfiles:\n  app: "+figmatest.FileKey+"\n  design-system: OtHeRkEy000000000000x\n")

	r := execute(t, "", "file", "info", "desing-system")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitUsage, r.stdout)
	}
	for _, want := range []string{"app", "design-system"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the error should list %q:\n%s", want, r.stdout)
		}
	}
}

// TestNoRefAndNoDefaultExplainsTheChoice covers several files with no default.
func TestNoRefAndNoDefaultExplainsTheChoice(t *testing.T) {
	setup(t)
	writeProject(t, "profile: env\nfiles:\n  app: "+figmatest.FileKey+"\n  website: OtHeRkEy000000000000x\n")

	r := execute(t, "", "file", "info")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exit = %d, want %d", r.code, figctl.ExitUsage)
	}
	if !strings.Contains(r.stdout, "app") || !strings.Contains(r.stdout, "--default") {
		t.Fatalf("the hint should name the files and how to pick one:\n%s", r.stdout)
	}
}

// TestKeysAndURLsStillWork keeps named files additive: a key or a URL
// must resolve whether or not a repository config exists.
func TestKeysAndURLsStillWork(t *testing.T) {
	setup(t)
	writeProject(t, "profile: env\nfiles:\n  app: OtHeRkEy000000000000x\n")

	for _, arg := range []string{
		figmatest.FileKey,
		"https://www.figma.com/design/" + figmatest.FileKey + "/Fixture",
	} {
		r := execute(t, "", "file", "info", arg)
		ok(t, r)
		if decodeEnvelope(t, r.stdout)["file"].(map[string]any)["key"] != figmatest.FileKey {
			t.Fatalf("%s should win over the configured name", arg)
		}
	}
}

// TestInitWritesNamedFiles covers the config that produces all of the above.
func TestInitWritesNamedFiles(t *testing.T) {
	setup(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	r := execute(t, "", "init", "--profile", "env",
		"--file", "app=https://www.figma.com/design/"+figmatest.FileKey+"/App",
		"--file", "design-system=OtHeRkEy000000000000x",
		"--default", "app")
	ok(t, r)

	raw, err := os.ReadFile(filepath.Join(cwd, ".figctl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"profile: env", "default: app", "app: " + figmatest.FileKey, "design-system: OtHeRkEy000000000000x"} {
		if !strings.Contains(body, want) {
			t.Errorf("config is missing %q:\n%s", want, body)
		}
	}
	// A secret must never reach a file meant to be committed.
	if strings.Contains(body, figmatest.Token) {
		t.Fatal("the project config must not contain a token")
	}
}

// TestInitRejectsAMalformedFileFlag and a default that does not exist: both
// mistakes are cheaper to catch here than at the first command that relies
// on them.
func TestInitRejectsBadInput(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"a file without a name", []string{"--file", figmatest.FileKey}, "<name>=<key or URL>"},
		{"an empty key", []string{"--file", "app="}, "<name>=<key or URL>"},
		{"a default that is not configured", []string{"--file", "app=" + figmatest.FileKey, "--default", "web"}, "no such file is configured"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setup(t)
			args := append([]string{"init", "--profile", "env"}, tt.args...)
			r := execute(t, "", args...)
			if r.code != figctl.ExitUsage {
				t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitUsage, r.stdout)
			}
			if !strings.Contains(r.stdout, tt.want) {
				t.Fatalf("want %q in:\n%s", tt.want, r.stdout)
			}
		})
	}
}
