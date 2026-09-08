package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/assets"
	"github.com/tiaanduplessis/figctl/internal/figctl"
)

const manifestFileName = "manifest.json"

// Asset categories.
const (
	categoryIcon   = "icon"
	categoryExport = "export"
	categoryFill   = "imageFill"
)

// fillEntry is one image fill in the assets manifest.
type fillEntry struct {
	ImageRef  string           `json:"imageRef"`
	ScaleMode string           `json:"scaleMode,omitempty"`
	Nodes     []assets.FillUse `json:"nodes"`
	Path      string           `json:"path,omitempty"`
	Bytes     int64            `json:"bytes"`
	Cached    bool             `json:"cached"`
	Error     string           `json:"error,omitempty"`
}

// assetFailure is one asset that produced no file.
type assetFailure struct {
	Category string `json:"category"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Error    string `json:"error"`
}

// assetsData is the manifest printed by assets export.
type assetsData struct {
	OutDir       string          `json:"outDir"`
	DryRun       bool            `json:"dryRun"`
	Icons        []assets.Result `json:"icons"`
	Exports      []assets.Result `json:"exports"`
	ImageFills   []fillEntry     `json:"imageFills"`
	Failures     []assetFailure  `json:"failures"`
	Requests     int             `json:"requests"`
	ManifestPath string          `json:"manifestPath,omitempty"`
}

func (d assetsData) Columns() []string {
	return []string{"category", "id", "name", "format", "scale", "path", "cached", "error"}
}

func (d assetsData) Rows() [][]string {
	var rows [][]string
	add := func(category string, r assets.Result) {
		row := resultRow(r)
		rows = append(rows, []string{category, row[0], row[1], row[2], row[3], row[4], row[6], row[7]})
	}
	for _, r := range d.Icons {
		add(categoryIcon, r)
	}
	for _, r := range d.Exports {
		add(categoryExport, r)
	}
	for _, f := range d.ImageFills {
		rows = append(rows, []string{categoryFill, f.ImageRef, fillUsers(f.Nodes), filepath.Ext(f.Path), "", f.Path, yesNo(f.Cached), f.Error})
	}
	return rows
}

func fillUsers(uses []assets.FillUse) string {
	names := make([]string, 0, len(uses))
	for _, u := range uses {
		names = append(names, u.Name+" ("+u.NodeID+")")
	}
	return strings.Join(names, ", ")
}

// candidate is one row of assets list.
type candidate struct {
	Category string `json:"category"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Formats  string `json:"formats"`
}

type candidateList []candidate

func (l candidateList) Columns() []string {
	return []string{"category", "id", "name", "type", "formats"}
}

func (l candidateList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, c := range l {
		rows = append(rows, []string{c.Category, c.ID, c.Name, c.Type, c.Formats})
	}
	return rows
}

// manifestEntry is one file in manifest.json.
type manifestEntry struct {
	Category string  `json:"category"`
	Name     string  `json:"name,omitempty"`
	Format   string  `json:"format"`
	Scale    float64 `json:"scale"`
	Path     string  `json:"path"`
}

// manifestFile is the on-disk manifest.json.
type manifestFile struct {
	File      string                     `json:"file"`
	Version   string                     `json:"version,omitempty"`
	Nodes     map[string][]manifestEntry `json:"nodes"`
	ImageRefs map[string]string          `json:"imageRefs"`
}

var assetsCmd = &cobra.Command{
	Use:   "assets",
	Short: "Find and export icons, export-marked layers, and image fills",
}

var assetsListCmd = &cobra.Command{
	Use:   "list <ref>",
	Short: "List export candidates under a node without rendering anything",
	Long: `List the assets found under the given nodes (the whole file when no --node is
given): layers with export settings, icon-like nodes (name matches an icon
pattern, or a frame made only of vector shapes), and raster image fills.
Nothing is rendered or downloaded.`,
	Example: `  figctl assets list KEY
  figctl assets list KEY --node 2:2 --icon-pattern "ic/*" -o table`,
	Args: cobra.MaximumNArgs(1),
}

var assetsExportCmd = &cobra.Command{
	Use:   "export <ref>",
	Short: "Export icons as SVG, export-marked layers, and image fills to a directory",
	Long: `Discover assets under the given nodes (the whole file when no --node is
given) and write them to the output directory: icons as SVG (or --format
png), export-marked layers at the format, scale, and suffix stored in Figma,
and raster image fills as imageref-<ref>.<ext>. A manifest.json mapping
node ids and image refs to paths is written next to the files, and the same
manifest is printed.

Renders are batched per format and scale and cached by file version, so a
repeated export of an unchanged file costs no image requests. --dry-run
prints the plan without any image request.`,
	Example: `  figctl assets export KEY --node 2:2 --out ./src/assets
  figctl assets export KEY --icons-only --svg-current-color --svg-strip-dimensions
  figctl assets export KEY --dry-run`,
	Args: cobra.MaximumNArgs(1),
}

type discoverFlags struct {
	nodes         []string
	iconPatterns  []string
	includeHidden bool
}

func addDiscoverFlags(cmd *cobra.Command, d *discoverFlags) {
	cmd.Flags().StringArrayVar(&d.nodes, "node", nil, "node id (1:2, 1-2, or a URL with node-id) to search under; repeatable")
	cmd.Flags().StringArrayVar(&d.iconPatterns, "icon-pattern", nil, "name glob that marks an icon (default icon/*, Icon*, icons/*); repeatable")
	cmd.Flags().BoolVar(&d.includeHidden, "include-hidden", false, "include hidden nodes")
}

func (d *discoverFlags) options(icons, exports, fills bool) (assets.DiscoverOptions, error) {
	if err := assets.ValidateIconPatterns(d.iconPatterns); err != nil {
		return assets.DiscoverOptions{}, err
	}
	return assets.DiscoverOptions{
		IconPatterns:          d.iconPatterns,
		IncludeIcons:          icons,
		IncludeExportSettings: exports,
		IncludeImageFills:     fills,
		IncludeHidden:         d.includeHidden,
	}, nil
}

func init() {
	var listFlags discoverFlags
	addDiscoverFlags(assetsListCmd, &listFlags)
	assetsListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		ids, err := parseNodeFlags(listFlags.nodes, r)
		if err != nil {
			return err
		}
		opts, err := listFlags.options(true, true, true)
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		roots, _, err := subtrees(rctx, session, r.FileKey, ids, ctx.Flags.MaxFileMB)
		if err != nil {
			return err
		}
		d := assets.Discover(roots, opts)
		list := candidateList{}
		for _, icon := range d.Icons {
			list = append(list, candidate{Category: categoryIcon, ID: icon.NodeID, Name: icon.Name, Type: icon.Type, Formats: "svg"})
		}
		byNode := map[string][]string{}
		var exportOrder []assets.Export
		for _, e := range d.Exports {
			if _, seen := byNode[e.NodeID]; !seen {
				exportOrder = append(exportOrder, e)
			}
			byNode[e.NodeID] = append(byNode[e.NodeID], e.Format+assets.ScaleSuffix(e.Scale))
		}
		for _, e := range exportOrder {
			list = append(list, candidate{Category: categoryExport, ID: e.NodeID, Name: e.Name, Type: e.Type, Formats: strings.Join(byNode[e.NodeID], ", ")})
		}
		for _, f := range d.ImageFills {
			list = append(list, candidate{Category: categoryFill, ID: f.ImageRef, Name: fillUsers(f.Nodes), Type: "IMAGE", Formats: strings.ToLower(f.ScaleMode)})
		}
		env := ctx.Envelope(list)
		if len(list) == 0 {
			env.AddHint("No assets found; widen the search with --icon-pattern or --include-hidden, or drop --node.")
		} else {
			env.AddHint("Export them with figctl assets export " + r.FileKey + " (add --node to narrow, --dry-run to preview).")
		}
		return ctx.Printer.Print(env)
	})

	var exportFlags discoverFlags
	var outDir, format string
	var scale float64
	var concurrency int
	var iconsOnly, fillsOnly, exportsOnly, dryRun bool
	var clean assets.SVGCleanOptions
	addDiscoverFlags(assetsExportCmd, &exportFlags)
	f := assetsExportCmd.Flags()
	f.StringVar(&outDir, "out", "", "output directory (default ./.figctl/<fileKey>/assets)")
	f.BoolVar(&iconsOnly, "icons-only", false, "export icons only")
	f.BoolVar(&fillsOnly, "fills-only", false, "download image fills only")
	f.BoolVar(&exportsOnly, "exports-only", false, "export layers with export settings only")
	f.StringVar(&format, "format", "svg", "icon format: svg or png")
	f.Float64Var(&scale, "scale", 0, "icon scale (default 1 for svg, 2 for png)")
	f.BoolVar(&clean.StripIDs, "svg-strip-ids", false, "svg: remove unreferenced id attributes")
	f.BoolVar(&clean.StripDimensions, "svg-strip-dimensions", false, "svg: remove root width and height when a viewBox exists")
	f.BoolVar(&clean.CurrentColor, "svg-current-color", false, "svg: replace a single-color icon's fill and stroke with currentColor")
	f.BoolVar(&dryRun, "dry-run", false, "print what would be exported without any image request")
	f.IntVar(&concurrency, "concurrency", assets.DefaultConcurrency, "maximum simultaneous downloads")
	assetsExportCmd.MarkFlagsMutuallyExclusive("icons-only", "fills-only", "exports-only")
	assetsExportCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		ids, err := parseNodeFlags(exportFlags.nodes, r)
		if err != nil {
			return err
		}
		format = strings.ToLower(format)
		if format != "svg" && format != "png" {
			return figctl.Newf(figctl.CodeUsage, "invalid --format %q", format).WithHint("Icons export as svg or png.")
		}
		if scale == 0 {
			scale = assets.DefaultScale(format)
		}
		if err := assets.ValidateScale(scale); err != nil {
			return err
		}
		anyOnly := iconsOnly || fillsOnly || exportsOnly
		opts, err := exportFlags.options(!anyOnly || iconsOnly, !anyOnly || exportsOnly, !anyOnly || fillsOnly)
		if err != nil {
			return err
		}
		dir, err := outputDir(outDir, r.FileKey, "assets")
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		roots, version, err := subtrees(rctx, session, r.FileKey, ids, ctx.Flags.MaxFileMB)
		if err != nil {
			return err
		}
		d := assets.Discover(roots, opts)

		// One render item per (node, format, scale); an icon that is also
		// export-marked at the same format and scale shares the file.
		req := assets.RenderRequest{OutDir: dir, NameBy: assets.NameByName, Concurrency: concurrency, Clean: clean, DryRun: dryRun}
		index := map[string]int{}
		itemKey := func(id, format string, scale float64) string {
			return id + "|" + format + "|" + strconv.FormatFloat(scale, 'f', -1, 64)
		}
		addItem := func(item assets.Item) int {
			k := itemKey(item.NodeID, item.Format, item.Scale)
			if i, ok := index[k]; ok {
				return i
			}
			index[k] = len(req.Items)
			req.Items = append(req.Items, item)
			return index[k]
		}
		exportIdx := make([]int, len(d.Exports))
		for i, e := range d.Exports {
			exportIdx[i] = addItem(assets.Item{NodeID: e.NodeID, Name: e.Name, Format: e.Format, Scale: e.Scale, Suffix: e.Suffix})
		}
		iconIdx := make([]int, len(d.Icons))
		for i, icon := range d.Icons {
			iconIdx[i] = addItem(assets.Item{NodeID: icon.NodeID, Name: icon.Name, Format: format, Scale: scale})
		}
		rendered, err := assets.Render(rctx, session.Client, session.Cache, r.FileKey, version, req)
		if err != nil {
			return err
		}
		data := assetsData{OutDir: dir, DryRun: dryRun, Icons: []assets.Result{}, Exports: []assets.Result{}, ImageFills: []fillEntry{}, Failures: []assetFailure{}, Requests: rendered.Requests}
		for _, i := range iconIdx {
			data.Icons = append(data.Icons, rendered.Items[i])
		}
		for _, i := range exportIdx {
			data.Exports = append(data.Exports, rendered.Items[i])
		}

		refs := make([]string, 0, len(d.ImageFills))
		for _, fill := range d.ImageFills {
			refs = append(refs, fill.ImageRef)
			data.ImageFills = append(data.ImageFills, fillEntry{ImageRef: fill.ImageRef, ScaleMode: fill.ScaleMode, Nodes: fill.Nodes})
		}
		if !dryRun && len(refs) > 0 {
			fills, err := assets.DownloadFills(rctx, session.Client, session.Cache, r.FileKey, version, refs, dir, concurrency)
			if err != nil {
				return err
			}
			data.Requests += fills.Requests
			for i, res := range fills.Items {
				entry := &data.ImageFills[i]
				entry.Path, entry.Bytes, entry.Cached, entry.Error = res.Path, res.Bytes, res.Cached, res.Error
			}
		}

		for _, res := range data.Icons {
			if res.Error != "" {
				data.Failures = append(data.Failures, assetFailure{Category: categoryIcon, ID: res.NodeID, Name: res.Name, Error: res.Error})
			}
		}
		for _, res := range data.Exports {
			if res.Error != "" {
				data.Failures = append(data.Failures, assetFailure{Category: categoryExport, ID: res.NodeID, Name: res.Name, Error: res.Error})
			}
		}
		for _, fill := range data.ImageFills {
			if fill.Error != "" {
				data.Failures = append(data.Failures, assetFailure{Category: categoryFill, ID: fill.ImageRef, Name: fillUsers(fill.Nodes), Error: fill.Error})
			}
		}
		total := len(data.Icons) + len(data.Exports) + len(data.ImageFills)
		if !dryRun && total > 0 {
			path, err := writeManifest(dir, r.FileKey, version, data)
			if err != nil {
				return err
			}
			data.ManifestPath = path
		}

		env := ctx.Envelope(data)
		switch {
		case total == 0:
			env.AddHint("No assets found; widen the search with --icon-pattern or --include-hidden, or drop --node.")
		case dryRun:
			env.AddHint(fmt.Sprintf("Dry run: %d file(s) would be written to %s; image fill extensions are detected at download. Run again without --dry-run.", len(data.Icons)+len(data.Exports)+len(data.ImageFills), dir))
		default:
			env.AddHint("Files are listed in " + data.ManifestPath + "; SVG icons can be inlined or imported as components, image fills referenced by path.")
		}
		if total == 0 {
			return ctx.Printer.Print(env)
		}
		return finishWithFailures(ctx, env, "asset", len(data.Failures), total, failureDetails(data.Failures))
	})

	assetsCmd.AddCommand(assetsListCmd, assetsExportCmd)
	rootCmd.AddCommand(assetsCmd)
}

// writeManifest writes manifest.json mapping node ids and image refs to
// the files written, and returns its path.
func writeManifest(dir, fileKey, version string, data assetsData) (string, error) {
	m := manifestFile{File: fileKey, Version: version, Nodes: map[string][]manifestEntry{}, ImageRefs: map[string]string{}}
	add := func(category string, r assets.Result) {
		if r.Error != "" {
			return
		}
		for _, existing := range m.Nodes[r.NodeID] {
			if existing.Path == r.Path {
				return
			}
		}
		m.Nodes[r.NodeID] = append(m.Nodes[r.NodeID], manifestEntry{Category: category, Name: r.Name, Format: r.Format, Scale: r.Scale, Path: r.Path})
	}
	for _, r := range data.Exports {
		add(categoryExport, r)
	}
	for _, r := range data.Icons {
		add(categoryIcon, r)
	}
	for _, f := range data.ImageFills {
		if f.Error == "" && f.Path != "" {
			m.ImageRefs[f.ImageRef] = f.Path
		}
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "encoding manifest: "+err.Error())
	}
	path := filepath.Join(dir, manifestFileName)
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil { //nolint:gosec // manifest is a project file
		return "", figctl.Wrap(figctl.CodeInternal, err, "writing manifest: "+err.Error())
	}
	return path, nil
}
