package assets

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// DefaultConcurrency is the download worker pool size.
const DefaultConcurrency = 8

// Formats accepted by the images endpoint.
var Formats = []string{"png", "jpg", "svg", "pdf"}

// BlobStore is the subset of the disk cache used for rendered images and
// downloaded assets. *cache.Cache implements it; nil disables caching.
type BlobStore interface {
	GetBlob(fileKey, key string) ([]byte, bool, error)
	PutBlob(fileKey, key string, data []byte) error
}

// Item is one node to render at one format and scale.
type Item struct {
	NodeID string
	// Name is the node name used for the filename. Empty falls back to
	// the node id.
	Name   string
	Format string
	Scale  float64
	// Suffix is the filename suffix before the extension. Empty means the
	// suffix is derived from the scale ("@2x").
	Suffix string
}

// SVGOptions map to the svg_* query parameters. Nil leaves Figma's default.
type SVGOptions struct {
	OutlineText    *bool
	IncludeID      *bool
	IncludeNodeID  *bool
	SimplifyStroke *bool
}

// RenderRequest describes one render run. Items sharing a format and scale
// are sent in one images request.
type RenderRequest struct {
	Items             []Item
	SVG               SVGOptions
	ContentsOnly      *bool
	UseAbsoluteBounds *bool
	// OutDir receives the files. It is created when missing.
	OutDir string
	// NameBy is NameByName (default) or NameByID.
	NameBy string
	// Concurrency bounds simultaneous downloads. Zero means
	// DefaultConcurrency.
	Concurrency int
	// Clean post-processes SVG output.
	Clean SVGCleanOptions
	// DryRun assigns filenames without any request or write.
	DryRun bool
}

// Result is the manifest entry of one item.
type Result struct {
	NodeID string  `json:"nodeId"`
	Name   string  `json:"name"`
	Format string  `json:"format"`
	Scale  float64 `json:"scale"`
	Suffix string  `json:"suffix,omitempty"`
	Path   string  `json:"path"`
	Bytes  int64   `json:"bytes"`
	Cached bool    `json:"cached"`
	Error  string  `json:"error,omitempty"`
}

// RenderOutput is the manifest of a render run.
type RenderOutput struct {
	Items []Result `json:"items"`
	// Requests counts images API calls made.
	Requests int `json:"requests"`
	// Downloaded, Cached, and Failed count items by outcome.
	Downloaded int `json:"downloaded"`
	Cached     int `json:"cached"`
	Failed     int `json:"failed"`
}

// Failures returns the items that did not produce a file.
func (o *RenderOutput) Failures() []Result {
	var out []Result
	for _, r := range o.Items {
		if r.Error != "" {
			out = append(out, r)
		}
	}
	return out
}

// ValidateFormat checks a render format.
func ValidateFormat(format string) error {
	for _, f := range Formats {
		if f == strings.ToLower(format) {
			return nil
		}
	}
	return figctl.Newf(figctl.CodeUsage, "invalid format %q", format).
		WithHint("Use one of %s.", strings.Join(Formats, ", "))
}

// ValidateScale checks a render scale against Figma's 0.01 to 4 range.
func ValidateScale(scale float64) error {
	if scale < 0.01 || scale > 4 {
		return figctl.Newf(figctl.CodeUsage, "invalid scale %s", strconv.FormatFloat(scale, 'f', -1, 64)).
			WithHint("Figma renders at scales between 0.01 and 4.")
	}
	return nil
}

// DefaultScale is the scale used when none is given: 2 for raster
// formats, 1 for svg and pdf.
func DefaultScale(format string) float64 {
	switch strings.ToLower(format) {
	case "svg", "pdf":
		return 1
	default:
		return 2
	}
}

type group struct {
	format string
	scale  float64
	ids    []string
}

type job struct {
	index int
	url   string
	key   string
}

// Render renders every item, serving repeats from the blob cache and
// downloading the rest with a bounded worker pool. Nodes Figma cannot
// render become per-item errors; API failures abort the run.
func Render(ctx context.Context, client *figma.Client, store BlobStore, fileKey, version string, req RenderRequest) (*RenderOutput, error) {
	if len(req.Items) == 0 {
		return &RenderOutput{Items: []Result{}}, nil
	}
	for _, it := range req.Items {
		if err := ValidateFormat(it.Format); err != nil {
			return nil, err
		}
		if err := ValidateScale(it.Scale); err != nil {
			return nil, err
		}
	}
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	names := assignNames(req.Items, req.NameBy)
	out := &RenderOutput{Items: make([]Result, len(req.Items))}
	for i, it := range req.Items {
		out.Items[i] = Result{
			NodeID: it.NodeID,
			Name:   it.Name,
			Format: Extension(it.Format),
			Scale:  it.Scale,
			Suffix: it.Suffix,
			Path:   filepath.Join(req.OutDir, names[i]),
		}
	}
	if req.DryRun {
		return out, nil
	}
	if err := os.MkdirAll(req.OutDir, 0o750); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "creating output directory: "+err.Error())
	}

	// Serve cached items and collect the misses per (format, scale).
	var groups []*group
	byGroup := map[string]*group{}
	pending := map[string][]int{}
	for i, it := range req.Items {
		key := cache.BlobKey(it.NodeID, req.variant(it.Format), it.Scale, version)
		if store != nil {
			data, hit, err := store.GetBlob(fileKey, key)
			if err != nil {
				return nil, err
			}
			if hit {
				out.Items[i].Cached = true
				if err := writeResult(&out.Items[i], data, req.Clean); err != nil {
					return nil, err
				}
				out.Cached++
				continue
			}
		}
		gk := Extension(it.Format) + "@" + strconv.FormatFloat(it.Scale, 'f', -1, 64)
		g, ok := byGroup[gk]
		if !ok {
			g = &group{format: Extension(it.Format), scale: it.Scale}
			byGroup[gk] = g
			groups = append(groups, g)
		}
		if _, seen := pending[gk+"/"+it.NodeID]; !seen {
			g.ids = append(g.ids, it.NodeID)
		}
		pending[gk+"/"+it.NodeID] = append(pending[gk+"/"+it.NodeID], i)
	}

	var jobs []job
	for _, g := range groups {
		opts := figma.ImageOptions{
			Version:           version,
			Scale:             g.scale,
			Format:            g.format,
			ContentsOnly:      req.ContentsOnly,
			UseAbsoluteBounds: req.UseAbsoluteBounds,
		}
		if g.format == "svg" {
			opts.SVGOutlineText = req.SVG.OutlineText
			opts.SVGIncludeID = req.SVG.IncludeID
			opts.SVGIncludeNodeID = req.SVG.IncludeNodeID
			opts.SVGSimplifyStroke = req.SVG.SimplifyStroke
		}
		resp, err := client.GetImages(ctx, fileKey, g.ids, opts)
		if err != nil {
			return nil, err
		}
		out.Requests++
		gk := g.format + "@" + strconv.FormatFloat(g.scale, 'f', -1, 64)
		for _, id := range g.ids {
			u := resp.Images[id]
			for _, i := range pending[gk+"/"+id] {
				if u == nil || *u == "" {
					out.Items[i].Error = fmt.Sprintf("Figma returned no image for node %s (%s); check that the node is visible and has renderable content", id, req.Items[i].Name)
					out.Failed++
					continue
				}
				jobs = append(jobs, job{index: i, url: *u, key: cache.BlobKey(id, req.variant(g.format), g.scale, version)})
			}
		}
	}

	// Download with a bounded worker pool. Each job owns its result slot.
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var fatal error
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := &out.Items[j.index]
			var buf bytes.Buffer
			if err := client.Download(ctx, j.url, &buf); err != nil {
				res.Error = "download failed: " + figctl.From(err).Message
				return
			}
			data := buf.Bytes()
			if store != nil {
				if err := store.PutBlob(fileKey, j.key, data); err != nil {
					mu.Lock()
					if fatal == nil {
						fatal = err
					}
					mu.Unlock()
					return
				}
			}
			if err := writeResult(res, data, req.Clean); err != nil {
				mu.Lock()
				if fatal == nil {
					fatal = err
				}
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
	if fatal != nil {
		return nil, fatal
	}
	for _, j := range jobs {
		if out.Items[j.index].Error != "" {
			out.Failed++
		} else {
			out.Downloaded++
		}
	}
	return out, nil
}

// variant extends the format with the render options that change the
// bytes Figma returns, so the blob cache never serves a render made with
// different options.
func (r RenderRequest) variant(format string) string {
	format = Extension(format)
	var parts []string
	add := func(name string, v *bool) {
		if v != nil {
			parts = append(parts, name+"="+strconv.FormatBool(*v))
		}
	}
	add("contents_only", r.ContentsOnly)
	add("use_absolute_bounds", r.UseAbsoluteBounds)
	if format == "svg" {
		add("svg_outline_text", r.SVG.OutlineText)
		add("svg_include_id", r.SVG.IncludeID)
		add("svg_include_node_id", r.SVG.IncludeNodeID)
		add("svg_simplify_stroke", r.SVG.SimplifyStroke)
	}
	if len(parts) == 0 {
		return format
	}
	return format + ";" + strings.Join(parts, ";")
}

// writeResult writes data to the result path, cleaning SVG when asked.
func writeResult(res *Result, data []byte, clean SVGCleanOptions) error {
	if res.Format == "svg" && clean.Enabled() {
		data = CleanSVG(data, clean)
	}
	if err := os.WriteFile(res.Path, data, 0o644); err != nil { //nolint:gosec // exported assets are project files
		return figctl.Wrap(figctl.CodeInternal, err, "writing "+res.Path+": "+err.Error())
	}
	res.Bytes = int64(len(data))
	return nil
}
