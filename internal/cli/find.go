package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// findTextLimit bounds the characters echoed for a text hit.
const findTextLimit = 120

type findHit struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Page string `json:"page,omitempty"`
	Path string `json:"path"`
	W    *int   `json:"w,omitempty"`
	H    *int   `json:"h,omitempty"`
	Text string `json:"text,omitempty"`
}

type findHits []findHit

func (hits findHits) Columns() []string {
	return []string{"id", "type", "name", "page", "path", "size", "text"}
}

func (hits findHits) Rows() [][]string {
	out := make([][]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, []string{h.ID, h.Type, h.Name, h.Page, h.Path, sizeString(h.W, h.H), h.Text})
	}
	return out
}

// searchNodes walks the whole scope in document order and returns every
// node passing the filter with the names of its ancestors.
func searchNodes(sc *docScope, filter nodeFilter) findHits {
	hits := findHits{}
	var visit func(n *figma.Node, ancestors []string, page string)
	visit = func(n *figma.Node, ancestors []string, page string) {
		if n.Type == "CANVAS" {
			page = n.Name
		}
		if n.Type != "DOCUMENT" && filter.matches(n) {
			hit := findHit{ID: n.ID, Type: n.Type, Name: n.Name, Page: page, Path: strings.Join(ancestors, " / ")}
			_, _, hit.W, hit.H = bounds(n)
			if filter.text != "" {
				hit.Text = clipRunes(n.Characters, findTextLimit)
			}
			hits = append(hits, hit)
		}
		if len(n.Children) == 0 {
			return
		}
		childAncestors := ancestors
		if n.Type != "DOCUMENT" {
			childAncestors = append(ancestors[:len(ancestors):len(ancestors)], n.Name)
		}
		for _, child := range n.Children {
			visit(child, childAncestors, page)
		}
	}
	visit(sc.root, sc.ancestorNames(), sc.pageOf())
	return hits
}

var fileFindCmd = &cobra.Command{
	Use:   "find <ref>",
	Short: "Search nodes by name glob, type, or text content",
	Long: `Search every node of the document (or of the subtree given with --node or
--page) and return the matches with the path of ancestor names that leads
to them. At least one of --name, --type, or --text is required; when
several are given a node must match all of them.

--name is a case-insensitive glob (* and ?). --type is a comma separated
list of node types. --text is a case-insensitive substring searched in the
characters of TEXT nodes; matching text is echoed, cut at 120 characters.
Results are paged 200 at a time. The document is fetched once per file
version and cached, so searching is free after the first call.`,
	Example: `  figctl file find KEY --name "Login*"
  figctl file find KEY --type COMPONENT,COMPONENT_SET --page "Design System"
  figctl file find KEY --text "sign in"
  figctl file find KEY --name "icon/*" --type VECTOR --node 2:2`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var node, types, name, text, page string
	fileFindCmd.Flags().StringVar(&node, "node", "", "search only inside this node id (1:2, 1-2, or a URL with node-id)")
	fileFindCmd.Flags().StringVar(&types, "type", "", "node types to match, comma separated (FRAME,TEXT,INSTANCE,...)")
	fileFindCmd.Flags().StringVar(&name, "name", "", "name glob to match (* and ?, case-insensitive)")
	fileFindCmd.Flags().StringVar(&text, "text", "", "substring to find in TEXT node characters (case-insensitive)")
	fileFindCmd.Flags().StringVar(&page, "page", "", "search only this page, by name or id")
	fileFindCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		filter := newNodeFilter(types, name, text)
		if filter.empty() {
			return figctl.New(figctl.CodeUsage, "file find needs at least one of --name, --type, or --text").
				WithHint("Examples: --name \"Login*\", --type FRAME,COMPONENT, --text \"sign in\". Use figctl file tree for an outline instead.")
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
		sc, err := session.loadScope(rctx, r.FileKey, nodeID, page, 0)
		if err != nil {
			return err
		}
		hits := searchNodes(sc, filter)
		pageHits, next, err := paginate(hits, ctx.Flags.Limit, ctx.Flags.Cursor)
		if err != nil {
			return err
		}
		env := ctx.Envelope(findHits(pageHits))
		if next != nil {
			env.Truncated = true
			env.NextCursor = next
			env.AddHint(fmt.Sprintf("%d nodes matched. Narrow the search with --node <id>, --page <name>, a more specific --name, or --type.", len(hits)))
		}
		if sc.partial {
			env.AddHint(sc.hint)
		}
		if len(hits) == 0 {
			env.AddHint("No nodes matched. Globs are anchored: use --name \"*login*\" to match anywhere in a name.")
		} else {
			env.AddHint("Next: figctl node context " + r.FileKey + " --node <id> for layout, styles, tokens, and a screenshot of a hit.")
		}
		return ctx.Printer.Print(env)
	})
	fileCmd.AddCommand(fileFindCmd)
}
