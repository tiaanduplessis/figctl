package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/inspect"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Inspect nodes: resolved layout, styles, typography, tokens, and CSS",
}

var nodeInspectCmd = &cobra.Command{
	Use:   "inspect <ref> --node ID",
	Short: "Resolved layout, visual, text, tokens, and component info for nodes",
	Long: `Inspect nodes and their children (to --depth, default 3) as a normalized
model: layout in CSS terms (row, column, wrap, grid; gap, padding,
justify, align, sizing fixed/hug/fill), visual (fills as hex and rgba,
gradients as CSS, strokes, radius, effects as box-shadow and filter),
text (font, size, line height in px and unitless, letter spacing, case,
mixed runs), component info for instances, and every bound variable and
style as a token reference. Values without a token are reported with
"token": null so it is clear what is not tokenized.

The document is fetched once per file version and cached, so repeated
calls cost no extra Tier 1 requests. Variables need an Enterprise plan;
on other plans the command falls back to styles and raw values with a
hint. A node-id in the ref URL is used when no --node is given.`,
	Example: `  figctl node inspect KEY --node 2:2
  figctl node inspect "https://www.figma.com/design/KEY/App?node-id=2-2" --depth 1
  figctl node inspect KEY --node 2:3 --css --web
  figctl node inspect KEY --node 2:6 --interactions -o md`,
	Args: cobra.MaximumNArgs(1),
}

// inspectResult is the data of node inspect: one entry per requested node.
type inspectResult []*model.Node

func (l inspectResult) Columns() []string {
	return []string{"id", "type", "name", "layout", "fills", "text"}
}

func (l inspectResult) Rows() [][]string {
	var rows [][]string
	var walk func(n *model.Node, depth int)
	walk = func(n *model.Node, depth int) {
		rows = append(rows, []string{n.ID, n.Type, strings.Repeat("  ", depth) + n.Name, layoutSummary(n), fillsSummary(n), textSummary(n)})
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, n := range l {
		walk(n, 0)
	}
	return rows
}

// Markdown renders a compact section per node.
func (l inspectResult) Markdown() string {
	var b strings.Builder
	for _, n := range l {
		writeNodeMarkdown(&b, n, 2)
	}
	return b.String()
}

func writeNodeMarkdown(b *strings.Builder, n *model.Node, level int) {
	if level > 6 {
		level = 6
	}
	fmt.Fprintf(b, "%s %s %s (%s)\n\n", strings.Repeat("#", level), n.ID, n.Name, n.Type)
	if n.Box != nil {
		fmt.Fprintf(b, "- box: %s x %s at %s, %s", resolve.Num(n.Box.W, 2), resolve.Num(n.Box.H, 2), resolve.Num(n.Box.X, 2), resolve.Num(n.Box.Y, 2))
		if n.Box.Relative != nil {
			fmt.Fprintf(b, " (relative %s, %s)", resolve.Num(n.Box.Relative.X, 2), resolve.Num(n.Box.Relative.Y, 2))
		}
		b.WriteString("\n")
	}
	if s := layoutSummary(n); s != "" {
		b.WriteString("- layout: " + s + "\n")
	}
	if n.Visual != nil {
		if s := fillsSummary(n); s != "" {
			b.WriteString("- fills: " + s + "\n")
		}
		if len(n.Visual.Strokes) > 0 {
			parts := []string{}
			for _, st := range n.Visual.Strokes {
				parts = append(parts, fillText(st))
			}
			line := strings.Join(parts, ", ")
			if w := n.Visual.StrokeWeight; w != nil {
				if w.Uniform() {
					line += " " + resolve.Px(w.Top)
				} else {
					line += fmt.Sprintf(" %s %s %s %s", resolve.Px(w.Top), resolve.Px(w.Right), resolve.Px(w.Bottom), resolve.Px(w.Left))
				}
			}
			if n.Visual.StrokeAlign != "" {
				line += " " + n.Visual.StrokeAlign
			}
			if len(n.Visual.StrokeDashes) > 0 {
				line += " dashed"
			}
			b.WriteString("- strokes: " + line + "\n")
		}
		if r := n.Visual.Radius; r != nil {
			line := ""
			if r.Uniform != nil {
				line = resolve.Px(*r.Uniform)
			} else if r.Corners != nil {
				line = fmt.Sprintf("%s %s %s %s", resolve.Px(r.Corners[0]), resolve.Px(r.Corners[1]), resolve.Px(r.Corners[2]), resolve.Px(r.Corners[3]))
			}
			b.WriteString("- radius: " + line + tokenSuffix(r.Token) + "\n")
		}
		if len(n.Visual.Effects) > 0 {
			parts := []string{}
			for _, e := range n.Visual.Effects {
				parts = append(parts, e.Property+": "+e.CSS+tokenSuffix(e.Token)+styleSuffix(e.Style))
			}
			b.WriteString("- effects: " + strings.Join(parts, "; ") + "\n")
		}
		if n.Visual.Opacity != nil {
			b.WriteString("- opacity: " + resolve.Num(*n.Visual.Opacity, 4) + "\n")
		}
		if n.Visual.BlendMode != "" {
			b.WriteString("- blend: " + n.Visual.BlendMode + "\n")
		}
	}
	if t := n.Text; t != nil {
		fmt.Fprintf(b, "- text: %q\n", t.Characters)
		b.WriteString("- font: " + fontSummary(t) + styleSuffix(t.Style) + "\n")
		for _, run := range t.Runs {
			fmt.Fprintf(b, "- run %d-%d %q: %s\n", run.Start, run.End, run.Text, runSummary(run))
		}
	}
	if c := n.Component; c != nil {
		b.WriteString("- component: " + componentSummary(c) + "\n")
	}
	if p := n.Prototype; p != nil {
		for _, it := range p.Interactions {
			line := it.Trigger + " -> " + it.Action
			if it.DestinationID != "" {
				line += " " + it.DestinationID
			}
			if it.URL != "" {
				line += " " + it.URL
			}
			if it.Transition != nil {
				line += fmt.Sprintf(" (%s %ss)", it.Transition.Type, resolve.Num(it.Transition.Duration, 2))
			}
			b.WriteString("- interaction: " + line + "\n")
		}
	}
	if n.Tokens != nil {
		if len(n.Tokens.Variables) > 0 {
			b.WriteString("- tokens: " + mapSummary(n.Tokens.Variables) + "\n")
		}
		if len(n.Tokens.Styles) > 0 {
			b.WriteString("- styles: " + mapSummary(n.Tokens.Styles) + "\n")
		}
	}
	if len(n.CSS) > 0 {
		b.WriteString("\n```css\n")
		keys := make([]string, 0, len(n.CSS))
		for k := range n.CSS {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(k + ": " + n.CSS[k] + ";\n")
		}
		b.WriteString("```\n")
	}
	if n.Truncated {
		fmt.Fprintf(b, "- truncated: %d nodes omitted\n", n.Omitted)
	} else if n.ChildCount > 0 && len(n.Children) == 0 {
		fmt.Fprintf(b, "- children: %d (not expanded)\n", n.ChildCount)
	}
	b.WriteString("\n")
	for _, c := range n.Children {
		writeNodeMarkdown(b, c, level+1)
	}
}

func mapSummary(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ", ")
}

func tokenSuffix(t *model.TokenRef) string {
	if t == nil {
		return ""
	}
	if t.Name == "" {
		return " [" + t.VariableID + "]"
	}
	return " [" + t.Name + "]"
}

func styleSuffix(s *model.StyleRef) string {
	if s == nil || s.Name == "" {
		return ""
	}
	return " {" + s.Name + "}"
}

func fillText(f model.Fill) string {
	switch f.Type {
	case "solid":
		v := f.Hex
		if f.Hex8 != "" {
			v = f.Hex8
		}
		return v + tokenSuffix(f.Token) + styleSuffix(f.Style)
	case "gradient":
		if f.Gradient != nil {
			return f.Gradient.CSS + styleSuffix(f.Style)
		}
	case "image":
		return "image " + f.ImageRef + " " + f.ScaleMode
	}
	return f.Type
}

func layoutSummary(n *model.Node) string {
	l := n.Layout
	if l == nil {
		return ""
	}
	parts := []string{}
	if l.Mode != "none" {
		parts = append(parts, l.Mode)
		if l.Grid != nil {
			parts = append(parts, fmt.Sprintf("%dx%d", l.Grid.Rows, l.Grid.Columns), "gap "+resolve.Px(l.Grid.RowGap)+" "+resolve.Px(l.Grid.ColumnGap))
		}
		if l.Gap != nil {
			gap := "gap " + resolve.Px(*l.Gap) + tokenSuffix(l.Tokens["gap"])
			if l.RowGap != nil && *l.RowGap != *l.Gap {
				gap += " row-gap " + resolve.Px(*l.RowGap) + tokenSuffix(l.Tokens["rowGap"])
			}
			parts = append(parts, gap)
		}
		if l.Padding != nil && l.Padding.Shorthand != "0px" {
			parts = append(parts, "padding "+l.Padding.Shorthand)
		}
		if l.Justify != "" {
			parts = append(parts, "justify "+l.Justify)
		}
		if l.Align != "" {
			parts = append(parts, "align "+l.Align)
		}
	}
	if s := l.Sizing; s != nil {
		parts = append(parts, fmt.Sprintf("w %s %s", s.Horizontal.Mode, resolve.Px(s.Horizontal.Px)), fmt.Sprintf("h %s %s", s.Vertical.Mode, resolve.Px(s.Vertical.Px)))
	}
	if l.Position == "absolute" {
		pos := "absolute"
		if l.Offset != nil {
			pos += fmt.Sprintf(" at %s, %s", resolve.Num(l.Offset.X, 2), resolve.Num(l.Offset.Y, 2))
		}
		if l.Constraints != nil {
			pos += " (" + l.Constraints.Horizontal + "/" + l.Constraints.Vertical + ")"
		}
		parts = append(parts, pos)
	}
	if g := l.GridChild; g != nil {
		parts = append(parts, fmt.Sprintf("cell %d,%d span %dx%d", g.Row+1, g.Column+1, g.RowSpan, g.ColumnSpan))
	}
	if l.Clips {
		parts = append(parts, "clips")
	}
	return strings.Join(parts, "; ")
}

func fillsSummary(n *model.Node) string {
	if n.Visual == nil || len(n.Visual.Fills) == 0 {
		return ""
	}
	parts := make([]string, 0, len(n.Visual.Fills))
	for _, f := range n.Visual.Fills {
		parts = append(parts, fillText(f))
	}
	return strings.Join(parts, ", ")
}

func textSummary(n *model.Node) string {
	if n.Text == nil {
		return ""
	}
	return fmt.Sprintf("%q %s", n.Text.Characters, fontSummary(n.Text))
}

func fontSummary(t *model.Text) string {
	parts := []string{}
	if t.FontFamily != "" {
		parts = append(parts, t.FontFamily+tokenSuffix(t.Tokens["fontFamily"]))
	}
	if t.Weight != nil {
		parts = append(parts, resolve.Num(*t.Weight, 0)+tokenSuffix(t.Tokens["fontWeight"]))
	}
	if t.Italic {
		parts = append(parts, "italic")
	}
	if t.Size != nil {
		size := resolve.Px(*t.Size) + tokenSuffix(t.Tokens["fontSize"])
		if t.LineHeight != nil {
			size += "/" + resolve.Px(t.LineHeight.Px) + tokenSuffix(t.Tokens["lineHeight"])
		}
		parts = append(parts, size)
	}
	if t.LetterSpacing != nil && t.LetterSpacing.Px != 0 {
		parts = append(parts, "tracking "+resolve.Px(t.LetterSpacing.Px)+tokenSuffix(t.Tokens["letterSpacing"]))
	}
	if t.Case != "" {
		parts = append(parts, t.Case)
	}
	if t.Decoration != "" {
		parts = append(parts, t.Decoration)
	}
	if t.AlignH != "" && t.AlignH != "left" {
		parts = append(parts, "align "+t.AlignH)
	}
	if t.Truncation != "" {
		trunc := "truncate"
		if t.MaxLines != nil {
			trunc += fmt.Sprintf(" %d lines", *t.MaxLines)
		}
		parts = append(parts, trunc)
	}
	return strings.Join(parts, " ")
}

func runSummary(run model.TextRun) string {
	parts := []string{}
	if run.Overrides != nil {
		if s := fontSummary(run.Overrides); s != "" {
			parts = append(parts, s)
		}
		if run.Overrides.Hyperlink != "" {
			parts = append(parts, "link "+run.Overrides.Hyperlink)
		}
	}
	if run.Fill != nil {
		parts = append(parts, "color "+fillText(*run.Fill))
	}
	return strings.Join(parts, " ")
}

// overrideSummary says what an instance changes rather than listing the
// internal node path of every change. Someone implementing the design needs to
// know which properties differ from the main component; the nested instance
// ids that Figma reports are noise at that point.
func overrideSummary(overrides []model.Override) string {
	if len(overrides) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var fields []string
	for _, o := range overrides {
		for _, f := range o.Fields {
			if !seen[f] {
				seen[f] = true
				fields = append(fields, f)
			}
		}
	}
	if len(fields) == 0 {
		return ""
	}
	sort.Strings(fields)
	layers := "1 layer"
	if len(overrides) > 1 {
		layers = strconv.Itoa(len(overrides)) + " layers"
	}
	return strings.Join(fields, ", ") + " on " + layers
}

func componentSummary(c *model.Component) string {
	switch {
	case c.IsInstance:
		// Figma names a variant component by its variant values, so showing
		// the main component name next to the variant map prints the same
		// thing twice. Prefer the set name and state the variants once.
		name := c.MainComponentName
		if c.SetName != "" {
			name = c.SetName
		}
		parts := []string{"instance of " + name}
		switch {
		case len(c.VariantProperties) > 0:
			parts = append(parts, "variant "+mapSummary(c.VariantProperties))
		case c.SetName != "" && c.MainComponentName != "" && c.MainComponentName != c.SetName:
			parts = append(parts, "variant "+c.MainComponentName)
		}
		if len(c.Properties) > 0 {
			props := map[string]string{}
			for k, p := range c.Properties {
				props[k] = fmt.Sprint(p.Value)
			}
			parts = append(parts, "props "+mapSummary(props))
		}
		if summary := overrideSummary(c.Overrides); summary != "" {
			parts = append(parts, "overrides "+summary)
		}
		return strings.Join(parts, "; ")
	case c.IsComponentSet:
		return "component set with " + propertyDefinitionsSummary(c)
	default:
		line := "component"
		if c.SetName != "" {
			line += " in " + c.SetName
		}
		if len(c.VariantProperties) > 0 {
			line += " " + mapSummary(c.VariantProperties)
		}
		if len(c.PropertyDefinitions) > 0 {
			line += " with " + propertyDefinitionsSummary(c)
		}
		return line
	}
}

func propertyDefinitionsSummary(c *model.Component) string {
	keys := make([]string, 0, len(c.PropertyDefinitions))
	for k := range c.PropertyDefinitions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		d := c.PropertyDefinitions[k]
		s := k + " (" + d.Type
		if len(d.VariantOptions) > 0 {
			s += ": " + strings.Join(d.VariantOptions, "|")
		}
		s += ")"
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

func init() {
	var nodes []string
	var depth, maxNodes int
	var css, interactions, includeHidden, web bool
	f := nodeInspectCmd.Flags()
	f.StringArrayVar(&nodes, "node", nil, "node id (1:2, 1-2, or a URL with node-id); repeatable")
	f.IntVar(&depth, "depth", inspect.DefaultDepth, "levels of children to include (-1 for all)")
	f.BoolVar(&css, "css", false, "add a flat map of CSS declarations per node")
	f.BoolVar(&interactions, "interactions", false, "include prototype interactions")
	f.BoolVar(&includeHidden, "include-hidden", false, "include hidden nodes")
	f.IntVar(&maxNodes, "max-nodes", inspect.DefaultMaxNodes, "stop after this many nodes and report truncation")
	f.BoolVar(&web, "web", false, "add the var(--name) form to tokens with WEB code syntax")
	nodeInspectCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		ids, err := parseNodeFlags(nodes, r)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return figctl.New(figctl.CodeUsage, "node id required").
				WithHint("Pass --node <id> or a Figma URL with node-id; find ids with figctl file tree %s.", r.FileKey)
		}
		rctx, cancel := session.Context()
		defer cancel()
		result, err := session.Nodes(rctx, r.FileKey, ids, figma.NodesOptions{})
		if err != nil {
			return err
		}
		file, err := session.fileMetadata(rctx, r.FileKey, result, ids...)
		if err != nil {
			return err
		}
		var roots []*figma.Node
		var missing []string
		for _, id := range ids {
			entry := result.Nodes[id]
			if entry == nil || entry.Document == nil {
				missing = append(missing, id)
				continue
			}
			roots = append(roots, entry.Document)
		}
		if len(roots) == 0 {
			return nodeNotFound(strings.Join(missing, ", "), r.FileKey)
		}
		resolver, hints, err := session.Resolver(rctx, r.FileKey, file, roots...)
		if err != nil {
			return err
		}
		opts := inspect.Options{Depth: depth, CSS: css, Interactions: interactions, IncludeHidden: includeHidden, MaxNodes: maxNodes, Web: web}
		out := inspectResult{}
		omitted := 0
		for _, root := range roots {
			m := inspect.Inspect(root, nil, resolver, file, opts)
			out = append(out, m)
			omitted += inspect.Omitted(m)
		}
		env := ctx.Envelope(out)
		for _, h := range hints {
			env.AddHint(h)
		}
		for _, id := range missing {
			env.AddHint("Node " + id + " does not exist in file " + r.FileKey + "; check the id with figctl file tree.")
		}
		if omitted > 0 {
			env.Truncated = true
			env.AddHint(fmt.Sprintf("%d nodes were omitted by --max-nodes %d; lower --depth, raise --max-nodes, or inspect a child node id directly.", omitted, maxNodes))
		}
		if !resolver.HasVariables() && len(hints) == 0 {
			env.AddHint("This file has no local variables; bound values, if any, are reported by id only.")
		}
		return ctx.Printer.Print(env)
	})
	nodeCmd.AddCommand(nodeInspectCmd)
	rootCmd.AddCommand(nodeCmd)
}
