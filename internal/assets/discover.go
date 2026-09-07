package assets

import (
	"path"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// DefaultIconPatterns are the name globs that mark a node as an icon.
// Matching is case insensitive.
var DefaultIconPatterns = []string{"icon/*", "Icon*", "icons/*"}

// vectorTypes are the node types a vector-only frame may contain.
var vectorTypes = map[string]bool{
	"VECTOR":            true,
	"BOOLEAN_OPERATION": true,
	"LINE":              true,
	"ELLIPSE":           true,
	"RECTANGLE":         true,
	"STAR":              true,
	"REGULAR_POLYGON":   true,
}

// iconTypes are the node types an icon pattern applies to.
var iconTypes = map[string]bool{
	"INSTANCE":  true,
	"COMPONENT": true,
	"FRAME":     true,
	"VECTOR":    true,
}

// iconContainerTypes are the node types the vector-only heuristic applies to.
// Icon libraries publish icons as components and place instances of them, so
// restricting the heuristic to frames misses most real icon sets. GROUP is
// deliberately absent: a group is an arbitrary selection rather than a
// deliberate asset, and grouped artwork would be misread as an icon.
var iconContainerTypes = map[string]bool{
	"FRAME":     true,
	"COMPONENT": true,
	"INSTANCE":  true,
}

// DiscoverOptions selects what Discover looks for.
type DiscoverOptions struct {
	// IconPatterns are name globs (path.Match syntax). Empty means
	// DefaultIconPatterns.
	IconPatterns []string
	// IncludeExportSettings collects nodes with export settings.
	IncludeExportSettings bool
	// IncludeIcons collects icon-like nodes.
	IncludeIcons bool
	// IncludeImageFills collects raster image fills.
	IncludeImageFills bool
	// IncludeHidden visits hidden nodes too.
	IncludeHidden bool
}

// Export is one export setting on one node.
type Export struct {
	NodeID string  `json:"nodeId"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Format string  `json:"format"`
	Scale  float64 `json:"scale"`
	Suffix string  `json:"suffix,omitempty"`
	// Constraint is the export constraint as Figma stores it, for example
	// "SCALE 2" or "WIDTH 48". Width and height constraints are converted
	// to a scale from the node's bounding box.
	Constraint string `json:"constraint"`
}

// Icon is an icon-like node.
type Icon struct {
	NodeID string  `json:"nodeId"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	// MatchedBy is "pattern" when the name matched an icon pattern, or
	// "vectors" when the frame contains only vector-like children.
	MatchedBy string `json:"matchedBy"`
}

// FillUse is one node using an image fill.
type FillUse struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
}

// ImageFill is one raster image and the nodes that use it.
type ImageFill struct {
	ImageRef  string    `json:"imageRef"`
	ScaleMode string    `json:"scaleMode,omitempty"`
	Nodes     []FillUse `json:"nodes"`
}

// Discovery is the categorized result of Discover.
type Discovery struct {
	Exports    []Export    `json:"exports"`
	Icons      []Icon      `json:"icons"`
	ImageFills []ImageFill `json:"imageFills"`
}

// ValidateIconPatterns rejects malformed globs.
func ValidateIconPatterns(patterns []string) error {
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return figctl.Newf(figctl.CodeUsage, "invalid --icon-pattern %q", p).
				WithHint("Patterns use glob syntax, for example icon/* or Icon*.")
		}
	}
	return nil
}

// Discover walks the given subtrees in document order and returns the
// export-marked nodes, icons, and image fills found. Hidden nodes and
// their subtrees are skipped unless IncludeHidden is set. Nodes inside an
// icon are not reported as icons themselves.
func Discover(roots []*figma.Node, opts DiscoverOptions) *Discovery {
	patterns := opts.IconPatterns
	if len(patterns) == 0 {
		patterns = DefaultIconPatterns
	}
	d := &Discovery{Exports: []Export{}, Icons: []Icon{}, ImageFills: []ImageFill{}}
	w := &walker{opts: opts, patterns: patterns, out: d, fills: map[string]int{}, fillUses: map[string]bool{}}
	for _, root := range roots {
		w.visit(root, false)
	}
	return d
}

type walker struct {
	opts     DiscoverOptions
	patterns []string
	out      *Discovery
	fills    map[string]int
	fillUses map[string]bool
}

func (w *walker) visit(n *figma.Node, insideIcon bool) {
	if n == nil {
		return
	}
	if !w.opts.IncludeHidden && !n.IsVisible() {
		return
	}
	if w.opts.IncludeExportSettings {
		for _, es := range n.ExportSettings {
			w.out.Exports = append(w.out.Exports, exportOf(n, es))
		}
	}
	if w.opts.IncludeImageFills {
		w.collectFills(n)
	}
	isIcon := false
	if w.opts.IncludeIcons && !insideIcon {
		if by := w.iconMatch(n); by != "" {
			isIcon = true
			icon := Icon{NodeID: n.ID, Name: n.Name, Type: n.Type, MatchedBy: by}
			if n.AbsoluteBoundingBox != nil {
				icon.Width = n.AbsoluteBoundingBox.Width
				icon.Height = n.AbsoluteBoundingBox.Height
			}
			w.out.Icons = append(w.out.Icons, icon)
		}
	}
	for _, child := range n.Children {
		w.visit(child, insideIcon || isIcon)
	}
}

func (w *walker) iconMatch(n *figma.Node) string {
	if iconTypes[n.Type] && matchesAny(w.patterns, n.Name) {
		return "pattern"
	}
	if iconContainerTypes[n.Type] && vectorOnly(n) {
		return "vectors"
	}
	return ""
}

func matchesAny(patterns []string, name string) bool {
	lower := strings.ToLower(name)
	for _, p := range patterns {
		if ok, _ := path.Match(strings.ToLower(p), lower); ok {
			return true
		}
	}
	return false
}

// vectorOnly reports whether a container has children and every child is a
// vector-like shape without an image fill.
func vectorOnly(n *figma.Node) bool {
	if len(n.Children) == 0 {
		return false
	}
	for _, c := range n.Children {
		if !vectorTypes[c.Type] || hasImageFill(c) {
			return false
		}
	}
	return true
}

func hasImageFill(n *figma.Node) bool {
	for _, p := range n.Fills {
		if p.Type == "IMAGE" && p.ImageRef != "" {
			return true
		}
	}
	return false
}

func (w *walker) collectFills(n *figma.Node) {
	for _, paints := range [][]figma.Paint{n.Fills, n.Strokes, n.Background} {
		for _, p := range paints {
			if p.Type != "IMAGE" || p.ImageRef == "" || !p.IsVisible() {
				continue
			}
			i, ok := w.fills[p.ImageRef]
			if !ok {
				i = len(w.out.ImageFills)
				w.fills[p.ImageRef] = i
				w.out.ImageFills = append(w.out.ImageFills, ImageFill{ImageRef: p.ImageRef, ScaleMode: p.ScaleMode, Nodes: []FillUse{}})
			}
			use := p.ImageRef + "/" + n.ID
			if !w.fillUses[use] {
				w.fillUses[use] = true
				w.out.ImageFills[i].Nodes = append(w.out.ImageFills[i].Nodes, FillUse{NodeID: n.ID, Name: n.Name})
			}
		}
	}
}

// exportOf converts an export setting into a render plan. Width and
// height constraints become a scale relative to the node's bounding box.
func exportOf(n *figma.Node, es figma.ExportSetting) Export {
	e := Export{
		NodeID: n.ID,
		Name:   n.Name,
		Type:   n.Type,
		Format: Extension(es.Format),
		Scale:  1,
		Suffix: es.Suffix,
	}
	value := es.Constraint.Value
	switch strings.ToUpper(es.Constraint.Type) {
	case "SCALE":
		if value > 0 {
			e.Scale = value
		}
		e.Constraint = "SCALE " + formatFloat(e.Scale)
	case "WIDTH":
		e.Constraint = "WIDTH " + formatFloat(value)
		if n.AbsoluteBoundingBox != nil && n.AbsoluteBoundingBox.Width > 0 && value > 0 {
			e.Scale = value / n.AbsoluteBoundingBox.Width
		}
	case "HEIGHT":
		e.Constraint = "HEIGHT " + formatFloat(value)
		if n.AbsoluteBoundingBox != nil && n.AbsoluteBoundingBox.Height > 0 && value > 0 {
			e.Scale = value / n.AbsoluteBoundingBox.Height
		}
	default:
		e.Constraint = "SCALE 1"
	}
	if e.Scale < 0.01 {
		e.Scale = 0.01
	}
	if e.Scale > 4 {
		e.Scale = 4
	}
	if e.Format == "svg" || e.Format == "pdf" {
		e.Scale = 1
	}
	return e
}
