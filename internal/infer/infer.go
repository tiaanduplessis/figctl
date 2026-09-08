// Package infer reconstructs what it can of a file's variables from how they
// are used, for accounts that cannot read the variables endpoint.
//
// Reading variables needs an Enterprise plan, but the bindings do not: every
// node reports which variable governs each of its properties with ordinary
// file content access, and the value that variable resolved to sits on the
// same node. Walking a file therefore recovers the shape of the design system
// even when its names are out of reach: which values are governed, which
// places share one, and what each one resolves to.
//
// What cannot be recovered is the name the designer gave a variable and the
// collection it belongs to. Names here are derived from the value, and are
// labelled as such, because a plausible invented name is worse than an honest
// identifier: it reads as authoritative and cannot be checked.
package infer

import (
	"sort"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// Category groups a variable by what it is used for, taken from the property
// it is bound to rather than guessed from its value.
const (
	CategoryColor      = "color"
	CategoryFontFamily = "fontFamily"
	CategoryFontSize   = "fontSize"
	CategoryLineHeight = "lineHeight"
	CategorySpacing    = "spacing"
	CategoryRadius     = "radius"
	CategoryStroke     = "strokeWidth"
	CategoryOpacity    = "opacity"
	CategoryText       = "text"
	CategoryOther      = "other"
)

// propertyCategory maps a bound property onto what the variable is for. A
// property figctl does not know still yields a usable record, just without a
// category to name it by.
var propertyCategory = map[string]string{
	"fills":              CategoryColor,
	"strokes":            CategoryColor,
	"color":              CategoryColor,
	"backgroundColor":    CategoryColor,
	"fontFamily":         CategoryFontFamily,
	"fontSize":           CategoryFontSize,
	"fontStyle":          CategoryText,
	"fontWeight":         CategoryText,
	"lineHeight":         CategoryLineHeight,
	"letterSpacing":      CategoryText,
	"paragraphSpacing":   CategorySpacing,
	"paragraphIndent":    CategorySpacing,
	"itemSpacing":        CategorySpacing,
	"counterAxisSpacing": CategorySpacing,
	"paddingLeft":        CategorySpacing,
	"paddingRight":       CategorySpacing,
	"paddingTop":         CategorySpacing,
	"paddingBottom":      CategorySpacing,
	"topLeftRadius":      CategoryRadius,
	"topRightRadius":     CategoryRadius,
	"bottomLeftRadius":   CategoryRadius,
	"bottomRightRadius":  CategoryRadius,
	"cornerRadius":       CategoryRadius,
	"strokeWeight":       CategoryStroke,
	"opacity":            CategoryOpacity,
	"characters":         CategoryText,
	"visible":            CategoryOther,
	"width":              CategorySpacing,
	"height":             CategorySpacing,
	"minWidth":           CategorySpacing,
	"maxWidth":           CategorySpacing,
	"minHeight":          CategorySpacing,
	"maxHeight":          CategorySpacing,
}

// Observation is one value a variable was seen to resolve to, and how often.
type Observation struct {
	Value string `json:"value"`
	Count int    `json:"count"`
	// Nodes samples where the value was seen, for checking a surprising one.
	Nodes []string `json:"nodes,omitempty"`
}

// Variable is what one variable's usage reveals about it.
type Variable struct {
	ID string `json:"variableId"`
	// Name is derived from the category and the most common value. It is not
	// the designer's name, which needs an Enterprise plan to read.
	Name string `json:"name"`
	// NameSource always says the name was derived, so nothing downstream
	// mistakes it for the real one.
	NameSource string        `json:"nameSource"`
	Category   string        `json:"category"`
	Properties []string      `json:"properties"`
	Values     []Observation `json:"values"`
	Usages     int           `json:"usages"`
	// LikelyModes marks a variable seen resolving to more than one value.
	// A variable with one value per theme is what light and dark look like
	// from the outside, though a component variant can look the same.
	LikelyModes bool `json:"likelyModes"`
}

// Inventory is every variable a file was seen to use.
type Inventory struct {
	Variables []Variable `json:"variables"`
	// Sites counts the bindings walked, which is how much evidence backs
	// the inventory.
	Sites int `json:"sites"`
	// Nodes counts the nodes inspected.
	Nodes int `json:"nodes"`
}

// Options controls the walk.
type Options struct {
	// IncludeHidden visits nodes the designer turned off.
	IncludeHidden bool
	// MaxSamples caps the node ids kept per observed value.
	MaxSamples int
	// MinUsages drops variables seen fewer times than this. A binding seen
	// once is real but is weak evidence, and a large file has many.
	MinUsages int
}

const defaultSamples = 3

type collector struct {
	opts  Options
	seen  map[string]*Variable
	order []string
	sites int
	nodes int
}

// Infer walks a document and reports what the variable bindings reveal.
func Infer(root *figma.Node, opts Options) *Inventory {
	if root == nil {
		return &Inventory{Variables: []Variable{}}
	}
	if opts.MaxSamples <= 0 {
		opts.MaxSamples = defaultSamples
	}
	c := &collector{opts: opts, seen: map[string]*Variable{}}
	root.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
		if !opts.IncludeHidden && !n.IsVisible() {
			return false
		}
		c.nodes++
		c.node(n)
		return true
	})
	return c.inventory()
}

// node records every binding on one node.
func (c *collector) node(n *figma.Node) {
	for property, binding := range n.BoundVariables {
		for _, alias := range binding.All() {
			c.record(alias.ID, property, nodeValue(n, property), n.ID)
		}
	}
	if n.Style != nil {
		for property, binding := range n.Style.BoundVariables {
			for _, alias := range binding.All() {
				c.record(alias.ID, property, typeStyleValue(n.Style, property), n.ID)
			}
		}
	}
	c.paints(n, n.Fills)
	c.paints(n, n.Strokes)
	for _, e := range n.Effects {
		for property, binding := range e.BoundVariables {
			for _, alias := range binding.All() {
				c.record(alias.ID, property, colorValue(e.Color), n.ID)
			}
		}
	}
}

// paints records bindings on a paint and on its gradient stops, where a
// colour variable is most often attached.
func (c *collector) paints(n *figma.Node, paints []figma.Paint) {
	for _, p := range paints {
		for property, binding := range p.BoundVariables {
			for _, alias := range binding.All() {
				c.record(alias.ID, property, colorValue(p.Color), n.ID)
			}
		}
		for _, stop := range p.GradientStops {
			for property, binding := range stop.BoundVariables {
				for _, alias := range binding.All() {
					c.record(alias.ID, property, colorValue(&stop.Color), n.ID)
				}
			}
		}
	}
}

func (c *collector) record(id, property, value, nodeID string) {
	if id == "" {
		return
	}
	c.sites++
	v, ok := c.seen[id]
	if !ok {
		v = &Variable{ID: id, NameSource: "derived from the observed value"}
		c.seen[id] = v
		c.order = append(c.order, id)
	}
	v.Usages++
	if !contains(v.Properties, property) {
		v.Properties = append(v.Properties, property)
	}
	if cat, ok := propertyCategory[property]; ok && (v.Category == "" || v.Category == CategoryOther) {
		v.Category = cat
	} else if v.Category == "" {
		v.Category = CategoryOther
	}
	if value == "" {
		return
	}
	for i := range v.Values {
		if v.Values[i].Value == value {
			v.Values[i].Count++
			if len(v.Values[i].Nodes) < c.opts.MaxSamples && !contains(v.Values[i].Nodes, nodeID) {
				v.Values[i].Nodes = append(v.Values[i].Nodes, nodeID)
			}
			return
		}
	}
	v.Values = append(v.Values, Observation{Value: value, Count: 1, Nodes: []string{nodeID}})
}

func (c *collector) inventory() *Inventory {
	out := &Inventory{Variables: make([]Variable, 0, len(c.order)), Sites: c.sites, Nodes: c.nodes}
	names := map[string]bool{}
	for _, id := range c.order {
		v := c.seen[id]
		if c.opts.MinUsages > 0 && v.Usages < c.opts.MinUsages {
			continue
		}
		sort.Strings(v.Properties)
		// Most used value first: it is the one a reader should see, and the
		// one the derived name is built from.
		sort.SliceStable(v.Values, func(i, j int) bool { return v.Values[i].Count > v.Values[j].Count })
		v.LikelyModes = len(v.Values) > 1
		v.Name = uniqueName(deriveName(*v), v.ID, names)
		out.Variables = append(out.Variables, *v)
	}
	sort.SliceStable(out.Variables, func(i, j int) bool {
		a, b := out.Variables[i], out.Variables[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		// Most used first inside a category: usage is the evidence, and a
		// variable used throughout the file is the one worth mapping first.
		if a.Usages != b.Usages {
			return a.Usages > b.Usages
		}
		return a.Name < b.Name
	})
	return out
}

// deriveName builds a name from what is actually known: the category and the
// value most often seen. It is deliberately descriptive rather than
// invented, so nobody mistakes it for the designer's own name.
func deriveName(v Variable) string {
	category := v.Category
	if category == "" {
		category = CategoryOther
	}
	value := ""
	if len(v.Values) > 0 {
		value = v.Values[0].Value
	}
	slug := slugify(value)
	if slug == "" {
		// Nothing was observed, so fall back to the id, which at least
		// stays stable between runs.
		slug = idSuffix(v.ID)
	}
	return slugify(category) + "/" + slug
}

// uniqueName keeps two variables that resolve to the same value apart. They
// are genuinely different variables however alike they look, and a counter
// would depend on the order the document happened to be walked, so the
// variable's own id is used: it is stable between runs and points at the
// thing it distinguishes.
func uniqueName(name, id string, used map[string]bool) string {
	if !used[name] {
		used[name] = true
		return name
	}
	qualified := name + "-" + idSuffix(id)
	used[qualified] = true
	return qualified
}

// idSuffix is the short, stable part of a variable id. A variable subscribed
// from another library carries the library key as well, which is forty
// characters of hash that identify the library rather than the variable, so
// only the trailing local id is kept.
func idSuffix(id string) string {
	id = strings.TrimPrefix(id, "VariableID:")
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	return slugify(id)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// nodeValue reads the value a node-level binding resolved to.
func nodeValue(n *figma.Node, property string) string {
	switch property {
	case "itemSpacing":
		return pxOf(n.ItemSpacing)
	case "counterAxisSpacing":
		return pxOf(n.CounterAxisSpacing)
	case "paddingLeft":
		return pxOf(n.PaddingLeft)
	case "paddingRight":
		return pxOf(n.PaddingRight)
	case "paddingTop":
		return pxOf(n.PaddingTop)
	case "paddingBottom":
		return pxOf(n.PaddingBottom)
	case "cornerRadius", "topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius":
		return pxOf(n.CornerRadius)
	case "strokeWeight":
		return pxOf(n.StrokeWeight)
	case "opacity":
		return numOf(n.Opacity)
	case "characters":
		return n.Characters
	case "fills":
		return firstPaintColor(n.Fills)
	case "strokes":
		return firstPaintColor(n.Strokes)
	}
	if n.Style != nil {
		return typeStyleValue(n.Style, property)
	}
	return ""
}

// typeStyleValue reads the value a typography binding resolved to.
func typeStyleValue(ts *figma.TypeStyle, property string) string {
	if ts == nil {
		return ""
	}
	switch property {
	case "fontFamily":
		return ts.FontFamily
	case "fontSize":
		return pxOf(ts.FontSize)
	case "fontWeight":
		return numOf(ts.FontWeight)
	case "fontStyle":
		return ts.FontStyle
	case "lineHeight":
		return pxOf(ts.LineHeightPx)
	case "letterSpacing":
		return pxOf(ts.LetterSpacing)
	case "paragraphSpacing":
		return pxOf(ts.ParagraphSpacing)
	case "paragraphIndent":
		return pxOf(ts.ParagraphIndent)
	}
	return ""
}

func firstPaintColor(paints []figma.Paint) string {
	for _, p := range paints {
		if !p.IsVisible() {
			continue
		}
		if c := colorValue(p.Color); c != "" {
			return c
		}
	}
	return ""
}

func colorValue(c *figma.Color) string {
	if c == nil {
		return ""
	}
	return resolve.FromFigma(*c).Hex()
}

func pxOf(v *float64) string {
	if v == nil {
		return ""
	}
	return resolve.Px(*v)
}

func numOf(v *float64) string {
	if v == nil {
		return ""
	}
	return resolve.Num(*v, 2)
}
