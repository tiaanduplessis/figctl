package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/output"
)

type componentRow struct {
	Key                string            `json:"key"`
	NodeID             string            `json:"nodeId"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	SetName            string            `json:"setName,omitempty"`
	SetNodeID          string            `json:"setNodeId,omitempty"`
	VariantProperties  map[string]string `json:"variantProperties,omitempty"`
	Page               string            `json:"page,omitempty"`
	Frame              string            `json:"frame,omitempty"`
	DocumentationLinks []string          `json:"documentationLinks"`
	Remote             bool              `json:"remote"`
	Published          bool              `json:"published"`
	FileKey            string            `json:"fileKey,omitempty"`
}

type componentList []componentRow

func (l componentList) Columns() []string {
	return []string{"key", "nodeId", "name", "setName", "variants", "page", "description"}
}

func (l componentList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, c := range l {
		rows = append(rows, []string{c.Key, c.NodeID, c.Name, c.SetName, variantString(c.VariantProperties), c.Page, truncate(c.Description, 80)})
	}
	return rows
}

// componentSetInfo is what is known about a component set, merged from
// the document and the published list.
type componentSetInfo struct {
	Key                string
	Name               string
	Description        string
	DocumentationLinks []string
	Remote             bool
	Published          bool
	Page               string
	Frame              string
}

// componentCatalog merges the published component lists of a file with
// the components and component sets of its cached document.
type componentCatalog struct {
	rows   []componentRow
	byNode map[string]componentRow
	sets   map[string]componentSetInfo
}

// componentNodeInfo locates a component node in the document.
type componentNodeInfo struct {
	page  string
	frame string
}

// indexComponentNodes records the page and parent of every component and
// component set node in a document.
func indexComponentNodes(doc *figma.Node) map[string]componentNodeInfo {
	index := map[string]componentNodeInfo{}
	if doc == nil {
		return index
	}
	for _, canvas := range doc.Children {
		canvas.Walk(func(n *figma.Node, parent *figma.Node, _ int) bool {
			if n.Type == "COMPONENT" || n.Type == "COMPONENT_SET" {
				info := componentNodeInfo{page: canvas.Name}
				if parent != nil && parent.Type != "CANVAS" {
					info.frame = parent.Name
				}
				index[n.ID] = info
			}
			return true
		})
	}
	return index
}

func rowFromPublished(c figma.PublishedComponent) componentRow {
	row := componentRow{
		Key:                c.Key,
		NodeID:             c.NodeID,
		Name:               c.Name,
		Description:        c.Description,
		VariantProperties:  parseVariantProperties(c.Name),
		DocumentationLinks: []string{},
		Published:          true,
	}
	if cf := c.ContainingFrame; cf != nil {
		row.Page = cf.PageName
		row.Frame = cf.Name
		if cf.ContainingStateGroup != nil {
			row.SetName = cf.ContainingStateGroup.Name
			row.SetNodeID = cf.ContainingStateGroup.NodeID
		}
	}
	return row
}

func documentationLinks(links []figma.DocumentationLink) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		if l.URI != "" {
			out = append(out, l.URI)
		}
	}
	return out
}

// parseVariantProperties reads "Variant=Primary, Size=md" into a map. A
// name where any comma separated part lacks "=" is not a variant name.
func parseVariantProperties(name string) map[string]string {
	if !strings.Contains(name, "=") {
		return nil
	}
	props := map[string]string{}
	for _, part := range strings.Split(name, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil
		}
		props[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return props
}

func variantString(props map[string]string) string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+props[k])
	}
	return strings.Join(parts, ", ")
}

// loadComponentCatalog fetches the published components and component sets
// of a file and merges them with the cached document when it is available.
// Components that exist in the document but are not published are appended
// after the published ones.
func (s *Session) loadComponentCatalog(ctx context.Context, key string) (*componentCatalog, error) {
	published, err := cached(ctx, s, key, "components", func(ctx context.Context) (*figma.ComponentsResponse, error) {
		return s.Client.GetFileComponents(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	publishedSets, err := cached(ctx, s, key, "component_sets", func(ctx context.Context) (*figma.ComponentSetsResponse, error) {
		return s.Client.GetFileComponentSets(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	s.Touch(key)
	cat := &componentCatalog{byNode: map[string]componentRow{}, sets: map[string]componentSetInfo{}}
	for _, set := range publishedSets.Meta.ComponentSets {
		info := componentSetInfo{Key: set.Key, Name: set.Name, Description: set.Description, DocumentationLinks: []string{}, Published: true}
		if set.ContainingFrame != nil {
			info.Page = set.ContainingFrame.PageName
			info.Frame = set.ContainingFrame.Name
		}
		cat.sets[set.NodeID] = info
	}
	var doc *figma.GetFileResponse
	if file, err := s.File(ctx, key); err == nil {
		doc = file
	} else if !errors.Is(err, errTooLarge) {
		s.ctx.Log.Debugf("component catalog for %s uses the published lists only: %v", key, err)
	}
	var index map[string]componentNodeInfo
	if doc != nil {
		index = indexComponentNodes(doc.Document)
		for id, set := range doc.ComponentSets {
			info := cat.sets[id]
			info.Key, info.Name, info.Description = set.Key, set.Name, set.Description
			info.DocumentationLinks = documentationLinks(set.DocumentationLinks)
			info.Remote = set.Remote
			if loc, ok := index[id]; ok {
				info.Page, info.Frame = loc.page, loc.frame
			}
			cat.sets[id] = info
		}
	}
	add := func(row componentRow) {
		if doc != nil {
			if dc, ok := doc.Components[row.NodeID]; ok {
				row.DocumentationLinks = documentationLinks(dc.DocumentationLinks)
				row.Remote = dc.Remote
				if dc.ComponentSetID != "" {
					row.SetNodeID = dc.ComponentSetID
					if set, ok := cat.sets[dc.ComponentSetID]; ok {
						row.SetName = set.Name
					}
				}
			}
			if loc, ok := index[row.NodeID]; ok {
				row.Page, row.Frame = loc.page, loc.frame
			}
		}
		cat.rows = append(cat.rows, row)
		cat.byNode[row.NodeID] = row
	}
	for _, c := range published.Meta.Components {
		add(rowFromPublished(c))
	}
	if doc != nil {
		ids := make([]string, 0, len(doc.Components))
		for id := range doc.Components {
			if _, seen := cat.byNode[id]; !seen {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			dc := doc.Components[id]
			add(componentRow{
				Key:                dc.Key,
				NodeID:             id,
				Name:               dc.Name,
				Description:        dc.Description,
				VariantProperties:  parseVariantProperties(dc.Name),
				DocumentationLinks: []string{},
			})
		}
	}
	return cat, nil
}

var componentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List and inspect components, component sets, and variant properties",
}

var componentsListCmd = &cobra.Command{
	Use:     "list [ref]",
	Aliases: []string{"search"},
	Short:   "List the components of a file or a team library",
	Long: `List components with their keys, node ids, component set, variant
properties parsed from the name, page, documentation links, and whether
they are remote. For a file the published components are merged with the
components of the cached document, so unpublished ones appear too with
"published": false. With --team the team library endpoint is paged with
--limit and --cursor. --query keeps components whose name, description, or
set name contains the text. "components search" is an alias of
"components list".`,
	Example: `  figctl components list KEY
  figctl components search KEY --query button
  figctl components list --team 555000111 --limit 50
  figctl components list --team 555000111 --cursor 50`,
	Args: cobra.MaximumNArgs(1),
}

var componentsGetCmd = &cobra.Command{
	Use:   "get <ref>",
	Short: "Show one component or component set with its property definitions and variants",
	Long: `Show a component or component set by node id or by published key. A
component set lists its componentPropertyDefinitions (type, default,
variant options, preferred values) and its variant components. A component
shows its set, its size, and its own definitions when it has any.`,
	Example: `  figctl components get KEY --node 3:10
  figctl components get KEY --key a1b2c3d4e5f60718293a4b5c6d7e8f9012345311
  figctl components get "https://www.figma.com/design/KEY/App?node-id=3-20" -o md`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var team, query string
	componentsListCmd.Flags().StringVar(&team, "team", "", "list a team library instead of a file")
	componentsListCmd.Flags().StringVar(&query, "query", "", "keep components whose name, description, or set contains this text (case-insensitive)")
	componentsListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if (len(args) == 0) == (team == "") {
			return figctl.New(figctl.CodeUsage, "pass a file ref or --team <id>, not both").
				WithHint("figctl components list KEY lists a file; figctl components list --team 123 lists a team library.")
		}
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		if team != "" {
			return listTeamComponents(rctx, ctx, session, team, query)
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		cat, err := session.loadComponentCatalog(rctx, r.FileKey)
		if err != nil {
			return err
		}
		rows := filterComponents(cat.rows, query)
		pageRows, next, err := paginate(rows, ctx.Flags.Limit, ctx.Flags.Cursor)
		if err != nil {
			return err
		}
		env := ctx.Envelope(componentList(pageRows))
		if next != nil {
			env.Truncated = true
			env.NextCursor = next
		}
		if len(rows) == 0 {
			env.AddHint("No components matched. Drop --query, or run figctl file find " + r.FileKey + " --type COMPONENT,COMPONENT_SET to see unpublished ones by node.")
		} else {
			env.AddHint("Next: figctl components get " + r.FileKey + " --node <nodeId> for property definitions and variants.")
		}
		return ctx.Printer.Print(env)
	})

	var node, key string
	componentsGetCmd.Flags().StringVar(&node, "node", "", "component or component set node id (1:2, 1-2, or a URL with node-id)")
	componentsGetCmd.Flags().StringVar(&key, "key", "", "published component or component set key")
	componentsGetCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if node != "" && key != "" {
			return figctl.New(figctl.CodeUsage, "pass either --node or --key").WithHint("Use --node for a node id in the file, --key for a published library key.")
		}
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		fileKey, nodeID := r.FileKey, ""
		var published *figma.PublishedComponent
		if key != "" {
			published, err = fetchPublishedByKey(rctx, session, key)
			if err != nil {
				return err
			}
			nodeID = published.NodeID
			if published.FileKey != "" {
				fileKey = published.FileKey
			}
		} else {
			ids, err := parseNodeFlags(nonEmpty(node), r)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return figctl.New(figctl.CodeUsage, "components get needs --node or --key").
					WithHint("%s", "Find node ids with figctl components list "+r.FileKey+" or figctl file find "+r.FileKey+" --type COMPONENT,COMPONENT_SET.")
			}
			nodeID = ids[0]
		}
		nodes, err := session.Nodes(rctx, fileKey, []string{nodeID}, figma.NodesOptions{Depth: 2})
		if err != nil {
			return err
		}
		entry := nodes.Nodes[nodeID]
		if entry == nil || entry.Document == nil {
			return nodeNotFound(nodeID, fileKey)
		}
		n := entry.Document
		if n.Type != "COMPONENT" && n.Type != "COMPONENT_SET" {
			return figctl.Newf(figctl.CodeUsage, "node %s is a %s, not a component or component set", nodeID, n.Type).
				WithHint("%s", "Find components with figctl file find "+fileKey+" --type COMPONENT,COMPONENT_SET, or inspect this node with figctl node inspect "+fileKey+" --node "+nodeID+".")
		}
		cat, err := session.loadComponentCatalog(rctx, fileKey)
		if err != nil {
			return err
		}
		detail := buildComponentDetail(cat, n, published)
		env := ctx.Envelope(detail)
		if n.Type == "COMPONENT_SET" {
			env.AddHint("Next: figctl components get " + fileKey + " --node <variant nodeId> for one variant, or figctl node context " + fileKey + " --node " + nodeID + " to implement the set.")
		} else {
			env.AddHint("Next: figctl node context " + fileKey + " --node " + nodeID + " for layout, styles, tokens, and a screenshot.")
		}
		return ctx.Printer.Print(env)
	})

	componentsCmd.AddCommand(componentsListCmd, componentsGetCmd)
	rootCmd.AddCommand(componentsCmd)
}

func listTeamComponents(rctx context.Context, ctx *Context, session *Session, team, query string) error {
	resp, err := session.Client.GetTeamComponents(rctx, team, figma.PageOptions{PageSize: ctx.Flags.Limit, After: ctx.Flags.Cursor})
	if err != nil {
		return err
	}
	rows := make([]componentRow, 0, len(resp.Meta.Components))
	for _, c := range resp.Meta.Components {
		row := rowFromPublished(c)
		row.FileKey = c.FileKey
		rows = append(rows, row)
	}
	rows = filterComponents(rows, query)
	env := ctx.Envelope(componentList(rows))
	if resp.Meta.Cursor != nil && resp.Meta.Cursor.After != nil {
		next := strconv.Itoa(*resp.Meta.Cursor.After)
		env.NextCursor = &next
		env.Truncated = true
	}
	if query != "" {
		env.AddHint("--query filters the current page only; page through with --cursor to search the whole library.")
	}
	env.AddHint("Next: figctl components get <fileKey> --key <key> for property definitions and variants.")
	return ctx.Printer.Print(env)
}

func filterComponents(rows []componentRow, query string) []componentRow {
	if query == "" {
		return rows
	}
	q := strings.ToLower(query)
	out := make([]componentRow, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), q) || strings.Contains(strings.ToLower(row.Description), q) || strings.Contains(strings.ToLower(row.SetName), q) {
			out = append(out, row)
		}
	}
	return out
}

// fetchPublishedByKey resolves a library key as a component first and as
// a component set second.
func fetchPublishedByKey(ctx context.Context, session *Session, key string) (*figma.PublishedComponent, error) {
	component, err := session.Client.GetComponent(ctx, key)
	if err == nil {
		return &component.Meta, nil
	}
	if figctl.From(err).Code != figctl.CodeNotFound {
		return nil, err
	}
	set, err := session.Client.GetComponentSet(ctx, key)
	if err == nil {
		return &set.Meta, nil
	}
	if figctl.From(err).Code != figctl.CodeNotFound {
		return nil, err
	}
	return nil, figctl.Newf(figctl.CodeNotFound, "no published component or component set with key %s", key).
		WithHint("Keys come from figctl components list; for an unpublished component use --node <id> instead.")
}

type propertyDefinition struct {
	Type            string                             `json:"type"`
	Default         any                                `json:"default"`
	VariantOptions  []string                           `json:"variantOptions,omitempty"`
	PreferredValues []figma.InstanceSwapPreferredValue `json:"preferredValues,omitempty"`
}

type variantRow struct {
	NodeID            string            `json:"nodeId"`
	Key               string            `json:"key,omitempty"`
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	VariantProperties map[string]string `json:"variantProperties,omitempty"`
	W                 *int              `json:"w,omitempty"`
	H                 *int              `json:"h,omitempty"`
}

type componentDetail struct {
	componentRow
	Type                string                        `json:"type"`
	SetKey              string                        `json:"setKey,omitempty"`
	W                   *int                          `json:"w,omitempty"`
	H                   *int                          `json:"h,omitempty"`
	PropertyDefinitions map[string]propertyDefinition `json:"propertyDefinitions,omitempty"`
	Variants            []variantRow                  `json:"variants,omitempty"`
}

func (d componentDetail) Columns() []string { return []string{"field", "value"} }

func (d componentDetail) Rows() [][]string {
	kv := output.KeyValues{
		{"key", d.Key},
		{"nodeId", d.NodeID},
		{"type", d.Type},
		{"name", d.Name},
		{"description", d.Description},
	}
	if d.SetName != "" {
		kv = append(kv, [2]string{"set", d.SetName + " (" + d.SetNodeID + ")"})
	}
	if len(d.VariantProperties) > 0 {
		kv = append(kv, [2]string{"variantProperties", variantString(d.VariantProperties)})
	}
	if d.Page != "" {
		kv = append(kv, [2]string{"page", d.Page})
	}
	if d.Frame != "" {
		kv = append(kv, [2]string{"frame", d.Frame})
	}
	if size := sizeString(d.W, d.H); size != "" {
		kv = append(kv, [2]string{"size", size})
	}
	kv = append(kv,
		[2]string{"documentationLinks", strings.Join(d.DocumentationLinks, "; ")},
		[2]string{"remote", strconv.FormatBool(d.Remote)},
		[2]string{"published", strconv.FormatBool(d.Published)},
	)
	if len(d.PropertyDefinitions) > 0 {
		names := make([]string, 0, len(d.PropertyDefinitions))
		for name := range d.PropertyDefinitions {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			def := d.PropertyDefinitions[name]
			part := fmt.Sprintf("%s: %s default %v", name, def.Type, def.Default)
			if len(def.VariantOptions) > 0 {
				part += " options " + strings.Join(def.VariantOptions, "|")
			}
			parts = append(parts, part)
		}
		kv = append(kv, [2]string{"propertyDefinitions", strings.Join(parts, "; ")})
	}
	if len(d.Variants) > 0 {
		parts := make([]string, 0, len(d.Variants))
		for _, v := range d.Variants {
			part := v.NodeID + " " + v.Name
			if size := sizeString(v.W, v.H); size != "" {
				part += " " + size
			}
			parts = append(parts, part)
		}
		kv = append(kv, [2]string{"variants", strings.Join(parts, "; ")})
	}
	return kv.Rows()
}

func definitionsOf(n *figma.Node) map[string]propertyDefinition {
	if len(n.ComponentPropertyDefinitions) == 0 {
		return nil
	}
	defs := make(map[string]propertyDefinition, len(n.ComponentPropertyDefinitions))
	for name, def := range n.ComponentPropertyDefinitions {
		defs[name] = propertyDefinition{Type: def.Type, Default: def.DefaultValue, VariantOptions: def.VariantOptions, PreferredValues: def.PreferredValues}
	}
	return defs
}

// buildComponentDetail joins the node with the catalog entry, falling
// back to the published record given with --key and then to the node
// itself.
func buildComponentDetail(cat *componentCatalog, n *figma.Node, published *figma.PublishedComponent) componentDetail {
	detail := componentDetail{Type: n.Type, PropertyDefinitions: definitionsOf(n)}
	_, _, detail.W, detail.H = bounds(n)
	if n.Type == "COMPONENT_SET" {
		detail.componentRow = componentRow{NodeID: n.ID, Name: n.Name, DocumentationLinks: []string{}}
		if published != nil {
			detail.componentRow = rowFromPublished(*published)
		}
		if set, ok := cat.sets[n.ID]; ok {
			detail.Key, detail.Name, detail.Description = set.Key, set.Name, set.Description
			detail.DocumentationLinks, detail.Remote, detail.Published = set.DocumentationLinks, set.Remote, set.Published
			if set.Page != "" {
				detail.Page, detail.Frame = set.Page, set.Frame
			}
		}
		detail.Variants = []variantRow{}
		for _, child := range n.Children {
			if child.Type != "COMPONENT" {
				continue
			}
			v := variantRow{NodeID: child.ID, Name: child.Name, VariantProperties: parseVariantProperties(child.Name)}
			_, _, v.W, v.H = bounds(child)
			if row, ok := cat.byNode[child.ID]; ok {
				v.Key, v.Description = row.Key, row.Description
			}
			detail.Variants = append(detail.Variants, v)
		}
		return detail
	}
	detail.componentRow = componentRow{NodeID: n.ID, Name: n.Name, VariantProperties: parseVariantProperties(n.Name), DocumentationLinks: []string{}}
	if published != nil {
		detail.componentRow = rowFromPublished(*published)
	}
	if row, ok := cat.byNode[n.ID]; ok {
		detail.componentRow = row
	}
	if set, ok := cat.sets[detail.SetNodeID]; ok {
		detail.SetKey = set.Key
		if detail.SetName == "" {
			detail.SetName = set.Name
		}
	}
	return detail
}
