package infer

import (
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

func alias(id string) figma.VariableBinding {
	a := figma.VariableAlias{ID: id, Type: "VARIABLE_ALIAS"}
	return figma.VariableBinding{Alias: &a}
}

func aliasList(id string) figma.VariableBinding {
	return figma.VariableBinding{Aliases: []figma.VariableAlias{{ID: id, Type: "VARIABLE_ALIAS"}}}
}

func color(r, g, b float64) *figma.Color { return &figma.Color{R: r, G: g, B: b, A: 1} }

func f(v float64) *float64 { return &v }

// TestInferRecoversWhatBindingsReveal is the point of the package: a plan that
// cannot read the variables endpoint can still see which values a variable
// governs, how widely, and what it resolves to.
func TestInferRecoversWhatBindingsReveal(t *testing.T) {
	doc := &figma.Node{
		ID: "0:0", Type: "DOCUMENT",
		Children: []*figma.Node{
			{
				ID: "1:1", Type: "FRAME", Name: "Card",
				BoundVariables: map[string]figma.VariableBinding{"itemSpacing": alias("VariableID:1:1")},
				ItemSpacing:    f(16),
				Fills: []figma.Paint{{
					Type: "SOLID", Color: color(1, 1, 1),
					BoundVariables: map[string]figma.VariableBinding{"color": aliasList("VariableID:2:2")},
				}},
			},
			{
				ID: "1:2", Type: "FRAME", Name: "Panel",
				BoundVariables: map[string]figma.VariableBinding{"itemSpacing": alias("VariableID:1:1")},
				ItemSpacing:    f(16),
			},
		},
	}
	inv := Infer(doc, Options{})
	if len(inv.Variables) != 2 {
		t.Fatalf("expected two variables, got %d: %+v", len(inv.Variables), inv.Variables)
	}
	if inv.Sites != 3 {
		t.Errorf("bindings seen = %d, want 3", inv.Sites)
	}

	byID := map[string]Variable{}
	for _, v := range inv.Variables {
		byID[v.ID] = v
	}
	spacing := byID["VariableID:1:1"]
	if spacing.Category != CategorySpacing || spacing.Usages != 2 {
		t.Errorf("spacing variable: %+v", spacing)
	}
	if len(spacing.Values) != 1 || spacing.Values[0].Value != "16px" || spacing.Values[0].Count != 2 {
		t.Errorf("the shared value should be counted once per use: %+v", spacing.Values)
	}
	// Two places sharing one variable is the fact worth recovering.
	if spacing.LikelyModes {
		t.Error("one consistent value is not a mode")
	}
	fill := byID["VariableID:2:2"]
	if fill.Category != CategoryColor || fill.Values[0].Value != "#ffffff" {
		t.Errorf("fill variable: %+v", fill)
	}
	for _, v := range inv.Variables {
		if v.NameSource == "" {
			t.Errorf("%s must say its name was derived", v.ID)
		}
	}
}

// TestInferDetectsLikelyModes covers one variable resolving to two values,
// which is what a light and dark pair looks like from the outside.
func TestInferDetectsLikelyModes(t *testing.T) {
	doc := &figma.Node{
		ID: "0:0", Type: "DOCUMENT",
		Children: []*figma.Node{
			{ID: "1:1", Type: "FRAME", Fills: []figma.Paint{{Type: "SOLID", Color: color(1, 1, 1),
				BoundVariables: map[string]figma.VariableBinding{"color": alias("VariableID:9:9")}}}},
			{ID: "1:2", Type: "FRAME", Fills: []figma.Paint{{Type: "SOLID", Color: color(0, 0, 0),
				BoundVariables: map[string]figma.VariableBinding{"color": alias("VariableID:9:9")}}}},
		},
	}
	inv := Infer(doc, Options{})
	if len(inv.Variables) != 1 {
		t.Fatalf("expected one variable, got %d", len(inv.Variables))
	}
	v := inv.Variables[0]
	if !v.LikelyModes || len(v.Values) != 2 {
		t.Fatalf("two values should read as likely modes: %+v", v)
	}
}

// TestDerivedNamesAreStableAndDistinct: two different variables that happen
// to resolve to the same value must not collapse, and the name that separates
// them must not depend on the order the document was walked.
func TestDerivedNamesAreStableAndDistinct(t *testing.T) {
	build := func() *figma.Node {
		return &figma.Node{
			ID: "0:0", Type: "DOCUMENT",
			Children: []*figma.Node{
				{ID: "1:1", Type: "FRAME", Fills: []figma.Paint{{Type: "SOLID", Color: color(0, 0, 0),
					BoundVariables: map[string]figma.VariableBinding{"color": alias("VariableID:3:7")}}}},
				{ID: "1:2", Type: "FRAME", Fills: []figma.Paint{{Type: "SOLID", Color: color(0, 0, 0),
					BoundVariables: map[string]figma.VariableBinding{"color": alias("VariableID:9:1")}}}},
			},
		}
	}
	first := Infer(build(), Options{})
	second := Infer(build(), Options{})
	if len(first.Variables) != 2 {
		t.Fatalf("two variables sharing a value must stay separate: %+v", first.Variables)
	}
	names := map[string]bool{}
	for i, v := range first.Variables {
		if names[v.Name] {
			t.Fatalf("duplicate name %q", v.Name)
		}
		names[v.Name] = true
		if v.Name != second.Variables[i].Name {
			t.Errorf("name is not stable between runs: %q then %q", v.Name, second.Variables[i].Name)
		}
	}
}

// TestSubscribedVariableNameStaysShort keeps a library key out of the name.
// A variable used from another library carries forty characters of hash that
// identify the library rather than the variable.
func TestSubscribedVariableNameStaysShort(t *testing.T) {
	const id = "VariableID:fc721e4b29bb725ed369ab44a527a14d57666833/1156:324"
	doc := &figma.Node{ID: "0:0", Type: "DOCUMENT", Children: []*figma.Node{
		{ID: "1:1", Type: "FRAME", Fills: []figma.Paint{{Type: "SOLID", Color: color(0, 0, 0),
			BoundVariables: map[string]figma.VariableBinding{"color": alias("VariableID:3:7")}}}},
		{ID: "1:2", Type: "FRAME", Fills: []figma.Paint{{Type: "SOLID", Color: color(0, 0, 0),
			BoundVariables: map[string]figma.VariableBinding{"color": alias(id)}}}},
	}}
	for _, v := range Infer(doc, Options{}).Variables {
		if len(v.Name) > 40 {
			t.Errorf("name carries the library hash: %q", v.Name)
		}
	}
}

// TestTypographyBindingsAreRead covers the placement a real design system
// uses, where the binding is on the node and the value in its text style.
func TestTypographyBindingsAreRead(t *testing.T) {
	doc := &figma.Node{ID: "0:0", Type: "DOCUMENT", Children: []*figma.Node{{
		ID: "1:1", Type: "TEXT", Name: "Heading",
		BoundVariables: map[string]figma.VariableBinding{
			"fontSize":   aliasList("VariableID:31:15"),
			"fontFamily": aliasList("VariableID:3:11"),
		},
		Style: &figma.TypeStyle{FontFamily: "Inter", FontSize: f(28)},
	}}}
	inv := Infer(doc, Options{})
	got := map[string]string{}
	for _, v := range inv.Variables {
		got[v.Category] = v.Values[0].Value
	}
	if got[CategoryFontSize] != "28px" {
		t.Errorf("font size not read from the text style: %v", got)
	}
	if got[CategoryFontFamily] != "Inter" {
		t.Errorf("font family not read from the text style: %v", got)
	}
}

// TestMinUsagesAndHidden cover the two ways of narrowing a large file.
func TestMinUsagesAndHidden(t *testing.T) {
	hidden := false
	doc := &figma.Node{ID: "0:0", Type: "DOCUMENT", Children: []*figma.Node{
		{ID: "1:1", Type: "FRAME", ItemSpacing: f(8),
			BoundVariables: map[string]figma.VariableBinding{"itemSpacing": alias("VariableID:1:1")}},
		{ID: "1:2", Type: "FRAME", Visible: &hidden, ItemSpacing: f(24),
			BoundVariables: map[string]figma.VariableBinding{"itemSpacing": alias("VariableID:2:2")}},
	}}
	if got := Infer(doc, Options{}); len(got.Variables) != 1 {
		t.Errorf("a hidden node should be skipped by default: %+v", got.Variables)
	}
	if got := Infer(doc, Options{IncludeHidden: true}); len(got.Variables) != 2 {
		t.Errorf("--include-hidden should reach it: %+v", got.Variables)
	}
	if got := Infer(doc, Options{MinUsages: 2}); len(got.Variables) != 0 {
		t.Errorf("a single binding is below a minimum of two: %+v", got.Variables)
	}
}

func TestInferWithoutADocument(t *testing.T) {
	if got := Infer(nil, Options{}); len(got.Variables) != 0 {
		t.Fatalf("no document yields nothing, got %+v", got)
	}
}
