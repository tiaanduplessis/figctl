package inspect

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

func ptr[T any](v T) *T { return &v }

type fixture struct {
	file     *figma.GetFileResponse
	resolver *resolve.Resolver
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	var vars figma.GetLocalVariablesResponse
	figmatest.Decode(t, "variables_local.json", &vars)
	var nodes figma.GetFileNodesResponse
	figmatest.Decode(t, "nodes.json", &nodes)
	styleNodes := map[string]*figma.Node{}
	for id, entry := range nodes.Nodes {
		styleNodes[id] = entry.Document
	}
	return fixture{file: &file, resolver: resolve.New(resolve.Options{File: &file, Variables: &vars, StyleNodes: styleNodes})}
}

func (f fixture) inspect(t *testing.T, id string, opts Options) *model.Node {
	t.Helper()
	node := f.file.Document.Find(id)
	if node == nil {
		t.Fatalf("fixture node %s missing", id)
	}
	return Inspect(node, nil, f.resolver, f.file, opts)
}

func child(t *testing.T, n *model.Node, id string) *model.Node {
	t.Helper()
	for _, c := range n.Children {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("child %s not found under %s", id, n.ID)
	return nil
}

func TestLayoutMapping(t *testing.T) {
	empty := resolve.New(resolve.Options{})
	tests := []struct {
		name  string
		node  figma.Node
		want  model.Layout
		check func(t *testing.T, l *model.Layout)
	}{
		{
			name: "horizontal is row with flex mappings",
			node: figma.Node{LayoutMode: "HORIZONTAL", ItemSpacing: ptr(8.0), PrimaryAxisAlignItems: "SPACE_BETWEEN", CounterAxisAlignItems: "BASELINE", PaddingLeft: ptr(4.0), PaddingRight: ptr(4.0), PaddingTop: ptr(2.0), PaddingBottom: ptr(2.0)},
			check: func(t *testing.T, l *model.Layout) {
				if l.Mode != "row" || *l.Gap != 8 || l.Justify != "space-between" || l.Align != "baseline" || l.Padding.Shorthand != "2px 4px" {
					t.Fatalf("%+v", l)
				}
				if _, present := l.Tokens["gap"]; !present || l.Tokens["gap"] != nil {
					t.Fatalf("gap token must be present and null: %+v", l.Tokens)
				}
			},
		},
		{
			name: "wrap carries row gap",
			node: figma.Node{LayoutMode: "HORIZONTAL", LayoutWrap: "WRAP", ItemSpacing: ptr(12.0), CounterAxisSpacing: ptr(6.0), CounterAxisAlignContent: "SPACE_BETWEEN"},
			check: func(t *testing.T, l *model.Layout) {
				if l.Mode != "wrap" || *l.RowGap != 6 || l.AlignContent != "space-between" {
					t.Fatalf("%+v", l)
				}
			},
		},
		{
			name: "vertical is column with default alignment",
			node: figma.Node{LayoutMode: "VERTICAL", PrimaryAxisAlignItems: "MAX", CounterAxisAlignItems: "CENTER"},
			check: func(t *testing.T, l *model.Layout) {
				if l.Mode != "column" || l.Justify != "flex-end" || l.Align != "center" || *l.Gap != 0 {
					t.Fatalf("%+v", l)
				}
			},
		},
		{
			name: "grid",
			node: figma.Node{LayoutMode: "GRID", GridRowCount: ptr(3), GridColumnCount: ptr(2), GridRowGap: ptr(4.0), GridColumnGap: ptr(8.0), GridRowsSizing: "auto auto auto"},
			check: func(t *testing.T, l *model.Layout) {
				if l.Mode != "grid" || l.Grid.Rows != 3 || l.Grid.Columns != 2 || l.Grid.RowGap != 4 || l.Grid.ColumnGap != 8 || l.Grid.RowsSizing != "auto auto auto" || l.Gap != nil {
					t.Fatalf("%+v", l)
				}
			},
		},
		{
			name: "none with overflow and clips",
			node: figma.Node{ClipsContent: ptr(true), OverflowDirection: "VERTICAL_SCROLLING", MinWidth: ptr(100.0), MaxHeight: ptr(400.0), ItemReverseZIndex: ptr(true)},
			check: func(t *testing.T, l *model.Layout) {
				if l.Mode != "none" || !l.Clips || l.Overflow != "scroll-y" || *l.MinWidth != 100 || *l.MaxHeight != 400 || l.Tokens != nil || l.ReverseZIndex {
					t.Fatalf("%+v", l)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := &inspector{r: empty, opts: Options{}}
			tc.check(t, in.layout(&tc.node, nil))
		})
	}
}

func TestSizingFallbacks(t *testing.T) {
	tests := []struct {
		name   string
		node   figma.Node
		parent *figma.Node
		h, v   string
	}{
		{"explicit sizing", figma.Node{LayoutSizingHorizontal: "FILL", LayoutSizingVertical: "HUG"}, nil, "fill", "hug"},
		{"own auto layout hug on primary axis", figma.Node{LayoutMode: "HORIZONTAL", PrimaryAxisSizingMode: "AUTO", CounterAxisSizingMode: "FIXED"}, nil, "hug", "fixed"},
		{"own vertical auto layout hug on counter axis", figma.Node{LayoutMode: "VERTICAL", PrimaryAxisSizingMode: "FIXED", CounterAxisSizingMode: "AUTO"}, nil, "hug", "fixed"},
		{"grow fills primary axis of a row parent", figma.Node{LayoutGrow: ptr(1.0)}, &figma.Node{LayoutMode: "HORIZONTAL"}, "fill", "fixed"},
		{"stretch fills counter axis of a row parent", figma.Node{LayoutAlign: "STRETCH"}, &figma.Node{LayoutMode: "HORIZONTAL"}, "fixed", "fill"},
		{"grow and stretch in a column parent", figma.Node{LayoutGrow: ptr(1.0), LayoutAlign: "STRETCH"}, &figma.Node{LayoutMode: "VERTICAL"}, "fill", "fill"},
		{"text auto resize hugs", figma.Node{Type: "TEXT", Style: &figma.TypeStyle{TextAutoResize: "WIDTH_AND_HEIGHT"}}, nil, "hug", "hug"},
		{"text height auto resize hugs vertically", figma.Node{Type: "TEXT", Style: &figma.TypeStyle{TextAutoResize: "HEIGHT"}}, &figma.Node{LayoutMode: "VERTICAL"}, "fixed", "hug"},
		{"partial explicit sizing keeps the fallback for the other axis", figma.Node{LayoutSizingHorizontal: "FIXED", LayoutGrow: ptr(1.0)}, &figma.Node{LayoutMode: "VERTICAL"}, "fixed", "fill"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sizing(&tc.node, tc.parent)
			if s.Horizontal.Mode != tc.h || s.Vertical.Mode != tc.v {
				t.Fatalf("got %s/%s, want %s/%s", s.Horizontal.Mode, s.Vertical.Mode, tc.h, tc.v)
			}
		})
	}
}

func TestAbsoluteChildren(t *testing.T) {
	empty := resolve.New(resolve.Options{})
	in := &inspector{r: empty}
	parent := &figma.Node{Type: "FRAME", AbsoluteBoundingBox: &figma.Rectangle{X: 100, Y: 50, Width: 300, Height: 200}}
	childNode := &figma.Node{Type: "RECTANGLE", AbsoluteBoundingBox: &figma.Rectangle{X: 110, Y: 70, Width: 20, Height: 20}, Constraints: &figma.LayoutConstraint{Horizontal: "RIGHT", Vertical: "SCALE"}}
	l := in.layout(childNode, parent)
	if l.Position != "absolute" || l.Offset.X != 10 || l.Offset.Y != 20 || l.Constraints.Horizontal != "right" || l.Constraints.Vertical != "scale" {
		t.Fatalf("absolute child: %+v", l)
	}
	if b := box(childNode, parent); b.Relative.X != 10 || b.Relative.Y != 20 {
		t.Fatalf("relative box: %+v", b)
	}

	auto := &figma.Node{Type: "FRAME", LayoutMode: "VERTICAL", AbsoluteBoundingBox: &figma.Rectangle{}}
	if l := in.layout(childNode, auto); l.Position != "auto" || l.Offset != nil || l.Constraints != nil {
		t.Fatalf("auto layout child: %+v", l)
	}
	pinned := *childNode
	pinned.LayoutPositioning = "ABSOLUTE"
	if l := in.layout(&pinned, auto); l.Position != "absolute" || l.Offset.X != 110 {
		t.Fatalf("absolute inside auto layout: %+v", l)
	}
	canvas := &figma.Node{Type: "CANVAS"}
	if l := in.layout(childNode, canvas); l.Position != "auto" {
		t.Fatalf("canvas children are not absolute: %+v", l)
	}
	if b := box(childNode, canvas); b.Relative != nil {
		t.Fatalf("no relative box on a canvas: %+v", b)
	}
	if l := in.layout(childNode, nil); l.Position != "auto" {
		t.Fatalf("no parent: %+v", l)
	}
	grid := &figma.Node{Type: "FRAME", LayoutMode: "GRID", AbsoluteBoundingBox: &figma.Rectangle{}}
	cell := &figma.Node{GridRowSpan: ptr(2), GridColumnAnchorIndex: ptr(1), GridChildHorizontalAlign: "MAX", GridChildVerticalAlign: "STRETCH"}
	if l := in.layout(cell, grid); l.GridChild == nil || l.GridChild.RowSpan != 2 || l.GridChild.ColumnSpan != 1 || l.GridChild.Column != 1 || l.GridChild.AlignH != "end" || l.GridChild.AlignV != "stretch" {
		t.Fatalf("grid child: %+v", l.GridChild)
	}
}

func TestRadiusAndStrokes(t *testing.T) {
	f := loadFixture(t)
	in := &inspector{r: f.resolver}
	if r := in.radius(&figma.Node{CornerRadius: ptr(8.0), CornerSmoothing: ptr(0.6)}); r == nil || *r.Uniform != 8 || r.Smoothing != 0.6 || r.Token != nil {
		t.Fatalf("uniform: %+v", r)
	}
	if r := in.radius(&figma.Node{CornerRadius: ptr(0.0)}); r != nil {
		t.Fatalf("zero radius omitted: %+v", r)
	}
	if r := in.radius(&figma.Node{RectangleCornerRadii: []float64{1, 2, 3, 4}, CornerRadius: ptr(1.0)}); r == nil || r.Corners == nil || r.Corners[3] != 4 || r.Uniform != nil {
		t.Fatalf("corners: %+v", r)
	}
	if r := in.radius(&figma.Node{RectangleCornerRadii: []float64{6, 6, 6, 6}}); r == nil || *r.Uniform != 6 {
		t.Fatalf("equal corners collapse: %+v", r)
	}
	bound := &figma.Node{CornerRadius: ptr(0.0), BoundVariables: map[string]figma.VariableBinding{"topLeftRadius": {Alias: &figma.VariableAlias{ID: "VariableID:1:104"}}}}
	if r := in.radius(bound); r == nil || r.Token == nil || r.Token.Name != "space/4" {
		t.Fatalf("bound radius: %+v", r)
	}

	if w := strokeWeight(&figma.Node{StrokeWeight: ptr(2.0)}); !w.Uniform() || w.Top != 2 {
		t.Fatalf("uniform weight: %+v", w)
	}
	if w := strokeWeight(&figma.Node{StrokeWeight: ptr(2.0), IndividualStrokeWeights: &figma.StrokeWeights{Top: 1, Right: 0, Bottom: 3, Left: 0}}); w.Uniform() || w.Bottom != 3 || w.Right != 0 {
		t.Fatalf("per side weight: %+v", w)
	}
	v := in.visual(&figma.Node{Type: "LINE", StrokeWeight: ptr(1.0), StrokeAlign: "CENTER", StrokeDashes: []float64{4, 4}, Strokes: []figma.Paint{{Type: "SOLID", Color: &figma.Color{R: 0.95, G: 0.95, B: 0.96, A: 1}}}})
	if len(v.Strokes) != 1 || v.StrokeWeight == nil || v.StrokeAlign != "center" || len(v.StrokeDashes) != 2 || v.Strokes[0].Token != nil {
		t.Fatalf("stroke visual: %+v", v)
	}
	if v := in.visual(&figma.Node{Type: "FRAME", StrokeWeight: ptr(1.0)}); v.StrokeWeight != nil || v.StrokeAlign != "" || len(v.Fills) != 0 {
		t.Fatalf("no strokes means no weight: %+v", v)
	}
	if in.visual(&figma.Node{Type: "CANVAS"}) != nil {
		t.Fatal("canvas has no visual")
	}
}

func TestTextMetricsAndRuns(t *testing.T) {
	f := loadFixture(t)
	heading := f.inspect(t, "2:3", Options{})
	tx := heading.Text
	if tx.Characters != "Welcome back" || tx.FontFamily != "Inter" || *tx.Weight != 700 || *tx.Size != 28 || tx.Italic {
		t.Fatalf("heading: %+v", tx)
	}
	if tx.LineHeight.Px != 34 || *tx.LineHeight.Unitless != 1.21 || *tx.LineHeight.Percent != 121.4 || tx.LineHeight.Unit != "px" {
		t.Fatalf("line height: %+v", tx.LineHeight)
	}
	if tx.LetterSpacing.Px != -0.5 || *tx.LetterSpacing.Em != -0.018 {
		t.Fatalf("letter spacing: %+v", tx.LetterSpacing)
	}
	if tx.Style == nil || tx.Style.Name != "heading/lg" || tx.Tokens["fontFamily"] == nil || tx.Tokens["fontFamily"].Name != "font/family/sans" {
		t.Fatalf("refs: %+v", tx)
	}
	for _, key := range []string{"fontSize", "fontWeight", "lineHeight", "letterSpacing"} {
		if v, present := tx.Tokens[key]; !present || v != nil {
			t.Fatalf("%s must be present and null", key)
		}
	}
	if heading.Tokens.Variables["fills[0]"] != "text/primary" || heading.Tokens.Styles["text"] != "heading/lg" {
		t.Fatalf("index: %+v", heading.Tokens)
	}

	body := f.inspect(t, "2:4", Options{})
	if len(body.Text.Runs) != 1 || body.Text.Runs[0].Text != "continue" || body.Text.Runs[0].Fill.Token.Name != "brand/500" || body.Text.LineHeight.Unit != "percent" || *body.Text.LineHeight.Unitless != 1.5 {
		t.Fatalf("body: %+v", body.Text)
	}
	if body.Tokens == nil || body.Tokens.Variables["runs[0].fill"] != "brand/500" {
		t.Fatalf("run token index: %+v", body.Tokens)
	}
	count := f.inspect(t, "2:16", Options{})
	if count.Text.Case != "uppercase" || !count.Text.TabularNumbers || count.Text.AlignH != "center" {
		t.Fatalf("count: %+v", count.Text)
	}
	caption := f.inspect(t, "2:17", Options{})
	if caption.Text.Truncation != "ending" || *caption.Text.MaxLines != 1 {
		t.Fatalf("caption: %+v", caption.Text)
	}
	if bare := Inspect(&figma.Node{ID: "t", Type: "TEXT", Characters: "x"}, nil, f.resolver, nil, Options{}); bare.Text == nil || len(bare.Text.Tokens) != 5 {
		t.Fatalf("text without style: %+v", bare.Text)
	}
}

func TestInstancesAndComponents(t *testing.T) {
	f := loadFixture(t)
	button := f.inspect(t, "2:6", Options{Interactions: true})
	c := button.Component
	if !c.IsInstance || c.MainComponentID != "3:11" || c.MainComponentName != "Variant=Primary, Size=md" || c.SetID != "3:10" || c.SetName != "Button" || c.Key == "" {
		t.Fatalf("instance: %+v", c)
	}
	if c.VariantProperties["Variant"] != "Primary" || c.VariantProperties["Size"] != "md" {
		t.Fatalf("variants: %+v", c.VariantProperties)
	}
	if p := c.Properties["Label"]; p.Type != "TEXT" || p.Value != "Sign in" || p.Key != "Label#5:0" {
		t.Fatalf("properties: %+v", c.Properties)
	}
	if len(c.Overrides) != 2 || c.Overrides[0].NodeID != "2:6" || strings.Join(c.Overrides[0].Fields, ",") != "layoutSizingHorizontal,layoutAlign" || c.Overrides[1].Fields[0] != "characters" {
		t.Fatalf("overrides: %+v", c.Overrides)
	}
	if c.Description != "Primary call to action." || len(c.DocumentationLinks) != 1 {
		t.Fatalf("metadata: %+v", c)
	}
	if button.Prototype == nil || len(button.Prototype.Interactions) != 1 || button.Prototype.Interactions[0].DestinationID != "2:14" || button.Prototype.Interactions[0].Transition.Easing != "EASE_OUT" {
		t.Fatalf("prototype: %+v", button.Prototype)
	}
	if button.Visual.Fills[0].Style == nil || button.Visual.Fills[0].Style.Name != "color/brand/500" || button.Visual.Fills[0].Token.Name != "brand/500" {
		t.Fatalf("fill refs: %+v", button.Visual.Fills[0])
	}

	input := f.inspect(t, "2:5", Options{})
	if input.Component.SetName != "" || input.Component.Properties["Show helper"].Value != false || input.Component.Properties["Show helper"].Type != "BOOLEAN" {
		t.Fatalf("input instance: %+v", input.Component)
	}
	if f.inspect(t, "2:5", Options{}).Prototype != nil {
		t.Fatal("interactions are opt in")
	}

	set := f.inspect(t, "3:10", Options{Depth: 1})
	if !set.Component.IsComponentSet || set.Component.Key == "" || len(set.Component.PropertyDefinitions) != 3 {
		t.Fatalf("component set: %+v", set.Component)
	}
	if d := set.Component.PropertyDefinitions["Label"]; d.Type != "TEXT" || d.DefaultValue != "Button" || d.Key != "Label#5:0" {
		t.Fatalf("definition: %+v", d)
	}
	if d := set.Component.PropertyDefinitions["Variant"]; strings.Join(d.VariantOptions, ",") != "Primary,Secondary" {
		t.Fatalf("variant options: %+v", d)
	}
	variant := child(t, set, "3:11")
	if !variant.Component.IsComponent || variant.Component.SetName != "Button" || variant.Component.VariantProperties["Size"] != "md" || variant.Component.Description != "Primary call to action." {
		t.Fatalf("variant component: %+v", variant.Component)
	}
	if plain := f.inspect(t, "3:20", Options{}); !plain.Component.IsComponent || plain.Component.VariantProperties != nil || len(plain.Component.PropertyDefinitions) != 2 {
		t.Fatalf("plain component: %+v", plain.Component)
	}
	if frame := f.inspect(t, "2:7", Options{}); frame.Component != nil {
		t.Fatal("frames have no component block")
	}
}

func TestLoginFrameRefs(t *testing.T) {
	f := loadFixture(t)
	login := f.inspect(t, "2:2", Options{Depth: 1, Web: true})
	if strings.Join(login.Path, "/") != "Screens/Onboarding" || login.Box.Relative.X != 40 || login.Layout.Position != "absolute" {
		t.Fatalf("root: path=%v box=%+v", login.Path, login.Box)
	}
	fill := login.Visual.Fills[0]
	if fill.Hex != "#ffffff" || fill.Token == nil || fill.Token.Name != "bg/surface" || fill.Token.Values["Light"] != "#ffffff" || fill.Token.Values["Dark"] != "#111827" || fill.Token.Web != "var(--bg-surface)" {
		t.Fatalf("fill: %+v", fill)
	}
	if strings.Join(fill.Token.Chain, ">") != "bg/surface>neutral/0" {
		t.Fatalf("chain: %v", fill.Token.Chain)
	}
	gap := login.Layout.Tokens["gap"]
	if gap == nil || gap.Name != "space/4" || gap.Values["Default"] != float64(16) || gap.Web != "var(--space-4)" {
		t.Fatalf("gap: %+v", gap)
	}
	if login.Layout.Tokens["paddingLeft"] != nil {
		t.Fatal("padding is not bound")
	}
	if len(login.Visual.Effects) != 1 || login.Visual.Effects[0].Style.Name != "shadow/md" || login.Visual.Effects[0].Token != nil {
		t.Fatalf("effects: %+v", login.Visual.Effects)
	}
	if len(login.Visual.LayoutGrids) != 1 || login.Visual.LayoutGrids[0].Style.Name != "grid/12" {
		t.Fatalf("grids: %+v", login.Visual.LayoutGrids)
	}
	if login.Tokens.Variables["fills[0]"] != "bg/surface" || login.Tokens.Variables["gap"] != "space/4" || login.Tokens.Styles["effect"] != "shadow/md" || login.Tokens.Styles["grid"] != "grid/12" {
		t.Fatalf("index: %+v", login.Tokens)
	}
	if login.ChildCount != 8 || len(login.Children) != 7 {
		t.Fatalf("children: count=%d shown=%d", login.ChildCount, len(login.Children))
	}
	heading := child(t, login, "2:3")
	if strings.Join(heading.Path, "/") != "Screens/Onboarding/Login" || heading.Layout.Position != "auto" || heading.Layout.Sizing.Horizontal.Mode != "fill" {
		t.Fatalf("heading: %+v", heading.Layout)
	}
	blob := f.inspect(t, "2:11", Options{})
	g := blob.Visual.Fills[0].Gradient
	if blob.Visual.Fills[0].Type != "gradient" || g == nil || g.Angle != 90 || g.Stops[0].Token == nil || g.Stops[0].Token.Name != "brand/500" || g.CSS != "linear-gradient(90deg, #3366ff 0%, #9933ff 100%)" {
		t.Fatalf("gradient: %+v", g)
	}
	if blob.Tokens.Variables["fills[0].stops[0]"] != "brand/500" || blob.Visual.Effects[0].CSS != "blur(1px)" || blob.Layout.Position != "absolute" {
		t.Fatalf("blob: %+v", blob)
	}
	avatar := f.inspect(t, "2:8", Options{})
	if avatar.Visual.Fills[0].Type != "image" || avatar.Visual.Fills[0].ImageRef != "img1" || *avatar.Visual.Radius.Uniform != 20 || avatar.Visual.Radius.Smoothing != 0.6 {
		t.Fatalf("avatar: %+v", avatar.Visual)
	}
	decor := f.inspect(t, "2:10", Options{})
	if *decor.Visual.Opacity != 0.9 || decor.Tokens != nil {
		t.Fatalf("decor: %+v", decor.Visual)
	}
}

func TestTruncationAndHidden(t *testing.T) {
	f := loadFixture(t)
	login := f.inspect(t, "2:2", Options{Depth: -1, MaxNodes: 4})
	if !login.Truncated || login.Omitted == 0 || len(login.Children) != 3 {
		t.Fatalf("root: truncated=%v omitted=%d children=%d", login.Truncated, login.Omitted, len(login.Children))
	}
	// 4 nodes emitted; the whole visible subtree has 19 nodes. The count
	// is spread over the nodes whose children were cut.
	if Omitted(login) != 15 || Omitted(login) <= login.Omitted {
		t.Fatalf("omitted = %d, want 15", Omitted(login))
	}
	full := f.inspect(t, "2:2", Options{Depth: -1})
	if full.Truncated || Omitted(full) != 0 || countNodes(full) != 19 {
		t.Fatalf("full: truncated=%v omitted=%d nodes=%d", full.Truncated, Omitted(full), countNodes(full))
	}
	for _, c := range full.Children {
		if c.ID == "2:13" {
			t.Fatal("hidden child should be skipped")
		}
	}
	withHidden := f.inspect(t, "2:2", Options{Depth: -1, IncludeHidden: true})
	if countNodes(withHidden) != 20 || child(t, withHidden, "2:13").Visible {
		t.Fatalf("with hidden: %d", countNodes(withHidden))
	}
	depth1 := f.inspect(t, "2:2", Options{Depth: 1})
	if input := child(t, depth1, "2:5"); input.ChildCount != 2 || input.Children != nil {
		t.Fatalf("depth 1 must not expand grandchildren: %+v", input)
	}
	def := f.inspect(t, "2:2", Options{})
	if countNodes(def) != 19 {
		t.Fatalf("default depth 3 covers the fixture: %d", countNodes(def))
	}
	if Inspect(nil, nil, f.resolver, nil, Options{}) != nil || Omitted(nil) != 0 {
		t.Fatal("nil node")
	}
}

func countNodes(n *model.Node) int {
	total := 1
	for _, c := range n.Children {
		total += countNodes(c)
	}
	return total
}

func TestCSS(t *testing.T) {
	f := loadFixture(t)
	login := f.inspect(t, "2:2", Options{Depth: 2, CSS: true})
	want := map[string]string{
		"display":         "flex",
		"flex-direction":  "column",
		"gap":             "var(--space-4)",
		"padding":         "24px",
		"justify-content": "flex-start",
		"align-items":     "flex-start",
		"width":           "390px",
		"height":          "844px",
		"position":        "absolute",
		"left":            "40px",
		"top":             "80px",
		"overflow":        "hidden",
		"background":      "var(--bg-surface)",
		"box-shadow":      "0px 4px 12px rgba(0, 0, 0, 0.1)",
		"box-sizing":      "",
	}
	delete(want, "box-sizing")
	assertCSS(t, "login", login.CSS, want)

	heading := child(t, login, "2:3")
	assertCSS(t, "heading", heading.CSS, map[string]string{
		"align-self":     "stretch",
		"color":          "var(--text-primary)",
		"font-family":    "var(--font-sans)",
		"font-size":      "28px",
		"font-weight":    "700",
		"line-height":    "34px",
		"letter-spacing": "-0.5px",
	})
	body := child(t, login, "2:4")
	if body.CSS["line-height"] != "1.5" || body.CSS["color"] != "#6b7280" || body.CSS["font-family"] != "Inter" {
		t.Fatalf("body css: %v", body.CSS)
	}
	button := child(t, login, "2:6")
	if button.CSS["background"] != "var(--color-brand-500)" || button.CSS["border-radius"] != "8px" || button.CSS["padding"] != "12px 20px" || button.CSS["align-self"] != "stretch" || button.CSS["height"] != "48px" {
		t.Fatalf("button css: %v", button.CSS)
	}
	label := child(t, button, "I2:6;3:12")
	if label.CSS["color"] != "#ffffff" || label.CSS["text-align"] != "center" || label.CSS["width"] != "" {
		t.Fatalf("label css: %v", label.CSS)
	}
	social := child(t, login, "2:7")
	if social.CSS["flex-wrap"] != "wrap" || social.CSS["column-gap"] != "12px" || social.CSS["row-gap"] != "8px" || social.CSS["gap"] != "" {
		t.Fatalf("social css: %v", social.CSS)
	}
	avatar := child(t, social, "2:8")
	if avatar.CSS["background-image"] != "url(img1)" || avatar.CSS["background-size"] != "cover" || avatar.CSS["border-radius"] != "20px" || avatar.CSS["width"] != "40px" {
		t.Fatalf("avatar css: %v", avatar.CSS)
	}
	stats := child(t, login, "2:14")
	if stats.CSS["display"] != "grid" || stats.CSS["grid-template-rows"] != "minmax(0, 1fr) minmax(0, 1fr)" || stats.CSS["row-gap"] != "8px" || stats.CSS["column-gap"] != "8px" {
		t.Fatalf("stats css: %v", stats.CSS)
	}
	stat := child(t, stats, "2:15")
	if stat.CSS["grid-row"] != "1 / span 2" || stat.CSS["grid-column"] != "1 / span 1" || stat.CSS["width"] != "" || stat.CSS["flex"] != "" {
		t.Fatalf("stat css: %v", stat.CSS)
	}
	count := child(t, stats, "2:16")
	if count.CSS["text-transform"] != "uppercase" || count.CSS["font-variant-numeric"] != "tabular-nums" || count.CSS["justify-self"] != "center" {
		t.Fatalf("count css: %v", count.CSS)
	}
	caption := child(t, stats, "2:17")
	if caption.CSS["text-overflow"] != "ellipsis" || caption.CSS["white-space"] != "nowrap" || caption.CSS["overflow"] != "hidden" || caption.CSS["letter-spacing"] != "0.2px" {
		t.Fatalf("caption css: %v", caption.CSS)
	}
	decor := f.inspect(t, "2:10", Options{CSS: true, Depth: 1})
	if decor.CSS["opacity"] != "0.9" {
		t.Fatalf("decor css: %v", decor.CSS)
	}
	blob := child(t, decor, "2:11")
	if blob.CSS["background-image"] != "linear-gradient(90deg, #3366ff 0%, #9933ff 100%)" || blob.CSS["filter"] != "blur(1px)" || blob.CSS["position"] != "absolute" || blob.CSS["left"] != "0px" {
		t.Fatalf("blob css: %v", blob.CSS)
	}
	divider := child(t, decor, "2:12")
	if divider.CSS["border"] != "1px dashed #f3f4f6" {
		t.Fatalf("divider css: %v", divider.CSS)
	}
	field := f.inspect(t, "I2:5;3:22", Options{CSS: true})
	if field.CSS["border"] != "1px solid #6b7280" || field.CSS["box-sizing"] != "border-box" {
		t.Fatalf("field css: %v", field.CSS)
	}
	if plain := f.inspect(t, "2:2", Options{}); plain.CSS != nil {
		t.Fatal("css is opt in")
	}

	empty := resolve.New(resolve.Options{})
	sides := Inspect(&figma.Node{ID: "s", Type: "RECTANGLE", Strokes: []figma.Paint{{Type: "SOLID", Color: &figma.Color{A: 1}}}, IndividualStrokeWeights: &figma.StrokeWeights{Top: 2, Bottom: 1}, Effects: []figma.Effect{{Type: "BACKGROUND_BLUR", Radius: ptr(8.0)}, {Type: "INNER_SHADOW", Color: &figma.Color{A: 0.5}, Radius: ptr(2.0)}, {Type: "DROP_SHADOW", Color: &figma.Color{A: 0.5}, Radius: ptr(2.0)}}, Opacity: ptr(0.5), BlendMode: "MULTIPLY", RectangleCornerRadii: []float64{1, 2, 3, 4}, MinWidth: ptr(10.0), MaxWidth: ptr(20.0), MinHeight: ptr(5.0), MaxHeight: ptr(50.0), OverflowDirection: "HORIZONTAL_SCROLLING"}, nil, empty, nil, Options{CSS: true})
	if sides.CSS["border-top"] != "2px solid #000000" || sides.CSS["border-bottom"] != "1px solid #000000" || sides.CSS["border-left"] != "" || sides.CSS["border"] != "" {
		t.Fatalf("per side borders: %v", sides.CSS)
	}
	if sides.CSS["backdrop-filter"] != "blur(4px)" || sides.CSS["box-shadow"] != "inset 0px 0px 2px rgba(0, 0, 0, 0.5), 0px 0px 2px rgba(0, 0, 0, 0.5)" || sides.CSS["opacity"] != "0.5" || sides.CSS["mix-blend-mode"] != "multiply" {
		t.Fatalf("effects css: %v", sides.CSS)
	}
	if sides.CSS["border-radius"] != "1px 2px 3px 4px" || sides.CSS["min-width"] != "10px" || sides.CSS["max-height"] != "50px" || sides.CSS["overflow-x"] != "auto" {
		t.Fatalf("misc css: %v", sides.CSS)
	}
	multiline := Inspect(&figma.Node{ID: "m", Type: "TEXT", Characters: "x", Style: &figma.TypeStyle{FontFamily: "Source Serif", TextTruncation: "ENDING", MaxLines: ptr(3), TextCase: "SMALL_CAPS", TextDecoration: "UNDERLINE", Italic: ptr(true), LineHeightUnit: "INTRINSIC_%"}}, nil, empty, nil, Options{CSS: true})
	if multiline.CSS["-webkit-line-clamp"] != "3" || multiline.CSS["display"] != "-webkit-box" || multiline.CSS["font-variant-caps"] != "small-caps" || multiline.CSS["text-decoration"] != "underline" || multiline.CSS["font-style"] != "italic" || multiline.CSS["font-family"] != `"Source Serif"` || multiline.CSS["line-height"] != "normal" {
		t.Fatalf("multiline css: %v", multiline.CSS)
	}
	fillRoot := Inspect(&figma.Node{ID: "r", Type: "FRAME", LayoutSizingHorizontal: "FILL", LayoutSizingVertical: "FILL", AbsoluteBoundingBox: &figma.Rectangle{Width: 10, Height: 10}}, nil, empty, nil, Options{CSS: true})
	if fillRoot.CSS["width"] != "100%" || fillRoot.CSS["height"] != "100%" {
		t.Fatalf("fill without parent: %v", fillRoot.CSS)
	}
	if CSS(&model.Node{}, parentInfo{}) != nil {
		t.Fatal("empty css is nil")
	}
	if gridTemplate("", 0) != "none" || gridTemplate("", 3) != "repeat(3, minmax(0, 1fr))" {
		t.Fatal("grid template fallback")
	}
}

func assertCSS(t *testing.T, name string, got, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s css %s = %q, want %q", name, k, got[k], v)
		}
	}
	if len(got) != len(want) {
		raw, _ := json.Marshal(got)
		t.Errorf("%s css has %d declarations, want %d: %s", name, len(got), len(want), raw)
	}
}

func TestHelpers(t *testing.T) {
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	if p := Parent(file.Document, "2:3"); p == nil || p.ID != "2:2" {
		t.Fatalf("parent: %+v", p)
	}
	if Parent(file.Document, "0:0") != nil || Parent(file.Document, "nope") != nil {
		t.Fatal("root and unknown have no parent")
	}
	if strings.Join(Path(file.Document, "I2:5;3:23"), "/") != "Screens/Onboarding/Login/Input/Text/Field" {
		t.Fatalf("path: %v", Path(file.Document, "I2:5;3:23"))
	}
	if Path(file.Document, "nope") != nil {
		t.Fatal("unknown path")
	}
	ids := StyleIDs(file.Document.Find("2:2"))
	if strings.Join(ids, ",") != "5:3,5:4,5:2,5:1" {
		t.Fatalf("style ids: %v", ids)
	}
	if StyleIDs(nil) != nil {
		t.Fatal("nil root")
	}
	if paddingShorthand(1, 2, 3, 2) != "1px 2px 3px" || paddingShorthand(1, 2, 3, 4) != "1px 2px 3px 4px" || paddingShorthand(0, 0, 0, 0) != "0px" {
		t.Fatal("padding shorthand")
	}
	if propertyName("Label#5:0") != "Label" || propertyName("Variant") != "Variant" {
		t.Fatal("property name")
	}
	if v := variantProperties("A=1, B=two"); v["A"] != "1" || v["B"] != "two" {
		t.Fatalf("variants: %v", v)
	}
	if variantProperties("Plain") != nil || variantProperties("A=1, broken") != nil {
		t.Fatal("non variant names")
	}
	if stringValue(nil) != "" || stringValue(3.0) != "3" || stringValue(true) != "true" || stringValue("s") != "s" {
		t.Fatal("string value")
	}
	if overflow("HORIZONTAL_AND_VERTICAL_SCROLLING") != "scroll" || overflow("NONE") != "" {
		t.Fatal("overflow")
	}
}
