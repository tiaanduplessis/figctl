// Package inspect converts raw Figma nodes into the normalized model:
// layout in CSS terms, visual properties with resolved colors, typography
// with mixed runs, component information, tokens and style references,
// and an optional flat CSS declaration map.
package inspect

import (
	"strconv"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// Defaults for Options.
const (
	DefaultDepth    = 3
	DefaultMaxNodes = 500
)

// Options control the walk.
type Options struct {
	// Depth is how many levels of children to include below the node.
	// Zero uses DefaultDepth; negative includes the whole subtree.
	Depth int
	// CSS emits a flat map of CSS declarations per node.
	CSS bool
	// Interactions includes prototype interactions.
	Interactions bool
	// IncludeHidden keeps invisible nodes.
	IncludeHidden bool
	// MaxNodes caps the number of nodes emitted; zero uses
	// DefaultMaxNodes.
	MaxNodes int
	// Web adds the var(--name) form to tokens with WEB code syntax.
	Web bool
}

func (o Options) depth() int {
	if o.Depth == 0 {
		return DefaultDepth
	}
	return o.Depth
}

func (o Options) maxNodes() int {
	if o.MaxNodes <= 0 {
		return DefaultMaxNodes
	}
	return o.MaxNodes
}

// Inspect converts a node and its subtree. parent is the node's parent
// when known (it decides absolute positioning and grid placement) and
// file supplies component, component set, and style metadata plus the
// ancestor path. resolver must not be nil.
func Inspect(node, parent *figma.Node, resolver *resolve.Resolver, file *figma.GetFileResponse, opts Options) *model.Node {
	if node == nil {
		return nil
	}
	in := &inspector{r: resolver, file: file, opts: opts, budget: opts.maxNodes()}
	if file != nil && file.Document != nil {
		if parent == nil {
			parent = Parent(file.Document, node.ID)
		}
		in.path = Path(file.Document, node.ID)
	}
	in.budget--
	return in.node(node, parent, in.path, 0)
}

// Omitted returns the number of nodes left out of a model tree by the
// node budget.
func Omitted(n *model.Node) int {
	if n == nil {
		return 0
	}
	total := n.Omitted
	for _, c := range n.Children {
		total += Omitted(c)
	}
	return total
}

// Parent finds the parent of a node in a document tree.
func Parent(root *figma.Node, id string) *figma.Node {
	var found *figma.Node
	root.Walk(func(n *figma.Node, parent *figma.Node, _ int) bool {
		if found != nil {
			return false
		}
		if n.ID == id {
			found = parent
			return false
		}
		return true
	})
	return found
}

// Path returns the names of the ancestors of a node from the page down,
// excluding the document root and the node itself.
func Path(root *figma.Node, id string) []string {
	var path []string
	var walk func(n *figma.Node, ancestors []string) bool
	walk = func(n *figma.Node, ancestors []string) bool {
		if n.ID == id {
			path = ancestors
			return true
		}
		next := ancestors
		if n.Type != "DOCUMENT" {
			next = append(append([]string{}, ancestors...), n.Name)
		}
		for _, c := range n.Children {
			if walk(c, next) {
				return true
			}
		}
		return false
	}
	walk(root, nil)
	return path
}

// StyleIDs returns every style id referenced in a subtree, in document
// order without duplicates.
func StyleIDs(nodes ...*figma.Node) []string {
	seen := map[string]bool{}
	var ids []string
	for _, root := range nodes {
		if root == nil {
			continue
		}
		root.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
			for _, key := range []string{"fill", "fills", "stroke", "strokes", "text", "effect", "grid"} {
				if id, ok := n.Styles[key]; ok && id != "" && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			return true
		})
	}
	return ids
}

type inspector struct {
	r      *resolve.Resolver
	file   *figma.GetFileResponse
	opts   Options
	budget int
	path   []string
}

func (in *inspector) node(n, parent *figma.Node, path []string, level int) *model.Node {
	m := &model.Node{
		ID:      n.ID,
		Name:    n.Name,
		Type:    n.Type,
		Visible: n.IsVisible(),
		Locked:  n.Locked != nil && *n.Locked,
		Path:    path,
	}
	m.Box = box(n, parent)
	m.Layout = in.layout(n, parent)
	m.Visual = in.visual(n)
	if n.Type == "TEXT" {
		m.Text = in.text(n)
	}
	m.Component = in.component(n)
	if in.opts.Interactions {
		m.Prototype = prototype(n)
	}
	m.Tokens = tokenIndex(m)
	if in.opts.Web {
		applyWeb(m)
	}
	if in.opts.CSS {
		m.CSS = CSS(m, parentLayout(parent))
	}
	in.children(m, n, level)
	return m
}

func (in *inspector) children(m *model.Node, n *figma.Node, level int) {
	m.ChildCount = len(n.Children)
	if len(n.Children) == 0 {
		return
	}
	depth := in.opts.depth()
	if depth >= 0 && level >= depth {
		return
	}
	childPath := append(append([]string{}, m.Path...), n.Name)
	for i, c := range n.Children {
		if !in.opts.IncludeHidden && !c.IsVisible() {
			continue
		}
		if in.budget <= 0 {
			m.Truncated = true
			m.Omitted += in.remaining(n.Children[i:])
			return
		}
		in.budget--
		m.Children = append(m.Children, in.node(c, n, childPath, level+1))
	}
}

// remaining counts the nodes of the given subtrees that the walk would
// have emitted.
func (in *inspector) remaining(nodes []*figma.Node) int {
	count := 0
	for _, n := range nodes {
		if !in.opts.IncludeHidden && !n.IsVisible() {
			continue
		}
		n.Walk(func(c *figma.Node, _ *figma.Node, _ int) bool {
			if !in.opts.IncludeHidden && !c.IsVisible() {
				return false
			}
			count++
			return true
		})
	}
	return count
}

func box(n, parent *figma.Node) *model.Box {
	if n.AbsoluteBoundingBox == nil {
		return nil
	}
	b := n.AbsoluteBoundingBox
	m := &model.Box{X: b.X, Y: b.Y, W: b.Width, H: b.Height}
	if parent != nil && parent.AbsoluteBoundingBox != nil && parent.Type != "CANVAS" && parent.Type != "DOCUMENT" {
		m.Relative = &model.Point{X: resolve.Round(b.X-parent.AbsoluteBoundingBox.X, 2), Y: resolve.Round(b.Y-parent.AbsoluteBoundingBox.Y, 2)}
	}
	return m
}

func prototype(n *figma.Node) *model.Prototype {
	if len(n.Interactions) == 0 {
		return nil
	}
	p := &model.Prototype{Interactions: []model.Interaction{}}
	for _, it := range n.Interactions {
		trigger := ""
		if it.Trigger != nil {
			trigger = it.Trigger.Type
		}
		if len(it.Actions) == 0 {
			p.Interactions = append(p.Interactions, model.Interaction{Trigger: trigger})
			continue
		}
		for _, a := range it.Actions {
			m := model.Interaction{Trigger: trigger, Action: a.Type, Navigation: a.Navigation, URL: a.URL}
			if a.DestinationID != nil {
				m.DestinationID = *a.DestinationID
			}
			if a.Transition != nil {
				m.Transition = &model.Transition{Type: a.Transition.Type, Duration: a.Transition.Duration, Direction: a.Transition.Direction}
				if a.Transition.Easing != nil {
					m.Transition.Easing = a.Transition.Easing.Type
				}
			}
			p.Interactions = append(p.Interactions, m)
		}
	}
	return p
}

// tokenIndex builds the compact index of variable and style names used
// by a node, keyed by property.
func tokenIndex(m *model.Node) *model.Tokens {
	vars := map[string]string{}
	styles := map[string]string{}
	addVar := func(key string, t *model.TokenRef) {
		if t == nil {
			return
		}
		name := t.Name
		if name == "" {
			name = t.VariableID
		}
		vars[key] = name
	}
	addStyle := func(key string, s *model.StyleRef) {
		if s != nil {
			styles[key] = s.Name
		}
	}
	if v := m.Visual; v != nil {
		for i, f := range v.Fills {
			addVar("fills["+itoa(i)+"]", f.Token)
			if f.Gradient != nil {
				for j, s := range f.Gradient.Stops {
					addVar("fills["+itoa(i)+"].stops["+itoa(j)+"]", s.Token)
				}
			}
			if i == 0 {
				addStyle("fill", f.Style)
			}
		}
		for i, s := range v.Strokes {
			addVar("strokes["+itoa(i)+"]", s.Token)
			if i == 0 {
				addStyle("stroke", s.Style)
			}
		}
		if v.Radius != nil {
			addVar("radius", v.Radius.Token)
		}
		for i, e := range v.Effects {
			addVar("effects["+itoa(i)+"]", e.Token)
			if i == 0 {
				addStyle("effect", e.Style)
			}
		}
		for i, g := range v.LayoutGrids {
			if i == 0 {
				addStyle("grid", g.Style)
			}
		}
	}
	if l := m.Layout; l != nil {
		for key, t := range l.Tokens {
			addVar(key, t)
		}
	}
	if t := m.Text; t != nil {
		for key, tok := range t.Tokens {
			addVar(key, tok)
		}
		for i, run := range t.Runs {
			if run.Fill != nil {
				addVar("runs["+itoa(i)+"].fill", run.Fill.Token)
			}
		}
		addStyle("text", t.Style)
	}
	if c := m.Component; c != nil {
		for key, p := range c.Properties {
			addVar("properties."+key, p.Token)
		}
	}
	if len(vars) == 0 && len(styles) == 0 {
		return nil
	}
	out := &model.Tokens{}
	if len(vars) > 0 {
		out.Variables = vars
	}
	if len(styles) > 0 {
		out.Styles = styles
	}
	return out
}

// applyWeb fills TokenRef.Web on every token of a node.
func applyWeb(m *model.Node) {
	set := func(t *model.TokenRef) {
		if t != nil {
			t.Web = resolve.CSSVar(t)
		}
	}
	if v := m.Visual; v != nil {
		for i := range v.Fills {
			set(v.Fills[i].Token)
			if g := v.Fills[i].Gradient; g != nil {
				for j := range g.Stops {
					set(g.Stops[j].Token)
				}
			}
		}
		for i := range v.Strokes {
			set(v.Strokes[i].Token)
		}
		if v.Radius != nil {
			set(v.Radius.Token)
		}
		for i := range v.Effects {
			set(v.Effects[i].Token)
		}
	}
	if l := m.Layout; l != nil {
		for _, t := range l.Tokens {
			set(t)
		}
	}
	if t := m.Text; t != nil {
		for _, tok := range t.Tokens {
			set(tok)
		}
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
