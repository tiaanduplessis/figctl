package resolve

import (
	"errors"
	"math"
	"strings"
	"testing"
	"testing/quick"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func ptr[T any](v T) *T { return &v }

func TestColorConversions(t *testing.T) {
	tests := []struct {
		name    string
		color   Color
		opacity float64
		hex     string
		hex8    string
		rgba    string
		css     string
	}{
		{"opaque brand", Color{R: 0.2, G: 0.4, B: 1, A: 1}, 1, "#3366ff", "", "rgba(51, 102, 255, 1)", "#3366ff"},
		{"alpha only", Color{R: 0, G: 0, B: 0, A: 0.1}, 1, "#000000", "#0000001a", "rgba(0, 0, 0, 0.1)", "rgba(0, 0, 0, 0.1)"},
		{"paint opacity", Color{R: 1, G: 1, B: 1, A: 1}, 0.5, "#ffffff", "#ffffff80", "rgba(255, 255, 255, 0.5)", "rgba(255, 255, 255, 0.5)"},
		{"alpha and opacity multiply", Color{R: 1, G: 0, B: 0, A: 0.5}, 0.5, "#ff0000", "#ff000040", "rgba(255, 0, 0, 0.25)", "rgba(255, 0, 0, 0.25)"},
		{"rounded channels", Color{R: 0.0666667, G: 0.0941176, B: 0.1529412, A: 1}, 1, "#111827", "", "rgba(17, 24, 39, 1)", "#111827"},
		{"alpha trimmed to two decimals", Color{R: 0, G: 0, B: 0, A: 0.333}, 1, "#000000", "#00000055", "rgba(0, 0, 0, 0.33)", "rgba(0, 0, 0, 0.33)"},
		{"clamped", Color{R: 1.2, G: -0.1, B: 0.5, A: 1}, 1, "#ff0080", "", "rgba(255, 0, 128, 1)", "#ff0080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.color.WithOpacity(tc.opacity)
			if got := c.Hex(); got != tc.hex {
				t.Errorf("Hex = %s, want %s", got, tc.hex)
			}
			if got := c.Hex8(); got != tc.hex8 {
				t.Errorf("Hex8 = %s, want %s", got, tc.hex8)
			}
			if got := c.RGBA(); got != tc.rgba {
				t.Errorf("RGBA = %s, want %s", got, tc.rgba)
			}
			if got := c.CSS(); got != tc.css {
				t.Errorf("CSS = %s, want %s", got, tc.css)
			}
		})
	}
}

func TestParseHex(t *testing.T) {
	c, ok := ParseHex("#3366FF")
	if !ok || c.Hex() != "#3366ff" || c.A != 1 {
		t.Fatalf("parse: %v %v", c, ok)
	}
	c, ok = ParseHex("fff")
	if !ok || c.Hex() != "#ffffff" {
		t.Fatalf("short form: %v %v", c, ok)
	}
	c, ok = ParseHex("#00000080")
	if !ok || c.Hex8() != "#00000080" {
		t.Fatalf("alpha form: %v %v", c, ok)
	}
	for _, bad := range []string{"", "#12", "#gggggg", "#12345"} {
		if _, ok := ParseHex(bad); ok {
			t.Errorf("ParseHex(%q) should fail", bad)
		}
	}
}

// TestHexRoundTrip checks that any 8-bit color survives Hex8 and ParseHex.
func TestHexRoundTrip(t *testing.T) {
	roundTrip := func(r, g, b, a uint8) bool {
		c := Color{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255, A: float64(a) / 255}
		s := c.Hex8()
		if a == 255 {
			s = c.Hex()
		}
		parsed, ok := ParseHex(s)
		if !ok {
			return false
		}
		return channel(parsed.R) == int(r) && channel(parsed.G) == int(g) && channel(parsed.B) == int(b) && channel(parsed.A) == int(a)
	}
	if err := quick.Check(roundTrip, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}

func TestNum(t *testing.T) {
	tests := []struct {
		v        float64
		decimals int
		want     string
	}{
		{16, 2, "16"}, {0.5, 2, "0.5"}, {1.005, 2, "1"}, {33.3333, 2, "33.33"}, {-0.5, 2, "-0.5"}, {0, 2, "0"}, {math.NaN(), 2, "0"}, {-0.001, 2, "0"},
	}
	for _, tc := range tests {
		if got := Num(tc.v, tc.decimals); got != tc.want {
			t.Errorf("Num(%v, %d) = %s, want %s", tc.v, tc.decimals, got, tc.want)
		}
	}
	if Px(12.5) != "12.5px" {
		t.Errorf("Px = %s", Px(12.5))
	}
}

func TestGradientAngle(t *testing.T) {
	tests := []struct {
		name    string
		handles []figma.Vector
		want    float64
	}{
		{"left to right is 90", []figma.Vector{{X: 0, Y: 0.5}, {X: 1, Y: 0.5}, {X: 0, Y: 1}}, 90},
		{"top to bottom is 180", []figma.Vector{{X: 0.5, Y: 0}, {X: 0.5, Y: 1}, {X: 1, Y: 0}}, 180},
		{"bottom to top is 0", []figma.Vector{{X: 0.5, Y: 1}, {X: 0.5, Y: 0}, {X: 1, Y: 1}}, 0},
		{"right to left is 270", []figma.Vector{{X: 1, Y: 0.5}, {X: 0, Y: 0.5}, {X: 1, Y: 1}}, 270},
		{"bottom left to top right is 45", []figma.Vector{{X: 0, Y: 1}, {X: 1, Y: 0}, {X: 1, Y: 1}}, 45},
		{"top left to bottom right is 135", []figma.Vector{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 1, Y: 0}}, 135},
		{"missing handles default to 180", nil, 180},
		{"coincident handles default to 180", []figma.Vector{{X: 0.5, Y: 0.5}, {X: 0.5, Y: 0.5}}, 180},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := GradientAngle(tc.handles); got != tc.want {
				t.Errorf("GradientAngle = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGradientCSS(t *testing.T) {
	r := New(Options{})
	stops := []figma.ColorStop{
		{Position: 0, Color: figma.Color{R: 0.2, G: 0.4, B: 1, A: 1}},
		{Position: 1, Color: figma.Color{R: 0.6, G: 0.2, B: 1, A: 0.5}},
	}
	linear := r.Gradient(figma.Paint{Type: "GRADIENT_LINEAR", GradientHandlePositions: []figma.Vector{{X: 0, Y: 0.5}, {X: 1, Y: 0.5}, {X: 0, Y: 1}}, GradientStops: stops})
	if linear.Type != "linear" || linear.Angle != 90 || linear.Approximate {
		t.Fatalf("linear: %+v", linear)
	}
	if linear.CSS != "linear-gradient(90deg, #3366ff 0%, rgba(153, 51, 255, 0.5) 100%)" {
		t.Fatalf("linear css: %s", linear.CSS)
	}
	if linear.Stops[0].Hex != "#3366ff" || linear.Stops[1].RGBA != "rgba(153, 51, 255, 0.5)" || linear.Stops[0].Token != nil {
		t.Fatalf("stops: %+v", linear.Stops)
	}

	withOpacity := r.Gradient(figma.Paint{Type: "GRADIENT_LINEAR", Opacity: ptr(0.5), GradientStops: stops[:1]})
	if withOpacity.Stops[0].RGBA != "rgba(51, 102, 255, 0.5)" || withOpacity.Angle != 180 {
		t.Fatalf("paint opacity should apply to stops: %+v", withOpacity)
	}

	radial := r.Gradient(figma.Paint{Type: "GRADIENT_RADIAL", GradientHandlePositions: []figma.Vector{{X: 0.5, Y: 0.5}, {X: 1, Y: 0.5}, {X: 0.5, Y: 1}}, GradientStops: stops})
	if radial.Type != "radial" || !radial.Approximate || !strings.HasPrefix(radial.CSS, "radial-gradient(50% 50% at 50% 50%, #3366ff 0%") {
		t.Fatalf("radial: %+v", radial)
	}
	angular := r.Gradient(figma.Paint{Type: "GRADIENT_ANGULAR", GradientHandlePositions: []figma.Vector{{X: 0.5, Y: 0.5}, {X: 1, Y: 0.5}, {X: 0.5, Y: 1}}, GradientStops: stops})
	if angular.Type != "angular" || !angular.Approximate || !strings.HasPrefix(angular.CSS, "conic-gradient(from 90deg at 50% 50%, ") {
		t.Fatalf("angular: %+v", angular)
	}
	diamond := r.Gradient(figma.Paint{Type: "GRADIENT_DIAMOND", GradientStops: stops})
	if diamond.Type != "diamond" || !diamond.Approximate || !strings.HasPrefix(diamond.CSS, "radial-gradient(") {
		t.Fatalf("diamond: %+v", diamond)
	}
	if r.Gradient(figma.Paint{Type: "SOLID"}) != nil {
		t.Fatal("solid paint is not a gradient")
	}
}

func fixtureResolver(t *testing.T) *Resolver {
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
	return New(Options{File: &file, Variables: &vars, StyleNodes: styleNodes})
}

func TestVariableLookupAndModes(t *testing.T) {
	r := fixtureResolver(t)
	if !r.HasVariables() {
		t.Fatal("fixture has variables")
	}
	v, ok := r.Variable("VariableID:1:201")
	if !ok || v.Name != "bg/surface" || v.Collection != "Semantic" || v.ResolvedType != "COLOR" || v.CodeSyntax["WEB"] != "--bg-surface" {
		t.Fatalf("variable: %+v", v)
	}
	if len(v.Modes) != 2 || v.Modes[0].Name != "Light" || !v.Modes[0].Default || v.Modes[1].Name != "Dark" {
		t.Fatalf("modes: %+v", v.Modes)
	}
	if v, ok := r.Variable("VariableID:1:201/1"); !ok || v.ID != "VariableID:1:201" {
		t.Fatalf("subscribed style id should match by prefix: %+v", v)
	}
	if v, ok := r.Variable("VariableID:7c1e5a9b3d2f4e6a8c0b1d3f5e7a9c2b4d6f8104/1:2"); !ok || v.Name != "space/4" {
		t.Fatalf("id carrying a key should match by key: %+v", v)
	}
	if _, ok := r.Variable("VariableID:9:9"); ok {
		t.Fatal("unknown id must not resolve")
	}
	if v, ok := r.VariableByName("Semantic/text/primary"); !ok || v.ID != "VariableID:1:202" {
		t.Fatalf("qualified name: %+v", v)
	}
	if v, ok := r.VariableByName("BRAND/500"); !ok || v.ID != "VariableID:1:101" {
		t.Fatalf("case insensitive name: %+v", v)
	}
	collections := r.Modes()
	if len(collections) != 2 || collections[0].Name != "Primitives" || collections[1].Name != "Semantic" || collections[1].DefaultModeID != "2:0" || len(collections[1].Modes) != 2 {
		t.Fatalf("collections: %+v", collections)
	}
	all := r.Variables()
	if len(all) != 8 || all[0].Name != "brand/500" || all[7].Name != "text/primary" {
		t.Fatalf("sorted variables: %v", all)
	}
}

func TestAliasChainAcrossCollectionsAndModes(t *testing.T) {
	r := fixtureResolver(t)
	light, err := r.Value("VariableID:1:201", "2:0")
	if err != nil {
		t.Fatal(err)
	}
	if light.Type != "COLOR" || light.Color.Hex() != "#ffffff" || strings.Join(light.Chain, ">") != "bg/surface>neutral/0" {
		t.Fatalf("light: %+v", light)
	}
	dark, err := r.Value("VariableID:1:201", "2:1")
	if err != nil {
		t.Fatal(err)
	}
	if dark.Color.Hex() != "#111827" || strings.Join(dark.Chain, ">") != "bg/surface>neutral/900" {
		t.Fatalf("dark: %+v", dark)
	}
	def, err := r.Value("VariableID:1:201", "")
	if err != nil || def.Any() != "#ffffff" {
		t.Fatalf("default mode: %+v %v", def, err)
	}
	// A mode of an unrelated collection falls back to the default mode.
	prim, err := r.Value("VariableID:1:101", "2:1")
	if err != nil || prim.Any() != "#3366ff" || len(prim.Chain) != 1 {
		t.Fatalf("foreign mode: %+v %v", prim, err)
	}
	num, err := r.Value("VariableID:1:104", "")
	if err != nil || num.Any() != float64(16) || num.Text() != "16" {
		t.Fatalf("number: %+v %v", num, err)
	}
	str, _ := r.Value("VariableID:1:105", "")
	boolean, _ := r.Value("VariableID:1:106", "")
	if str.Any() != "Inter" || boolean.Any() != true || boolean.Text() != "true" {
		t.Fatalf("scalars: %v %v", str.Any(), boolean.Any())
	}
	if _, err := r.Value("VariableID:9:9", ""); !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("unknown: %v", err)
	}

	all, err := r.ResolveAll("VariableID:1:202")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Mode != "Light" || !all[0].Default || all[0].Value.Any() != "#111827" || all[1].Mode != "Dark" || all[1].Value.Any() != "#ffffff" {
		t.Fatalf("resolve all: %+v", all)
	}
	if all[0].Label("VariableCollectionId:1:200") != "Light" || all[0].Label("other") != "Semantic/Light" {
		t.Fatalf("labels: %s %s", all[0].Label("VariableCollectionId:1:200"), all[0].Label("other"))
	}
}

func testVariables() *figma.GetLocalVariablesResponse {
	vars := &figma.GetLocalVariablesResponse{}
	vars.Meta.VariableCollections = map[string]figma.VariableCollection{
		"C:base":  {ID: "C:base", Name: "Base", DefaultModeID: "m:default", Modes: []figma.VariableMode{{ModeID: "m:default", Name: "Default"}}},
		"C:theme": {ID: "C:theme", Name: "Theme", DefaultModeID: "m:light", Modes: []figma.VariableMode{{ModeID: "m:light", Name: "Light"}, {ModeID: "m:dark", Name: "Dark"}}},
		"C:brandx": {
			ID: "C:brandx", Name: "Brand X", DefaultModeID: "m:x-light", ParentVariableCollectionID: "C:theme",
			Modes: []figma.VariableMode{{ModeID: "m:x-light", Name: "Light", ParentModeID: "m:light"}, {ModeID: "m:x-dark", Name: "Dark", ParentModeID: "m:dark"}},
			VariableOverrides: map[string]map[string]figma.VariableValue{
				"V:accent": {"m:x-dark": {Color: &figma.Color{R: 1, G: 0, B: 0, A: 1}}},
			},
		},
		"C:component": {ID: "C:component", Name: "Component", DefaultModeID: "m:one", Modes: []figma.VariableMode{{ModeID: "m:one", Name: "Only"}}},
	}
	vars.Meta.Variables = map[string]figma.Variable{
		"V:blue":  {ID: "V:blue", Name: "blue", Key: "k-blue", VariableCollectionID: "C:base", ResolvedType: "COLOR", ValuesByMode: map[string]figma.VariableValue{"m:default": {Color: &figma.Color{R: 0, G: 0, B: 1, A: 1}}}},
		"V:black": {ID: "V:black", Name: "black", Key: "k-black", VariableCollectionID: "C:base", ResolvedType: "COLOR", ValuesByMode: map[string]figma.VariableValue{"m:default": {Color: &figma.Color{A: 1}}}},
		"V:accent": {ID: "V:accent", Name: "accent", VariableCollectionID: "C:theme", ResolvedType: "COLOR", ValuesByMode: map[string]figma.VariableValue{
			"m:light": {Alias: &figma.VariableAlias{Type: "VARIABLE_ALIAS", ID: "V:blue"}},
			"m:dark":  {Alias: &figma.VariableAlias{Type: "VARIABLE_ALIAS", ID: "V:black"}},
		}},
		"V:button": {ID: "V:button", Name: "button/bg", VariableCollectionID: "C:component", ResolvedType: "COLOR", ValuesByMode: map[string]figma.VariableValue{
			"m:one": {Alias: &figma.VariableAlias{Type: "VARIABLE_ALIAS", ID: "V:accent"}},
		}},
		"V:loopA":    {ID: "V:loopA", Name: "loopA", VariableCollectionID: "C:base", ResolvedType: "FLOAT", ValuesByMode: map[string]figma.VariableValue{"m:default": {Alias: &figma.VariableAlias{Type: "VARIABLE_ALIAS", ID: "V:loopB"}}}},
		"V:loopB":    {ID: "V:loopB", Name: "loopB", VariableCollectionID: "C:base", ResolvedType: "FLOAT", ValuesByMode: map[string]figma.VariableValue{"m:default": {Alias: &figma.VariableAlias{Type: "VARIABLE_ALIAS", ID: "V:loopA"}}}},
		"V:dangling": {ID: "V:dangling", Name: "dangling", VariableCollectionID: "C:base", ResolvedType: "FLOAT", ValuesByMode: map[string]figma.VariableValue{"m:default": {Alias: &figma.VariableAlias{Type: "VARIABLE_ALIAS", ID: "V:missing"}}}},
	}
	return vars
}

func TestCycleProtection(t *testing.T) {
	r := New(Options{Variables: testVariables()})
	_, err := r.Value("V:loopA", "")
	if !errors.Is(err, ErrCycle) || !strings.Contains(err.Error(), "loopA -> loopB -> loopA") {
		t.Fatalf("cycle: %v", err)
	}
	_, err = r.Value("V:dangling", "")
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("dangling alias: %v", err)
	}
	all, err := r.ResolveAll("V:loopA")
	if err != nil || len(all) != 1 || all[0].Error == "" {
		t.Fatalf("resolve all with cycle should report per mode: %+v %v", all, err)
	}
	ref := r.Token("V:loopA")
	if ref == nil || ref.Name != "loopA" || len(ref.Values) != 0 {
		t.Fatalf("token with cycle: %+v", ref)
	}
}

func TestExtendedCollectionOverrides(t *testing.T) {
	r := New(Options{Variables: testVariables()})
	// Inherited from the parent mode: Brand X light has no override.
	v, err := r.Value("V:accent", "m:x-light")
	if err != nil || v.Any() != "#0000ff" || strings.Join(v.Chain, ">") != "accent>blue" {
		t.Fatalf("inherited: %+v %v", v, err)
	}
	// Overridden in the extension: Brand X dark is red.
	v, err = r.Value("V:accent", "m:x-dark")
	if err != nil || v.Any() != "#ff0000" || strings.Join(v.Chain, ">") != "accent" {
		t.Fatalf("override: %+v %v", v, err)
	}
	all, err := r.ResolveAll("V:accent")
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	values := []string{}
	for _, mv := range all {
		labels = append(labels, mv.Label("C:theme"))
		values = append(values, mv.Value.Text())
	}
	if strings.Join(labels, ",") != "Light,Dark,Brand X/Light,Brand X/Dark" || strings.Join(values, ",") != "#0000ff,#000000,#0000ff,#ff0000" {
		t.Fatalf("resolve all: %v %v", labels, values)
	}
}

func TestResolveAllExpandsAliasedCollectionModes(t *testing.T) {
	r := New(Options{Variables: testVariables()})
	all, err := r.ResolveAll("V:button")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, mv := range all {
		got[mv.Label("C:component")] = mv.Value.Text()
	}
	if got["Only"] != "#0000ff" || got["Theme/Light"] != "#0000ff" || got["Theme/Dark"] != "#000000" || len(got) != 3 {
		t.Fatalf("expanded modes: %v", got)
	}
	ref := r.Token("V:button")
	if ref.Values["Only"] != "#0000ff" || ref.Values["Theme/Dark"] != "#000000" || strings.Join(ref.Chain, ">") != "button/bg>accent>blue" {
		t.Fatalf("token: %+v", ref)
	}
}

func TestPublishedFallback(t *testing.T) {
	var published figma.GetPublishedVariablesResponse
	figmatest.Decode(t, "variables_published.json", &published)
	r := New(Options{Published: &published})
	if r.HasVariables() || !r.HasPublished() {
		t.Fatal("published only")
	}
	v, ok := r.Variable("VariableID:1:201/1")
	if !ok || v.Name != "bg/surface" || v.Collection != "Semantic" || v.ResolvedType != "COLOR" || !v.Published {
		t.Fatalf("published variable: %+v", v)
	}
	ref := r.Token("VariableID:1:201")
	if ref == nil || ref.Name != "bg/surface" || !ref.Unresolved || ref.Values != nil {
		t.Fatalf("published token: %+v", ref)
	}
	empty := New(Options{})
	ref = empty.Token("VariableID:1:201")
	if ref == nil || ref.VariableID != "VariableID:1:201" || !ref.Unresolved || ref.Name != "" {
		t.Fatalf("unknown token: %+v", ref)
	}
	if empty.Token("") != nil {
		t.Fatal("empty id has no token")
	}

	// Subscribed ids map to local definitions through the published list.
	var vars figma.GetLocalVariablesResponse
	figmatest.Decode(t, "variables_local.json", &vars)
	both := New(Options{Variables: &vars, Published: &published})
	val, err := both.Value("VariableID:1:104/1", "")
	if err != nil || val.Any() != float64(16) {
		t.Fatalf("subscribed id: %+v %v", val, err)
	}
}

func TestBoundTokens(t *testing.T) {
	r := fixtureResolver(t)
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	login := file.Document.Find("2:2")
	gap := r.Bound(login, "itemSpacing")
	if gap == nil || gap.Name != "space/4" || gap.Values["Default"] != float64(16) || gap.CodeSyntax["WEB"] != "--space-4" {
		t.Fatalf("gap: %+v", gap)
	}
	if r.Bound(login, "paddingLeft") != nil {
		t.Fatal("unbound property must be nil")
	}
	fill := r.Bound(login, "fills")
	if fill == nil || fill.Name != "bg/surface" || fill.Values["Dark"] != "#111827" || strings.Join(fill.Chain, ">") != "bg/surface>neutral/0" {
		t.Fatalf("fills: %+v", fill)
	}
	if r.BoundAt(login, "fills", 1) != nil || r.BoundAt(login, "itemSpacing", 1) != nil {
		t.Fatal("out of range bindings must be nil")
	}
	paint := r.PaintToken(login, "fills", 0, login.Fills[0])
	if paint == nil || paint.VariableID != "VariableID:1:201" {
		t.Fatalf("paint token: %+v", paint)
	}
	heading := file.Document.Find("2:3")
	family := r.AliasToken(heading.Style.BoundVariables, "fontFamily")
	if family == nil || family.Name != "font/family/sans" || family.Values["Default"] != "Inter" {
		t.Fatalf("font family token: %+v", family)
	}
	if CSSVar(family) != "var(--font-sans)" || CSSVar(nil) != "" || CSSVar(r.Token("VariableID:1:102")) != "" {
		t.Fatalf("css var: %s", CSSVar(family))
	}
	fields := &figma.Node{BoundVariables: map[string]figma.VariableBinding{"size": {Fields: map[string]figma.VariableAlias{"size": {ID: "VariableID:1:104"}}}}}
	if got := r.Bound(fields, "size"); got == nil || got.Name != "space/4" {
		t.Fatalf("field binding: %+v", got)
	}
}

func TestStyles(t *testing.T) {
	r := fixtureResolver(t)
	styles := r.Styles()
	if len(styles) != 4 || styles[0].Type != "EFFECT" || styles[1].Type != "FILL" || styles[2].Type != "GRID" || styles[3].Type != "TEXT" {
		t.Fatalf("styles: %+v", styles)
	}
	fill, ok := r.Style("5:1")
	if !ok || fill.Name != "color/brand/500" || fill.Value == nil || len(fill.Value.Fills) != 1 {
		t.Fatalf("fill style: %+v", fill)
	}
	if f := fill.Value.Fills[0]; f.Hex != "#3366ff" || f.Token == nil || f.Token.Name != "brand/500" {
		t.Fatalf("fill value: %+v", f)
	}
	text, ok := r.StyleByKey("5f4e3d2c1b0a9f8e7d6c5b4a39281706f5e4d502")
	if !ok || text.ID != "5:2" || text.Value.Typography == nil {
		t.Fatalf("text style: %+v", text)
	}
	typo := text.Value.Typography
	if typo.FontFamily != "Inter" || *typo.Weight != 700 || *typo.Size != 28 || typo.LineHeight.Px != 34 || *typo.LineHeight.Unitless != 1.21 || *typo.LetterSpacing.Em != -0.018 {
		t.Fatalf("typography: %+v", typo)
	}
	if typo.Tokens["fontFamily"] == nil || typo.Tokens["fontSize"] != nil || typo.Characters != "" {
		t.Fatalf("typography tokens: %+v", typo.Tokens)
	}
	effect, ok := r.StyleByName("shadow/md")
	if !ok || len(effect.Value.Effects) != 1 || effect.Value.Effects[0].CSS != "0px 4px 12px rgba(0, 0, 0, 0.1)" || effect.Value.Effects[0].Property != "box-shadow" {
		t.Fatalf("effect style: %+v", effect)
	}
	grid, ok := r.Style("5:4")
	if !ok || len(grid.Value.Grids) != 1 || grid.Value.Grids[0].Pattern != "columns" || grid.Value.Grids[0].Count != 12 || grid.Value.Grids[0].Gutter != 16 {
		t.Fatalf("grid style: %+v", grid)
	}
	if ref := grid.Ref(); ref.ID != "5:4" || ref.Type != "GRID" || ref.Name != "grid/12" {
		t.Fatalf("ref: %+v", ref)
	}
	if _, ok := r.Style("9:9"); ok {
		t.Fatal("unknown style")
	}
	if ref := r.StyleRef("5:3"); ref == nil || ref.Name != "shadow/md" {
		t.Fatalf("style ref: %+v", ref)
	}
	if ref := r.StyleRef("9:9"); ref == nil || ref.ID != "9:9" || ref.Name != "" {
		t.Fatalf("unknown style ref keeps the id: %+v", ref)
	}
	// Without the style node the metadata still resolves.
	noNodes := New(Options{File: fixtureFile(t)})
	s, ok := noNodes.Style("5:1")
	if !ok || s.Value != nil {
		t.Fatalf("style without node: %+v", s)
	}
	noNodes.AddStyleNode("5:1", &figma.Node{Fills: []figma.Paint{{Type: "SOLID", Color: &figma.Color{R: 1, A: 1}}}})
	s, _ = noNodes.Style("5:1")
	if s.Value == nil || s.Value.Fills[0].Hex != "#ff0000" {
		t.Fatalf("added style node: %+v", s)
	}
	noNodes.AddStyle("7:7", figma.Style{Key: "k7", Name: "extra", StyleType: "FILL"})
	if _, ok := noNodes.Style("7:7"); !ok {
		t.Fatal("added style metadata")
	}
	if r.StyleValueOf("BOGUS", &figma.Node{}) != nil {
		t.Fatal("unknown style type has no value")
	}
}

func fixtureFile(t *testing.T) *figma.GetFileResponse {
	t.Helper()
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	return &file
}

func TestPaintsAndEffects(t *testing.T) {
	r := New(Options{})
	solid := r.Paint(figma.Paint{Type: "SOLID", Color: &figma.Color{R: 1, G: 1, B: 1, A: 1}, Opacity: ptr(0.5), BlendMode: "MULTIPLY"})
	if solid.Type != "solid" || solid.Hex != "#ffffff" || solid.Hex8 != "#ffffff80" || solid.RGBA != "rgba(255, 255, 255, 0.5)" || *solid.Opacity != 0.5 || solid.BlendMode != "multiply" {
		t.Fatalf("solid: %+v", solid)
	}
	image := r.Paint(figma.Paint{Type: "IMAGE", ImageRef: "img1", ScaleMode: "FILL"})
	if image.Type != "image" || image.ImageRef != "img1" || image.ScaleMode != "fill" {
		t.Fatalf("image: %+v", image)
	}
	hidden := r.Fills(&figma.Node{}, "fills", []figma.Paint{{Type: "SOLID", Visible: ptr(false), Color: &figma.Color{A: 1}}}, nil)
	if len(hidden) != 0 {
		t.Fatal("hidden paints are dropped")
	}
	if BlendMode("PASS_THROUGH") != "" || BlendMode("LINEAR_BURN") != "plus-darker" || BlendMode("SOFT_LIGHT") != "soft-light" {
		t.Fatal("blend modes")
	}

	effects := r.Effects([]figma.Effect{
		{Type: "DROP_SHADOW", Color: &figma.Color{A: 0.25}, Offset: &figma.Vector{X: 0, Y: 2}, Radius: ptr(4.0), Spread: ptr(1.0)},
		{Type: "INNER_SHADOW", Color: &figma.Color{R: 1, A: 1}, Offset: &figma.Vector{X: 1, Y: 1}, Radius: ptr(0.0)},
		{Type: "LAYER_BLUR", Radius: ptr(4.0)},
		{Type: "BACKGROUND_BLUR", Radius: ptr(10.0)},
		{Type: "LAYER_BLUR", Radius: ptr(4.0), Visible: ptr(false)},
		{Type: "NOISE"},
	}, nil)
	if len(effects) != 4 {
		t.Fatalf("effects: %+v", effects)
	}
	if effects[0].CSS != "0px 2px 4px 1px rgba(0, 0, 0, 0.25)" || effects[0].Type != "drop-shadow" || effects[0].Spread != 1 {
		t.Fatalf("drop shadow: %+v", effects[0])
	}
	if effects[1].CSS != "inset 1px 1px 0px #ff0000" || effects[1].Type != "inner-shadow" {
		t.Fatalf("inner shadow: %+v", effects[1])
	}
	if effects[2].CSS != "blur(2px)" || effects[2].Property != "filter" || effects[3].CSS != "blur(5px)" || effects[3].Property != "backdrop-filter" {
		t.Fatalf("blurs: %+v %+v", effects[2], effects[3])
	}
}

func TestTypographyHelpers(t *testing.T) {
	tests := []struct {
		name  string
		style figma.TypeStyle
		want  float64
	}{
		{"explicit", figma.TypeStyle{FontWeight: ptr(300.0)}, 300},
		{"semibold style", figma.TypeStyle{FontStyle: "Semi Bold"}, 600},
		{"bold italic", figma.TypeStyle{FontStyle: "Bold Italic"}, 700},
		{"italic only", figma.TypeStyle{FontStyle: "Italic"}, 400},
		{"postscript", figma.TypeStyle{FontPostScriptName: ptr("Inter-ExtraLight")}, 200},
		{"black", figma.TypeStyle{FontStyle: "Black"}, 900},
		{"regular", figma.TypeStyle{FontStyle: "Regular"}, 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := Weight(&tc.style)
			if w == nil || *w != tc.want {
				t.Fatalf("Weight = %v, want %v", w, tc.want)
			}
		})
	}
	if Weight(&figma.TypeStyle{}) != nil || Weight(&figma.TypeStyle{FontStyle: "Display"}) != nil {
		t.Fatal("unknown weights are nil")
	}
	if !Italic(&figma.TypeStyle{FontStyle: "Bold Italic"}) || Italic(&figma.TypeStyle{Italic: ptr(false), FontStyle: "Italic"}) || !Italic(&figma.TypeStyle{FontPostScriptName: ptr("Inter-Italic")}) {
		t.Fatal("italic detection")
	}
	cases := map[string]string{"UPPER": "uppercase", "LOWER": "lowercase", "TITLE": "capitalize", "SMALL_CAPS": "small-caps", "SMALL_CAPS_FORCED": "all-small-caps", "ORIGINAL": "", "": ""}
	for in, want := range cases {
		if got := TextCase(in); got != want {
			t.Errorf("TextCase(%s) = %s, want %s", in, got, want)
		}
	}
	decorations := map[string]string{"UNDERLINE": "underline", "STRIKETHROUGH": "line-through", "NONE": ""}
	for in, want := range decorations {
		if got := TextDecoration(in); got != want {
			t.Errorf("TextDecoration(%s) = %s, want %s", in, got, want)
		}
	}

	r := New(Options{})
	ts := &figma.TypeStyle{FontFamily: "Inter", FontSize: ptr(16.0), LineHeightPx: ptr(24.0), LineHeightUnit: "FONT_SIZE_%", LineHeightPercentFontSize: ptr(150.0), LetterSpacing: ptr(0.8), TextCase: "UPPER", TextDecoration: "UNDERLINE", TextAlignHorizontal: "JUSTIFIED", TextAlignVertical: "CENTER", TextAutoResize: "NONE", TextTruncation: "ENDING", MaxLines: ptr(2), ParagraphSpacing: ptr(8.0), OpentypeFlags: map[string]float64{"TNUM": 1}, Hyperlink: &figma.Hyperlink{Type: "NODE", NodeID: "1:1"}}
	text := r.Typography(ts, nil)
	if text.LineHeight.Unit != "percent" || *text.LineHeight.Unitless != 1.5 || *text.LineHeight.Percent != 150 || *text.LetterSpacing.Em != 0.05 {
		t.Fatalf("metrics: %+v %+v", text.LineHeight, text.LetterSpacing)
	}
	if text.Case != "uppercase" || text.Decoration != "underline" || text.AlignH != "justify" || text.AlignV != "middle" || text.AutoResize != "fixed" || text.Truncation != "ending" || *text.MaxLines != 2 || *text.ParagraphSpacing != 8 || !text.TabularNumbers || text.Hyperlink != "node:1:1" {
		t.Fatalf("text: %+v", text)
	}
	if len(text.Tokens) != 5 {
		t.Fatalf("tokens: %+v", text.Tokens)
	}
	intrinsic := r.Typography(&figma.TypeStyle{FontSize: ptr(12.0), LineHeightPx: ptr(12.0), LineHeightUnit: "INTRINSIC_%"}, nil)
	if intrinsic.LineHeight.Unit != "auto" || *intrinsic.LineHeight.Percent != 100 {
		t.Fatalf("intrinsic: %+v", intrinsic.LineHeight)
	}
	if r.Typography(nil, nil) != nil {
		t.Fatal("nil style")
	}
}

func TestRuns(t *testing.T) {
	r := fixtureResolver(t)
	file := fixtureFile(t)
	body := file.Document.Find("2:4")
	runs := r.Runs(body)
	if len(runs) != 1 || runs[0].Start != 11 || runs[0].End != 19 || runs[0].Text != "continue" {
		t.Fatalf("runs: %+v", runs)
	}
	if runs[0].Overrides == nil || *runs[0].Overrides.Weight != 600 || runs[0].Overrides.Hyperlink != "https://example.com/help" || runs[0].Overrides.Size != nil {
		t.Fatalf("overrides: %+v", runs[0].Overrides)
	}
	if runs[0].Fill == nil || runs[0].Fill.Hex != "#3366ff" || runs[0].Fill.Token == nil || runs[0].Fill.Token.Name != "brand/500" {
		t.Fatalf("run fill: %+v", runs[0].Fill)
	}
	mixed := &figma.Node{
		Characters:              "héllo wörld",
		CharacterStyleOverrides: []int{1, 1, 0, 0, 0, 2, 2, 2, 2, 2, 2},
		StyleOverrideTable:      map[string]figma.TypeStyle{"1": {FontSize: ptr(20.0)}, "2": {Italic: ptr(true)}},
		Style:                   &figma.TypeStyle{FontSize: ptr(10.0), LetterSpacing: ptr(1.0)},
	}
	runs = r.Runs(mixed)
	if len(runs) != 2 || runs[0].Text != "hé" || *runs[0].Overrides.Size != 20 || runs[1].Text != " wörld" || !runs[1].Overrides.Italic || runs[1].Start != 5 {
		t.Fatalf("mixed runs: %+v", runs)
	}
	if r.Runs(file.Document.Find("2:3")) != nil {
		t.Fatal("no overrides means no runs")
	}
}
