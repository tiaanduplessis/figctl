package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tiaanduplessis/figctl/internal/assets"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// renderFailure is one node that produced no file.
type renderFailure struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name,omitempty"`
	Error  string `json:"error"`
}

// renderData is the manifest printed by render.
type renderData struct {
	OutDir   string          `json:"outDir"`
	Items    []assets.Result `json:"items"`
	Failures []renderFailure `json:"failures"`
	Requests int             `json:"requests"`
}

func (d renderData) Columns() []string {
	return []string{"nodeId", "name", "format", "scale", "path", "bytes", "cached", "error"}
}

func (d renderData) Rows() [][]string {
	rows := make([][]string, 0, len(d.Items))
	for _, r := range d.Items {
		rows = append(rows, resultRow(r))
	}
	return rows
}

func resultRow(r assets.Result) []string {
	path := r.Path
	if r.Error != "" {
		path = ""
	}
	return []string{r.NodeID, r.Name, r.Format, strconv.FormatFloat(r.Scale, 'f', -1, 64), path, strconv.FormatInt(r.Bytes, 10), yesNo(r.Cached), r.Error}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

var renderCmd = &cobra.Command{
	Use:   "render <ref>",
	Short: "Render nodes to PNG, JPG, SVG, or PDF files",
	Long: `Render one or more nodes to image files and print a manifest of the paths.
Nodes sharing a format and scale are rendered in one API request, and every
render is cached by node, format, scale, and file version, so repeating a
render of an unchanged file costs no image requests.

A node-id in the ref URL is used when no --node is given. Nodes Figma cannot
render (hidden, empty) are reported per node; the good files are kept and
the command exits 6 (PARTIAL).`,
	Example: `  figctl render KEY --node 2:2
  figctl render "https://www.figma.com/design/KEY/App?node-id=2-2" --scale 1 --out ./design
  figctl render KEY --node 2:9 -f svg --svg-outline-text --name-by id`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var nodes []string
	var format, outDir, nameBy string
	var scale float64
	var concurrency int
	var svgOutlineText, svgIncludeID, svgIncludeNodeID, noSVGSimplifyStroke, noContentsOnly, absoluteBounds bool
	f := renderCmd.Flags()
	f.StringArrayVar(&nodes, "node", nil, "node id (1:2, 1-2, or a URL with node-id); repeatable")
	f.StringVarP(&format, "format", "f", "png", "png, jpg, svg, or pdf")
	f.Float64Var(&scale, "scale", 0, "render scale between 0.01 and 4 (default 2 for png and jpg, 1 for svg and pdf)")
	f.StringVar(&outDir, "out", "", "output directory (default ./.figctl/<fileKey>/renders)")
	f.StringVar(&nameBy, "name-by", assets.NameByName, "file naming: name (slug of the node name) or id")
	f.IntVar(&concurrency, "concurrency", assets.DefaultConcurrency, "maximum simultaneous downloads")
	f.BoolVar(&svgOutlineText, "svg-outline-text", false, "svg: render text as outlines")
	f.BoolVar(&svgIncludeID, "svg-include-id", false, "svg: include id attributes for every element")
	f.BoolVar(&svgIncludeNodeID, "svg-include-node-id", false, "svg: include data-node-id attributes")
	f.BoolVar(&noSVGSimplifyStroke, "no-svg-simplify-stroke", false, "svg: keep strokes as strokes instead of simplifying them")
	f.BoolVar(&noContentsOnly, "no-contents-only", false, "include overlapping content from other nodes")
	f.BoolVar(&absoluteBounds, "absolute-bounds", false, "use the full node dimensions regardless of cropping")
	renderCmd.RunE = run(func(ctx *Context, cmd *cobra.Command, args []string) error {
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
			return figctl.New(figctl.CodeUsage, "render needs at least one node").
				WithHint("Pass --node <id> or a Figma URL containing node-id. Find ids with figctl file tree %s.", r.FileKey)
		}
		format = strings.ToLower(format)
		if err := assets.ValidateFormat(format); err != nil {
			return err
		}
		if scale == 0 {
			scale = assets.DefaultScale(format)
		}
		if err := assets.ValidateScale(scale); err != nil {
			return err
		}
		if err := validateNameBy(nameBy); err != nil {
			return err
		}
		dir, err := outputDir(outDir, r.FileKey, "renders")
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		found, version, missing, err := lookupNodes(rctx, session, r.FileKey, ids)
		if err != nil {
			return err
		}
		req := assets.RenderRequest{
			OutDir:      dir,
			NameBy:      nameBy,
			Concurrency: concurrency,
			SVG: assets.SVGOptions{
				OutlineText:   flagBool(cmd.Flags(), "svg-outline-text", svgOutlineText),
				IncludeID:     flagBool(cmd.Flags(), "svg-include-id", svgIncludeID),
				IncludeNodeID: flagBool(cmd.Flags(), "svg-include-node-id", svgIncludeNodeID),
			},
		}
		if noSVGSimplifyStroke {
			req.SVG.SimplifyStroke = boolPtr(false)
		}
		if noContentsOnly {
			req.ContentsOnly = boolPtr(false)
		}
		if absoluteBounds {
			req.UseAbsoluteBounds = boolPtr(true)
		}
		for _, id := range ids {
			if node, ok := found[id]; ok {
				req.Items = append(req.Items, assets.Item{NodeID: id, Name: node.Name, Format: format, Scale: scale})
			}
		}
		out, err := assets.Render(rctx, session.Client, session.Cache, r.FileKey, version, req)
		if err != nil {
			return err
		}
		data := renderData{OutDir: dir, Items: []assets.Result{}, Failures: []renderFailure{}, Requests: out.Requests}
		next := 0
		for _, id := range ids {
			if msg, isMissing := missing[id]; isMissing {
				data.Items = append(data.Items, assets.Result{NodeID: id, Format: format, Scale: scale, Error: msg})
				data.Failures = append(data.Failures, renderFailure{NodeID: id, Error: msg})
				continue
			}
			res := out.Items[next]
			next++
			data.Items = append(data.Items, res)
			if res.Error != "" {
				data.Failures = append(data.Failures, renderFailure{NodeID: res.NodeID, Name: res.Name, Error: res.Error})
			}
		}
		env := ctx.Envelope(data)
		var paths []string
		for _, res := range data.Items {
			if res.Error == "" {
				paths = append(paths, res.Path)
			}
		}
		if len(paths) > 0 {
			switch format {
			case "png", "jpg":
				env.AddHint("Read the PNG with your image/vision tool to see the design: " + strings.Join(paths, ", "))
			default:
				env.AddHint("Files written: " + strings.Join(paths, ", "))
			}
		}
		if out.Cached > 0 {
			env.AddHint(fmt.Sprintf("%d of %d files came from the render cache; pass --refresh to re-render.", out.Cached, len(paths)))
		}
		return finishWithFailures(ctx, env, "render", len(data.Failures), len(ids), failureDetails(data.Failures))
	})
	rootCmd.AddCommand(renderCmd)
}

func validateNameBy(v string) error {
	if v != assets.NameByName && v != assets.NameByID {
		return figctl.Newf(figctl.CodeUsage, "invalid --name-by %q", v).WithHint("Use name or id.")
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }

// flagBool returns a pointer to value when the flag was set explicitly,
// so Figma's default applies otherwise.
func flagBool(fs *pflag.FlagSet, name string, value bool) *bool {
	if fs.Changed(name) {
		return &value
	}
	return nil
}

// outputDir resolves the output directory flag to an absolute path,
// defaulting to ./.figctl/<fileKey>/<kind>.
func outputDir(flag, fileKey, kind string) (string, error) {
	dir := flag
	if dir == "" {
		dir = filepath.Join(".figctl", fileKey, kind)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "resolving output directory: "+err.Error())
	}
	return abs, nil
}

// lookupNodes resolves node ids to their nodes (without children) from
// the cached document. Missing ids map to an error message.
func lookupNodes(ctx context.Context, session *Session, key string, ids []string) (map[string]*figma.Node, string, map[string]string, error) {
	resp, err := session.Nodes(ctx, key, ids, figma.NodesOptions{Depth: 1})
	if err != nil {
		return nil, "", nil, err
	}
	found := map[string]*figma.Node{}
	missing := map[string]string{}
	for _, id := range ids {
		entry := resp.Nodes[id]
		if entry == nil || entry.Document == nil {
			missing[id] = "node " + id + " not found in file " + key + "; check the id with figctl file tree"
			continue
		}
		found[id] = entry.Document
	}
	return found, resp.Version, missing, nil
}

// subtrees returns the full subtrees of the given ids, or the whole
// document when none are given, along with the file version.
func subtrees(ctx context.Context, session *Session, key string, ids []string, maxFileMB int) ([]*figma.Node, string, error) {
	if len(ids) == 0 {
		file, err := session.File(ctx, key)
		if errors.Is(err, errTooLarge) {
			return nil, "", figctl.Newf(figctl.CodeUsage, "file %s is larger than --max-file-mb (%d MB)", key, maxFileMB).
				WithHint("Narrow the request with --node <id>, or raise --max-file-mb.")
		}
		if err != nil {
			return nil, "", err
		}
		return []*figma.Node{file.Document}, file.Version, nil
	}
	resp, err := session.Nodes(ctx, key, ids, figma.NodesOptions{})
	if err != nil {
		return nil, "", err
	}
	roots := make([]*figma.Node, 0, len(ids))
	for _, id := range ids {
		entry := resp.Nodes[id]
		if entry == nil || entry.Document == nil {
			return nil, "", figctl.Newf(figctl.CodeNotFound, "node %s not found in file %s", id, key).
				WithHint("Check the id with figctl file tree %s.", key)
		}
		roots = append(roots, entry.Document)
	}
	return roots, resp.Version, nil
}

func failureDetails[T any](failures []T) map[string]any {
	return map[string]any{"failures": failures}
}

// finishWithFailures prints the envelope and maps the outcome to an exit
// code: 0 when nothing failed, 6 (PARTIAL) when some items failed, and
// RENDER_FAILED when every item failed.
func finishWithFailures(ctx *Context, env *output.Envelope, what string, failed, total int, details map[string]any) error {
	switch {
	case failed == 0:
		return ctx.Printer.Print(env)
	case failed >= total:
		return figctl.Newf(figctl.CodeRenderFailed, "every %s failed (%d of %d)", what, failed, total).
			WithHint("Check that the nodes are visible and have renderable content; see details.failures.").
			WithDetails(details)
	default:
		env.AddHint(fmt.Sprintf("%d of %d %ss failed; the other files were written. See data.failures.", failed, total, what))
		return ctx.PrintPartial(env, figctl.Newf(figctl.CodePartial, "%d of %d %ss failed", failed, total, what).
			WithHint("The successful files were written; see data.failures for the rest."))
	}
}
