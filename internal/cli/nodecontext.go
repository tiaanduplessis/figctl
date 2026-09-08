package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/assets"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/inspect"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// Defaults of node context.
const (
	contextDepth           = 4
	contextScreenshotScale = 2
	contextScreenshot      = "png"
	contextIconFormat      = "svg"
)

// Failure kinds reported by node context.
const (
	failureScreenshot = "screenshot"
	failureIcon       = "icon"
	failureImageFill  = "imageFill"
)

var nodeContextCmd = &cobra.Command{
	Use:   "context <ref> --node ID",
	Short: "Everything needed to implement one node: inspect, screenshot, assets, tokens, components, comments",
	Long: `Bundle in one call everything an agent needs to implement a screen or a
component: the normalized model at --depth (default 4) with CSS and
prototype interactions, a PNG screenshot of each node written to disk, SVG
exports of the icon-like layers and the raster image fills used in the
subtree, only the design tokens and styles the subtree actually uses, the
component and variant definitions of every instance, the comments pinned
on the node or its children, and the dev resource links attached to them.

File paths in the output are absolute, so a screenshot can be read
straight away with a vision tool. Values with a null token are not bound
to a variable and have to be hardcoded. Screenshot and asset failures do
not fail the command: they are listed in data.failures and the exit code
is 6 (PARTIAL) with the rest of the bundle intact.

Trim the work with --no-screenshot, --no-assets, --no-comments, --no-css,
--depth, and --max-nodes. A node-id in the ref URL is used when no --node
is given.`,
	Example: `  figctl node context KEY --node 2:2
  figctl node context "https://www.figma.com/design/KEY/App?node-id=2-2" -o md
  figctl node context KEY --node 2:6 --depth 2 --no-assets
  figctl node context KEY --node 2:2 --out ./design/login --screenshot-scale 1`,
	Args: cobra.ExactArgs(1),
}

// contextScreenshotInfo is the rendered PNG of one requested node.
type contextScreenshotInfo struct {
	Path   string  `json:"path"`
	Format string  `json:"format"`
	Scale  float64 `json:"scale"`
	Bytes  int64   `json:"bytes"`
	Cached bool    `json:"cached"`
}

// contextNode is one requested node: where it lives, its screenshot, and
// its inspected model.
type contextNode struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Page       string                 `json:"page,omitempty"`
	Path       []string               `json:"path"`
	Screenshot *contextScreenshotInfo `json:"screenshot,omitempty"`
	Inspect    *model.Node            `json:"inspect"`
}

// contextAssets are the files written for the subtree.
type contextAssets struct {
	Icons      []assets.Result `json:"icons"`
	ImageFills []fillEntry     `json:"imageFills"`
}

// contextComponent is the definition behind one instance in the subtree,
// deduplicated by component key.
type contextComponent struct {
	Key                 string                        `json:"key"`
	NodeID              string                        `json:"nodeId,omitempty"`
	Name                string                        `json:"name"`
	Description         string                        `json:"description,omitempty"`
	SetKey              string                        `json:"setKey,omitempty"`
	SetNodeID           string                        `json:"setNodeId,omitempty"`
	SetName             string                        `json:"setName,omitempty"`
	Remote              bool                          `json:"remote"`
	VariantProperties   map[string]string             `json:"variantProperties,omitempty"`
	PropertyDefinitions map[string]propertyDefinition `json:"propertyDefinitions,omitempty"`
	DocumentationLinks  []string                      `json:"documentationLinks"`
	DevResources        []devResourceRow              `json:"devResources"`
	Instances           []string                      `json:"instances"`
}

// contextFailure is one screenshot or asset that produced no file.
type contextFailure struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Error string `json:"error"`
}

// contextSummary counts what the bundle contains.
type contextSummary struct {
	Nodes        int      `json:"nodes"`
	Tokens       int      `json:"tokens"`
	Styles       int      `json:"styles"`
	Components   int      `json:"components"`
	Assets       int      `json:"assets"`
	Comments     int      `json:"comments"`
	DevResources int      `json:"devResources"`
	Measurements int      `json:"measurements"`
	Page         string   `json:"page,omitempty"`
	Path         []string `json:"path"`
}

// contextData is the data of node context.
type contextData struct {
	OutDir       string              `json:"outDir"`
	Nodes        []contextNode       `json:"nodes"`
	Tokens       []model.TokenRef    `json:"tokens"`
	Styles       []model.StyleRef    `json:"styles"`
	Components   []contextComponent  `json:"components"`
	Assets       contextAssets       `json:"assets"`
	Comments     []commentRow        `json:"comments"`
	DevResources []devResourceRow    `json:"devResources"`
	Measurements []model.Measurement `json:"measurements"`
	Summary      contextSummary      `json:"summary"`
	Failures     []contextFailure    `json:"failures"`
	Requests     int                 `json:"requests"`

	// devResources indexes every dev resource of the file by node id so
	// component entries can pick up the ones on their main component. It
	// is not part of the output.
	devResources map[string][]devResourceRow
}

// Columns implements output.Tabular with a summary view; the full bundle
// is only meaningful as JSON or markdown.
func (d contextData) Columns() []string { return []string{"field", "value"} }

// Rows implements output.Tabular.
func (d contextData) Rows() [][]string {
	kv := output.KeyValues{
		{"nodes", strings.Join(contextNodeLabels(d.Nodes), ", ")},
		{"page", d.Summary.Page},
		{"path", strings.Join(d.Summary.Path, " / ")},
		{"inspected", strconv.Itoa(d.Summary.Nodes)},
		{"tokens", strconv.Itoa(d.Summary.Tokens)},
		{"styles", strconv.Itoa(d.Summary.Styles)},
		{"components", strconv.Itoa(d.Summary.Components)},
		{"assets", strconv.Itoa(d.Summary.Assets)},
		{"comments", strconv.Itoa(d.Summary.Comments)},
		{"devResources", strconv.Itoa(d.Summary.DevResources)},
		{"outDir", d.OutDir},
	}
	for _, n := range d.Nodes {
		if n.Screenshot != nil {
			kv = append(kv, [2]string{"screenshot " + n.ID, n.Screenshot.Path})
		}
	}
	if len(d.Failures) > 0 {
		kv = append(kv, [2]string{"failures", strconv.Itoa(len(d.Failures))})
	}
	return kv.Rows()
}

func contextNodeLabels(nodes []contextNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Name+" ("+n.ID+")")
	}
	return out
}

// Markdown implements output.Markdowner: a compact implementation brief
// rather than a JSON dump, because agents read markdown output as text.
func (d contextData) Markdown() string {
	var b strings.Builder
	for _, n := range d.Nodes {
		fmt.Fprintf(&b, "## Node: %s (%s, %s)\n\n", n.Name, n.ID, n.Type)
		if n.Page != "" {
			b.WriteString("- page: " + n.Page + "\n")
		}
		if len(n.Path) > 0 {
			b.WriteString("- path: " + strings.Join(n.Path, " / ") + "\n")
		}
		if box := n.Inspect; box != nil && box.Box != nil {
			fmt.Fprintf(&b, "- size: %s x %s\n", resolve.Num(box.Box.W, 2), resolve.Num(box.Box.H, 2))
		}
		b.WriteString("\n### Layout\n\n")
		if n.Inspect != nil {
			writeNodeMarkdown(&b, n.Inspect, 4)
		}
	}
	d.writeTokensMarkdown(&b)
	d.writeComponentsMarkdown(&b)
	d.writeAssetsMarkdown(&b)
	d.writeScreenshotsMarkdown(&b)
	d.writeMeasurementsMarkdown(&b)
	d.writeCommentsMarkdown(&b)
	d.writeDevResourcesMarkdown(&b)
	d.writeFailuresMarkdown(&b)
	return b.String()
}

func (d contextData) writeTokensMarkdown(b *strings.Builder) {
	b.WriteString("## Tokens used\n\n")
	if len(d.Tokens) == 0 && len(d.Styles) == 0 {
		b.WriteString("None: no value in this subtree is bound to a variable or a style. Hardcode the raw values above.\n\n")
		return
	}
	if len(d.Tokens) > 0 {
		rows := make([][]string, 0, len(d.Tokens))
		for _, t := range d.Tokens {
			rows = append(rows, []string{t.Name, t.Collection, t.ResolvedType, modeValues(t), codeSyntax(t)})
		}
		writeContextTable(b, []string{"token", "collection", "type", "values", "code"}, rows)
		b.WriteString("\n")
	}
	if len(d.Styles) > 0 {
		rows := make([][]string, 0, len(d.Styles))
		for _, s := range d.Styles {
			rows = append(rows, []string{s.Name, s.Type, s.ID})
		}
		writeContextTable(b, []string{"style", "type", "nodeId"}, rows)
		b.WriteString("\n")
	}
}

func (d contextData) writeComponentsMarkdown(b *strings.Builder) {
	b.WriteString("## Components\n\n")
	if len(d.Components) == 0 {
		b.WriteString("None: this subtree has no component instances.\n\n")
		return
	}
	for _, c := range d.Components {
		fmt.Fprintf(b, "### %s\n\n", componentLabel(c))
		if c.SetName != "" {
			b.WriteString("- set: " + c.SetName + " (" + c.SetNodeID + ")\n")
		}
		b.WriteString("- key: " + c.Key + "\n")
		if len(c.VariantProperties) > 0 {
			b.WriteString("- variant: " + variantString(c.VariantProperties) + "\n")
		}
		if len(c.PropertyDefinitions) > 0 {
			b.WriteString("- properties: " + definitionsSummary(c.PropertyDefinitions) + "\n")
		}
		if c.Description != "" {
			b.WriteString("- description: " + truncate(c.Description, 200) + "\n")
		}
		if len(c.DocumentationLinks) > 0 {
			b.WriteString("- docs: " + strings.Join(c.DocumentationLinks, ", ") + "\n")
		}
		for _, dr := range c.DevResources {
			b.WriteString("- code: " + dr.Name + " " + dr.URL + "\n")
		}
		b.WriteString("- instances: " + strings.Join(c.Instances, ", ") + "\n\n")
	}
}

// componentLabel names a component the way a developer refers to it: the
// set name carries the meaning, the variant name the configuration.
func componentLabel(c contextComponent) string {
	if c.SetName != "" && c.SetName != c.Name {
		return c.SetName + " (" + c.Name + ")"
	}
	return c.Name
}

func (d contextData) writeAssetsMarkdown(b *strings.Builder) {
	b.WriteString("## Assets\n\n")
	if len(d.Assets.Icons) == 0 && len(d.Assets.ImageFills) == 0 {
		b.WriteString("None: no icon-like layer and no raster image fill was found under this node.\n\n")
		return
	}
	var rows [][]string
	for _, icon := range d.Assets.Icons {
		rows = append(rows, []string{"icon", icon.NodeID, icon.Name, icon.Path, icon.Error})
	}
	for _, fill := range d.Assets.ImageFills {
		rows = append(rows, []string{"imageFill", fill.ImageRef, fillUsers(fill.Nodes), fill.Path, fill.Error})
	}
	writeContextTable(b, []string{"kind", "id", "name", "path", "error"}, rows)
	b.WriteString("\n")
}

func (d contextData) writeScreenshotsMarkdown(b *strings.Builder) {
	var rows [][]string
	for _, n := range d.Nodes {
		if n.Screenshot != nil {
			rows = append(rows, []string{n.ID, n.Name, n.Screenshot.Path, resolve.Num(n.Screenshot.Scale, 2) + "x"})
		}
	}
	b.WriteString("## Screenshot path\n\n")
	if len(rows) == 0 {
		b.WriteString("None: rendering was skipped or failed. Run figctl render with --node to try again.\n\n")
		return
	}
	writeContextTable(b, []string{"nodeId", "name", "path", "scale"}, rows)
	b.WriteString("\nRead these files with your image or vision tool before writing code.\n\n")
}

// writeMeasurementsMarkdown lists the distances a designer pinned in Dev Mode.
// They are spacing stated outright rather than inferred from layout, so they
// are worth reading before deciding what a gap should be.
func (d contextData) writeMeasurementsMarkdown(b *strings.Builder) {
	if len(d.Measurements) == 0 {
		return
	}
	b.WriteString("## Measurements\n\n")
	b.WriteString("Distances the designer pinned in Dev Mode. Prefer these over a gap read off the screenshot.\n\n")
	rows := make([][]string, 0, len(d.Measurements))
	for _, m := range d.Measurements {
		value := m.Label
		if value == "" {
			value = "unresolved"
		}
		if m.FreeText != "" {
			value += " (set by the designer)"
		}
		note := m.Unresolved
		rows = append(rows, []string{
			value,
			m.Axis,
			pinLabel(m.Start),
			pinLabel(m.End),
			note,
		})
	}
	writeContextTable(b, []string{"value", "axis", "from", "to", "note"}, rows)
	b.WriteString("\n")
}

// pinLabel names one end of a measurement the way a reader would: the layer
// name and the side, falling back to the id when the node is not in view.
func pinLabel(p model.MeasurementPin) string {
	name := p.Name
	if name == "" {
		name = p.NodeID
	}
	return name + " " + strings.ToLower(p.Side)
}

func (d contextData) writeCommentsMarkdown(b *strings.Builder) {
	b.WriteString("## Comments\n\n")
	if len(d.Comments) == 0 {
		b.WriteString("None pinned on this node or its children.\n\n")
		return
	}
	for _, c := range d.Comments {
		state := "open"
		if c.ResolvedAt != "" {
			state = "resolved " + c.ResolvedAt
		}
		prefix := "- "
		if c.ParentID != "" {
			prefix = "  - reply "
		}
		where := ""
		if c.NodeID != "" {
			where = " on " + c.NodeID
		}
		fmt.Fprintf(b, "%s%s%s (%s, %s): %s\n", prefix, c.User, where, c.CreatedAt, state, truncate(c.Message, 400))
	}
	b.WriteString("\n")
}

func (d contextData) writeDevResourcesMarkdown(b *strings.Builder) {
	if len(d.DevResources) == 0 {
		return
	}
	b.WriteString("## Dev resources\n\n")
	for _, dr := range d.DevResources {
		b.WriteString("- " + dr.NodeID + " " + dr.Name + ": " + dr.URL + "\n")
	}
	b.WriteString("\n")
}

func (d contextData) writeFailuresMarkdown(b *strings.Builder) {
	if len(d.Failures) == 0 {
		return
	}
	b.WriteString("## Failures\n\n")
	for _, f := range d.Failures {
		b.WriteString("- " + f.Kind + " " + f.ID + ": " + f.Error + "\n")
	}
	b.WriteString("\n")
}

// modeValues renders a token's value per mode for the markdown brief.
func modeValues(t model.TokenRef) string {
	if len(t.Values) == 0 {
		return ""
	}
	names := make([]string, 0, len(t.Values))
	for name := range t.Values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+fmt.Sprint(t.Values[name]))
	}
	return strings.Join(parts, ", ")
}

func codeSyntax(t model.TokenRef) string {
	if len(t.CodeSyntax) == 0 {
		return ""
	}
	platforms := make([]string, 0, len(t.CodeSyntax))
	for p := range t.CodeSyntax {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	parts := make([]string, 0, len(platforms))
	for _, p := range platforms {
		parts = append(parts, p+": "+t.CodeSyntax[p])
	}
	return strings.Join(parts, ", ")
}

func definitionsSummary(defs map[string]propertyDefinition) string {
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		def := defs[name]
		part := name + " (" + def.Type
		if len(def.VariantOptions) > 0 {
			part += ": " + strings.Join(def.VariantOptions, "|")
		}
		if def.Default != nil && def.Default != "" {
			part += ", default " + fmt.Sprint(def.Default)
		}
		parts = append(parts, part+")")
	}
	return strings.Join(parts, "; ")
}

// contextOptions holds the flags of node context.
type contextOptions struct {
	nodes           []string
	out             string
	iconPatterns    []string
	depth           int
	maxNodes        int
	screenshotScale float64
	noScreenshot    bool
	noAssets        bool
	noComments      bool
	noCSS           bool
}

func init() {
	var opts contextOptions
	f := nodeContextCmd.Flags()
	f.StringArrayVar(&opts.nodes, "node", nil, "node id (1:2, 1-2, or a URL with node-id); repeatable")
	f.IntVar(&opts.depth, "depth", contextDepth, "levels of children to inspect (-1 for all)")
	f.IntVar(&opts.maxNodes, "max-nodes", inspect.DefaultMaxNodes, "stop inspecting after this many nodes and report truncation")
	f.StringVar(&opts.out, "out", "", "directory for the screenshot and assets (default ./.figctl/<fileKey>/context)")
	f.Float64Var(&opts.screenshotScale, "screenshot-scale", contextScreenshotScale, "screenshot render scale between 0.01 and 4")
	f.StringArrayVar(&opts.iconPatterns, "icon-pattern", nil, "name glob that marks an icon (default icon/*, Icon*, icons/*); repeatable")
	f.BoolVar(&opts.noScreenshot, "no-screenshot", false, "skip the PNG render")
	f.BoolVar(&opts.noAssets, "no-assets", false, "skip icon exports and image fill downloads")
	f.BoolVar(&opts.noComments, "no-comments", false, "skip comments and dev resources")
	f.BoolVar(&opts.noCSS, "no-css", false, "omit the CSS declaration map from the inspected nodes")
	nodeContextCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		return runNodeContext(ctx, args[0], &opts)
	})
	nodeCmd.AddCommand(nodeContextCmd)
}

func runNodeContext(ctx *Context, arg string, opts *contextOptions) error {
	session, err := ctx.Session()
	if err != nil {
		return err
	}
	r, err := session.Ref(arg)
	if err != nil {
		return err
	}
	ids, err := parseNodeFlags(opts.nodes, r)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return figctl.New(figctl.CodeUsage, "node id required").
			WithHint("Pass --node <id> or a Figma URL with node-id; find ids with figctl file tree %s.", r.FileKey)
	}
	if err := assets.ValidateIconPatterns(opts.iconPatterns); err != nil {
		return err
	}
	if !opts.noScreenshot {
		if err := assets.ValidateScale(opts.screenshotScale); err != nil {
			return err
		}
	}
	dir, err := outputDir(opts.out, r.FileKey, "context")
	if err != nil {
		return err
	}
	rctx, cancel := session.Context()
	defer cancel()

	resp, err := session.Nodes(rctx, r.FileKey, ids, figma.NodesOptions{})
	if err != nil {
		return err
	}
	file, err := session.fileMetadata(rctx, r.FileKey, resp)
	if err != nil {
		return err
	}
	var roots []*figma.Node
	var missing []string
	for _, id := range ids {
		entry := resp.Nodes[id]
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

	data := contextData{
		OutDir:       dir,
		Nodes:        []contextNode{},
		Tokens:       []model.TokenRef{},
		Styles:       []model.StyleRef{},
		Components:   []contextComponent{},
		Assets:       contextAssets{Icons: []assets.Result{}, ImageFills: []fillEntry{}},
		Comments:     []commentRow{},
		DevResources: []devResourceRow{},
		Measurements: []model.Measurement{},
		Failures:     []contextFailure{},
	}
	inspectOpts := inspect.Options{
		Depth:        opts.depth,
		CSS:          !opts.noCSS,
		Interactions: true,
		MaxNodes:     opts.maxNodes,
	}
	omitted := 0
	models := make([]*model.Node, 0, len(roots))
	for _, root := range roots {
		m := inspect.Inspect(root, nil, resolver, file, inspectOpts)
		models = append(models, m)
		omitted += inspect.Omitted(m)
		data.Nodes = append(data.Nodes, contextNode{ID: m.ID, Name: m.Name, Type: m.Type, Page: pageOf(m.Path), Path: nodePathOf(m), Inspect: m})
		data.Summary.Nodes += countModelNodes(m)
	}
	data.Summary.Page = data.Nodes[0].Page
	data.Summary.Path = data.Nodes[0].Path

	data.Tokens, data.Styles = usedTokens(models)
	data.Summary.Tokens, data.Summary.Styles = len(data.Tokens), len(data.Styles)

	// Measurements are pinned on the page rather than on the nodes they
	// measure, so they are only available when the whole document is in hand.
	if file != nil && file.Document != nil {
		data.Measurements = inspect.Measurements(file.Document, roots...)
	}
	data.Summary.Measurements = len(data.Measurements)

	components, err := contextComponentsOf(rctx, session, r.FileKey, file, models)
	if err != nil {
		return err
	}
	data.Components = components
	data.Summary.Components = len(components)

	extraHints, err := collectContextExtras(rctx, session, ctx, r.FileKey, resp.Version, dir, roots, opts, &data)
	if err != nil {
		return err
	}
	hints = append(hints, extraHints...)
	attachComponentDevResources(&data)

	env := ctx.Envelope(data)
	for _, h := range hints {
		env.AddHint(h)
	}
	for _, id := range missing {
		env.AddHint("Node " + id + " does not exist in file " + r.FileKey + "; check the id with figctl file tree.")
	}
	if omitted > 0 {
		env.Truncated = true
		env.AddHint(fmt.Sprintf("%d nodes were omitted by --max-nodes %d; raise --max-nodes, lower --depth, or run figctl node context %s --node <child id> for one part of the screen.", omitted, opts.maxNodes, r.FileKey))
	}
	for _, h := range contextHints(r.FileKey, data, opts) {
		env.AddHint(h)
	}
	if len(data.Failures) == 0 {
		return ctx.Printer.Print(env)
	}
	return ctx.PrintPartial(env, figctl.Newf(figctl.CodePartial, "%d of the requested files could not be written", len(data.Failures)).
		WithHint("The rest of the bundle is intact; see data.failures for what failed."))
}

// collectContextExtras renders the screenshot, exports assets, and reads
// comments and dev resources. Missing optional data becomes a hint;
// attempted work that failed becomes an entry in data.Failures.
func collectContextExtras(rctx context.Context, session *Session, ctx *Context, key, version, dir string, roots []*figma.Node, opts *contextOptions, data *contextData) ([]string, error) {
	var hints []string
	if !opts.noScreenshot {
		if err := renderContextScreenshots(rctx, session, key, version, dir, roots, opts, data); err != nil {
			return nil, err
		}
	}
	if !opts.noAssets {
		if err := exportContextAssets(rctx, session, key, version, dir, roots, opts, data); err != nil {
			return nil, err
		}
	}
	if opts.noComments {
		return hints, nil
	}
	subtree := subtreeIDs(roots)
	comments, hint, err := contextComments(rctx, session, key, subtree)
	if err != nil {
		return nil, err
	}
	if hint != "" {
		hints = append(hints, hint)
		ctx.Log.Warnf("%s", hint)
	}
	data.Comments = comments
	data.Summary.Comments = len(comments)

	resources, hint, err := contextDevResources(rctx, session, key)
	if err != nil {
		return nil, err
	}
	if hint != "" {
		hints = append(hints, hint)
		ctx.Log.Warnf("%s", hint)
	}
	for _, dr := range resources {
		if subtree[dr.NodeID] {
			data.DevResources = append(data.DevResources, dr)
		}
	}
	data.Summary.DevResources = len(data.DevResources)
	data.devResources = devResourceIndex(resources)
	return hints, nil
}

func renderContextScreenshots(rctx context.Context, session *Session, key, version, dir string, roots []*figma.Node, opts *contextOptions, data *contextData) error {
	req := assets.RenderRequest{OutDir: dir, NameBy: assets.NameByName}
	for _, root := range roots {
		req.Items = append(req.Items, assets.Item{NodeID: root.ID, Name: root.Name, Format: contextScreenshot, Scale: opts.screenshotScale})
	}
	out, err := assets.Render(rctx, session.Client, session.Cache, key, version, req)
	if err != nil {
		return err
	}
	data.Requests += out.Requests
	for i, res := range out.Items {
		if res.Error != "" {
			data.Failures = append(data.Failures, contextFailure{Kind: failureScreenshot, ID: res.NodeID, Name: res.Name, Error: res.Error})
			continue
		}
		data.Nodes[i].Screenshot = &contextScreenshotInfo{Path: res.Path, Format: res.Format, Scale: res.Scale, Bytes: res.Bytes, Cached: res.Cached}
	}
	return nil
}

func exportContextAssets(rctx context.Context, session *Session, key, version, dir string, roots []*figma.Node, opts *contextOptions, data *contextData) error {
	found := assets.Discover(roots, assets.DiscoverOptions{
		IconPatterns:      opts.iconPatterns,
		IncludeIcons:      true,
		IncludeImageFills: true,
	})
	req := assets.RenderRequest{OutDir: dir, NameBy: assets.NameByName}
	for _, icon := range found.Icons {
		req.Items = append(req.Items, assets.Item{NodeID: icon.NodeID, Name: icon.Name, Format: contextIconFormat, Scale: assets.DefaultScale(contextIconFormat)})
	}
	out, err := assets.Render(rctx, session.Client, session.Cache, key, version, req)
	if err != nil {
		return err
	}
	data.Requests += out.Requests
	for _, res := range out.Items {
		data.Assets.Icons = append(data.Assets.Icons, res)
		if res.Error != "" {
			data.Failures = append(data.Failures, contextFailure{Kind: failureIcon, ID: res.NodeID, Name: res.Name, Error: res.Error})
		}
	}
	refs := make([]string, 0, len(found.ImageFills))
	for _, fill := range found.ImageFills {
		refs = append(refs, fill.ImageRef)
		data.Assets.ImageFills = append(data.Assets.ImageFills, fillEntry{ImageRef: fill.ImageRef, ScaleMode: fill.ScaleMode, Nodes: fill.Nodes})
	}
	if len(refs) > 0 {
		fills, err := assets.DownloadFills(rctx, session.Client, session.Cache, key, version, refs, dir, assets.DefaultConcurrency)
		if err != nil {
			return err
		}
		data.Requests += fills.Requests
		for i, res := range fills.Items {
			entry := &data.Assets.ImageFills[i]
			entry.Path, entry.Bytes, entry.Cached, entry.Error = res.Path, res.Bytes, res.Cached, res.Error
			if res.Error != "" {
				data.Failures = append(data.Failures, contextFailure{Kind: failureImageFill, ID: res.ImageRef, Error: res.Error})
			}
		}
	}
	data.Summary.Assets = len(data.Assets.Icons) + len(data.Assets.ImageFills) - failuresOfKind(data.Failures, failureIcon, failureImageFill)
	return nil
}

func failuresOfKind(failures []contextFailure, kinds ...string) int {
	n := 0
	for _, f := range failures {
		for _, kind := range kinds {
			if f.Kind == kind {
				n++
			}
		}
	}
	return n
}

// contextComments returns the comments pinned on the subtree and their
// replies. A missing comments scope degrades to a hint.
func contextComments(rctx context.Context, session *Session, key string, subtree map[string]bool) ([]commentRow, string, error) {
	resp, err := session.Client.GetComments(rctx, key, false)
	if err != nil {
		if hint, soft := softFailure(err, "Comments were not read"); soft {
			return []commentRow{}, hint, nil
		}
		return nil, "", err
	}
	out := []commentRow{}
	pinned := map[string]bool{}
	for _, c := range resp.Comments {
		if c.ClientMeta != nil && subtree[c.ClientMeta.NodeID] {
			out = append(out, commentRowFrom(c))
			pinned[c.ID] = true
		}
	}
	for _, c := range resp.Comments {
		if c.ParentID != "" && pinned[c.ParentID] && !pinned[c.ID] {
			out = append(out, commentRowFrom(c))
		}
	}
	return out, "", nil
}

// contextDevResources reads every dev resource of the file in one request
// so both the subtree and the main components can be matched locally.
func contextDevResources(rctx context.Context, session *Session, key string) ([]devResourceRow, string, error) {
	resp, err := session.Client.GetDevResources(rctx, key, nil)
	if err != nil {
		if hint, soft := softFailure(err, "Dev resources were not read"); soft {
			return nil, hint, nil
		}
		return nil, "", err
	}
	out := make([]devResourceRow, 0, len(resp.DevResources))
	for _, dr := range resp.DevResources {
		out = append(out, devResourceRow{ID: dr.ID, Name: dr.Name, URL: dr.URL, NodeID: dr.NodeID})
	}
	return out, "", nil
}

// softFailure turns a failure to read one optional extra into a hint. The
// caller has already produced the inspected model, the screenshot, and the
// tokens, which is what implementing the design depends on. Comments and dev
// resource links are additional context, so losing them must not discard the
// rest: a large file can time out or exhaust the rate limit on a follow-up
// request long after the core data is in hand.
func softFailure(err error, what string) (string, bool) {
	e := figctl.From(err)
	switch e.Code {
	case figctl.CodeAuthScope, figctl.CodeForbidden, figctl.CodeNotFound,
		figctl.CodeNetwork, figctl.CodeRateLimited:
		return joinSentences(what+": "+e.Message, e.Hint), true
	}
	return "", false
}

// joinSentences joins two fragments into readable prose, adding the sentence
// break the first fragment is missing. API messages rarely end in punctuation,
// so concatenating them with a space alone runs two sentences together.
func joinSentences(first, second string) string {
	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	if second == "" {
		return first
	}
	if first == "" {
		return second
	}
	if !strings.ContainsAny(first[len(first)-1:], ".!?:;") {
		first += "."
	}
	return first + " " + second
}

func devResourceIndex(resources []devResourceRow) map[string][]devResourceRow {
	out := map[string][]devResourceRow{}
	for _, dr := range resources {
		out[dr.NodeID] = append(out[dr.NodeID], dr)
	}
	return out
}

// attachComponentDevResources links the dev resources of a main component
// or its set to the component entry, so an agent sees the code mapping of
// an instance even though the resource is not attached to the instance.
func attachComponentDevResources(data *contextData) {
	for i := range data.Components {
		c := &data.Components[i]
		c.DevResources = append(c.DevResources, data.devResources[c.NodeID]...)
		if c.SetNodeID != "" {
			c.DevResources = append(c.DevResources, data.devResources[c.SetNodeID]...)
		}
	}
}

// contextComponentsOf collects the definition behind every instance in the
// inspected trees, deduplicated by component key.
func contextComponentsOf(rctx context.Context, session *Session, key string, file *figma.GetFileResponse, models []*model.Node) ([]contextComponent, error) {
	var order []string
	instances := map[string][]string{}
	for _, m := range models {
		walkModel(m, func(n *model.Node) {
			c := n.Component
			if c == nil || !c.IsInstance || c.MainComponentID == "" {
				return
			}
			if _, seen := instances[c.MainComponentID]; !seen {
				order = append(order, c.MainComponentID)
			}
			instances[c.MainComponentID] = append(instances[c.MainComponentID], n.ID)
		})
	}
	if len(order) == 0 {
		return []contextComponent{}, nil
	}
	lookup := append([]string{}, order...)
	for _, id := range order {
		if comp, ok := file.Components[id]; ok && comp.ComponentSetID != "" {
			lookup = append(lookup, comp.ComponentSetID)
		}
	}
	nodes, err := session.Nodes(rctx, key, lookup, figma.NodesOptions{Depth: 1})
	if err != nil {
		return nil, err
	}
	out := make([]contextComponent, 0, len(order))
	byKey := map[string]int{}
	for _, id := range order {
		entry := buildContextComponent(id, file, nodes, instances[id])
		dedupe := entry.Key
		if dedupe == "" {
			dedupe = id
		}
		if i, seen := byKey[dedupe]; seen {
			out[i].Instances = append(out[i].Instances, entry.Instances...)
			continue
		}
		byKey[dedupe] = len(out)
		out = append(out, entry)
	}
	return out, nil
}

func buildContextComponent(id string, file *figma.GetFileResponse, nodes *figma.GetFileNodesResponse, instances []string) contextComponent {
	c := contextComponent{NodeID: id, Instances: instances, DocumentationLinks: []string{}, DevResources: []devResourceRow{}}
	if comp, ok := file.Components[id]; ok {
		c.Key, c.Name, c.Description, c.Remote = comp.Key, comp.Name, comp.Description, comp.Remote
		c.DocumentationLinks = documentationLinks(comp.DocumentationLinks)
		c.SetNodeID = comp.ComponentSetID
		if set, ok := file.ComponentSets[comp.ComponentSetID]; ok {
			c.SetKey, c.SetName = set.Key, set.Name
			if c.Description == "" {
				c.Description = set.Description
			}
			if len(c.DocumentationLinks) == 0 {
				c.DocumentationLinks = documentationLinks(set.DocumentationLinks)
			}
		}
	}
	defs := map[string]propertyDefinition{}
	if c.SetNodeID != "" {
		for name, def := range definitionsAt(nodes, c.SetNodeID) {
			defs[name] = def
		}
	}
	for name, def := range definitionsAt(nodes, id) {
		defs[name] = def
	}
	if len(defs) > 0 {
		c.PropertyDefinitions = defs
	}
	if node := nodeAt(nodes, id); node != nil {
		if c.Name == "" {
			c.Name = node.Name
		}
		c.VariantProperties = parseVariantProperties(node.Name)
	}
	if c.VariantProperties == nil {
		c.VariantProperties = parseVariantProperties(c.Name)
	}
	return c
}

func nodeAt(nodes *figma.GetFileNodesResponse, id string) *figma.Node {
	if nodes == nil {
		return nil
	}
	if entry := nodes.Nodes[id]; entry != nil {
		return entry.Document
	}
	return nil
}

func definitionsAt(nodes *figma.GetFileNodesResponse, id string) map[string]propertyDefinition {
	if node := nodeAt(nodes, id); node != nil {
		return definitionsOf(node)
	}
	return nil
}

// usedTokens collects every variable and style referenced anywhere in the
// inspected trees, deduplicated and sorted by name.
func usedTokens(models []*model.Node) ([]model.TokenRef, []model.StyleRef) {
	tokens := map[string]model.TokenRef{}
	styles := map[string]model.StyleRef{}
	addToken := func(t *model.TokenRef) {
		if t == nil || t.VariableID == "" {
			return
		}
		if _, seen := tokens[t.VariableID]; !seen {
			tokens[t.VariableID] = *t
		}
	}
	addStyle := func(s *model.StyleRef) {
		if s == nil || s.ID == "" {
			return
		}
		if _, seen := styles[s.ID]; !seen {
			styles[s.ID] = *s
		}
	}
	for _, m := range models {
		walkModel(m, func(n *model.Node) {
			collectNodeTokens(n, addToken, addStyle)
		})
	}
	outTokens := make([]model.TokenRef, 0, len(tokens))
	for _, t := range tokens {
		outTokens = append(outTokens, t)
	}
	sort.Slice(outTokens, func(i, j int) bool {
		if outTokens[i].Name != outTokens[j].Name {
			return outTokens[i].Name < outTokens[j].Name
		}
		return outTokens[i].VariableID < outTokens[j].VariableID
	})
	outStyles := make([]model.StyleRef, 0, len(styles))
	for _, s := range styles {
		outStyles = append(outStyles, s)
	}
	sort.Slice(outStyles, func(i, j int) bool {
		if outStyles[i].Name != outStyles[j].Name {
			return outStyles[i].Name < outStyles[j].Name
		}
		return outStyles[i].ID < outStyles[j].ID
	})
	return outTokens, outStyles
}

// collectNodeTokens visits every token and style reference on one node.
func collectNodeTokens(n *model.Node, addToken func(*model.TokenRef), addStyle func(*model.StyleRef)) {
	if v := n.Visual; v != nil {
		for _, f := range v.Fills {
			collectFillTokens(f, addToken, addStyle)
		}
		for _, s := range v.Strokes {
			collectFillTokens(s, addToken, addStyle)
		}
		if v.Radius != nil {
			addToken(v.Radius.Token)
		}
		for _, e := range v.Effects {
			addToken(e.Token)
			addStyle(e.Style)
		}
		for _, g := range v.LayoutGrids {
			addStyle(g.Style)
		}
	}
	if l := n.Layout; l != nil {
		for _, t := range l.Tokens {
			addToken(t)
		}
	}
	if t := n.Text; t != nil {
		for _, tok := range t.Tokens {
			addToken(tok)
		}
		addStyle(t.Style)
		for _, run := range t.Runs {
			if run.Fill != nil {
				collectFillTokens(*run.Fill, addToken, addStyle)
			}
			if run.Overrides != nil {
				for _, tok := range run.Overrides.Tokens {
					addToken(tok)
				}
			}
		}
	}
	if c := n.Component; c != nil {
		for _, p := range c.Properties {
			addToken(p.Token)
		}
	}
}

func collectFillTokens(f model.Fill, addToken func(*model.TokenRef), addStyle func(*model.StyleRef)) {
	addToken(f.Token)
	addStyle(f.Style)
	if f.Gradient != nil {
		for _, stop := range f.Gradient.Stops {
			addToken(stop.Token)
		}
	}
}

func walkModel(n *model.Node, fn func(*model.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		walkModel(c, fn)
	}
}

func countModelNodes(n *model.Node) int {
	if n == nil {
		return 0
	}
	total := 1
	for _, c := range n.Children {
		total += countModelNodes(c)
	}
	return total
}

// subtreeIDs is the set of node ids under the requested roots, including
// the roots themselves.
func subtreeIDs(roots []*figma.Node) map[string]bool {
	out := map[string]bool{}
	for _, root := range roots {
		root.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
			out[n.ID] = true
			return true
		})
	}
	return out
}

func pageOf(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[0]
}

func nodePathOf(m *model.Node) []string {
	path := append([]string{}, m.Path...)
	return append(path, m.Name)
}

// contextHints tells the agent what to do next in concrete terms.
func contextHints(fileKey string, data contextData, opts *contextOptions) []string {
	var hints []string
	var shots []string
	for _, n := range data.Nodes {
		if n.Screenshot != nil {
			shots = append(shots, n.Screenshot.Path)
		}
	}
	switch {
	case len(shots) > 0:
		hints = append(hints, "Read the screenshot with your image or vision tool before writing code: "+strings.Join(shots, ", "))
	case opts.noScreenshot:
		hints = append(hints, "No screenshot was rendered (--no-screenshot); drop the flag or run figctl render "+fileKey+" --node "+data.Nodes[0].ID+" to see the design.")
	}
	hints = append(hints, "Every value with \"token\": null is not bound to a variable: hardcode it, or ask the designer for a token. Values with a token name should use that design token in code.")
	switch {
	case len(data.Assets.Icons) > 0 || len(data.Assets.ImageFills) > 0:
		hints = append(hints, "Assets were written to "+data.OutDir+": inline or import the SVG icons and reference the image fills by path.")
	case opts.noAssets:
		hints = append(hints, "Assets were skipped (--no-assets); run figctl assets export "+fileKey+" --node "+data.Nodes[0].ID+" when you need the icons and images.")
	default:
		hints = append(hints, "No icons or image fills were found under this node; widen the search with figctl assets export "+fileKey+" --icon-pattern <glob> or --include-hidden.")
	}
	if len(data.Tokens) == 0 && len(data.Styles) == 0 {
		hints = append(hints, "No variable or style is bound in this subtree, so every value above is raw; figctl tokens export "+fileKey+" lists the design system vocabulary of the whole file.")
	}
	if n := len(data.Measurements); n > 0 {
		hints = append(hints, fmt.Sprintf("The designer pinned %d measurement(s) in Dev Mode; see data.measurements. They state a spacing decision outright, so prefer them over a gap read off the screenshot.", n))
	}
	if opts.noComments {
		hints = append(hints, "Comments and dev resources were skipped (--no-comments); run figctl comments list "+fileKey+" --node "+data.Nodes[0].ID+" to read designer intent.")
	} else if len(data.Comments) == 0 {
		hints = append(hints, "No comment is pinned on this node or its children.")
	}
	hints = append(hints, "When the implementation is ready, run figctl render "+fileKey+" --node "+data.Nodes[0].ID+" and compare it with your result.")
	return hints
}

// writeContextTable writes a markdown table; empty rows are skipped so a
// section never renders a header with no body.
func writeContextTable(b *strings.Builder, columns []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	b.WriteString("| " + strings.Join(columns, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(columns)) + "\n")
	for _, row := range rows {
		cells := make([]string, len(columns))
		for i := range columns {
			if i < len(row) {
				cells[i] = strings.ReplaceAll(strings.ReplaceAll(row[i], "|", "\\|"), "\n", " ")
			}
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
}
