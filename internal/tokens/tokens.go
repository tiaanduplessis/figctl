// Package tokens converts the variables and styles of a Figma file into
// design token documents. Build turns a resolved file into a Document,
// and the Document renders itself as DTCG 2025.10 JSON (which Style
// Dictionary v4 consumes unchanged), CSS custom properties, a Tailwind
// theme module, or figctl's own flat JSON listing.
//
// Figma concepts that the Design Tokens Format Module cannot express are
// handled explicitly rather than silently dropped; every such decision is
// documented on the function that makes it and recorded in the token's
// $extensions under the com.figma key.
package tokens

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// Token kinds.
const (
	// KindVariable is a token built from a Figma variable.
	KindVariable = "variable"
	// KindStyle is a token built from a Figma style.
	KindStyle = "style"
)

// DTCG $type values emitted by the exporter.
const (
	TypeColor      = "color"
	TypeDimension  = "dimension"
	TypeNumber     = "number"
	TypeFontFamily = "fontFamily"
	TypeString     = "string"
	TypeBoolean    = "boolean"
	TypeTypography = "typography"
	TypeShadow     = "shadow"
	TypeGradient   = "gradient"
)

// Mode strategies.
const (
	// StrategyDefault emits the default mode of every collection and
	// records the other modes under $extensions.com.figma.modes.
	StrategyDefault = "default"
	// StrategySeparate emits one document per mode name.
	StrategySeparate = "separate"
	// StrategySelect emits one document using the modes named by
	// BuildOptions.Modes.
	StrategySelect = "select"
)

// Strategies lists the accepted --mode-strategy values.
var Strategies = []string{StrategyDefault, StrategySeparate, StrategySelect}

// StyleCollection is the collection name reported for style tokens,
// which do not belong to a variable collection.
const StyleCollection = "Styles"

// dimensionScopes are the Figma variable scopes that make a FLOAT a
// length rather than a plain number, so it is emitted as the DTCG
// dimension type in px.
var dimensionScopes = map[string]bool{
	"WIDTH_HEIGHT":      true,
	"GAP":               true,
	"CORNER_RADIUS":     true,
	"STROKE_FLOAT":      true,
	"PARAGRAPH_SPACING": true,
	"PARAGRAPH_INDENT":  true,
}

// Dimension is the DTCG 2025.10 dimension value: a number plus a unit.
type Dimension struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// Shadow is the DTCG shadow value. Inset marks a Figma INNER_SHADOW.
type Shadow struct {
	Color   string    `json:"color"`
	OffsetX Dimension `json:"offsetX"`
	OffsetY Dimension `json:"offsetY"`
	Blur    Dimension `json:"blur"`
	Spread  Dimension `json:"spread"`
	Inset   bool      `json:"inset,omitempty"`
}

// Typography is the DTCG typography composite value.
type Typography struct {
	FontFamily    string     `json:"fontFamily,omitempty"`
	FontSize      *Dimension `json:"fontSize,omitempty"`
	FontWeight    *float64   `json:"fontWeight,omitempty"`
	LetterSpacing *Dimension `json:"letterSpacing,omitempty"`
	LineHeight    *float64   `json:"lineHeight,omitempty"`
}

// GradientStop is one stop of the DTCG gradient value.
type GradientStop struct {
	Color    string  `json:"color"`
	Position float64 `json:"position"`
}

// Part is one scalar component of a composite token. CSS and Tailwind
// have no composite form, so they expand a typography or grid token into
// one entry per part.
type Part struct {
	Suffix string `json:"suffix"`
	Value  string `json:"value"`
}

// Token is one exported design token.
type Token struct {
	// Name is the dotted path, which is also the DTCG reference target.
	Name string `json:"name"`
	// Path is the normalized group path; the last segment names the token.
	Path []string `json:"path"`
	// Source is the Figma name the path was derived from.
	Source string `json:"source"`
	// Type is the DTCG $type.
	Type string `json:"type"`
	// Value is the DTCG $value of the selected mode.
	Value any `json:"value"`
	// Ref is the dotted name of the token this one aliases, when that
	// token is part of the same document. Empty when the alias was
	// inlined.
	Ref string `json:"ref,omitempty"`
	// RefCSSName is the custom property name of the Ref target.
	RefCSSName string `json:"refCssName,omitempty"`
	// CSS is the value as a CSS declaration value.
	CSS string `json:"css,omitempty"`
	// Parts expands a composite token for CSS and Tailwind.
	Parts []Part `json:"parts,omitempty"`

	Description string `json:"description,omitempty"`
	// Kind is variable or style.
	Kind string `json:"kind"`
	// Collection is the variable collection, or Styles.
	Collection string `json:"collection"`
	// ID is the variable id or the style node id.
	ID string `json:"id"`
	// Mode is the mode name the value came from, empty for styles and
	// for collections with a single unnamed mode.
	Mode string `json:"mode,omitempty"`
	// Modes holds the value in every mode of the variable's collection,
	// keyed by mode name. Only set when the collection has more than one
	// mode.
	Modes map[string]any `json:"modes,omitempty"`
	// CSSName is the custom property name without the leading "--".
	CSSName string `json:"cssName,omitempty"`
	// Category is the Tailwind theme key, empty when the token lands
	// under extend.
	Category   string            `json:"category,omitempty"`
	Scopes     []string          `json:"scopes,omitempty"`
	CodeSyntax map[string]string `json:"codeSyntax,omitempty"`
	// Extensions is the body written under $extensions["com.figma"].
	Extensions map[string]any `json:"extensions,omitempty"`

	// aliasTarget is the variable id this token aliases in the selected
	// mode, before the target is known to be part of the document.
	aliasTarget string
	// chain is the alias chain of variable names for the selected mode.
	chain []string
}

// Set is one flat group of tokens: the single set of the default and
// select strategies, or one set per mode for the separate strategy.
type Set struct {
	// Mode is the mode name of a separate strategy set, empty otherwise.
	Mode   string  `json:"mode,omitempty"`
	Tokens []Token `json:"tokens"`
}

// CollectionSummary counts the tokens of one collection.
type CollectionSummary struct {
	Name   string   `json:"name"`
	Tokens int      `json:"tokens"`
	Modes  []string `json:"modes,omitempty"`
}

// Summary counts the exported tokens by type and by collection.
type Summary struct {
	Tokens      int                 `json:"tokens"`
	Variables   int                 `json:"variables"`
	Styles      int                 `json:"styles"`
	ByType      map[string]int      `json:"byType"`
	Collections []CollectionSummary `json:"collections"`
}

// Document is the result of a build: the token sets plus the metadata
// the serializers need for their headers.
type Document struct {
	File     string  `json:"file,omitempty"`
	Version  string  `json:"version,omitempty"`
	Strategy string  `json:"strategy"`
	Sets     []Set   `json:"sets"`
	Summary  Summary `json:"summary"`
	// Warnings records values that could not be exported.
	Warnings []string `json:"warnings,omitempty"`
}

// BuildOptions configures Build.
type BuildOptions struct {
	// File and Version name the source file in generated headers.
	File    string
	Version string
	// NameCase is one of the Case constants; empty means CaseKebab.
	NameCase string
	// Strategy is one of the Strategy constants; empty means
	// StrategyDefault.
	Strategy string
	// Modes selects one mode name per collection for StrategySelect.
	Modes []string
	// Variables and Styles select the sources; both default to true when
	// neither is set.
	Variables bool
	Styles    bool
	// Collections filters variables by collection name (case
	// insensitive); empty exports every collection.
	Collections []string
	// ExcludeHidden drops variables marked hiddenFromPublishing.
	ExcludeHidden bool
	// ExcludeRemote drops variables and styles from remote libraries.
	ExcludeRemote bool
}

// Build converts the variables and styles of a resolved file into a
// token document. It fails only on a name collision; values it cannot
// convert are skipped and reported in Document.Warnings.
func Build(r *resolve.Resolver, opts BuildOptions) (*Document, error) {
	if opts.NameCase == "" {
		opts.NameCase = CaseKebab
	}
	if opts.Strategy == "" {
		opts.Strategy = StrategyDefault
	}
	if !opts.Variables && !opts.Styles {
		opts.Variables, opts.Styles = true, true
	}
	b := &builder{r: r, opts: opts}
	if opts.Variables && r != nil {
		b.variables = b.selectVariables()
	}
	if opts.Styles && r != nil {
		b.styles = b.selectStyles()
	}
	b.indexNames()
	doc := &Document{File: opts.File, Version: opts.Version, Strategy: opts.Strategy}
	for _, plan := range b.plans() {
		set, err := b.build(plan)
		if err != nil {
			return nil, err
		}
		doc.Sets = append(doc.Sets, set)
	}
	doc.Warnings = b.warnings
	doc.Summary = summarize(doc.Sets, b.collectionModes())
	return doc, nil
}

// builder holds the state shared by every set of one build.
type builder struct {
	r         *resolve.Resolver
	opts      BuildOptions
	variables []resolve.Variable
	styles    []resolve.Style
	// byName maps a variable name to its ids, used to turn an alias
	// chain (which carries names) back into a variable.
	byName   map[string][]string
	warnings []string
}

func (b *builder) warn(format string, args ...any) {
	b.warnings = append(b.warnings, fmt.Sprintf(format, args...))
}

func (b *builder) indexNames() {
	b.byName = map[string][]string{}
	for _, v := range b.variables {
		b.byName[v.Name] = append(b.byName[v.Name], v.ID)
	}
}

// selectVariables applies the collection, hidden, and remote filters.
func (b *builder) selectVariables() []resolve.Variable {
	var out []resolve.Variable
	for _, v := range b.r.Variables() {
		if b.opts.ExcludeHidden && v.HiddenFromPublishing {
			continue
		}
		if b.opts.ExcludeRemote && v.Remote {
			continue
		}
		if !b.wantCollection(v.Collection, v.CollectionID) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (b *builder) wantCollection(name, id string) bool {
	if len(b.opts.Collections) == 0 {
		return true
	}
	for _, want := range b.opts.Collections {
		if strings.EqualFold(want, name) || want == id {
			return true
		}
	}
	return false
}

// selectStyles keeps the styles whose value could be read. Styles have
// no collection, so --collection only filters them when it names the
// Styles pseudo collection.
func (b *builder) selectStyles() []resolve.Style {
	if len(b.opts.Collections) > 0 && !b.wantCollection(StyleCollection, "") {
		return nil
	}
	var out []resolve.Style
	for _, s := range b.r.Styles() {
		if b.opts.ExcludeRemote && s.Remote {
			continue
		}
		if s.Value == nil {
			b.warn("style %s (%s) has no value; its style node was not fetched", s.Name, s.ID)
			continue
		}
		out = append(out, s)
	}
	return out
}

// plan is one set to build: the mode chosen per collection.
type plan struct {
	// name is the set name, empty for a single set.
	name string
	// modes maps a collection id to the mode id to resolve in.
	modes map[string]string
	// modeNames maps a collection id to the chosen mode name.
	modeNames map[string]string
}

// collections lists the collections that own the selected variables, in
// a stable order.
func (b *builder) collections() []resolve.Collection {
	seen := map[string]bool{}
	var out []resolve.Collection
	for _, v := range b.variables {
		if seen[v.CollectionID] {
			continue
		}
		seen[v.CollectionID] = true
		if c, ok := b.r.Collection(v.CollectionID); ok {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (b *builder) collectionModes() map[string][]string {
	out := map[string][]string{}
	for _, c := range b.collections() {
		names := make([]string, 0, len(c.Modes))
		for _, m := range c.Modes {
			names = append(names, m.Name)
		}
		out[c.Name] = names
	}
	return out
}

// plans expands the mode strategy into the sets to build. The separate
// strategy emits one set per distinct mode name of the collections that
// have more than one mode; a single mode collection contributes its
// default values to every set.
func (b *builder) plans() []plan {
	switch b.opts.Strategy {
	case StrategySeparate:
		var names []string
		seen := map[string]bool{}
		for _, c := range b.collections() {
			if len(c.Modes) < 2 {
				continue
			}
			for _, m := range c.Modes {
				if !seen[m.Name] {
					seen[m.Name] = true
					names = append(names, m.Name)
				}
			}
		}
		if len(names) == 0 {
			return []plan{b.defaultPlan("")}
		}
		sort.Strings(names)
		out := make([]plan, 0, len(names))
		for _, name := range names {
			p := b.defaultPlan(name)
			for _, c := range b.collections() {
				for _, m := range c.Modes {
					if strings.EqualFold(m.Name, name) {
						p.modes[c.ID] = m.ID
						p.modeNames[c.ID] = m.Name
					}
				}
			}
			out = append(out, p)
		}
		return out
	case StrategySelect:
		p := b.defaultPlan("")
		for _, c := range b.collections() {
			for _, want := range b.opts.Modes {
				for _, m := range c.Modes {
					if strings.EqualFold(m.Name, want) {
						p.modes[c.ID] = m.ID
						p.modeNames[c.ID] = m.Name
					}
				}
			}
		}
		return []plan{p}
	default:
		return []plan{b.defaultPlan("")}
	}
}

// defaultPlan resolves every collection in its default mode.
func (b *builder) defaultPlan(name string) plan {
	p := plan{name: name, modes: map[string]string{}, modeNames: map[string]string{}}
	for _, c := range b.collections() {
		p.modes[c.ID] = c.DefaultModeID
		for _, m := range c.Modes {
			if m.ID == c.DefaultModeID {
				p.modeNames[c.ID] = m.Name
			}
		}
	}
	return p
}

// ModeNames lists the distinct mode names of the collections owning the
// selected variables, so a caller can validate a --mode value.
func ModeNames(r *resolve.Resolver) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range r.Modes() {
		for _, m := range c.Modes {
			if !seen[m.Name] {
				seen[m.Name] = true
				out = append(out, m.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// build creates one set: every variable in its planned mode, then every
// style, then the alias references between them.
func (b *builder) build(p plan) (Set, error) {
	set := Set{Mode: p.name}
	for _, v := range b.variables {
		t := b.variableToken(v, p)
		if t == nil {
			continue
		}
		set.Tokens = append(set.Tokens, *t)
	}
	for _, s := range b.styles {
		t := b.styleToken(s)
		if t == nil {
			continue
		}
		set.Tokens = append(set.Tokens, *t)
	}
	var err error
	if set.Tokens, err = b.resolveCollisions(set.Tokens); err != nil {
		return Set{}, err
	}
	b.link(set.Tokens)
	return set, nil
}

// link turns recorded alias targets into DTCG references when the target
// is part of the same set, and notes the inlined ones in $extensions.
func (b *builder) link(tokens []Token) {
	byVariable := map[string]Token{}
	for _, t := range tokens {
		if t.Kind == KindVariable {
			byVariable[t.ID] = t
		}
	}
	for i := range tokens {
		t := &tokens[i]
		if len(t.chain) < 2 {
			continue
		}
		if target, ok := byVariable[t.aliasTarget]; ok && t.aliasTarget != "" && target.Name != t.Name {
			t.Ref, t.RefCSSName = target.Name, target.CSSName
			continue
		}
		b.extend(t, "alias", strings.Join(t.chain, " -> "))
		b.extend(t, "aliasInlined", true)
	}
}

func (b *builder) extend(t *Token, key string, value any) {
	if t.Extensions == nil {
		t.Extensions = map[string]any{}
	}
	t.Extensions[key] = value
}

// variableToken converts one variable in one mode.
func (b *builder) variableToken(v resolve.Variable, p plan) *Token {
	modeID := p.modes[v.CollectionID]
	value, err := b.r.Value(v.ID, modeID)
	if err != nil {
		b.warn("variable %s could not be resolved: %v", v.Name, err)
		return nil
	}
	typ, dtcg := variableValue(v, value)
	if typ == "" {
		b.warn("variable %s has no value to export", v.Name)
		return nil
	}
	path := NormalizePath(v.Name, b.opts.NameCase)
	t := &Token{
		Name:        strings.Join(path, "."),
		Path:        path,
		Source:      v.Name,
		Type:        typ,
		Value:       dtcg,
		CSS:         cssText(typ, dtcg),
		Description: v.Description,
		Kind:        KindVariable,
		Collection:  v.Collection,
		ID:          v.ID,
		Mode:        p.modeNames[v.CollectionID],
		CSSName:     variableCSSName(v, path),
		Scopes:      exportScopes(v.Scopes),
		CodeSyntax:  v.CodeSyntax,
		chain:       value.Chain,
	}
	t.Category = category(t)
	if len(value.Chain) > 1 {
		if ids := b.byName[value.Chain[1]]; len(ids) == 1 {
			t.aliasTarget = ids[0]
		} else if len(ids) > 1 {
			b.warn("variable %s aliases %q, which names more than one variable; the value was inlined", v.Name, value.Chain[1])
		}
	}
	b.extend(t, "variableId", v.ID)
	b.extend(t, "collection", v.Collection)
	if len(v.Modes) > 1 {
		t.Modes = b.modeValues(v)
	}
	return t
}

// modeValues resolves a variable in every mode of its own collection.
func (b *builder) modeValues(v resolve.Variable) map[string]any {
	out := map[string]any{}
	for _, m := range v.Modes {
		value, err := b.r.Value(v.ID, m.ID)
		if err != nil {
			continue
		}
		if _, dtcg := variableValue(v, value); dtcg != nil {
			out[m.Name] = dtcg
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// variableValue maps a Figma resolved type and scopes onto a DTCG type
// and value. COLOR becomes a hex string, which Style Dictionary v4 and
// every CSS consumer accept; the 2025.10 color object form is not used
// because it carries no more information for sRGB Figma colors. FLOAT
// becomes a dimension in px when the scopes say the number is a length,
// and a plain number otherwise. STRING becomes fontFamily when scoped to
// font families, and string otherwise. BOOLEAN has no DTCG type, so it
// is emitted as the non standard boolean type rather than dropped.
func variableValue(v resolve.Variable, value resolve.Value) (string, any) {
	switch {
	case value.Color != nil:
		return TypeColor, colorHex(*value.Color)
	case value.Number != nil:
		n := resolve.Round(*value.Number, 4)
		if hasScope(v.Scopes, dimensionScopes) {
			return TypeDimension, Dimension{Value: n, Unit: "px"}
		}
		return TypeNumber, n
	case value.String != nil:
		if scoped(v.Scopes, "FONT_FAMILY") {
			return TypeFontFamily, *value.String
		}
		return TypeString, *value.String
	case value.Bool != nil:
		return TypeBoolean, *value.Bool
	}
	return "", nil
}

func hasScope(scopes []string, want map[string]bool) bool {
	for _, s := range scopes {
		if want[s] {
			return true
		}
	}
	return false
}

func scoped(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// exportScopes drops the ALL_SCOPES placeholder, which says nothing
// about how the variable is used.
func exportScopes(scopes []string) []string {
	var out []string
	for _, s := range scopes {
		if s == "ALL_SCOPES" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// variableCSSName prefers the WEB code syntax, which is exactly what
// Figma's codeSyntax field is for, and falls back to the kebab path.
func variableCSSName(v resolve.Variable, path []string) string {
	if name := strings.TrimSpace(v.CodeSyntax["WEB"]); name != "" {
		name = strings.TrimPrefix(name, "var(")
		name = strings.TrimSuffix(name, ")")
		return strings.TrimPrefix(strings.TrimSpace(name), "--")
	}
	return cssName(path)
}

func colorHex(c resolve.Color) string {
	if h := c.Hex8(); h != "" {
		return h
	}
	return c.Hex()
}

// summarize counts tokens by type and collection across the first set,
// which holds the same tokens as every other set of the same build.
func summarize(sets []Set, modes map[string][]string) Summary {
	s := Summary{ByType: map[string]int{}, Collections: []CollectionSummary{}}
	if len(sets) == 0 {
		return s
	}
	counts := map[string]int{}
	var order []string
	for _, t := range sets[0].Tokens {
		s.Tokens++
		s.ByType[t.Type]++
		if t.Kind == KindStyle {
			s.Styles++
		} else {
			s.Variables++
		}
		if _, seen := counts[t.Collection]; !seen {
			order = append(order, t.Collection)
		}
		counts[t.Collection]++
	}
	for _, name := range order {
		s.Collections = append(s.Collections, CollectionSummary{Name: name, Tokens: counts[name], Modes: modes[name]})
	}
	return s
}

// resolveCollisions handles two sources landing on the same token path.
//
// Real design systems carry duplicates: a style copied between pages or
// pulled in from a second library shows up twice under one name. Refusing to
// export because of that makes the command unusable on the files that need it
// most, so a duplicate is only fatal when it would change a value.
//
// Identical duplicates collapse into one token. Duplicates that disagree keep
// the first and report the rest, because picking silently would hide a real
// inconsistency in the design system.
func (b *builder) resolveCollisions(tokens []Token) ([]Token, error) {
	seen := map[string]int{}
	out := make([]Token, 0, len(tokens))
	for _, t := range tokens {
		at, exists := seen[t.Name]
		if !exists {
			seen[t.Name] = len(out)
			out = append(out, t)
			continue
		}
		kept := out[at]
		if sameValue(kept, t) {
			// The same token declared twice. Nothing is lost by keeping one.
			continue
		}
		if kept.Source == t.Source {
			// Identical Figma names, so no renaming or case option can
			// separate them; only the design system can.
			b.warn("%q is defined more than once with different values; keeping the first. Rename one of them in Figma so both can be exported", t.Source)
			continue
		}
		b.warn("%q and %q both become the token %q and their values differ; keeping %q. Use --name-case none to keep the Figma names, or --collection to export one collection at a time",
			kept.Source, t.Source, t.Name, kept.Source)
	}
	return out, nil
}

// sameValue reports whether two tokens carry the same value and type, which
// is what makes a duplicate safe to drop.
func sameValue(a, b Token) bool {
	if a.Type != b.Type {
		return false
	}
	x, err1 := json.Marshal(a.Value)
	y, err2 := json.Marshal(b.Value)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(x) == string(y)
}

// Preview is one entry of the short token listing table and markdown
// output show next to the summary.
type Preview struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Preview lists the first tokens of the first set with their types.
func (d *Document) Preview(limit int) []Preview {
	if len(d.Sets) == 0 {
		return nil
	}
	tokenList := d.Sets[0].Tokens
	if limit > 0 && len(tokenList) > limit {
		tokenList = tokenList[:limit]
	}
	out := make([]Preview, 0, len(tokenList))
	for _, t := range tokenList {
		out = append(out, Preview{Name: t.Name, Type: t.Type})
	}
	return out
}

// Names lists the token names of the first set, for a short preview.
func (d *Document) Names(limit int) []string {
	if len(d.Sets) == 0 {
		return nil
	}
	tokens := d.Sets[0].Tokens
	if limit > 0 && len(tokens) > limit {
		tokens = tokens[:limit]
	}
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, t.Name)
	}
	return out
}

// Empty reports whether the document holds no tokens at all.
func (d *Document) Empty() bool {
	for _, set := range d.Sets {
		if len(set.Tokens) > 0 {
			return false
		}
	}
	return true
}

// modeBlocks lists the mode names that need their own CSS block, in a
// stable order: every mode a token carries that did not produce its
// selected value.
func (d *Document) modeBlocks() []string {
	var out []string
	seen := map[string]bool{}
	for _, set := range d.Sets {
		for _, t := range set.Tokens {
			for name := range t.Modes {
				if name == t.Mode || seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// typographyParts expands a typography composite into the scalar parts
// CSS and Tailwind need.
func typographyParts(t Typography) []Part {
	var parts []Part
	if t.FontFamily != "" {
		parts = append(parts, Part{Suffix: "font-family", Value: quoteFamily(t.FontFamily)})
	}
	if t.FontSize != nil {
		parts = append(parts, Part{Suffix: "font-size", Value: resolve.Num(t.FontSize.Value, 4) + t.FontSize.Unit})
	}
	if t.FontWeight != nil {
		parts = append(parts, Part{Suffix: "font-weight", Value: resolve.Num(*t.FontWeight, 0)})
	}
	if t.LineHeight != nil {
		parts = append(parts, Part{Suffix: "line-height", Value: resolve.Num(*t.LineHeight, 4)})
	}
	if t.LetterSpacing != nil {
		parts = append(parts, Part{Suffix: "letter-spacing", Value: resolve.Num(t.LetterSpacing.Value, 4) + t.LetterSpacing.Unit})
	}
	return parts
}

// cssText renders a DTCG scalar value as a CSS declaration value.
func cssText(typ string, v any) string {
	switch x := v.(type) {
	case Dimension:
		return resolve.Num(x.Value, 4) + x.Unit
	case float64:
		return resolve.Num(x, 4)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		if typ == TypeFontFamily {
			return quoteFamily(x)
		}
		return x
	}
	return ""
}

// quoteFamily wraps a font family in quotes when CSS needs them.
func quoteFamily(name string) string {
	if strings.ContainsAny(name, " ,'\"") {
		return `"` + strings.ReplaceAll(name, `"`, `\"`) + `"`
	}
	return name
}

// typographyOf converts a resolved text style into the DTCG composite.
func typographyOf(t *model.Text) Typography {
	out := Typography{FontFamily: t.FontFamily}
	if t.Size != nil {
		out.FontSize = &Dimension{Value: resolve.Round(*t.Size, 4), Unit: "px"}
	}
	if t.Weight != nil {
		w := *t.Weight
		out.FontWeight = &w
	}
	if t.LetterSpacing != nil && t.LetterSpacing.Px != 0 {
		out.LetterSpacing = &Dimension{Value: resolve.Round(t.LetterSpacing.Px, 4), Unit: "px"}
	}
	if t.LineHeight != nil {
		switch {
		case t.LineHeight.Unitless != nil:
			u := *t.LineHeight.Unitless
			out.LineHeight = &u
		case t.LineHeight.Percent != nil:
			u := resolve.Round(*t.LineHeight.Percent/100, 4)
			out.LineHeight = &u
		}
	}
	return out
}
