package cli

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/tiaanduplessis/figctl/internal/cache"

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
	Short: "List a bounded page of comments or one discussion thread",
	Long: `Filter by node, thread id (a parent or reply), resolution state, and creation
date. Replies inherit the parent thread's resolution state. --since filters
individual comments by creation time. --limit defaults to 200; --cursor
continues the same query. Figma still returns the full upstream comment file,
cached for one minute independently of the design version. Use --refresh
for fresh collaboration data.`,
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
	var listNode, thread, resolved, since string
	commentsListCmd.Flags().BoolVar(&md, "md", false, "return messages as markdown (as_md)")
	commentsListCmd.Flags().StringVar(&listNode, "node", "", "only comments pinned on this node id")
	commentsListCmd.Flags().StringVar(&thread, "thread", "", "only this discussion, identified by a parent or reply comment id")
	commentsListCmd.Flags().StringVar(&resolved, "resolved", "any", "thread resolution state: any, true, or false")
	commentsListCmd.Flags().StringVar(&since, "since", "", "comments created on or after this RFC3339 timestamp or YYYY-MM-DD date")
	commentsListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if resolved != "any" && resolved != "true" && resolved != "false" {
			return usageErr("invalid --resolved value", "Use any, true, or false.")
		}
		var createdSince time.Time
		if since != "" {
			var err error
			createdSince, err = time.Parse(time.RFC3339, since)
			if err != nil {
				createdSince, err = time.Parse(time.DateOnly, since)
			}
			if err != nil {
				return usageErr("invalid --since date", "Use an RFC3339 timestamp or YYYY-MM-DD date (UTC).")
			}
		}
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
		resp, err := session.comments(rctx, r.FileKey, md)
		if err != nil {
			return err
		}
		session.Touch(r.FileKey)
		byID := map[string]figma.Comment{}
		for _, c := range resp.Comments {
			byID[c.ID] = c
		}
		rootOf := func(id string) figma.Comment {
			c := byID[id]
			for range len(byID) {
				parent, ok := byID[c.ParentID]
				if !ok || c.ParentID == "" {
					break
				}
				c = parent
			}
			return c
		}
		threadRoot := ""
		if thread != "" {
			if _, ok := byID[thread]; !ok {
				return figctl.Newf(figctl.CodeNotFound, "comment %s not found in file %s", thread, r.FileKey).WithHint("Check the comment id or use --refresh if it was just added.")
			}
			threadRoot = rootOf(thread).ID
		}
		list := commentList{}
		for _, c := range resp.Comments {
			root := rootOf(c.ID)
			if threadRoot != "" && root.ID != threadRoot {
				continue
			}
			if filter != "" && (root.ClientMeta == nil || root.ClientMeta.NodeID != filter) {
				continue
			}
			isResolved := root.ResolvedAt != nil && *root.ResolvedAt != ""
			if resolved != "any" && (resolved == "true") != isResolved {
				continue
			}
			if !createdSince.IsZero() {
				created, err := time.Parse(time.RFC3339, c.CreatedAt)
				if err != nil || created.Before(createdSince) {
					continue
				}
			}
			list = append(list, commentRowFrom(c))
		}
		page, next, err := paginate(list, ctx.Flags.Limit, ctx.Flags.Cursor)
		if err != nil {
			return err
		}
		env := ctx.Envelope(commentList(page))
		env.NextCursor, env.Truncated = next, next != nil
		env.AddHint("Comments are cached for up to one minute independently of the design version; use --refresh for current discussion state.")
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
		for _, form := range []string{"false", "true"} {
			if err := session.Cache.Invalidate(r.FileKey, cache.Key("comments", url.Values{"as_md": {form}})); err != nil {
				return err
			}
		}
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
