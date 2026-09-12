package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// TestEndToEndAgentWorkflow drives the real built binary through the five step
// workflow the skill teaches, against the fake API, and checks that the
// artifacts an agent depends on actually land on disk. Every other test runs
// the commands in process, so this is the only coverage of the shipped binary
// end to end. It skips when the binary has not been built.
func TestEndToEndAgentWorkflow(t *testing.T) {
	bin, err := filepath.Abs("../../bin/figctl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("binary not built")
	}
	srv := figmatest.NewServer(t)
	work := t.TempDir()
	home := t.TempDir()

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"USERPROFILE="+home,
			"XDG_CONFIG_HOME="+filepath.Join(home, "cfg"),
			"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
			"FIGMA_TOKEN="+figmatest.Token,
			"FIGMA_API_BASE_URL="+srv.URL,
			"FIGCTL_CREDENTIAL_STORE=file",
			"NO_COLOR=1",
		)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	url := "https://www.figma.com/design/" + figmatest.FileKey + "/Fixture?node-id=2-2"

	// 1. orient
	tree := run("file", "tree", url, "--json")
	if !strings.Contains(tree, `"Login"`) {
		t.Fatal("tree missing Login frame")
	}
	// 2. the screen, in one call
	out := run("node", "context", url, "--out", filepath.Join(work, "ctx"), "--json")
	var env struct {
		Data struct {
			Nodes []struct {
				ID         string `json:"id"`
				Screenshot *struct {
					Path string `json:"path"`
				} `json:"screenshot"`
			} `json:"nodes"`
			Tokens []struct {
				Name string `json:"name"`
			} `json:"tokens"`
			Components []json.RawMessage `json:"components"`
			Assets     struct {
				Icons []struct {
					Path string `json:"path"`
				} `json:"icons"`
				ImageFills []struct {
					Path string `json:"path"`
				} `json:"imageFills"`
			} `json:"assets"`
		} `json:"data"`
		Profile *struct {
			Name string `json:"name"`
		} `json:"profile"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env.Profile == nil || env.Profile.Name == "" {
		t.Fatal("goal: every response echoes the active profile")
	}
	if len(env.Data.Nodes) != 1 || env.Data.Nodes[0].Screenshot == nil {
		t.Fatal("goal 2: no screenshot")
	}
	for _, p := range []string{env.Data.Nodes[0].Screenshot.Path} {
		if st, err := os.Stat(p); err != nil || st.Size() == 0 {
			t.Fatalf("goal 2: screenshot not on disk: %v", err)
		}
	}
	if len(env.Data.Tokens) == 0 {
		t.Fatal("goal 4: no tokens resolved")
	}
	if len(env.Data.Components) == 0 {
		t.Fatal("goal 6: no components")
	}
	if len(env.Data.Assets.Icons) == 0 || len(env.Data.Assets.ImageFills) == 0 {
		t.Fatal("goal 5: assets missing")
	}
	for _, a := range env.Data.Assets.Icons {
		if _, err := os.Stat(a.Path); err != nil {
			t.Fatalf("goal 5: icon not on disk: %v", err)
		}
	}
	// 4. render to compare
	run("render", url, "--out", filepath.Join(work, "r"), "--json")
	// 5. design system
	css := run("tokens", "export", figmatest.FileKey, "--format", "css", "--json")
	if !strings.Contains(css, "--space-4") {
		t.Fatal("goal 4: css tokens missing codeSyntax name")
	}
	t.Logf("tokens=%d components=%d icons=%d fills=%d",
		len(env.Data.Tokens), len(env.Data.Components),
		len(env.Data.Assets.Icons), len(env.Data.Assets.ImageFills))
}
