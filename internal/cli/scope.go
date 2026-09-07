package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// defaultListLimit is the page size of locally paginated node listings.
const defaultListLimit = 200

// docScope is the part of a document a discovery command walks: the start
// node, its known ancestors, and the component maps needed to name
// instances. partial is set when the file was too large to fetch whole and
// only part of the document is available.
type docScope struct {
	root          *figma.Node
	ancestors     []*figma.Node
	components    map[string]figma.Component
	componentSets map[string]figma.ComponentSet
	partial       bool
	hint          string
}

// loadScope resolves the start node of a discovery command from the cached
// whole document. When the document is too large it falls back to one GET
// nodes call for --node, or to the depth 2 overview otherwise. depth bounds
// the fallback fetch; 0 fetches the whole subtree.
func (s *Session) loadScope(ctx context.Context, key, nodeID, page string, depth int) (*docScope, error) {
	file, err := s.File(ctx, key)
	if err == nil {
		return scopeFromDocument(file.Document, file.Components, file.ComponentSets, key, nodeID, page)
	}
	if !errors.Is(err, errTooLarge) {
		return nil, err
	}
	if nodeID != "" {
		opts := figma.NodesOptions{}
		if depth > 0 {
			opts.Depth = depth + 1
		}
		nodes, err := s.Nodes(ctx, key, []string{nodeID}, opts)
		if err != nil {
			return nil, err
		}
		entry := nodes.Nodes[nodeID]
		if entry == nil || entry.Document == nil {
			return nil, nodeNotFound(nodeID, key)
		}
		sc := &docScope{
			root:          entry.Document,
			components:    entry.Components,
			componentSets: entry.ComponentSets,
			partial:       true,
			hint:          "File " + key + " is above --max-file-mb, so only the subtree of " + nodeID + " was fetched.",
		}
		if page != "" {
			sc.hint += " --page was not checked."
		}
		return sc, nil
	}
	overview, err := s.Overview(ctx, key)
	if err != nil {
		return nil, err
	}
	sc, err := scopeFromDocument(overview.Document, overview.Components, overview.ComponentSets, key, "", page)
	if err != nil {
		return nil, err
	}
	sc.partial = true
	sc.hint = "File " + key + " is above --max-file-mb; only pages and their top level nodes are available. Use --node <id> to work on one subtree."
	return sc, nil
}

func scopeFromDocument(doc *figma.Node, components map[string]figma.Component, sets map[string]figma.ComponentSet, key, nodeID, page string) (*docScope, error) {
	if doc == nil {
		return nil, figctl.Newf(figctl.CodeInternal, "file %s has no document", key)
	}
	sc := &docScope{root: doc, components: components, componentSets: sets}
	if page != "" {
		canvas := findPage(doc, page)
		if canvas == nil {
			return nil, figctl.Newf(figctl.CodeNotFound, "page %q not found in file %s", page, key).
				WithHint("%s", "Pages: "+strings.Join(pageNames(doc), ", ")+". Pass a page name or id.")
		}
		sc.root = canvas
	}
	if nodeID == "" {
		return sc, nil
	}
	chain := findChain(sc.root, nodeID)
	if chain == nil {
		if page != "" {
			return nil, figctl.Newf(figctl.CodeNotFound, "node %s is not on page %q of file %s", nodeID, page, key).
				WithHint("%s", "Drop --page, or find the node with figctl file find "+key+" --name '<name>'.")
		}
		return nil, nodeNotFound(nodeID, key)
	}
	for _, n := range chain[:len(chain)-1] {
		if n.Type != "DOCUMENT" {
			sc.ancestors = append(sc.ancestors, n)
		}
	}
	sc.root = chain[len(chain)-1]
	return sc, nil
}

func nodeNotFound(nodeID, key string) error {
	return figctl.Newf(figctl.CodeNotFound, "node %s not found in file %s", nodeID, key).
		WithHint("%s", "List nodes with figctl file tree "+key+" or search with figctl file find "+key+" --name '<glob>'.")
}

// findPage returns the canvas matching an id or a case-insensitive name.
func findPage(doc *figma.Node, page string) *figma.Node {
	for _, canvas := range doc.Children {
		if canvas.ID == page || strings.EqualFold(canvas.Name, page) {
			return canvas
		}
	}
	if id, err := parseNodeArg(page); err == nil {
		for _, canvas := range doc.Children {
			if canvas.ID == id {
				return canvas
			}
		}
	}
	return nil
}

func pageNames(doc *figma.Node) []string {
	names := make([]string, 0, len(doc.Children))
	for _, canvas := range doc.Children {
		names = append(names, canvas.Name+" ("+canvas.ID+")")
	}
	return names
}

// findChain returns the nodes from root down to the node with the given
// id, inclusive, or nil.
func findChain(root *figma.Node, id string) []*figma.Node {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return []*figma.Node{root}
	}
	for _, child := range root.Children {
		if chain := findChain(child, id); chain != nil {
			return append([]*figma.Node{root}, chain...)
		}
	}
	return nil
}

// pageOf returns the name of the page containing the scope root, when
// known.
func (sc *docScope) pageOf() string {
	if sc.root != nil && sc.root.Type == "CANVAS" {
		return sc.root.Name
	}
	for _, n := range sc.ancestors {
		if n.Type == "CANVAS" {
			return n.Name
		}
	}
	return ""
}

// ancestorNames returns the names of the known ancestors of the scope
// root, page first.
func (sc *docScope) ancestorNames() []string {
	names := make([]string, 0, len(sc.ancestors))
	for _, n := range sc.ancestors {
		names = append(names, n.Name)
	}
	return names
}

// instanceOf names the main component of an instance, prefixed with its
// component set when it belongs to one.
func (sc *docScope) instanceOf(n *figma.Node) string {
	if n.ComponentID == "" {
		return ""
	}
	c, ok := sc.components[n.ComponentID]
	if !ok {
		return n.ComponentID
	}
	if set, ok := sc.componentSets[c.ComponentSetID]; ok {
		return set.Name + " / " + c.Name
	}
	return c.Name
}

// autoLayout maps Figma's layout mode to row, column, wrap, or grid.
func autoLayout(n *figma.Node) string {
	switch n.LayoutMode {
	case "HORIZONTAL":
		if n.LayoutWrap == "WRAP" {
			return "wrap"
		}
		return "row"
	case "VERTICAL":
		return "column"
	case "GRID":
		return "grid"
	}
	return ""
}

// bounds returns the absolute bounding box rounded to integers, or nils
// when the node has none.
func bounds(n *figma.Node) (x, y, w, h *int) {
	box := n.AbsoluteBoundingBox
	if box == nil {
		return nil, nil, nil, nil
	}
	round := func(v float64) *int {
		i := int(math.Round(v))
		return &i
	}
	return round(box.X), round(box.Y), round(box.Width), round(box.Height)
}

func sizeString(w, h *int) string {
	if w == nil || h == nil {
		return ""
	}
	return strconv.Itoa(*w) + "x" + strconv.Itoa(*h)
}

// nodeFilter is the shared name, type, and text filter of file tree and
// file find.
type nodeFilter struct {
	types map[string]bool
	name  *regexp.Regexp
	text  string
}

func newNodeFilter(types, name, text string) nodeFilter {
	f := nodeFilter{text: strings.ToLower(text)}
	for _, t := range strings.Split(types, ",") {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" {
			if f.types == nil {
				f.types = map[string]bool{}
			}
			f.types[t] = true
		}
	}
	if name != "" {
		f.name = globRegexp(name)
	}
	return f
}

func (f nodeFilter) empty() bool {
	return f.types == nil && f.name == nil && f.text == ""
}

func (f nodeFilter) matches(n *figma.Node) bool {
	if f.types != nil && !f.types[strings.ToUpper(n.Type)] {
		return false
	}
	if f.name != nil && !f.name.MatchString(n.Name) {
		return false
	}
	if f.text != "" && (n.Type != "TEXT" || !strings.Contains(strings.ToLower(n.Characters), f.text)) {
		return false
	}
	return true
}

// globRegexp compiles a case-insensitive glob where * matches any run of
// characters (including /) and ? matches one character.
func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// clipRunes shortens s to at most n runes, marking the cut with an
// ellipsis, and folds newlines into spaces.
func clipRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
}

const cursorPrefix = "offset:"

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err == nil && strings.HasPrefix(string(raw), cursorPrefix) {
		if offset, err := strconv.Atoi(strings.TrimPrefix(string(raw), cursorPrefix)); err == nil && offset >= 0 {
			return offset, nil
		}
	}
	return 0, figctl.Newf(figctl.CodeUsage, "invalid --cursor %q", cursor).
		WithHint("Pass the nextCursor value from the previous page unchanged.")
}

// paginate returns one page of items for --limit and --cursor, with the
// cursor of the next page when more remain.
func paginate[T any](items []T, limit int, cursor string) ([]T, *string, error) {
	offset, err := decodeCursor(cursor)
	if err != nil {
		return nil, nil, err
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset >= len(items) {
		return []T{}, nil, nil
	}
	end := offset + limit
	if end >= len(items) {
		return items[offset:], nil, nil
	}
	next := encodeCursor(end)
	return items[offset:end], &next, nil
}
