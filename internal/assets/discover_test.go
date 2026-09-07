package assets

import (
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func fixtureFile(t *testing.T) *figma.GetFileResponse {
	t.Helper()
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	return &file
}

func allOptions() DiscoverOptions {
	return DiscoverOptions{IncludeExportSettings: true, IncludeIcons: true, IncludeImageFills: true}
}

func TestDiscoverFixture(t *testing.T) {
	file := fixtureFile(t)
	d := Discover([]*figma.Node{file.Document}, allOptions())

	if len(d.Exports) != 3 {
		t.Fatalf("exports = %+v", d.Exports)
	}
	login := d.Exports[0]
	if login.NodeID != "2:2" || login.Name != "Login" || login.Format != "png" || login.Scale != 2 || login.Suffix != "" || login.Constraint != "SCALE 2" {
		t.Fatalf("login export: %+v", login)
	}
	if d.Exports[1].NodeID != "2:9" || d.Exports[1].Format != "svg" || d.Exports[1].Scale != 1 {
		t.Fatalf("arrow svg export: %+v", d.Exports[1])
	}
	if d.Exports[2].NodeID != "2:9" || d.Exports[2].Format != "png" || d.Exports[2].Scale != 2 || d.Exports[2].Suffix != "@2x" {
		t.Fatalf("arrow png export: %+v", d.Exports[2])
	}

	if len(d.Icons) != 1 {
		t.Fatalf("icons = %+v", d.Icons)
	}
	if icon := d.Icons[0]; icon.NodeID != "2:9" || icon.Name != "icon/arrow-right" || icon.Type != "VECTOR" || icon.MatchedBy != "pattern" || icon.Width != 24 {
		t.Fatalf("icon: %+v", icon)
	}

	if len(d.ImageFills) != 1 {
		t.Fatalf("image fills = %+v", d.ImageFills)
	}
	fill := d.ImageFills[0]
	if fill.ImageRef != "img1" || fill.ScaleMode != "FILL" || len(fill.Nodes) != 1 || fill.Nodes[0].NodeID != "2:8" || fill.Nodes[0].Name != "Avatar" {
		t.Fatalf("fill: %+v", fill)
	}
}

func TestDiscoverScopesAndOptions(t *testing.T) {
	file := fixtureFile(t)
	// Only the export category.
	d := Discover([]*figma.Node{file.Document}, DiscoverOptions{IncludeExportSettings: true})
	if len(d.Exports) != 3 || len(d.Icons) != 0 || len(d.ImageFills) != 0 {
		t.Fatalf("exports only: %+v", d)
	}
	// A subtree without any candidates.
	stats := file.Document.Find("2:14")
	d = Discover([]*figma.Node{stats}, allOptions())
	if len(d.Exports)+len(d.Icons)+len(d.ImageFills) != 0 {
		t.Fatalf("stats subtree: %+v", d)
	}
	// Several roots keep document order and do not duplicate.
	d = Discover([]*figma.Node{file.Document.Find("2:7"), file.Document.Find("2:2")}, allOptions())
	if len(d.Icons) != 2 || d.Icons[0].NodeID != "2:9" || d.Icons[1].NodeID != "2:9" {
		t.Fatalf("two roots: %+v", d.Icons)
	}
	if len(d.ImageFills) != 1 || len(d.ImageFills[0].Nodes) != 1 {
		t.Fatalf("fills across roots should merge by ref and node: %+v", d.ImageFills)
	}
	// Custom patterns.
	d = Discover([]*figma.Node{file.Document}, DiscoverOptions{IncludeIcons: true, IconPatterns: []string{"button/*"}})
	if len(d.Icons) != 1 || d.Icons[0].NodeID != "2:6" || d.Icons[0].Type != "INSTANCE" {
		t.Fatalf("custom pattern: %+v", d.Icons)
	}
	d = Discover([]*figma.Node{file.Document}, DiscoverOptions{IncludeIcons: true, IconPatterns: []string{"nothing/*"}})
	if len(d.Icons) != 0 {
		t.Fatalf("no match expected: %+v", d.Icons)
	}
}

func TestDiscoverHiddenAndVectorFrames(t *testing.T) {
	yes, no := true, false
	root := &figma.Node{ID: "1:0", Type: "FRAME", Name: "Root", Children: []*figma.Node{
		{ID: "1:1", Type: "FRAME", Name: "Debug", Visible: &no, ExportSettings: []figma.ExportSetting{{Format: "PNG", Constraint: figma.Constraint{Type: "SCALE", Value: 1}}}, Children: []*figma.Node{
			{ID: "1:2", Type: "VECTOR", Name: "icon/hidden"},
		}},
		{ID: "1:3", Type: "FRAME", Name: "Shapes", AbsoluteBoundingBox: &figma.Rectangle{Width: 48, Height: 24}, Children: []*figma.Node{
			{ID: "1:4", Type: "ELLIPSE", Name: "Dot"},
			{ID: "1:5", Type: "BOOLEAN_OPERATION", Name: "Union", Children: []*figma.Node{{ID: "1:6", Type: "VECTOR", Name: "icon/inner"}}},
		}},
		{ID: "1:7", Type: "FRAME", Name: "Photo", Children: []*figma.Node{
			{ID: "1:8", Type: "RECTANGLE", Name: "Pic", Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref-abc", ScaleMode: "FIT"}}},
		}},
		{ID: "1:9", Type: "FRAME", Name: "Empty"},
		{ID: "1:10", Type: "GROUP", Name: "Group", Children: []*figma.Node{{ID: "1:11", Type: "LINE", Name: "L"}}},
		{ID: "1:12", Type: "RECTANGLE", Name: "Banner", Visible: &yes, ExportSettings: []figma.ExportSetting{
			{Format: "JPG", Suffix: "-wide", Constraint: figma.Constraint{Type: "WIDTH", Value: 200}},
			{Format: "PNG", Constraint: figma.Constraint{Type: "HEIGHT", Value: 100}},
			{Format: "SVG", Constraint: figma.Constraint{Type: "SCALE", Value: 3}},
		}, AbsoluteBoundingBox: &figma.Rectangle{Width: 100, Height: 20}, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref-abc", Visible: &no}}},
	}}
	d := Discover([]*figma.Node{root}, allOptions())
	if len(d.Icons) != 1 || d.Icons[0].NodeID != "1:3" || d.Icons[0].MatchedBy != "vectors" || d.Icons[0].Width != 48 {
		t.Fatalf("icons = %+v", d.Icons)
	}
	if len(d.Exports) != 3 || d.Exports[0].NodeID != "1:12" {
		t.Fatalf("exports = %+v", d.Exports)
	}
	if e := d.Exports[0]; e.Format != "jpg" || e.Scale != 2 || e.Suffix != "-wide" || e.Constraint != "WIDTH 200" {
		t.Fatalf("width constraint: %+v", e)
	}
	if e := d.Exports[1]; e.Format != "png" || e.Scale != 4 || e.Constraint != "HEIGHT 100" {
		t.Fatalf("height constraint clamps to 4: %+v", e)
	}
	if e := d.Exports[2]; e.Format != "svg" || e.Scale != 1 || e.Constraint != "SCALE 3" {
		t.Fatalf("svg exports always render at 1x: %+v", e)
	}
	if len(d.ImageFills) != 1 || len(d.ImageFills[0].Nodes) != 1 || d.ImageFills[0].Nodes[0].NodeID != "1:8" || d.ImageFills[0].ScaleMode != "FIT" {
		t.Fatalf("fills = %+v", d.ImageFills)
	}

	d = Discover([]*figma.Node{root}, DiscoverOptions{IncludeExportSettings: true, IncludeIcons: true, IncludeHidden: true})
	if len(d.Exports) != 4 || d.Exports[0].NodeID != "1:1" {
		t.Fatalf("hidden exports = %+v", d.Exports)
	}
	if len(d.Icons) != 2 || d.Icons[0].NodeID != "1:1" || d.Icons[0].MatchedBy != "vectors" {
		t.Fatalf("hidden icons = %+v", d.Icons)
	}
	if Discover(nil, allOptions()) == nil || Discover([]*figma.Node{nil}, allOptions()) == nil {
		t.Fatal("nil roots")
	}
}
