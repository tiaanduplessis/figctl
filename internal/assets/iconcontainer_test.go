package assets

import (
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

func vec(id string) *figma.Node { return &figma.Node{ID: id, Type: "VECTOR", Name: "v" + id} }

func box(w, h float64) *figma.Rectangle { return &figma.Rectangle{Width: w, Height: h} }

// TestVectorOnlyIconsAcrossContainerTypes guards the container types the
// vector-only heuristic applies to. Icon libraries publish icons as components
// and place instances of them, so a frame-only rule silently finds no icons in
// a real icon set. Groups stay excluded so that grouped artwork, which is often
// large, is not reported as an icon.
func TestVectorOnlyIconsAcrossContainerTypes(t *testing.T) {
	tests := []struct {
		nodeType string
		wantIcon bool
	}{
		{"COMPONENT", true},
		{"INSTANCE", true},
		{"FRAME", true},
		{"GROUP", false},
		{"SECTION", false},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			root := &figma.Node{
				ID: "1:0", Type: tt.nodeType, Name: "artwork",
				AbsoluteBoundingBox: box(24, 24),
				Children:            []*figma.Node{vec("1:1"), vec("1:2")},
			}
			got := Discover([]*figma.Node{root}, DiscoverOptions{IncludeIcons: true})
			if tt.wantIcon {
				if len(got.Icons) != 1 || got.Icons[0].NodeID != "1:0" {
					t.Fatalf("%s should be an icon, got %+v", tt.nodeType, got.Icons)
				}
				if got.Icons[0].MatchedBy != "vectors" {
					t.Errorf("matchedBy = %q, want vectors", got.Icons[0].MatchedBy)
				}
				return
			}
			for _, ic := range got.Icons {
				if ic.NodeID == "1:0" {
					t.Fatalf("%s should not be an icon, got %+v", tt.nodeType, got.Icons)
				}
			}
		})
	}
}

// TestVectorOnlyIconIgnoresNonVectorChild keeps the heuristic honest: a
// container holding anything that is not a vector shape is not an icon.
func TestVectorOnlyIconIgnoresNonVectorChild(t *testing.T) {
	root := &figma.Node{
		ID: "2:0", Type: "COMPONENT", Name: "card",
		AbsoluteBoundingBox: box(300, 200),
		Children:            []*figma.Node{vec("2:1"), {ID: "2:2", Type: "TEXT", Name: "label"}},
	}
	if got := Discover([]*figma.Node{root}, DiscoverOptions{IncludeIcons: true}); len(got.Icons) != 0 {
		t.Fatalf("component with a TEXT child is not an icon, got %+v", got.Icons)
	}
}
