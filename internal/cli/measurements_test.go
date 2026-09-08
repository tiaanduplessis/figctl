package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// TestNodeContextIncludesMeasurements covers the Dev Mode measurements a
// designer pinned on the page. They state a spacing decision outright, which
// is the one place a gap does not have to be inferred from layout or read off
// a screenshot, so node context must surface them.
func TestNodeContextIncludesMeasurements(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2",
		"--out", dir, "--no-screenshot", "--no-assets")
	ok(t, r)
	d := data(t, r, "node.context")

	list, isList := d["measurements"].([]any)
	if !isList || len(list) != 2 {
		t.Fatalf("expected the two fixture measurements, got %v", d["measurements"])
	}
	if got := d["summary"].(map[string]any)["measurements"]; got != float64(2) {
		t.Fatalf("summary count = %v, want 2", got)
	}

	// M:1 pins the heading's bottom to the input's top: 114 - (24+34) = 56.
	first := list[0].(map[string]any)
	if first["id"] != "M:1" || first["axis"] != "vertical" {
		t.Fatalf("first measurement: %v", first)
	}
	if first["distance"] != float64(56) || first["css"] != "56px" || first["label"] != "56px" {
		t.Fatalf("expected a resolved 56px gap, got %v", first)
	}
	start := first["start"].(map[string]any)
	if start["nodeId"] != "2:3" || start["name"] != "Heading" || start["side"] != "BOTTOM" {
		t.Fatalf("start pin should name the layer: %v", start)
	}

	// M:2 measures 16px but the designer overrode the label, and the override
	// is what the design actually specifies.
	second := list[1].(map[string]any)
	if second["distance"] != float64(16) {
		t.Fatalf("second measurement should still report the measured gap: %v", second)
	}
	if second["freeText"] != "16-24 responsive" || second["label"] != "16-24 responsive" {
		t.Fatalf("the designer's own text should win: %v", second)
	}

	// The agent has to be told these exist, or it will read gaps off the image.
	env := decodeEnvelope(t, r.stdout)
	hints, _ := env["hints"].([]any)
	found := false
	for _, h := range hints {
		if s, isString := h.(string); isString && strings.Contains(s, "pinned 2 measurement") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a hint about the measurements, got %v", hints)
	}
}

// TestNodeContextMeasurementsMarkdown checks the form an agent reading text
// actually sees.
func TestNodeContextMeasurementsMarkdown(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2",
		"--out", dir, "--no-screenshot", "--no-assets", "-o", "md")
	ok(t, r)
	for _, want := range []string{
		"## Measurements",
		"Prefer these over a gap read off the screenshot.",
		"| value | axis | from | to | note |",
		"56px",
		"Heading bottom",
		"16-24 responsive (set by the designer)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
}

// TestNodeContextMeasurementsScopedToTheNode keeps one screen's measurements
// out of another's context.
func TestNodeContextMeasurementsScopedToTheNode(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	// 3:10 is the Button component set on the other page; nothing is pinned
	// to it.
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "3:10",
		"--out", dir, "--no-screenshot", "--no-assets")
	ok(t, r)
	d := data(t, r, "node.context")
	if list, _ := d["measurements"].([]any); len(list) != 0 {
		t.Fatalf("no measurement is pinned to this node, got %v", list)
	}
}

// TestNodeContextMeasurementsOnALargeFile covers the path that matters in
// practice. A production file is often too large for Figma to return whole,
// and measurements are pinned on the page rather than on the node, so a
// fallback that fetches only the node subtree loses them entirely along with
// the page name and the ancestor path.
func TestNodeContextMeasurementsOnALargeFile(t *testing.T) {
	api := setup(t)
	// Refuse the whole document exactly as Figma does for a large file, while
	// leaving the id-scoped fetch working.
	api.Handle(http.MethodGet, "/v1/files/"+figmatest.FileKey, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("ids") == "" && q.Get("depth") == "" {
			figmatest.WriteResponse(w, figmatest.Response{
				Status: http.StatusBadRequest,
				Body:   map[string]any{"status": 400, "err": "Request too large. If applicable, filter by query params."},
			})
			return
		}
		api.ServeDefault(w, r)
	})

	dir := t.TempDir()
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2",
		"--out", dir, "--no-screenshot", "--no-assets")
	ok(t, r)
	d := data(t, r, "node.context")

	if list, _ := d["measurements"].([]any); len(list) != 2 {
		t.Fatalf("measurements should survive the large-file fallback, got %v", d["measurements"])
	}
	node := d["nodes"].([]any)[0].(map[string]any)
	if node["page"] != "Screens" {
		t.Errorf("the page should be known through the scoped fetch, got %v", node["page"])
	}
	if path, _ := node["path"].([]any); len(path) == 0 {
		t.Error("the ancestor path should be known through the scoped fetch")
	}
}

// TestNodeContextSurvivesARenderTimeout covers a large frame that Figma is
// slow to render. The inspected model, the tokens, and the components are
// already resolved by then, and they are the expensive part; discarding them
// because an image did not arrive turns a recoverable problem into a failed
// command.
func TestNodeContextSurvivesARenderTimeout(t *testing.T) {
	api := setup(t)
	api.Handle(http.MethodGet, "/v1/images/"+figmatest.FileKey, func(w http.ResponseWriter, _ *http.Request) {
		figmatest.WriteResponse(w, figmatest.Response{
			Status: http.StatusGatewayTimeout,
			Body:   map[string]any{"status": 504, "err": "Gateway timeout"},
		})
	})

	dir := t.TempDir()
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2",
		"--out", dir, "--no-assets")
	// Partial, not a failure: the bundle is usable.
	if r.code != figctl.ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s\n%s", r.code, figctl.ExitPartial, r.stdout, r.stderr)
	}
	d := data(t, r, "node.context")
	if nodes, _ := d["nodes"].([]any); len(nodes) != 1 {
		t.Fatalf("the inspected model must survive: %v", d["nodes"])
	}
	if tokens, _ := d["tokens"].([]any); len(tokens) == 0 {
		t.Error("the tokens must survive a render failure")
	}
	failures, _ := d["failures"].([]any)
	if len(failures) == 0 {
		t.Fatal("the lost screenshot should be reported in failures")
	}
	if kind := failures[0].(map[string]any)["kind"]; kind != "screenshot" {
		t.Errorf("failure kind = %v, want screenshot", kind)
	}
}
