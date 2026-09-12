package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

type commentRow struct {
	ID         string   `json:"id"`
	ParentID   string   `json:"parentId,omitempty"`
	User       string   `json:"user"`
	CreatedAt  string   `json:"createdAt"`
	ResolvedAt string   `json:"resolvedAt,omitempty"`
	NodeID     string   `json:"nodeId,omitempty"`
	X          *float64 `json:"x,omitempty"`
	Y          *float64 `json:"y,omitempty"`
	Message    string   `json:"message"`
	Reactions  int      `json:"reactions"`
}

type commentList []commentRow

func (l commentList) Columns() []string {
	return []string{"id", "parentId", "user", "createdAt", "resolved", "nodeId", "message"}
}

func (l commentList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, c := range l {
		resolved := ""
		if c.ResolvedAt != "" {
			resolved = "yes"
		}
		rows = append(rows, []string{c.ID, c.ParentID, c.User, c.CreatedAt, resolved, c.NodeID, truncate(c.Message, 80)})
	}
	return rows
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func commentRowFrom(c figma.Comment) commentRow {
	row := commentRow{ID: c.ID, ParentID: c.ParentID, User: c.User.Handle, CreatedAt: c.CreatedAt, Message: c.Message, Reactions: len(c.Reactions)}
	if c.ResolvedAt != nil {
		row.ResolvedAt = *c.ResolvedAt
	}
	if c.ClientMeta != nil {
		row.NodeID = c.ClientMeta.NodeID
		if c.ClientMeta.NodeOffset != nil {
			x, y := c.ClientMeta.NodeOffset.X, c.ClientMeta.NodeOffset.Y
			row.X, row.Y = &x, &y
		} else {
			row.X, row.Y = c.ClientMeta.X, c.ClientMeta.Y
		}
	}
	return row
}

var commentsCmd = &cobra.Command{
	Use:   "comments",
	Short: "Read designer comments and leave implementation notes",
}

var commentsListCmd = &cobra.Command{
	Use:   "list <ref>",
	Short: "List comments, optionally only those pinned on a node",
	Example: `  figctl comments list KEY
  figctl comments list KEY --node 1:2 --md`,
	Args: cobra.MaximumNArgs(1),
}

var commentsAddCmd = &cobra.Command{
	Use:   "add <ref> <message|->",
	Short: "Add a comment, pinned on a node or at canvas coordinates",
	Long: `Add a comment to a file. Pass - as the message to read it from stdin. With
--node the comment is pinned on that node at the offset given by --x and
--y; without --node, --x and --y are absolute canvas coordinates. This is
a write to Figma, so it asks for confirmation on a terminal
and requires --yes otherwise.`,
	Example: `  figctl comments add KEY "Implemented in src/Login.tsx" --node 1:2 --yes
  echo "Note" | figctl comments add KEY - --x 120 --y 320 --yes`,
	Args: cobra.ExactArgs(2),
}

func init() {
	var md bool
	var listNode string
	commentsListCmd.Flags().BoolVar(&md, "md", false, "return messages as markdown (as_md)")
	commentsListCmd.Flags().StringVar(&listNode, "node", "", "only comments pinned on this node id")
	commentsListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		filter := ""
		if listNode != "" {
			filter, err = parseNodeArg(listNode)
			if err != nil {
				return err
			}
		} else {
			filter = r.NodeID
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.GetComments(rctx, r.FileKey, md)
		if err != nil {
			return err
		}
		session.Touch(r.FileKey)
		list := commentList{}
		parents := map[string]bool{}
		for _, c := range resp.Comments {
			if filter == "" || (c.ClientMeta != nil && c.ClientMeta.NodeID == filter) {
				list = append(list, commentRowFrom(c))
				parents[c.ID] = true
			}
		}
		if filter != "" {
			for _, c := range resp.Comments {
				if c.ParentID != "" && parents[c.ParentID] && !parents[c.ID] {
					list = append(list, commentRowFrom(c))
				}
			}
		}
		env := ctx.Envelope(list)
		if filter != "" {
			env.AddHint("Showing comments pinned on " + filter + " and their replies; drop --node to see all.")
		}
		return ctx.Printer.Print(env)
	})

	var addNode string
	var x, y float64
	commentsAddCmd.Flags().StringVar(&addNode, "node", "", "pin the comment on this node id")
	commentsAddCmd.Flags().Float64Var(&x, "x", 0, "x offset inside the node, or canvas x without --node")
	commentsAddCmd.Flags().Float64Var(&y, "y", 0, "y offset inside the node, or canvas y without --node")
	commentsAddCmd.RunE = run(func(ctx *Context, cmd *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		message := args[1]
		if message == "-" {
			raw, err := io.ReadAll(ctx.Stdin)
			if err != nil {
				return figctl.Wrap(figctl.CodeInternal, err, "reading message from stdin: "+err.Error())
			}
			message = strings.TrimSpace(string(raw))
		}
		if message == "" {
			return figctl.New(figctl.CodeUsage, "empty comment message").WithHint("Pass the message as the second argument, or - to read it from stdin.")
		}
		nodeID := r.NodeID
		if addNode != "" {
			nodeID, err = parseNodeArg(addNode)
			if err != nil {
				return err
			}
		}
		body := figma.PostCommentRequest{Message: message}
		hasXY := cmd.Flags().Changed("x") || cmd.Flags().Changed("y")
		switch {
		case nodeID != "":
			body.ClientMeta = &figma.CommentPin{NodeID: nodeID, NodeOffset: &figma.Vector{X: x, Y: y}}
		case hasXY:
			body.ClientMeta = &figma.CommentPin{X: &x, Y: &y}
		}
		where := "the canvas"
		if nodeID != "" {
			where = "node " + nodeID
		}
		if err := confirm(ctx, fmt.Sprintf("Post a comment on %s of file %s?", where, r.FileKey)); err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		posted, err := session.Client.PostComment(rctx, r.FileKey, body)
		if err != nil {
			return err
		}
		session.Touch(r.FileKey)
		return ctx.Print(commentRowFrom(*posted))
	})

	commentsCmd.AddCommand(commentsListCmd, commentsAddCmd)
	rootCmd.AddCommand(commentsCmd)
}
