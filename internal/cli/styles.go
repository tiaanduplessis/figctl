package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

const defaultStylesLimit = 100

// styleDetail is one style with its resolved value.
type styleDetail struct {
	ID          string              `json:"id"`
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	Remote      bool                `json:"remote,omitempty"`
	FileKey     string              `json:"fileKey,omitempty"`
	Value       *resolve.StyleValue `json:"value"`
	Summary     string              `json:"summary,omitempty"`
}

func styleDetailFrom(s *resolve.Style) *styleDetail {
	d := &styleDetail{ID: s.ID, Key: s.Key, Name: s.Name, Type: s.Type, Description: s.Description, Remote: s.Remote, Value: s.Value}
	d.Summary = styleSummary(s.Type, s.Value)
	return d
}

func (d styleDetail) Columns() []string { return []string{"field", "value"} }

func (d styleDetail) Rows() [][]string {
	kv := [][2]string{{"id", d.ID}, {"key", d.Key}, {"name", d.Name}, {"type", d.Type}, {"description", d.Description}, {"value", d.Summary}}
	if d.FileKey != "" {
		kv = append(kv, [2]string{"fileKey", d.FileKey})
	}
	rows := make([][]string, 0, len(kv))
	for _, pair := range kv {
		rows = append(rows, []string{pair[0], pair[1]})
	}
	return rows
}

type styleList []styleDetail

func (l styleList) Columns() []string {
	return []string{"id", "type", "name", "value", "description", "key"}
}

func (l styleList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, s := range l {
		rows = append(rows, []string{s.ID, s.Type, s.Name, s.Summary, s.Description, s.Key})
	}
	return rows
}

// styleSummary renders a resolved style value as one line.
func styleSummary(styleType string, v *resolve.StyleValue) string {
	if v == nil {
		return ""
	}
	switch styleType {
	case "FILL":
		parts := make([]string, 0, len(v.Fills))
		for _, f := range v.Fills {
			parts = append(parts, fillText(f))
		}
		return strings.Join(parts, ", ")
	case "TEXT":
		if v.Typography == nil {
			return ""
		}
		return fontSummary(v.Typography)
	case "EFFECT":
		parts := make([]string, 0, len(v.Effects))
		for _, e := range v.Effects {
			parts = append(parts, e.Property+": "+e.CSS+tokenSuffix(e.Token))
		}
		return strings.Join(parts, "; ")
	case "GRID":
		parts := make([]string, 0, len(v.Grids))
		for _, g := range v.Grids {
			s := g.Pattern
			if g.Count > 0 {
				s += " " + resolve.Num(g.Count, 0)
			}
			s += " " + g.Alignment + " gutter " + resolve.Px(g.Gutter) + " offset " + resolve.Px(g.Offset)
			parts = append(parts, strings.TrimSpace(strings.ReplaceAll(s, "  ", " ")))
		}
		return strings.Join(parts, "; ")
	}
	return ""
}

var stylesCmd = &cobra.Command{
	Use:   "styles",
	Short: "Styles with resolved values: fills, typography, effects, and grids",
}

var stylesListCmd = &cobra.Command{
	Use:   "list <ref> | --team ID",
	Short: "List styles with metadata and resolved values",
	Long: `List the styles of a file (the styles used by the document plus its
published styles) with their resolved values, read from the style nodes
in one batched request. With --team, list the team's published styles;
values are not available there because they live in the owning file.`,
	Example: `  figctl styles list KEY
  figctl styles list KEY --type TEXT -o table
  figctl styles list --team 555000111 --limit 50`,
	Args: cobra.MaximumNArgs(1),
}

var stylesGetCmd = &cobra.Command{
	Use:   "get <ref> --node ID | --key KEY",
	Short: "Show one style with its full resolved value",
	Example: `  figctl styles get KEY --node 5:2
  figctl styles get KEY --key 5f4e3d2c1b0a9f8e7d6c5b4a39281706f5e4d502`,
	Args: cobra.ExactArgs(1),
}

func validStyleType(t string) (string, error) {
	t = strings.ToUpper(strings.TrimSpace(t))
	switch t {
	case "", "FILL", "TEXT", "EFFECT", "GRID":
		return t, nil
	}
	return "", figctl.Newf(figctl.CodeUsage, "invalid --type %q", t).WithHint("Use FILL, TEXT, EFFECT, or GRID.")
}

func init() {
	var team, typ string
	f := stylesListCmd.Flags()
	f.StringVar(&team, "team", "", "list a team's published styles instead of a file's")
	f.StringVar(&typ, "type", "", "only this style type: FILL, TEXT, EFFECT, or GRID")
	stylesListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		typ, err = validStyleType(typ)
		if err != nil {
			return err
		}
		limit := ctx.Flags.Limit
		if limit <= 0 {
			limit = defaultStylesLimit
		}
		rctx, cancel := session.Context()
		defer cancel()
		if team != "" || len(args) == 0 {
			var teamArgs []string
			if team != "" {
				teamArgs = []string{team}
			}
			teamID, err := teamIDArg(session, teamArgs)
			if err != nil {
				return err
			}
			resp, err := session.Client.GetTeamStyles(rctx, teamID, figma.PageOptions{PageSize: limit, After: ctx.Flags.Cursor})
			if err != nil {
				return err
			}
			list := styleList{}
			for _, s := range resp.Meta.Styles {
				if typ != "" && s.StyleType != typ {
					continue
				}
				list = append(list, styleDetail{ID: s.NodeID, Key: s.Key, Name: s.Name, Type: s.StyleType, Description: s.Description, FileKey: s.FileKey})
			}
			env := ctx.Envelope(list)
			if resp.Meta.Cursor != nil && resp.Meta.Cursor.After != nil {
				next := strconv.Itoa(*resp.Meta.Cursor.After)
				env.NextCursor = &next
			}
			env.AddHint("Team styles carry metadata only; resolve values with figctl styles get <fileKey> --key <key> against the owning file.")
			return ctx.Printer.Print(env)
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		catalog, hint, err := session.StyleCatalog(rctx, r.FileKey)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(catalog))
		for id, s := range catalog {
			if typ == "" || s.StyleType == typ {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool {
			a, b := catalog[ids[i]], catalog[ids[j]]
			if a.StyleType != b.StyleType {
				return a.StyleType < b.StyleType
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return ids[i] < ids[j]
		})
		offset := 0
		if ctx.Flags.Cursor != "" {
			offset, err = strconv.Atoi(ctx.Flags.Cursor)
			if err != nil || offset < 0 {
				return figctl.Newf(figctl.CodeUsage, "invalid --cursor %q", ctx.Flags.Cursor).WithHint("Pass the nextCursor value from the previous page.")
			}
		}
		if offset > len(ids) {
			offset = len(ids)
		}
		end := offset + limit
		if end > len(ids) {
			end = len(ids)
		}
		page := ids[offset:end]
		styleNodes, err := session.StyleNodes(rctx, r.FileKey, page)
		if err != nil {
			return err
		}
		file, err := session.fileMetadata(rctx, r.FileKey, &figma.GetFileNodesResponse{})
		if err != nil {
			return err
		}
		vars, varsHint, err := session.Variables(rctx, r.FileKey)
		if err != nil {
			return err
		}
		resolver := resolve.New(resolve.Options{File: file, Variables: vars, StyleNodes: styleNodes})
		for id, s := range catalog {
			resolver.AddStyle(id, s)
		}
		list := styleList{}
		for _, id := range page {
			s, ok := resolver.Style(id)
			if !ok {
				continue
			}
			list = append(list, *styleDetailFrom(s))
		}
		env := ctx.Envelope(list)
		if end < len(ids) {
			next := strconv.Itoa(end)
			env.NextCursor = &next
		}
		if hint != "" {
			env.AddHint(hint)
		}
		if varsHint != "" {
			env.AddHint(varsHint)
		}
		if len(list) == 0 {
			env.AddHint("No styles found; the file may use variables only (figctl variables list " + r.FileKey + ").")
		}
		return ctx.Printer.Print(env)
	})

	var node, key string
	stylesGetCmd.Flags().StringVar(&node, "node", "", "style node id (from styles list, or the styles map of a node)")
	stylesGetCmd.Flags().StringVar(&key, "key", "", "style key (from a library listing)")
	stylesGetCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		if node == "" && key == "" {
			return figctl.New(figctl.CodeUsage, "--node or --key is required").
				WithHint("Find style ids with figctl styles list <ref>.")
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		fileKey := r.FileKey
		catalog, hint, err := session.StyleCatalog(rctx, fileKey)
		if err != nil {
			return err
		}
		id := ""
		if node != "" {
			id, err = parseNodeArg(node)
			if err != nil {
				return err
			}
			if _, ok := catalog[id]; !ok {
				return figctl.Newf(figctl.CodeNotFound, "style node %s not found in file %s", id, fileKey).
					WithHint("List styles with figctl styles list %s.", fileKey)
			}
		} else {
			for sid, s := range catalog {
				if s.Key == key {
					id = sid
					break
				}
			}
			if id == "" {
				resp, err := session.Client.GetStyle(rctx, key)
				if err != nil {
					return err
				}
				meta := resp.Meta
				if meta.FileKey != "" && meta.FileKey != fileKey {
					fileKey = meta.FileKey
					catalog, hint, err = session.StyleCatalog(rctx, fileKey)
					if err != nil {
						return err
					}
				}
				id = meta.NodeID
				catalog[id] = figma.Style{Key: meta.Key, Name: meta.Name, Description: meta.Description, StyleType: meta.StyleType}
			}
		}
		styleNodes, err := session.StyleNodes(rctx, fileKey, []string{id})
		if err != nil {
			return err
		}
		file, err := session.fileMetadata(rctx, fileKey, &figma.GetFileNodesResponse{})
		if err != nil {
			return err
		}
		vars, varsHint, err := session.Variables(rctx, fileKey)
		if err != nil {
			return err
		}
		resolver := resolve.New(resolve.Options{File: file, Variables: vars, StyleNodes: styleNodes})
		for sid, s := range catalog {
			resolver.AddStyle(sid, s)
		}
		style, ok := resolver.Style(id)
		if !ok {
			return figctl.Newf(figctl.CodeNotFound, "style %s not found", id)
		}
		detail := styleDetailFrom(style)
		if fileKey != r.FileKey {
			detail.FileKey = fileKey
		}
		env := ctx.Envelope(detail)
		if style.Value == nil {
			env.AddHint(fmt.Sprintf("The style node %s could not be fetched, so no value is shown.", id))
		}
		if hint != "" {
			env.AddHint(hint)
		}
		if varsHint != "" {
			env.AddHint(varsHint)
		}
		return ctx.Printer.Print(env)
	})

	stylesCmd.AddCommand(stylesListCmd, stylesGetCmd)
	rootCmd.AddCommand(stylesCmd)
}
