package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/output"
)

type pageInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Children int    `json:"children"`
}

type branchInfo struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	LastModified string `json:"lastModified"`
}

type fileInfo struct {
	Key           string       `json:"key"`
	Name          string       `json:"name"`
	EditorType    string       `json:"editorType"`
	Version       string       `json:"version"`
	LastModified  string       `json:"lastModified"`
	Role          string       `json:"role"`
	LinkAccess    string       `json:"linkAccess,omitempty"`
	ThumbnailURL  string       `json:"thumbnailUrl,omitempty"`
	MainFileKey   string       `json:"mainFileKey,omitempty"`
	Pages         []pageInfo   `json:"pages"`
	Branches      []branchInfo `json:"branches,omitempty"`
	Components    int          `json:"components"`
	ComponentSets int          `json:"componentSets"`
	Styles        int          `json:"styles"`
}

func (f fileInfo) Columns() []string { return []string{"field", "value"} }

func (f fileInfo) Rows() [][]string {
	kv := output.KeyValues{
		{"key", f.Key},
		{"name", f.Name},
		{"editorType", f.EditorType},
		{"version", f.Version},
		{"lastModified", f.LastModified},
		{"role", f.Role},
	}
	if f.LinkAccess != "" {
		kv = append(kv, [2]string{"linkAccess", f.LinkAccess})
	}
	if f.MainFileKey != "" {
		kv = append(kv, [2]string{"mainFileKey", f.MainFileKey})
	}
	pages := make([]string, 0, len(f.Pages))
	for _, p := range f.Pages {
		pages = append(pages, fmt.Sprintf("%s %s (%d children)", p.ID, p.Name, p.Children))
	}
	kv = append(kv, [2]string{"pages", strings.Join(pages, "; ")})
	if len(f.Branches) > 0 {
		branches := make([]string, 0, len(f.Branches))
		for _, b := range f.Branches {
			branches = append(branches, b.Name+" ("+b.Key+")")
		}
		kv = append(kv, [2]string{"branches", strings.Join(branches, "; ")})
	}
	kv = append(kv,
		[2]string{"components", strconv.Itoa(f.Components)},
		[2]string{"componentSets", strconv.Itoa(f.ComponentSets)},
		[2]string{"styles", strconv.Itoa(f.Styles)},
	)
	if f.ThumbnailURL != "" {
		kv = append(kv, [2]string{"thumbnailUrl", f.ThumbnailURL})
	}
	return kv.Rows()
}

var fileCmd = &cobra.Command{
	Use:   "file",
	Short: "Read a Figma file: info, tree, find, and raw JSON",
}

var fileInfoCmd = &cobra.Command{
	Use:   "info <ref>",
	Short: "Show file name, pages, version, editor type, role, and branches",
	Example: `  figctl file info https://www.figma.com/design/KEY/Web-App
  figctl file info KEY --json --fields name,pages`,
	Args: cobra.ExactArgs(1),
}

var fileGetCmd = &cobra.Command{
	Use:   "get <ref>",
	Short: "Print raw Figma API JSON for the document or specific nodes (escape hatch)",
	Long: `Print the JSON of the whole document, or of the nodes given with --node,
exactly as the Figma REST API returns it. Nothing is re-modelled, so every
property Figma sends is present, including ones figctl does not understand.
The document is fetched once per file version and cached, so repeated
calls with --node cost no extra API requests. A node-id in the ref URL is
used when no --node is given. --depth, --geometry, and --plugin-data are
passed to the API; --fields keeps only the named top level keys.`,
	Example: `  figctl file get KEY --node 1:2 --depth 2
  figctl file get "https://www.figma.com/design/KEY/App?node-id=1-2" --fields document
  figctl file get KEY --node 1:2 --geometry paths
  figctl file get KEY --depth 1 --fields document`,
	Args: cobra.ExactArgs(1),
}

func init() {
	fileInfoCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		file, err := session.Overview(rctx, r.FileKey)
		if err != nil {
			return err
		}
		info := fileInfo{
			Key:           r.FileKey,
			Name:          file.Name,
			EditorType:    file.EditorType,
			Version:       file.Version,
			LastModified:  file.LastModified,
			Role:          file.Role,
			LinkAccess:    file.LinkAccess,
			ThumbnailURL:  file.ThumbnailURL,
			MainFileKey:   file.MainFileKey,
			Pages:         []pageInfo{},
			Components:    len(file.Components),
			ComponentSets: len(file.ComponentSets),
			Styles:        len(file.Styles),
		}
		if file.Document != nil {
			for _, page := range file.Document.Children {
				info.Pages = append(info.Pages, pageInfo{ID: page.ID, Name: page.Name, Children: len(page.Children)})
			}
		}
		for _, b := range file.Branches {
			info.Branches = append(info.Branches, branchInfo{Key: b.Key, Name: b.Name, LastModified: b.LastModified})
		}
		env := ctx.Envelope(info)
		env.AddHint("Next: figctl file tree " + r.FileKey + " for the outline, then figctl node context " + r.FileKey + " --node <id>.")
		return ctx.Printer.Print(env)
	})

	var nodes []string
	var depth int
	var geometry, pluginData string
	fileGetCmd.Flags().StringArrayVar(&nodes, "node", nil, "node id (1:2, 1-2, or a URL with node-id); repeatable")
	fileGetCmd.Flags().IntVar(&depth, "depth", 0, "levels of children to include (0 for all)")
	fileGetCmd.Flags().StringVar(&geometry, "geometry", "", "set to paths to include vector geometry")
	fileGetCmd.Flags().StringVar(&pluginData, "plugin-data", "", "plugin ids (comma separated, or shared) whose data to include")
	fileGetCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
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
		if geometry != "" && geometry != "paths" {
			return figctl.Newf(figctl.CodeUsage, "invalid --geometry %q", geometry).WithHint("The only supported value is paths.")
		}
		rctx, cancel := session.Context()
		defer cancel()
		if len(ids) > 0 {
			nodes, err := session.NodesRaw(rctx, r.FileKey, ids, figma.NodesOptions{Depth: depth, Geometry: geometry, PluginData: pluginData})
			if err != nil {
				return err
			}
			env := ctx.Envelope(nodes)
			for _, id := range ids {
				if nodes[id] == nil {
					env.AddHint("Node " + id + " does not exist in file " + r.FileKey + "; check the id with figctl file tree.")
				}
			}
			return ctx.Printer.Print(env)
		}
		if depth > 0 || geometry != "" || pluginData != "" {
			raw, err := session.FileRawWith(rctx, r.FileKey, figma.FileOptions{Depth: depth, Geometry: geometry, PluginData: pluginData})
			if err != nil {
				return err
			}
			return ctx.Print(raw)
		}
		raw, err := session.FileRaw(rctx, r.FileKey)
		if errors.Is(err, errTooLarge) {
			return figctl.Newf(figctl.CodeUsage, "file %s is larger than --max-file-mb (%d MB)", r.FileKey, ctx.Flags.MaxFileMB).
				WithHint("Narrow the request with --node <id> or --depth N, or raise --max-file-mb.")
		}
		if err != nil {
			return err
		}
		env := ctx.Envelope(raw)
		env.AddHint("The whole document is large; prefer figctl file tree and figctl node inspect for token-efficient views.")
		return ctx.Printer.Print(env)
	})

	fileCmd.AddCommand(fileInfoCmd, fileGetCmd)
	rootCmd.AddCommand(fileCmd)
}
