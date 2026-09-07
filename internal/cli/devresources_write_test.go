package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func TestDevResourcesAddSingle(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "devresources", "add", figmatest.FileKey,
		"--node", "3:11", "--url", "https://storybook.example.com/?path=/story/button",
		"--name", "Storybook", "--yes")
	ok(t, r)
	d := data(t, r, "devresources.add")
	written := d["written"].([]any)
	if len(written) != 1 {
		t.Fatalf("written: %v", written)
	}
	got := written[0].(map[string]any)
	if got["nodeId"] != "3:11" || got["name"] != "Storybook" || got["id"] == "" {
		t.Fatalf("created link: %v", got)
	}
	if api.Count(http.MethodPost, "/v1/dev_resources") != 1 {
		t.Fatalf("expected one create call, got %d", api.Count(http.MethodPost, "/v1/dev_resources"))
	}
}

// TestDevResourcesAddDefaultsNameToHost keeps Dev Mode readable when the
// caller supplies only a URL.
func TestDevResourcesAddDefaultsNameToHost(t *testing.T) {
	setup(t)
	r := execute(t, "", "devresources", "add", figmatest.FileKey,
		"--node", "3-11", "--url", "https://storybook.example.com/x", "--yes")
	ok(t, r)
	written := data(t, r, "devresources.add")["written"].([]any)
	if got := written[0].(map[string]any)["name"]; got != "storybook.example.com" {
		t.Fatalf("name should default to the URL host, got %v", got)
	}
}

// TestDevResourcesAddFromFile covers the design-system case: a whole component
// library linked to its stories in one call.
func TestDevResourcesAddFromFile(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "stories.json")
	body := `[
	  {"nodeId":"3:11","name":"Button story","url":"https://sb.example.com/button"},
	  {"nodeId":"3:20","name":"Input story","url":"https://sb.example.com/input"}
	]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	r := execute(t, "", "devresources", "add", figmatest.FileKey, "--from-file", path, "--yes")
	ok(t, r)
	if written := data(t, r, "devresources.add")["written"].([]any); len(written) != 2 {
		t.Fatalf("expected two links, got %v", written)
	}

	// The same mapping over stdin, which is how a story index would pipe in.
	r = execute(t, body, "devresources", "add", figmatest.FileKey, "--from-file", "-", "--yes")
	ok(t, r)
	if written := data(t, r, "devresources.add")["written"].([]any); len(written) != 2 {
		t.Fatalf("stdin: expected two links, got %v", written)
	}
}

// TestDevResourcesAddPartial covers Figma answering 200 while rejecting some
// links. The accepted ones must survive and the rejected ones must be visible.
func TestDevResourcesAddPartial(t *testing.T) {
	setup(t)
	body := `[
	  {"nodeId":"3:11","name":"ok","url":"https://sb.example.com/ok"},
	  {"nodeId":"3:20","name":"dupe","url":"https://sb.example.com/duplicate"}
	]`
	r := execute(t, body, "devresources", "add", figmatest.FileKey, "--from-file", "-", "--yes")
	if r.code != figctl.ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitPartial, r.stdout)
	}
	d := data(t, r, "devresources.add")
	if len(d["written"].([]any)) != 1 || len(d["failures"].([]any)) != 1 {
		t.Fatalf("partial result: %v", d)
	}
	if fail := d["failures"].([]any)[0].(map[string]any); !strings.Contains(fail["error"].(string), "same url") {
		t.Fatalf("failure should explain the rejection: %v", fail)
	}
}

// TestDevResourcesAddAllRejected fails outright: nothing was written, so there
// is no partial result to keep.
func TestDevResourcesAddAllRejected(t *testing.T) {
	setup(t)
	r := execute(t, "", "devresources", "add", figmatest.FileKey,
		"--node", "3:11", "--url", "https://sb.example.com/duplicate", "--yes")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitUsage, r.stdout)
	}
	if !strings.Contains(r.stdout, "at most 10 dev resources") {
		t.Fatalf("hint should explain the limits:\n%s", r.stdout)
	}
}

func TestDevResourcesAddDryRunWritesNothing(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "devresources", "add", figmatest.FileKey,
		"--node", "3:11", "--url", "https://sb.example.com/x", "--dry-run")
	ok(t, r)
	if api.Count(http.MethodPost, "/v1/dev_resources") != 0 {
		t.Fatal("dry run must not write")
	}
	if !strings.Contains(r.stdout, "Dry run") {
		t.Fatalf("dry run should say so:\n%s", r.stdout)
	}
}

func TestDevResourcesUpdateAndRemove(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "devresources", "update", figmatest.FileKey,
		"--id", "dr-1", "--url", "https://sb.example.com/renamed", "--yes")
	ok(t, r)
	if written := data(t, r, "devresources.update")["written"].([]any); len(written) != 1 {
		t.Fatalf("updated: %v", written)
	}
	if api.Count(http.MethodPut, "/v1/dev_resources") != 1 {
		t.Fatal("expected one update call")
	}

	r = execute(t, "", "devresources", "remove", figmatest.FileKey, "--id", "dr-1", "--id", "dr-2", "--yes")
	ok(t, r)
	if written := data(t, r, "devresources.remove")["written"].([]any); len(written) != 2 {
		t.Fatalf("removed: %v", written)
	}
}

// TestDevResourcesWriteUsageErrors keeps the argument checks honest, since
// these commands change a shared design file.
func TestDevResourcesWriteUsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"add without a link", []string{"devresources", "add", figmatest.FileKey, "--yes"}, "no link to attach"},
		{"add without a node", []string{"devresources", "add", figmatest.FileKey, "--url", "https://x.test", "--yes"}, "no node selected"},
		{"update without a selector", []string{"devresources", "update", figmatest.FileKey, "--yes"}, "no dev resource selected"},
		{"update with nothing to change", []string{"devresources", "update", figmatest.FileKey, "--id", "dr-1", "--yes"}, "nothing to update"},
		{"remove without an id", []string{"devresources", "remove", figmatest.FileKey, "--yes"}, "no dev resource selected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setup(t)
			r := execute(t, "", tt.args...)
			if r.code != figctl.ExitUsage {
				t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitUsage, r.stdout)
			}
			if !strings.Contains(r.stdout, tt.want) {
				t.Fatalf("want %q in:\n%s", tt.want, r.stdout)
			}
		})
	}
}

// TestDevResourcesWriteRequiresConfirmation guards the write gate: these
// commands change someone's design file, so a non-interactive run without
// --yes must refuse.
func TestDevResourcesWriteRequiresConfirmation(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "devresources", "add", figmatest.FileKey,
		"--node", "3:11", "--url", "https://sb.example.com/x")
	if r.code == figctl.ExitOK {
		t.Fatalf("expected a refusal without --yes:\n%s", r.stdout)
	}
	if api.Count(http.MethodPost, "/v1/dev_resources") != 0 {
		t.Fatal("nothing may be written without confirmation")
	}
}
