package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// treeFlags are the sparse markers of a tree row. Only set flags are
// encoded.
type treeFlags struct {
	Component  bool   `json:"component,omitempty"`
	InstanceOf string `json:"instanceOf,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
	AutoLayout string `json:"autoLayout,omitempty"`
	HasExport  bool   `json:"hasExport,omitempty"`
	DevStatus  string `json:"devStatus,omitempty"`
}

func (f treeFlags) list() []string {
	var out []string
	if f.Component {
		out = append(out, "component")
	}
	if f.InstanceOf != "" {
		out = append(out, "instance of "+f.InstanceOf)
	}
	if f.Hidden {
		out = append(out, "hidden")
	}
	if f.AutoLayout != "" {
		out = append(out, "layout="+f.AutoLayout)
	}
	if f.HasExport {
		out = append(out, "export")
	}
	if f.DevStatus != "" {
		out = append(out, "dev="+f.DevStatus)
	}
	return out
}

type treeRow struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	X          *int      `json:"x,omitempty"`
	Y          *int      `json:"y,omitempty"`
	W          *int      `json:"w,omitempty"`
	H          *int      `json:"h,omitempty"`
	Depth      int       `json:"depth"`
	ChildCount int       `json:"childCount"`
	Flags      treeFlags `json:"flags"`
}

type treeRows []treeRow

func (rows treeRows) Columns() []string {
	return []string{"id", "type", "name", "size", "children", "flags"}
}

func (rows treeRows) Rows() [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{r.ID, r.Type, r.Name, sizeString(r.W, r.H), strconv.Itoa(r.ChildCount), strings.Join(r.Flags.list(), ", ")})
	}
	return out
}

// Markdown renders the rows as an indented list, one node per line.
func (rows treeRows) Markdown() string {
	if len(rows) == 0 {
		return "No nodes matched.\n"
	}
	minDepth := rows[0].Depth
	for _, r := range rows {
		if r.Depth < minDepth {
			minDepth = r.Depth
		}
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.Repeat("  ", r.Depth-minDepth))
		fmt.Fprintf(&b, "- %s %s %q", r.ID, r.Type, r.Name)
		if size := sizeString(r.W, r.H); size != "" {
			fmt.Fprintf(&b, " %s @%d,%d", size, *r.X, *r.Y)
		}
		if r.ChildCount > 0 {
			fmt.Fprintf(&b, " (%d children)", r.ChildCount)
		}
		if flags := r.Flags.list(); len(flags) > 0 {
			b.WriteString(" [" + strings.Join(flags, ", ") + "]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func treeRowFrom(sc *docScope, n *figma.Node, depth int) treeRow {
	row := treeRow{ID: n.ID, Type: n.Type, Name: n.Name, Depth: depth, ChildCount: len(n.Children)}
	row.X, row.Y, row.W, row.H = bounds(n)
	row.Flags = treeFlags{
		Component:  n.Type == "COMPONENT" || n.Type == "COMPONENT_SET",
		InstanceOf: sc.instanceOf(n),
		Hidden:     !n.IsVisible(),
		AutoLayout: autoLayout(n),
		HasExport:  len(n.ExportSettings) > 0,
	}
	if n.DevStatus != nil {
		row.Flags.DevStatus = n.DevStatus.Type
	}
	return row
}

// buildTreeRows walks the scope in document order to maxDepth levels below
// the start node (0 for no limit) and keeps the rows passing the filter.
// The DOCUMENT node itself is never a row.
func buildTreeRows(sc *docScope, maxDepth int, filter nodeFilter, visibleOnly bool) treeRows {
	rows := treeRows{}
	var visit func(n *figma.Node, depth int)
	visit = func(n *figma.Node, depth int) {
		if n.Type != "DOCUMENT" {
			if visibleOnly && !n.IsVisible() {
				return
			}
			if filter.matches(n) {
				rows = append(rows, treeRowFrom(sc, n, depth))
			}
		}
		if maxDepth > 0 && depth >= maxDepth {
			return
		}
		for _, child := range n.Children {
			visit(child, depth+1)
		}
	}
	visit(sc.root, 0)
	return rows
}

var fileTreeCmd = &cobra.Command{
	Use:   "tree <ref>",
	Short: "Sparse outline of pages, frames, and layers with ids, sizes, and flags",
	Long: `Print a sparse outline of the document: one row per node with its id, type,
name, position and size (from absoluteBoundingBox, rounded), depth, child
count, and flags (component, instance of a component, hidden, auto layout
mode, export settings, dev status). This is the cheapest way to find the
node ids to pass to figctl node context.

The walk starts at --node (or the node-id in the ref URL, or the page given
with --page) and goes --depth levels down, 2 by default, so a plain call
shows the pages and their top level frames. Rows are in document order and
paged 200 at a time; pass nextCursor back with --cursor. Filters keep
matching rows but still walk through their parents. The document is
fetched once per file version and cached.`,
	Example: `  figctl file tree KEY
  figctl file tree KEY --node 2:2 --depth 3 -o md
  figctl file tree KEY --type FRAME,SECTION --page Screens
  figctl file tree KEY --name "icon/*" --depth 0 --limit 50
  figctl file tree "https://www.figma.com/design/KEY/App?node-id=2-2" --visible-only`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var node, types, name, page string
	var depth int
	var visibleOnly bool
	fileTreeCmd.Flags().StringVar(&node, "node", "", "start node id (1:2, 1-2, or a URL with node-id); default is the document")
	fileTreeCmd.Flags().IntVar(&depth, "depth", 2, "levels below the start node to walk (0 for all)")
	fileTreeCmd.Flags().StringVar(&types, "type", "", "keep only these node types, comma separated (FRAME,SECTION,TEXT,...)")
	fileTreeCmd.Flags().StringVar(&name, "name", "", "keep only nodes whose name matches this glob (* and ?, case-insensitive)")
	fileTreeCmd.Flags().StringVar(&page, "page", "", "start at this page, by name or id")
	fileTreeCmd.Flags().BoolVar(&visibleOnly, "visible-only", false, "skip hidden nodes and their children")
	fileTreeCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if depth < 0 {
			return figctl.Newf(figctl.CodeUsage, "invalid --depth %d", depth).WithHint("Use a depth of 1 or more, or 0 for the whole subtree.")
		}
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		ids, err := parseNodeFlags(nonEmpty(node), r)
		if err != nil {
			return err
		}
		nodeID := ""
		if len(ids) > 0 {
			nodeID = ids[0]
		}
		rctx, cancel := session.Context()
		defer cancel()
		sc, err := session.loadScope(rctx, r.FileKey, nodeID, page, depth)
		if err != nil {
			return err
		}
		rows := buildTreeRows(sc, depth, newNodeFilter(types, name, ""), visibleOnly)
		pageRows, next, err := paginate(rows, ctx.Flags.Limit, ctx.Flags.Cursor)
		if err != nil {
			return err
		}
		env := ctx.Envelope(treeRows(pageRows))
		if next != nil {
			env.Truncated = true
			env.NextCursor = next
			env.AddHint(fmt.Sprintf("%d nodes matched. Narrow the outline with --node <id> for one subtree, --type FRAME,SECTION to keep containers, or a smaller --depth.", len(rows)))
		}
		if sc.partial {
			env.AddHint(sc.hint)
		}
		if len(rows) == 0 {
			env.AddHint("No nodes matched. Loosen --type or --name, raise --depth, or drop --visible-only.")
		} else {
			env.AddHint("Next: figctl file tree " + r.FileKey + " --node <id> --depth 3 to go deeper, or figctl node context " + r.FileKey + " --node <id> to implement a node.")
		}
		return ctx.Printer.Print(env)
	})
	fileCmd.AddCommand(fileTreeCmd)
}

// nonEmpty wraps a single optional flag value for parseNodeFlags.
func nonEmpty(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}
