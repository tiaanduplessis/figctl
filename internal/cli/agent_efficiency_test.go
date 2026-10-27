package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// An agent checking cache availability must not receive the entire entry index.
func TestCacheStatusBoundsLargeInventories(t *testing.T) {
	setup(t)
	store, err := cache.Open(cache.Options{Profile: "env"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 250 {
		key := fmt.Sprintf("nodes?ids=%s-%03d", strings.Repeat("2:2,", 100), i)
		if err := store.Put(figmatest.FileKey, key, cache.Stamp{}, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	r := execute(t, "", "cache", "status")
	ok(t, r)
	t.Logf("cache summary: %d bytes for 250 entries", len(r.stdout))
	if len(r.stdout) > 4096 {
		t.Fatalf("cache summary flooded stdout: %d bytes", len(r.stdout))
	}
	file := data(t, r, "cache.status")["profiles"].([]any)[0].(map[string]any)["files"].([]any)[0].(map[string]any)
	if file["entryCount"] != float64(250) || len(file["entries"].([]any)) != 0 {
		t.Fatalf("summary: %v", file)
	}
	r = execute(t, "", "cache", "status", "--details", "--file", figmatest.FileKey, "--limit", "2")
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	file = data(t, r, "cache.status")["profiles"].([]any)[0].(map[string]any)["files"].([]any)[0].(map[string]any)
	if len(file["entries"].([]any)) != 2 || env["nextCursor"] == nil || env["truncated"] != true {
		t.Fatalf("detail page: %s", r.stdout)
	}
}

// Ticket links identify a discussion by comment id, including replies.
func TestCommentsThreadAndPagination(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "comments", "list", figmatest.FileKey, "--thread", "9002", "--limit", "1")
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	rows := list(t, r, "comments.list")
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != "9001" || env["nextCursor"] == nil {
		t.Fatalf("thread page: %s", r.stdout)
	}
	r = execute(t, "", "comments", "list", figmatest.FileKey, "--thread", "9002", "--limit", "1", "--cursor", env["nextCursor"].(string))
	ok(t, r)
	if rows = list(t, r, "comments.list"); len(rows) != 1 || rows[0].(map[string]any)["id"] != "9002" {
		t.Fatalf("reply: %s", r.stdout)
	}
	if api.Count(http.MethodGet, fileKeyPath("/comments")) != 1 {
		t.Fatal("pagination refetched the comment file")
	}
	r = execute(t, "", "comments", "list", figmatest.FileKey, "--resolved", "true")
	ok(t, r)
	if rows = list(t, r, "comments.list"); len(rows) != 1 || rows[0].(map[string]any)["id"] != "9003" {
		t.Fatalf("resolved thread: %s", r.stdout)
	}
	r = execute(t, "", "comments", "list", figmatest.FileKey, "--resolved", "true", "--since", "2026-08-26T00:00:00Z")
	ok(t, r)
	if len(list(t, r, "comments.list")) != 0 {
		t.Fatalf("date filter: %s", r.stdout)
	}
}

// A compact implementation bundle must keep the screenshot and disclose skipped intent.
func TestContextImplementationPreset(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2", "--preset", "implement", "--out", t.TempDir())
	ok(t, r)
	d := data(t, r, "node.context")
	if nodeEntry(t, d, 0)["screenshot"].(map[string]any)["scale"] != float64(1) {
		t.Fatal("preset must capture at 1x")
	}
	sections := d["sections"].(map[string]any)
	if sections["comments"].(map[string]any)["status"] != "skipped" || sections["assets"].(map[string]any)["status"] != "skipped" {
		t.Fatalf("sections: %v", sections)
	}
	if api.Count(http.MethodGet, fileKeyPath("/comments")) != 0 || api.Count(http.MethodGet, fileKeyPath("/dev_resources")) != 0 {
		t.Fatal("compact preset fetched optional intent")
	}
}

// Consecutive commands must not repeat a whole-document attempt that just failed.
func TestWholeFileNetworkFallbackSurvivesNextCommand(t *testing.T) {
	api := setup(t)
	failure := figmatest.Response{Status: 500, Body: map[string]any{"status": 500, "err": "Internal server error"}}
	api.Enqueue(http.MethodGet, fileKeyPath(""), failure, failure, failure)
	ok(t, execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2", "--depth", "1"))
	countWhole := func() int {
		n := 0
		for _, r := range api.Requests() {
			if r.Path == fileKeyPath("") && r.Query.Get("depth") == "" && r.Query.Get("ids") == "" {
				n++
			}
		}
		return n
	}
	before := countWhole()
	ok(t, execute(t, "", "file", "tree", figmatest.FileKey, "--node", "2:3", "--depth", "1"))
	if countWhole() != before {
		t.Fatalf("whole-file attempt repeated: before %d, after %d", before, countWhole())
	}
	// Explicit refresh must recover full-file reads and leave the recovered
	// document usable by the next invocation instead of deferring it again.
	ok(t, execute(t, "", "file", "get", figmatest.FileKey, "--refresh"))
	recovered := countWhole()
	ok(t, execute(t, "", "file", "get", figmatest.FileKey))
	if countWhole() != recovered {
		t.Fatal("recovered whole document was not served from cache")
	}
}

// A render manifest must describe the actual downloaded pixels rather than inferred frame dimensions.
func TestRenderReportsCaptureGeometry(t *testing.T) {
	setup(t)
	r := execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--scale", "1", "--out", filepath.Join(t.TempDir(), "render"))
	ok(t, r)
	item := items(t, data(t, r, "render"), "items")[0]
	if item["pixelWidth"] != float64(24) || item["pixelHeight"] != float64(24) || item["bounds"] == nil || item["background"] == nil {
		raw, _ := json.Marshal(item)
		t.Fatalf("missing capture geometry: %s", raw)
	}
}

// Slow discussion endpoints must not hide the usable model or claim no comments exist.
func TestContextOptionalBudgetPreservesCore(t *testing.T) {
	api := setup(t)
	api.Handle(http.MethodGet, fileKeyPath("/comments"), func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2", "--no-assets", "--extras-timeout", "25ms", "--out", t.TempDir())
	ok(t, r)
	d := data(t, r, "node.context")
	shot := nodeEntry(t, d, 0)["screenshot"].(map[string]any)
	mustExist(t, shot["path"].(string))
	sections := d["sections"].(map[string]any)
	if sections["comments"].(map[string]any)["status"] != "failed" || sections["devResources"].(map[string]any)["status"] != "failed" {
		t.Fatalf("unavailable intent: %v", sections)
	}
	if nodeEntry(t, d, 0)["inspect"] == nil {
		t.Fatal("optional timeout lost the inspected model")
	}
}

// A local manifest must select an unambiguous capture and preserve design attribution.
func TestDiffSelectsManifestCapture(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	r := execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--node", "2:9", "--scale", "1", "--out", dir)
	ok(t, r)
	manifest := filepath.Join(dir, "render.json")
	if err := os.WriteFile(manifest, []byte(r.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	actual := items(t, data(t, r, "render"), "items")[1]["path"].(string)
	r = execute(t, "", "diff", "--manifest", manifest, actual)
	if r.code != 2 {
		t.Fatalf("ambiguous reference should be rejected: %s", r.stdout)
	}
	original, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	r = execute(t, "", "diff", "--manifest", manifest, "--node", "2-9", actual, "--out", manifest)
	if r.code != 2 {
		t.Fatalf("manifest overwrite must be rejected: %s", r.stdout)
	}
	if got, err := os.ReadFile(manifest); err != nil || string(got) != string(original) {
		t.Fatal("comparison overwrote the manifest")
	}
	r = execute(t, "", "diff", "--manifest", manifest, "--node", "2-9", actual)
	ok(t, r)
	d := data(t, r, "diff")
	ref := d["reference"].(map[string]any)
	if d["passed"] != true || ref["nodeId"] != "2:9" || ref["file"].(map[string]any)["version"] != figmatest.Version || ref["pixelWidth"] != float64(24) {
		t.Fatalf("comparison provenance: %s", r.stdout)
	}
	// Context envelopes expose the same exact paths without another render.
	r = execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2", "--preset", "implement", "--out", dir)
	ok(t, r)
	if err := os.WriteFile(manifest, []byte(r.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	r = execute(t, "", "diff", "--manifest", manifest, actual)
	ok(t, r)
	if data(t, r, "diff")["reference"].(map[string]any)["nodeId"] != "2:2" {
		t.Fatalf("wrong context capture: %s", r.stdout)
	}
}

// An authorised comment write must not be hidden by a cached discussion read.
func TestCommentsWriteInvalidatesReadCache(t *testing.T) {
	api := setup(t)
	ok(t, execute(t, "", "comments", "list", figmatest.FileKey))
	ok(t, execute(t, "", "comments", "add", figmatest.FileKey, "A fixture comment", "--yes"))
	ok(t, execute(t, "", "comments", "list", figmatest.FileKey))
	if api.Count(http.MethodGet, fileKeyPath("/comments")) != 2 {
		t.Fatal("read after write reused stale comments")
	}
}
