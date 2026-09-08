package tokens

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// fixtureResolver builds a resolver over the whole synthetic design
// system: variables, the styles listed by the file, and the style nodes
// that carry their values.
func fixtureResolver(t *testing.T) *resolve.Resolver {
	t.Helper()
	var vars figma.GetLocalVariablesResponse
	figmatest.Decode(t, "variables_local.json", &vars)
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	var nodes figma.GetFileNodesResponse
	figmatest.Decode(t, "nodes.json", &nodes)
	styleNodes := map[string]*figma.Node{}
	for id, entry := range nodes.Nodes {
		if entry != nil && entry.Document != nil {
			styleNodes[id] = entry.Document
		}
	}
	return resolve.New(resolve.Options{File: &file, Variables: &vars, StyleNodes: styleNodes})
}

func build(t *testing.T, opts BuildOptions) *Document {
	t.Helper()
	if opts.File == "" {
		opts.File, opts.Version = "Fixture Design System", "2100123456"
	}
	doc, err := Build(fixtureResolver(t), opts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return doc
}

func tokenNamed(t *testing.T, doc *Document, name string) Token {
	t.Helper()
	for _, set := range doc.Sets {
		for _, tok := range set.Tokens {
			if tok.Name == name {
				return tok
			}
		}
	}
	t.Fatalf("no token named %q in %v", name, doc.Names(0))
	return Token{}
}

func setNamed(t *testing.T, doc *Document, mode string) Set {
	t.Helper()
	for _, set := range doc.Sets {
		if set.Mode == mode {
			return set
		}
	}
	t.Fatalf("no set for mode %q", mode)
	return Set{}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name     string
		nameCase string
		want     []string
	}{
		{"color/brand/500", CaseKebab, []string{"color", "brand", "500"}},
		{"Color/Brand Primary/500", CaseKebab, []string{"color", "brand-primary", "500"}},
		{"Color/Brand Primary/500", CaseCamel, []string{"color", "brandPrimary", "500"}},
		{"Color/Brand Primary/500", CaseSnake, []string{"color", "brand_primary", "500"}},
		{"Color/Brand Primary/500", CaseNone, []string{"Color", "Brand Primary", "500"}},
		{"spacing/spaceLarge", CaseKebab, []string{"spacing", "space-large"}},
		{"spacing/space_large", CaseCamel, []string{"spacing", "spaceLarge"}},
		{"a//b", CaseKebab, []string{"a", "b"}},
		{"weird.{name}", CaseNone, []string{"weirdname"}},
		{"weird.$name", CaseKebab, []string{"weirdname"}},
		{"///", CaseKebab, []string{"token"}},
	}
	for _, c := range cases {
		got := NormalizePath(c.name, c.nameCase)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("NormalizePath(%q, %s) = %v, want %v", c.name, c.nameCase, got, c.want)
		}
	}
}

func TestValidNameCase(t *testing.T) {
	for _, c := range NameCases {
		if !ValidNameCase(c) {
			t.Errorf("%s should be valid", c)
		}
	}
	if ValidNameCase("pascal") {
		t.Error("pascal should not be valid")
	}
}

// TestCollidingNamesWithDifferentValues covers two distinct sources that
// normalize onto one token path. Only one value can be exported, so the other
// has to be reported rather than dropped in silence.
func TestCollidingNamesWithDifferentValues(t *testing.T) {
	r := syntheticVariables(t, []synthetic{
		{id: "VariableID:9:1", name: "Brand Color", typ: "COLOR", value: figma.VariableValue{Color: &figma.Color{R: 1, A: 1}}},
		{id: "VariableID:9:2", name: "brand-color", typ: "COLOR", value: figma.VariableValue{Color: &figma.Color{B: 1, A: 1}}},
	})
	doc, err := Build(r, BuildOptions{Variables: true})
	if err != nil {
		t.Fatalf("a collision must not fail the export: %v", err)
	}
	// One token survives, and it is the first.
	tok := tokenNamed(t, doc, "brand-color")
	if tok.Value != "#ff0000" {
		t.Fatalf("the first definition should win, got %v", tok.Value)
	}
	warning := strings.Join(doc.Warnings, " | ")
	for _, want := range []string{"Brand Color", "brand-color"} {
		if !strings.Contains(warning, want) {
			t.Errorf("the warning should name %q, got %q", want, warning)
		}
	}
	// The two names stop colliding when nothing is normalized.
	if doc, err := Build(r, BuildOptions{Variables: true, NameCase: CaseNone}); err != nil {
		t.Fatalf("--name-case none should separate them: %v", err)
	} else if len(doc.Warnings) != 0 {
		t.Errorf("no warning is due once they do not collide: %v", doc.Warnings)
	}
}

// TestIdenticalDuplicatesMerge covers what a real design system looks like: a
// style copied between pages, or pulled from a second library, appears twice
// under one name with one value. Nothing is lost by keeping a single token,
// and failing the export would make the command useless on the files that
// need it most.
func TestIdenticalDuplicatesMerge(t *testing.T) {
	r := syntheticVariables(t, []synthetic{
		{id: "VariableID:9:1", name: "Card shadow", typ: "COLOR", value: figma.VariableValue{Color: &figma.Color{R: 1, A: 1}}},
		{id: "VariableID:9:2", name: "Card shadow", typ: "COLOR", value: figma.VariableValue{Color: &figma.Color{R: 1, A: 1}}},
	})
	doc, err := Build(r, BuildOptions{Variables: true})
	if err != nil {
		t.Fatalf("identical duplicates must not fail the export: %v", err)
	}
	if tok := tokenNamed(t, doc, "card-shadow"); tok.Value != "#ff0000" {
		t.Fatalf("the merged token should keep the shared value, got %v", tok.Value)
	}
	// Nothing was lost, so there is nothing to warn about.
	if len(doc.Warnings) != 0 {
		t.Errorf("merging identical duplicates needs no warning, got %v", doc.Warnings)
	}
}

func TestVariableTypeMapping(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true})
	cases := []struct {
		name  string
		typ   string
		value any
		css   string
	}{
		{"brand.500", TypeColor, "#3366ff", "#3366ff"},
		{"neutral.0", TypeColor, "#ffffff", "#ffffff"},
		{"space.4", TypeDimension, Dimension{Value: 16, Unit: "px"}, "16px"},
		{"font.family.sans", TypeFontFamily, "Inter", "Inter"},
		{"feature.show-helper", TypeBoolean, true, "true"},
	}
	for _, c := range cases {
		tok := tokenNamed(t, doc, c.name)
		if tok.Type != c.typ {
			t.Errorf("%s type = %s, want %s", c.name, tok.Type, c.typ)
		}
		if !reflect.DeepEqual(tok.Value, c.value) {
			t.Errorf("%s value = %#v, want %#v", c.name, tok.Value, c.value)
		}
		if tok.CSS != c.css {
			t.Errorf("%s css = %q, want %q", c.name, tok.CSS, c.css)
		}
	}
}

func TestNumberWithoutDimensionScope(t *testing.T) {
	weight := 700.0
	r := syntheticVariables(t, []synthetic{
		{id: "VariableID:9:1", name: "font/weight/bold", typ: "FLOAT", scopes: []string{"FONT_WEIGHT"}, value: figma.VariableValue{Number: &weight}},
		{id: "VariableID:9:2", name: "radius/sm", typ: "FLOAT", scopes: []string{"CORNER_RADIUS"}, value: figma.VariableValue{Number: ptr(4.0)}},
		{id: "VariableID:9:3", name: "content/label", typ: "STRING", scopes: []string{"TEXT_CONTENT"}, value: figma.VariableValue{String: ptr("Sign in")}},
	})
	doc, err := Build(r, BuildOptions{Variables: true})
	if err != nil {
		t.Fatal(err)
	}
	if tok := tokenNamed(t, doc, "font.weight.bold"); tok.Type != TypeNumber || tok.Value != 700.0 || tok.Category != "fontWeight" {
		t.Fatalf("font weight: %s %v %s", tok.Type, tok.Value, tok.Category)
	}
	if tok := tokenNamed(t, doc, "radius.sm"); tok.Type != TypeDimension || tok.Category != "borderRadius" {
		t.Fatalf("radius: %s %s", tok.Type, tok.Category)
	}
	if tok := tokenNamed(t, doc, "content.label"); tok.Type != TypeString || tok.Value != "Sign in" {
		t.Fatalf("string: %s %v", tok.Type, tok.Value)
	}
}

func TestStyleTypeMapping(t *testing.T) {
	doc := build(t, BuildOptions{Styles: true})
	fill := tokenNamed(t, doc, "color.brand.500")
	if fill.Type != TypeColor || fill.Value != "#3366ff" {
		t.Fatalf("fill style: %s %v", fill.Type, fill.Value)
	}
	if fill.Extensions["styleId"] != "5:1" || fill.Extensions["styleType"] != "FILL" {
		t.Fatalf("fill extensions: %v", fill.Extensions)
	}

	text := tokenNamed(t, doc, "heading.lg")
	typo, ok := text.Value.(Typography)
	if !ok {
		t.Fatalf("text style value is %T", text.Value)
	}
	if typo.FontFamily != "Inter" || typo.FontSize.Value != 28 || *typo.FontWeight != 700 ||
		typo.LetterSpacing.Value != -0.5 || *typo.LineHeight != 1.21 {
		t.Fatalf("typography: %#v", typo)
	}
	if len(text.Parts) != 5 || text.Parts[1].Suffix != "font-size" || text.Parts[1].Value != "28px" {
		t.Fatalf("typography parts: %#v", text.Parts)
	}

	shadow := tokenNamed(t, doc, "shadow.md")
	got, ok := shadow.Value.(Shadow)
	if !ok {
		t.Fatalf("shadow value is %T", shadow.Value)
	}
	want := Shadow{
		Color:   "#0000001a",
		OffsetX: Dimension{Value: 0, Unit: "px"},
		OffsetY: Dimension{Value: 4, Unit: "px"},
		Blur:    Dimension{Value: 12, Unit: "px"},
		Spread:  Dimension{Value: 0, Unit: "px"},
	}
	if got != want {
		t.Fatalf("shadow = %#v, want %#v", got, want)
	}

	grid := tokenNamed(t, doc, "grid.12")
	if grid.Type != TypeString || !strings.Contains(grid.Value.(string), "columns 12") {
		t.Fatalf("grid: %s %v", grid.Type, grid.Value)
	}
	if _, present := grid.Extensions["grid"]; !present {
		t.Fatalf("grid extensions: %v", grid.Extensions)
	}
	if !strings.Contains(grid.Description, "no layout grid type") {
		t.Fatalf("grid description: %q", grid.Description)
	}
}

func TestStyleTypesDTCGCannotExpress(t *testing.T) {
	doc := buildStyles(t, map[string]styleFixture{
		"7:1": {name: "gradient/hero", styleType: "FILL", node: &figma.Node{Fills: []figma.Paint{{
			Type:                    "GRADIENT_LINEAR",
			GradientHandlePositions: []figma.Vector{{X: 0, Y: 0}, {X: 0, Y: 1}},
			GradientStops: []figma.ColorStop{
				{Position: 0, Color: figma.Color{R: 0.2, G: 0.4, B: 1, A: 1}},
				{Position: 1, Color: figma.Color{R: 1, G: 1, B: 1, A: 0.5}},
			},
		}}}},
		"7:2": {name: "gradient/glow", styleType: "FILL", node: &figma.Node{Fills: []figma.Paint{{
			Type:                    "GRADIENT_RADIAL",
			GradientHandlePositions: []figma.Vector{{X: 0.5, Y: 0.5}, {X: 1, Y: 0.5}, {X: 0.5, Y: 1}},
			GradientStops:           []figma.ColorStop{{Position: 0, Color: figma.Color{R: 1, A: 1}}},
		}}}},
		"7:3": {name: "blur/backdrop", styleType: "EFFECT", node: &figma.Node{Effects: []figma.Effect{
			{Type: "BACKGROUND_BLUR", Radius: ptr(24.0)},
		}}},
		"7:4": {name: "shadow/stack", styleType: "EFFECT", node: &figma.Node{Effects: []figma.Effect{
			{Type: "DROP_SHADOW", Color: &figma.Color{A: 0.2}, Offset: &figma.Vector{Y: 1}, Radius: ptr(2.0)},
			{Type: "INNER_SHADOW", Color: &figma.Color{A: 0.4}, Offset: &figma.Vector{Y: -1}, Radius: ptr(3.0), Spread: ptr(1.0)},
		}}},
		"7:5": {name: "image/hero", styleType: "FILL", node: &figma.Node{Fills: []figma.Paint{
			{Type: "IMAGE", ImageRef: "img1", ScaleMode: "FILL"},
		}}},
	})

	linear := tokenNamed(t, doc, "gradient.hero")
	stops, ok := linear.Value.([]GradientStop)
	if !ok || linear.Type != TypeGradient {
		t.Fatalf("linear gradient: %s %T", linear.Type, linear.Value)
	}
	if len(stops) != 2 || stops[0].Color != "#3366ff" || stops[1].Color != "#ffffff80" {
		t.Fatalf("stops: %#v", stops)
	}
	if _, present := linear.Extensions["gradient"]; !present {
		t.Fatalf("linear gradient should keep its angle in extensions: %v", linear.Extensions)
	}

	radial := tokenNamed(t, doc, "gradient.glow")
	if radial.Type != TypeString || !strings.HasPrefix(radial.Value.(string), "radial-gradient(") {
		t.Fatalf("radial gradient: %s %v", radial.Type, radial.Value)
	}
	if !strings.Contains(radial.Description, "not representable") {
		t.Fatalf("radial description: %q", radial.Description)
	}

	blur := tokenNamed(t, doc, "blur.backdrop")
	if blur.Type != TypeString || blur.Value != "backdrop-filter: blur(12px)" {
		t.Fatalf("blur: %s %v", blur.Type, blur.Value)
	}
	if !strings.Contains(blur.Description, "no DTCG type") {
		t.Fatalf("blur description: %q", blur.Description)
	}
	// A CSS custom property carries a value, never a whole declaration:
	// emitting "backdrop-filter: blur(12px)" here makes every var() use of
	// the token invalid CSS.
	if blur.CSS != "blur(12px)" {
		t.Fatalf("blur CSS value = %q, want %q", blur.CSS, "blur(12px)")
	}
	if blur.Extensions["cssProperty"] != "backdrop-filter" {
		t.Fatalf("blur should record its CSS property, got %v", blur.Extensions)
	}

	stack := tokenNamed(t, doc, "shadow.stack")
	shadows, ok := stack.Value.([]Shadow)
	if !ok || len(shadows) != 2 {
		t.Fatalf("shadow stack: %T", stack.Value)
	}
	if shadows[0].Inset || !shadows[1].Inset || shadows[1].Spread.Value != 1 {
		t.Fatalf("shadow stack: %#v", shadows)
	}

	image := tokenNamed(t, doc, "image.hero")
	if image.Type != TypeString || image.Value != "img1" {
		t.Fatalf("image fill: %s %v", image.Type, image.Value)
	}
}

func TestAliasEmissionAndInlining(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true})
	surface := tokenNamed(t, doc, "bg.surface")
	if surface.Ref != "neutral.0" || surface.RefCSSName != "neutral-0" {
		t.Fatalf("alias: ref=%q refCss=%q", surface.Ref, surface.RefCSSName)
	}
	if surface.Value != "#ffffff" {
		t.Fatalf("the inline value should still resolve: %v", surface.Value)
	}

	// With only the Semantic collection exported the target is missing,
	// so the value is inlined and the chain recorded.
	only := build(t, BuildOptions{Variables: true, Collections: []string{"Semantic"}})
	surface = tokenNamed(t, only, "bg.surface")
	if surface.Ref != "" {
		t.Fatalf("ref should be empty when the target is not exported: %q", surface.Ref)
	}
	if surface.Value != "#ffffff" || surface.Extensions["aliasInlined"] != true {
		t.Fatalf("inlined alias: %v %v", surface.Value, surface.Extensions)
	}
	if chain, _ := surface.Extensions["alias"].(string); !strings.Contains(chain, "neutral/0") {
		t.Fatalf("alias chain: %v", surface.Extensions["alias"])
	}
}

func TestModeStrategyDefault(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true, Strategy: StrategyDefault})
	if len(doc.Sets) != 1 {
		t.Fatalf("default strategy should emit one set, got %d", len(doc.Sets))
	}
	surface := tokenNamed(t, doc, "bg.surface")
	if surface.Mode != "Light" {
		t.Fatalf("mode = %q, want Light", surface.Mode)
	}
	if surface.Modes["Light"] != "#ffffff" || surface.Modes["Dark"] != "#111827" {
		t.Fatalf("modes: %v", surface.Modes)
	}
	// Single mode collections carry no mode map.
	if tok := tokenNamed(t, doc, "brand.500"); tok.Modes != nil {
		t.Fatalf("single mode collection should not record modes: %v", tok.Modes)
	}
	outs, err := doc.DTCG()
	if err != nil {
		t.Fatal(err)
	}
	tree := decodeTree(t, outs[0].Content)
	ext := tree["bg"].(map[string]any)["surface"].(map[string]any)["$extensions"].(map[string]any)
	modes := ext[FigmaExtension].(map[string]any)["modes"].(map[string]any)
	if modes["Dark"] != "#111827" {
		t.Fatalf("$extensions modes: %v", modes)
	}
}

func TestModeStrategySeparate(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true, Strategy: StrategySeparate})
	if len(doc.Sets) != 2 {
		t.Fatalf("expected one set per mode name, got %d", len(doc.Sets))
	}
	dark := setNamed(t, doc, "Dark")
	light := setNamed(t, doc, "Light")
	find := func(set Set, name string) Token {
		for _, tok := range set.Tokens {
			if tok.Name == name {
				return tok
			}
		}
		t.Fatalf("no %s in set %s", name, set.Mode)
		return Token{}
	}
	if find(dark, "bg.surface").Ref != "neutral.900" || find(light, "bg.surface").Ref != "neutral.0" {
		t.Fatalf("dark=%s light=%s", find(dark, "bg.surface").Ref, find(light, "bg.surface").Ref)
	}
	// A single mode collection contributes its default value to both.
	if find(dark, "space.4").CSS != "16px" || find(light, "space.4").CSS != "16px" {
		t.Fatal("single mode variables should be in every set")
	}
	outs, err := doc.DTCG()
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 || outs[0].Name != "tokens.dark.json" || outs[1].Name != "tokens.light.json" {
		t.Fatalf("file names: %s %s", outs[0].Name, outs[1].Name)
	}
	tree := decodeTree(t, outs[0].Content)
	surface := tree["bg"].(map[string]any)["surface"].(map[string]any)
	if _, present := surface["$extensions"].(map[string]any)[FigmaExtension].(map[string]any)["modes"]; present {
		t.Fatal("separate documents should not repeat the mode map")
	}
}

func TestModeStrategySelect(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true, Strategy: StrategySelect, Modes: []string{"Dark"}})
	if len(doc.Sets) != 1 {
		t.Fatalf("select should emit one set, got %d", len(doc.Sets))
	}
	surface := tokenNamed(t, doc, "bg.surface")
	if surface.Mode != "Dark" || surface.Ref != "neutral.900" || surface.Value != "#111827" {
		t.Fatalf("selected mode: %+v", surface)
	}
	// Case insensitive, and a collection without that mode keeps its default.
	doc = build(t, BuildOptions{Variables: true, Strategy: StrategySelect, Modes: []string{"dark"}})
	if tokenNamed(t, doc, "bg.surface").Value != "#111827" {
		t.Fatal("--mode should match case insensitively")
	}
	if tokenNamed(t, doc, "space.4").Mode != "Default" {
		t.Fatal("a collection without the selected mode keeps its default")
	}
}

func TestModeNames(t *testing.T) {
	got := ModeNames(fixtureResolver(t))
	want := []string{"Dark", "Default", "Light"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ModeNames = %v, want %v", got, want)
	}
}

func TestDTCGStructure(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true, Styles: true})
	outs, err := doc.DTCG()
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].Name != "tokens.json" {
		t.Fatalf("outputs: %+v", outs)
	}
	tree := decodeTree(t, outs[0].Content)
	leaves := map[string]map[string]any{}
	var walk func(node map[string]any, path []string)
	walk = func(node map[string]any, path []string) {
		if _, isToken := node["$value"]; isToken {
			if node["$type"] == nil || node["$type"] == "" {
				t.Fatalf("token %s has no $type", strings.Join(path, "."))
			}
			for key := range node {
				switch key {
				case "$value", "$type", "$description", "$extensions":
				default:
					t.Fatalf("token %s has unexpected key %q", strings.Join(path, "."), key)
				}
			}
			leaves[strings.Join(path, ".")] = node
			return
		}
		for key, child := range node {
			group, ok := child.(map[string]any)
			if !ok {
				t.Fatalf("group %s has a non object child %q", strings.Join(path, "."), key)
			}
			walk(group, append(append([]string{}, path...), key))
		}
	}
	walk(tree, nil)
	if len(leaves) != doc.Summary.Tokens {
		t.Fatalf("tree has %d tokens, summary says %d", len(leaves), doc.Summary.Tokens)
	}
	refs := 0
	for name, node := range leaves {
		value, isString := node["$value"].(string)
		if !isString || !strings.HasPrefix(value, "{") {
			continue
		}
		refs++
		target := strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}")
		if _, exists := leaves[target]; !exists {
			t.Fatalf("token %s references %s, which is not in the document", name, target)
		}
	}
	if refs != 2 {
		t.Fatalf("expected the two semantic aliases to be references, got %d", refs)
	}
}

func TestSummary(t *testing.T) {
	doc := build(t, BuildOptions{})
	s := doc.Summary
	if s.Tokens != 12 || s.Variables != 8 || s.Styles != 4 {
		t.Fatalf("summary counts: %+v", s)
	}
	if s.ByType[TypeColor] != 6 || s.ByType[TypeTypography] != 1 || s.ByType[TypeShadow] != 1 {
		t.Fatalf("byType: %v", s.ByType)
	}
	want := []CollectionSummary{
		{Name: "Primitives", Tokens: 6, Modes: []string{"Default"}},
		{Name: "Semantic", Tokens: 2, Modes: []string{"Light", "Dark"}},
		{Name: StyleCollection, Tokens: 4},
	}
	if !reflect.DeepEqual(s.Collections, want) {
		t.Fatalf("collections = %+v, want %+v", s.Collections, want)
	}
}

func TestFilters(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true, ExcludeHidden: true})
	for _, name := range doc.Names(0) {
		if name == "feature.show-helper" {
			t.Fatal("--exclude-hidden should drop hiddenFromPublishing variables")
		}
	}
	doc = build(t, BuildOptions{Styles: true})
	if doc.Summary.Variables != 0 || doc.Summary.Styles != 4 {
		t.Fatalf("styles only: %+v", doc.Summary)
	}
	doc = build(t, BuildOptions{Variables: true, Collections: []string{"Primitives"}})
	if doc.Summary.Tokens != 6 {
		t.Fatalf("collection filter: %+v", doc.Summary)
	}
}

func TestCSS(t *testing.T) {
	doc := build(t, BuildOptions{})
	outs, err := doc.CSS(CSSOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].Name != "tokens.css" {
		t.Fatalf("outputs: %+v", outs)
	}
	css := string(outs[0].Content)
	for _, want := range []string{
		"/* Design tokens from Fixture Design System (version 2100123456), generated by figctl. Do not edit by hand. */",
		":root {",
		// The WEB code syntax wins over the derived name.
		"--color-brand-500: #3366ff;",
		"--space-4: 16px;",
		"--font-sans: Inter;",
		// Aliases become var() references.
		"--bg-surface: var(--neutral-0);",
		"--text-primary: var(--neutral-900);",
		// Styles use the kebab path, and typography expands.
		"--shadow-md: 0px 4px 12px rgba(0, 0, 0, 0.1);",
		"--heading-lg-font-family: Inter;",
		"--heading-lg-font-size: 28px;",
		"--heading-lg-font-weight: 700;",
		"--heading-lg-line-height: 1.21;",
		"--heading-lg-letter-spacing: -0.5px;",
		"--grid-12-columns: 12;",
		// The mode block.
		"[data-theme=\"dark\"] {",
		"  --bg-surface: #111827;",
		"  --text-primary: #ffffff;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("css is missing %q:\n%s", want, css)
		}
	}
	// The Light mode filled :root, so it gets no block of its own.
	if strings.Contains(css, `[data-theme="light"]`) {
		t.Errorf("the default mode should not get a block:\n%s", css)
	}
	// The style color/brand/500 and the variable brand/500 share a
	// property name and a value, so it is written once.
	if n := strings.Count(css, "--color-brand-500:"); n != 1 {
		t.Errorf("--color-brand-500 written %d times", n)
	}
	if len(outs[0].Warnings) != 0 {
		t.Errorf("identical duplicates should not warn: %v", outs[0].Warnings)
	}
}

func TestCSSModeSelector(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true})
	outs, err := doc.CSS(CSSOptions{ModeSelector: ".theme-<mode>"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outs[0].Content), ".theme-dark {") {
		t.Fatalf("custom selector:\n%s", outs[0].Content)
	}
}

func TestCSSDuplicateWarning(t *testing.T) {
	r := syntheticVariables(t, []synthetic{
		{id: "VariableID:9:1", name: "one", typ: "COLOR", web: "--shared", value: figma.VariableValue{Color: &figma.Color{R: 1, A: 1}}},
		{id: "VariableID:9:2", name: "two", typ: "COLOR", web: "--shared", value: figma.VariableValue{Color: &figma.Color{B: 1, A: 1}}},
	})
	doc, err := Build(r, BuildOptions{Variables: true})
	if err != nil {
		t.Fatal(err)
	}
	outs, err := doc.CSS(CSSOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outs[0].Warnings) != 1 || !strings.Contains(outs[0].Warnings[0], "--shared") {
		t.Fatalf("warnings: %v", outs[0].Warnings)
	}
	if n := strings.Count(string(outs[0].Content), "--shared:"); n != 1 {
		t.Fatalf("--shared written %d times", n)
	}
}

func TestTailwindESM(t *testing.T) {
	doc := build(t, BuildOptions{})
	outs, err := doc.Tailwind(TailwindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if outs[0].Name != "tokens.js" {
		t.Fatalf("name = %s", outs[0].Name)
	}
	js := string(outs[0].Content)
	for _, want := range []string{
		"const theme = {",
		"export default theme;",
		`"colors": {`,
		`"500": "#3366ff"`,
		`"spacing": {`,
		`"4": "16px"`,
		`"boxShadow": {`,
		`"md": "0px 4px 12px rgba(0, 0, 0, 0.1)"`,
		"// Tailwind's theme has no key for these token types",
		`"extend": {`,
		`"typography": {`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("tailwind esm is missing %q:\n%s", want, js)
		}
	}
	if strings.Contains(js, "var(--") {
		t.Error("without --tailwind-vars the values should be literals")
	}
}

func TestTailwindCJSAndJSON(t *testing.T) {
	doc := build(t, BuildOptions{})
	outs, err := doc.Tailwind(TailwindOptions{Format: TailwindCJS})
	if err != nil {
		t.Fatal(err)
	}
	if outs[0].Name != "tokens.cjs" || !strings.Contains(string(outs[0].Content), "module.exports = theme;") {
		t.Fatalf("cjs:\n%s", outs[0].Content)
	}

	outs, err = doc.Tailwind(TailwindOptions{Format: TailwindJSON})
	if err != nil {
		t.Fatal(err)
	}
	if outs[0].Name != "tokens.json" {
		t.Fatalf("json name = %s", outs[0].Name)
	}
	body := decodeTree(t, outs[0].Content)
	colors := body["colors"].(map[string]any)["brand"].(map[string]any)
	if colors["500"] != "#3366ff" {
		t.Fatalf("colors: %v", colors)
	}
	if _, present := body["extend"]; !present {
		t.Fatalf("json theme should carry extend: %v", body)
	}

	if _, err := doc.Tailwind(TailwindOptions{Format: "umd"}); err == nil {
		t.Fatal("an unknown tailwind format should fail")
	}
}

func TestTailwindVars(t *testing.T) {
	doc := build(t, BuildOptions{})
	outs, err := doc.Tailwind(TailwindOptions{Format: TailwindJSON, Vars: true})
	if err != nil {
		t.Fatal(err)
	}
	body := decodeTree(t, outs[0].Content)
	if got := body["colors"].(map[string]any)["brand"].(map[string]any)["500"]; got != "var(--color-brand-500)" {
		t.Fatalf("value = %v", got)
	}
	typo := body["extend"].(map[string]any)["typography"].(map[string]any)["heading"].(map[string]any)["lg"].(map[string]any)
	if typo["font-size"] != "var(--heading-lg-font-size)" {
		t.Fatalf("typography: %v", typo)
	}
}

func TestJSONOutput(t *testing.T) {
	doc := build(t, BuildOptions{Variables: true, Strategy: StrategySeparate})
	outs, err := doc.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 || outs[0].Name != "tokens.dark.json" {
		t.Fatalf("outputs: %+v", outs)
	}
	var list []Token
	if err := json.Unmarshal(outs[0].Content, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != outs[0].Tokens {
		t.Fatalf("%d tokens in the file, manifest says %d", len(list), outs[0].Tokens)
	}
}

func TestEmptyDocument(t *testing.T) {
	doc, err := Build(resolve.New(resolve.Options{}), BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Empty() || doc.Summary.Tokens != 0 {
		t.Fatalf("empty build: %+v", doc)
	}
	outs, err := doc.DTCG()
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || strings.TrimSpace(string(outs[0].Content)) != "{}" {
		t.Fatalf("empty dtcg: %q", outs[0].Content)
	}
}

func decodeTree(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, raw)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// synthetic describes one variable of a hand built collection.
type synthetic struct {
	id     string
	name   string
	typ    string
	scopes []string
	web    string
	value  figma.VariableValue
}

// syntheticVariables builds a resolver over one collection with a single
// mode, for cases the fixture does not cover.
func syntheticVariables(t *testing.T, vars []synthetic) *resolve.Resolver {
	t.Helper()
	resp := &figma.GetLocalVariablesResponse{}
	resp.Meta.VariableCollections = map[string]figma.VariableCollection{
		"VariableCollectionId:9:0": {
			ID:            "VariableCollectionId:9:0",
			Name:          "Test",
			Modes:         []figma.VariableMode{{ModeID: "9:0", Name: "Default"}},
			DefaultModeID: "9:0",
		},
	}
	resp.Meta.Variables = map[string]figma.Variable{}
	for _, v := range vars {
		out := figma.Variable{
			ID:                   v.id,
			Name:                 v.name,
			VariableCollectionID: "VariableCollectionId:9:0",
			ResolvedType:         v.typ,
			Scopes:               v.scopes,
			ValuesByMode:         map[string]figma.VariableValue{"9:0": v.value},
		}
		if v.web != "" {
			out.CodeSyntax = figma.VariableCodeSyntax{Web: v.web}
		}
		resp.Meta.Variables[v.id] = out
	}
	return resolve.New(resolve.Options{Variables: resp})
}

// styleFixture is one hand built style plus the node holding its value.
type styleFixture struct {
	name      string
	styleType string
	node      *figma.Node
}

func buildStyles(t *testing.T, styles map[string]styleFixture) *Document {
	t.Helper()
	file := &figma.GetFileResponse{Styles: map[string]figma.Style{}}
	nodes := map[string]*figma.Node{}
	for id, s := range styles {
		file.Styles[id] = figma.Style{Name: s.name, StyleType: s.styleType}
		nodes[id] = s.node
	}
	doc, err := Build(resolve.New(resolve.Options{File: file, StyleNodes: nodes}), BuildOptions{Styles: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return doc
}
