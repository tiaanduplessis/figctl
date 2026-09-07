package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

type devResourceRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	NodeID string `json:"nodeId"`
}

type devResourceList []devResourceRow

func (l devResourceList) Columns() []string { return []string{"id", "nodeId", "name", "url"} }

func (l devResourceList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, d := range l {
		rows = append(rows, []string{d.ID, d.NodeID, d.Name, d.URL})
	}
	return rows
}

var devresourcesCmd = &cobra.Command{
	Use:   "devresources",
	Short: "Dev Mode resource links attached to nodes",
}

var devresourcesListCmd = &cobra.Command{
	Use:   "list <ref>",
	Short: "List dev resources, optionally for specific nodes",
	Example: `  figctl devresources list KEY
  figctl devresources list KEY --node 3:11 --node 3:20`,
	Args: cobra.ExactArgs(1),
}

func init() {
	var nodes []string
	devresourcesListCmd.Flags().StringArrayVar(&nodes, "node", nil, "node id to filter by; repeatable")
	devresourcesListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		ids, err := parseNodeFlags(nodes, r)
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.GetDevResources(rctx, r.FileKey, ids)
		if err != nil {
			return err
		}
		session.Touch(r.FileKey)
		list := devResourceList{}
		for _, d := range resp.DevResources {
			list = append(list, devResourceRow{ID: d.ID, Name: d.Name, URL: d.URL, NodeID: d.NodeID})
		}
		return ctx.Print(list)
	})
	devresourcesCmd.AddCommand(devresourcesListCmd)
	rootCmd.AddCommand(devresourcesCmd)
}

// devResourceInput is one link read from --from-file or stdin. It is the
// shape a Storybook or component index would generate, so a design system can
// pipe its whole mapping in at once.
type devResourceInput struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	// ID selects an existing link for update. It is ignored by add.
	ID string `json:"id,omitempty"`
}

type devResourceWriteData struct {
	FileKey  string            `json:"fileKey"`
	Written  []devResourceRow  `json:"written"`
	Failures []devResourceFail `json:"failures,omitempty"`
	DryRun   bool              `json:"dryRun,omitempty"`
}

type devResourceFail struct {
	NodeID string `json:"nodeId,omitempty"`
	ID     string `json:"id,omitempty"`
	URL    string `json:"url,omitempty"`
	Error  string `json:"error"`
}

func (d devResourceWriteData) Columns() []string {
	return []string{"id", "nodeId", "name", "url"}
}

func (d devResourceWriteData) Rows() [][]string {
	rows := make([][]string, 0, len(d.Written))
	for _, w := range d.Written {
		rows = append(rows, []string{w.ID, w.NodeID, w.Name, w.URL})
	}
	return rows
}

// readDevResourceInputs loads links from a JSON file, or from stdin when the
// path is "-". The file is a JSON array of {nodeId, name, url} objects.
func readDevResourceInputs(ctx *Context, path string) ([]devResourceInput, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(ctx.Stdin)
	} else {
		raw, err = os.ReadFile(path) //nolint:gosec // the path is the user's own argument
	}
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeUsage, err, "reading links: "+err.Error()).
			WithHint("Pass a JSON file of [{\"nodeId\":\"1:2\",\"name\":\"Story\",\"url\":\"https://...\"}], or - to read it from stdin.")
	}
	var items []devResourceInput
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, figctl.Wrap(figctl.CodeUsage, err, "parsing links: "+err.Error()).
			WithHint("The file must be a JSON array of {\"nodeId\", \"name\", \"url\"} objects.")
	}
	if len(items) == 0 {
		return nil, figctl.New(figctl.CodeUsage, "no links to write").
			WithHint("The JSON array is empty.")
	}
	return items, nil
}

// finishDevResourceWrite prints the envelope and reports partial success when
// Figma rejected some links. Figma answers 200 for a bulk write even when
// individual links fail, so a plain success would hide them.
func finishDevResourceWrite(ctx *Context, data devResourceWriteData, verb string) error {
	env := ctx.Envelope(data)
	switch {
	case len(data.Failures) == 0:
		if data.DryRun {
			env.AddHint("Dry run: nothing was written. Re-run without --dry-run to " + verb + " " + strconv.Itoa(len(data.Written)) + " link(s).")
		} else {
			env.AddHint(strconv.Itoa(len(data.Written)) + " link(s) " + verb + "d; they appear in Dev Mode on the linked nodes.")
		}
		return ctx.Printer.Print(env)
	case len(data.Written) == 0:
		return figctl.Newf(figctl.CodeUsage, "no link could be %sd", verb).
			WithHint("A node holds at most 10 dev resources and rejects a URL it already carries. See details.failures.").
			WithDetails(map[string]any{"failures": data.Failures})
	default:
		env.AddHint(strconv.Itoa(len(data.Failures)) + " of " + strconv.Itoa(len(data.Written)+len(data.Failures)) +
			" link(s) were rejected; the rest were " + verb + "d. See data.failures.")
		return ctx.PrintPartial(env, figctl.Newf(figctl.CodePartial, "%d of %d links were rejected",
			len(data.Failures), len(data.Written)+len(data.Failures)))
	}
}

var devresourcesAddCmd = &cobra.Command{
	Use:   "add <ref>",
	Short: "Attach a link to a node, for example a Storybook story or a pull request",
	Long: `Attach dev resource links to nodes. The links appear in Figma's Dev Mode
next to the design, which is how a design system points a component at the code
that implements it.

Pass one link with --node and --url, or many with --from-file for a whole
component library at once. Figma allows at most 10 links per node and rejects a
URL a node already carries.`,
	Example: `  figctl devresources add KEY --node 3:11 --url https://storybook.example.com/?path=/story/button --name Storybook
  figctl devresources add KEY --from-file stories.json
  storybook-index --json | figctl devresources add KEY --from-file - --yes`,
	Args: cobra.ExactArgs(1),
}

var devresourcesUpdateCmd = &cobra.Command{
	Use:     "update <ref>",
	Short:   "Change the name or URL of existing links",
	Example: `  figctl devresources update KEY --id abc123 --url https://storybook.example.com/?path=/story/button--primary`,
	Args:    cobra.ExactArgs(1),
}

var devresourcesRemoveCmd = &cobra.Command{
	Use:     "remove <ref>",
	Short:   "Delete links from a file",
	Example: "  figctl devresources remove KEY --id abc123 --id def456",
	Args:    cobra.ExactArgs(1),
}

func init() {
	var (
		addNode, addURL, addName, addFrom string
		addDryRun                         bool
	)
	devresourcesAddCmd.Flags().StringVar(&addNode, "node", "", "node id to attach the link to")
	devresourcesAddCmd.Flags().StringVar(&addURL, "url", "", "link URL")
	devresourcesAddCmd.Flags().StringVar(&addName, "name", "", "link name shown in Dev Mode (defaults to the URL host)")
	devresourcesAddCmd.Flags().StringVar(&addFrom, "from-file", "", "JSON array of {nodeId, name, url}, or - for stdin")
	devresourcesAddCmd.Flags().BoolVar(&addDryRun, "dry-run", false, "show what would be written without writing")
	devresourcesAddCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		items, err := collectAddInputs(ctx, r.NodeID, addNode, addURL, addName, addFrom)
		if err != nil {
			return err
		}
		resources := make([]figma.NewDevResource, 0, len(items))
		preview := make([]devResourceRow, 0, len(items))
		for _, it := range items {
			resources = append(resources, figma.NewDevResource{
				Name: it.Name, URL: it.URL, FileKey: r.FileKey, NodeID: it.NodeID,
			})
			preview = append(preview, devResourceRow{NodeID: it.NodeID, Name: it.Name, URL: it.URL})
		}
		data := devResourceWriteData{FileKey: r.FileKey, Written: preview, DryRun: addDryRun}
		if addDryRun {
			return finishDevResourceWrite(ctx, data, "create")
		}
		if err := confirm(ctx, fmt.Sprintf("Attach %d link(s) to file %s in Figma?", len(resources), r.FileKey)); err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.PostDevResources(rctx, resources)
		if err != nil {
			return err
		}
		data.Written = data.Written[:0]
		for _, d := range resp.LinksCreated {
			data.Written = append(data.Written, devResourceRow{ID: d.ID, Name: d.Name, URL: d.URL, NodeID: d.NodeID})
		}
		for _, e := range resp.Errors {
			data.Failures = append(data.Failures, devResourceFail{NodeID: e.NodeID, Error: e.Error})
		}
		return finishDevResourceWrite(ctx, data, "create")
	})

	var (
		upID, upURL, upName, upFrom string
	)
	devresourcesUpdateCmd.Flags().StringVar(&upID, "id", "", "dev resource id to update")
	devresourcesUpdateCmd.Flags().StringVar(&upURL, "url", "", "new URL")
	devresourcesUpdateCmd.Flags().StringVar(&upName, "name", "", "new name")
	devresourcesUpdateCmd.Flags().StringVar(&upFrom, "from-file", "", "JSON array of {id, name, url}, or - for stdin")
	devresourcesUpdateCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		var items []devResourceInput
		switch {
		case upFrom != "":
			if items, err = readDevResourceInputs(ctx, upFrom); err != nil {
				return err
			}
		case upID != "":
			if upURL == "" && upName == "" {
				return figctl.New(figctl.CodeUsage, "nothing to update").
					WithHint("Pass --url, --name, or both.")
			}
			items = []devResourceInput{{ID: upID, URL: upURL, Name: upName}}
		default:
			return figctl.New(figctl.CodeUsage, "no dev resource selected").
				WithHint("Pass --id, or --from-file with a JSON array of {id, name, url}. Run figctl devresources list <ref> to see the ids.")
		}
		updates := make([]figma.DevResourceUpdate, 0, len(items))
		for _, it := range items {
			if it.ID == "" {
				return figctl.New(figctl.CodeUsage, "every link to update needs an id").
					WithHint("Run figctl devresources list <ref> to see the ids.")
			}
			updates = append(updates, figma.DevResourceUpdate{ID: it.ID, Name: it.Name, URL: it.URL})
		}
		if err := confirm(ctx, fmt.Sprintf("Update %d link(s) in file %s in Figma?", len(updates), r.FileKey)); err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.PutDevResources(rctx, updates)
		if err != nil {
			return err
		}
		data := devResourceWriteData{FileKey: r.FileKey}
		for _, d := range resp.LinksUpdated {
			data.Written = append(data.Written, devResourceRow{ID: d.ID, Name: d.Name, URL: d.URL, NodeID: d.NodeID})
		}
		for _, e := range resp.Errors {
			data.Failures = append(data.Failures, devResourceFail{ID: e.ID, Error: e.Error})
		}
		return finishDevResourceWrite(ctx, data, "update")
	})

	var rmIDs []string
	devresourcesRemoveCmd.Flags().StringArrayVar(&rmIDs, "id", nil, "dev resource id to delete; repeatable")
	devresourcesRemoveCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		if len(rmIDs) == 0 {
			return figctl.New(figctl.CodeUsage, "no dev resource selected").
				WithHint("Pass --id (repeatable). Run figctl devresources list <ref> to see the ids.")
		}
		if err := confirm(ctx, fmt.Sprintf("Delete %d link(s) from file %s in Figma? This cannot be undone.", len(rmIDs), r.FileKey)); err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		data := devResourceWriteData{FileKey: r.FileKey}
		for _, id := range rmIDs {
			if err := session.Client.DeleteDevResource(rctx, r.FileKey, id); err != nil {
				data.Failures = append(data.Failures, devResourceFail{ID: id, Error: figctl.From(err).Message})
				continue
			}
			data.Written = append(data.Written, devResourceRow{ID: id})
		}
		return finishDevResourceWrite(ctx, data, "delete")
	})

	devresourcesCmd.AddCommand(devresourcesAddCmd, devresourcesUpdateCmd, devresourcesRemoveCmd)
}

// collectAddInputs builds the links to create from the flags or the file,
// defaulting a missing name to the URL host so Dev Mode always shows something
// readable.
func collectAddInputs(ctx *Context, refNode, node, rawURL, name, from string) ([]devResourceInput, error) {
	var items []devResourceInput
	switch {
	case from != "":
		var err error
		if items, err = readDevResourceInputs(ctx, from); err != nil {
			return nil, err
		}
	case rawURL != "":
		id := node
		if id == "" {
			id = refNode
		}
		if id == "" {
			return nil, figctl.New(figctl.CodeUsage, "no node selected").
				WithHint("Pass --node, or a Figma URL that carries a node-id.")
		}
		items = []devResourceInput{{NodeID: id, URL: rawURL, Name: name}}
	default:
		return nil, figctl.New(figctl.CodeUsage, "no link to attach").
			WithHint("Pass --node with --url, or --from-file with a JSON array of {nodeId, name, url}.")
	}
	for i := range items {
		parsed, err := parseNodeArg(items[i].NodeID)
		if err != nil {
			return nil, err
		}
		items[i].NodeID = parsed
		if items[i].URL == "" {
			return nil, figctl.Newf(figctl.CodeUsage, "link for node %s has no url", items[i].NodeID)
		}
		if items[i].Name == "" {
			items[i].Name = defaultLinkName(items[i].URL)
		}
	}
	return items, nil
}

// defaultLinkName falls back to the URL host, which reads better in Dev Mode
// than an empty label.
func defaultLinkName(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
