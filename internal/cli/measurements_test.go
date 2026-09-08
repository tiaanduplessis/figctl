package cli

import (
	"strings"
	"testing"

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
