package inspect

import (
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

func rect(x, y, w, h float64) *figma.Rectangle {
	return &figma.Rectangle{X: x, Y: y, Width: w, Height: h}
}

// doc builds a page carrying measurements over two stacked boxes:
// A occupies y 0..40, B occupies y 100..160, both x 0..50.
func doc(ms ...figma.Measurement) *figma.Node {
	return &figma.Node{
		ID: "0:0", Type: "DOCUMENT", Name: "Document",
		Children: []*figma.Node{{
			ID: "0:1", Type: "CANVAS", Name: "Page", Measurements: ms,
			Children: []*figma.Node{
				{ID: "1:1", Type: "FRAME", Name: "A", AbsoluteBoundingBox: rect(0, 0, 50, 40)},
				{ID: "1:2", Type: "FRAME", Name: "B", AbsoluteBoundingBox: rect(0, 100, 50, 60)},
			},
		}},
	}
}

func pin(id, side string) figma.MeasurementStartEnd {
	return figma.MeasurementStartEnd{NodeID: id, Side: side}
}

func TestMeasurementsResolveDistances(t *testing.T) {
	tests := []struct {
		name         string
		start, end   figma.MeasurementStartEnd
		wantDistance float64
		wantAxis     string
		wantCSS      string
	}{
		// A's bottom edge is at 40, B's top edge at 100.
		{"vertical gap between two boxes", pin("1:1", "BOTTOM"), pin("1:2", "TOP"), 60, "vertical", "60px"},
		// Direction does not matter: a measurement is a distance.
		{"the same gap measured the other way", pin("1:2", "TOP"), pin("1:1", "BOTTOM"), 60, "vertical", "60px"},
		// Top of A to top of B.
		{"top to top", pin("1:1", "TOP"), pin("1:2", "TOP"), 100, "vertical", "100px"},
		// Both boxes share an x range, so left to right is the width.
		{"horizontal across one box", pin("1:1", "LEFT"), pin("1:1", "RIGHT"), 50, "horizontal", "50px"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Measurements(doc(figma.Measurement{ID: "M:1", Start: tt.start, End: tt.end}))
			if len(got) != 1 {
				t.Fatalf("expected one measurement, got %d", len(got))
			}
			m := got[0]
			if m.Distance == nil {
				t.Fatalf("distance was not resolved: %s", m.Unresolved)
			}
			if *m.Distance != tt.wantDistance {
				t.Errorf("distance = %v, want %v", *m.Distance, tt.wantDistance)
			}
			if m.Axis != tt.wantAxis {
				t.Errorf("axis = %q, want %q", m.Axis, tt.wantAxis)
			}
			if m.CSS != tt.wantCSS || m.Label != tt.wantCSS {
				t.Errorf("css/label = %q/%q, want %q", m.CSS, m.Label, tt.wantCSS)
			}
		})
	}
}

// TestMeasurementFreeTextWins covers the designer overriding the measured
// value. The override is the specification, so it must not be replaced by the
// number figctl computed.
func TestMeasurementFreeTextWins(t *testing.T) {
	got := Measurements(doc(figma.Measurement{
		ID:       "M:1",
		Start:    pin("1:1", "BOTTOM"),
		End:      pin("1:2", "TOP"),
		FreeText: "16-24 responsive",
	}))
	if len(got) != 1 {
		t.Fatalf("expected one measurement, got %d", len(got))
	}
	m := got[0]
	if m.Label != "16-24 responsive" {
		t.Errorf("label = %q, want the designer's text", m.Label)
	}
	if m.FreeText != "16-24 responsive" {
		t.Errorf("freeText = %q", m.FreeText)
	}
	// The measured value is still reported, so a reader can see both.
	if m.Distance == nil || *m.Distance != 60 || m.CSS != "60px" {
		t.Errorf("the measured distance should still be reported, got %v / %q", m.Distance, m.CSS)
	}
}

// TestMeasurementsThatCannotBeResolved keeps figctl from inventing a number.
// Each of these has no single distance, and saying so is more useful than
// reporting a plausible but wrong value.
func TestMeasurementsThatCannotBeResolved(t *testing.T) {
	tests := []struct {
		name       string
		measure    figma.Measurement
		wantReason string
	}{
		{
			name:       "sides on different axes",
			measure:    figma.Measurement{ID: "M:1", Start: pin("1:1", "BOTTOM"), End: pin("1:2", "LEFT")},
			wantReason: "different axes",
		},
		{
			name:       "a node that is not in the document",
			measure:    figma.Measurement{ID: "M:1", Start: pin("9:9", "TOP"), End: pin("1:2", "TOP")},
			wantReason: "not in the document",
		},
		{
			name:       "an unrecognized side",
			measure:    figma.Measurement{ID: "M:1", Start: pin("1:1", "MIDDLE"), End: pin("1:2", "TOP")},
			wantReason: "unrecognized side",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Measurements(doc(tt.measure))
			if len(got) != 1 {
				t.Fatalf("the measurement should still be reported, got %d", len(got))
			}
			m := got[0]
			if m.Distance != nil {
				t.Fatalf("distance should be absent, got %v", *m.Distance)
			}
			if !strings.Contains(m.Unresolved, tt.wantReason) {
				t.Errorf("unresolved = %q, want it to mention %q", m.Unresolved, tt.wantReason)
			}
		})
	}
}

// TestMeasurementsFilterToTheSubtree keeps a screen's measurements out of a
// sibling screen's context.
func TestMeasurementsFilterToTheSubtree(t *testing.T) {
	document := doc(
		figma.Measurement{ID: "M:1", Start: pin("1:1", "BOTTOM"), End: pin("1:2", "TOP")},
		figma.Measurement{ID: "M:2", Start: pin("1:2", "TOP"), End: pin("1:2", "BOTTOM")},
	)
	page := document.Children[0]
	a, b := page.Children[0], page.Children[1]

	if got := Measurements(document); len(got) != 2 {
		t.Fatalf("with no root every measurement is returned, got %d", len(got))
	}
	// A is one end of M:1 only.
	got := Measurements(document, a)
	if len(got) != 1 || got[0].ID != "M:1" {
		t.Fatalf("subtree A should match M:1 only, got %+v", got)
	}
	// B is an end of both.
	if got := Measurements(document, b); len(got) != 2 {
		t.Fatalf("subtree B should match both, got %d", len(got))
	}
}

// TestMeasurementsNamesBothEnds gives the reader layer names rather than ids.
func TestMeasurementsNamesBothEnds(t *testing.T) {
	got := Measurements(doc(figma.Measurement{
		ID: "M:1", Start: pin("1:1", "BOTTOM"), End: pin("1:2", "TOP"),
		Offset: figma.MeasurementOffset{Type: "INNER", Relative: floatPtr(0.5)},
	}))
	m := got[0]
	if m.Start.Name != "A" || m.End.Name != "B" {
		t.Errorf("ends should carry layer names, got %q and %q", m.Start.Name, m.End.Name)
	}
	if m.Start.Type != "FRAME" || m.Start.Side != "BOTTOM" {
		t.Errorf("start pin = %+v", m.Start)
	}
	if m.Offset == nil || m.Offset.Type != "INNER" || m.Offset.Relative == nil || *m.Offset.Relative != 0.5 {
		t.Errorf("offset was not carried through: %+v", m.Offset)
	}
}

func TestMeasurementsWithoutADocument(t *testing.T) {
	if got := Measurements(nil); got != nil {
		t.Fatalf("a missing document yields nothing, got %+v", got)
	}
}

func floatPtr(f float64) *float64 { return &f }
