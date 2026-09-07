package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func imagesAPIPath() string { return "/v1/images/" + figmatest.FileKey }

func items(t *testing.T, d map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := d[key].([]any)
	if !ok {
		t.Fatalf("%s is %T: %v", key, d[key], d)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]any))
	}
	return out
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty file at %s: %v", path, err)
	}
}

func TestRender(t *testing.T) {
	api := setup(t)
	out := filepath.Join(t.TempDir(), "renders")

	r := execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--node", "2-9", "--out", out)
	ok(t, r)
	d := data(t, r, "render")
	if d["outDir"] != out || d["requests"] != float64(1) {
		t.Fatalf("data: %v", d)
	}
	list := items(t, d, "items")
	if len(list) != 2 || list[0]["nodeId"] != "2:2" || list[0]["name"] != "Login" || list[0]["format"] != "png" || list[0]["scale"] != float64(2) || list[0]["cached"] != false {
		t.Fatalf("items: %v", list)
	}
	if list[0]["path"] != filepath.Join(out, "login@2x.png") || list[1]["path"] != filepath.Join(out, "icon-arrow-right@2x.png") {
		t.Fatalf("paths: %v", list)
	}
	mustExist(t, list[0]["path"].(string))
	mustExist(t, list[1]["path"].(string))
	if failures := d["failures"].([]any); len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}
	env := decodeEnvelope(t, r.stdout)
	hints := env["hints"].([]any)
	if len(hints) != 1 || !strings.Contains(hints[0].(string), "Read the PNG with your image/vision tool") || !strings.Contains(hints[0].(string), "login@2x.png") {
		t.Fatalf("hints: %v", hints)
	}
	if env["file"].(map[string]any)["version"] != figmatest.Version {
		t.Fatalf("file block: %v", env["file"])
	}
	if api.Count(http.MethodGet, imagesAPIPath()) != 1 {
		t.Fatalf("images requests = %d, want 1", api.Count(http.MethodGet, imagesAPIPath()))
	}
	for _, req := range api.Requests() {
		if req.Path == imagesAPIPath() && (req.Query.Get("ids") != "2:2,2:9" || req.Query.Get("format") != "png" || req.Query.Get("scale") != "2") {
			t.Fatalf("images query: %v", req.Query)
		}
	}

	// A second run is served from the render cache: no image request and
	// no download.
	api.Reset()
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--node", "2:9", "--out", out)
	ok(t, r)
	d = data(t, r, "render")
	list = items(t, d, "items")
	if list[0]["cached"] != true || list[1]["cached"] != true || d["requests"] != float64(0) {
		t.Fatalf("second run should be cached: %v", d)
	}
	if api.Count(http.MethodGet, imagesAPIPath()) != 0 || api.CountPrefix(http.MethodGet, "/images/") != 0 {
		t.Fatalf("second run made requests: %v", api.Requests())
	}
	env = decodeEnvelope(t, r.stdout)
	if hints := env["hints"].([]any); len(hints) != 2 || !strings.Contains(hints[1].(string), "render cache") {
		t.Fatalf("cache hint: %v", hints)
	}

	// SVG with options, node from the URL, names by id, and the default
	// output directory under the working directory.
	api.Reset()
	r = execute(t, "", "render", "https://www.figma.com/design/"+figmatest.FileKey+"/App?node-id=2-9", "-f", "svg", "--svg-outline-text", "--svg-include-node-id", "--no-svg-simplify-stroke", "--no-contents-only", "--absolute-bounds", "--name-by", "id")
	ok(t, r)
	d = data(t, r, "render")
	list = items(t, d, "items")
	want := filepath.Join(".figctl", figmatest.FileKey, "renders", "2-9.svg")
	if len(list) != 1 || !strings.HasSuffix(list[0]["path"].(string), want) || list[0]["scale"] != float64(1) {
		t.Fatalf("svg render: %v", list)
	}
	mustExist(t, list[0]["path"].(string))
	if raw, _ := os.ReadFile(list[0]["path"].(string)); !strings.HasPrefix(string(raw), "<svg") {
		t.Fatalf("svg content: %s", raw)
	}
	for _, req := range api.Requests() {
		if req.Path != imagesAPIPath() {
			continue
		}
		q := req.Query
		if q.Get("format") != "svg" || q.Get("scale") != "1" || q.Get("svg_outline_text") != "true" || q.Get("svg_include_node_id") != "true" || q.Get("svg_simplify_stroke") != "false" || q.Get("contents_only") != "false" || q.Get("use_absolute_bounds") != "true" || q.Get("svg_include_id") != "" {
			t.Fatalf("svg query: %v", q)
		}
	}
	env = decodeEnvelope(t, r.stdout)
	if hints := env["hints"].([]any); !strings.Contains(hints[0].(string), "Files written") {
		t.Fatalf("svg hints: %v", hints)
	}

	// Table output.
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--out", out, "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "nodeId  name   format  scale  path") || !strings.Contains(r.stdout, "2:2     Login  png     2      "+filepath.Join(out, "login@2x.png")) || !strings.Contains(r.stderr, "hint: Read the PNG") {
		t.Fatalf("table:\n%s\n%s", r.stdout, r.stderr)
	}
}

func TestRenderPartialAndErrors(t *testing.T) {
	api := setup(t)
	out := filepath.Join(t.TempDir(), "renders")

	// A hidden node renders as null: the good file is written, the failure
	// is reported per node, and the process exits 6 with the data intact.
	r := execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--node", "2:13", "--out", out)
	if r.code != figctl.ExitPartial {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", r.code, figctl.ExitPartial, r.stdout, r.stderr)
	}
	if r.stderr != "" {
		t.Fatalf("stderr must stay empty in JSON mode: %q", r.stderr)
	}
	d := data(t, r, "render")
	list := items(t, d, "items")
	if len(list) != 2 || list[0]["error"] != nil || list[1]["nodeId"] != "2:13" {
		t.Fatalf("items: %v", list)
	}
	msg := list[1]["error"].(string)
	if !strings.Contains(msg, "2:13") || !strings.Contains(msg, "Debug") || !strings.Contains(msg, "visible") {
		t.Fatalf("null render message: %s", msg)
	}
	failures := items(t, d, "failures")
	if len(failures) != 1 || failures[0]["nodeId"] != "2:13" || failures[0]["error"] != msg {
		t.Fatalf("failures: %v", failures)
	}
	mustExist(t, list[0]["path"].(string))
	if _, err := os.Stat(filepath.Join(out, "debug@2x.png")); !os.IsNotExist(err) {
		t.Fatal("failed node must not leave a file")
	}
	env := decodeEnvelope(t, r.stdout)
	if _, hasError := env["error"]; hasError {
		t.Fatalf("partial success must keep the success envelope: %s", r.stdout)
	}
	hints := env["hints"].([]any)
	if !strings.Contains(hints[len(hints)-1].(string), "1 of 2 renders failed") {
		t.Fatalf("partial hint: %v", hints)
	}
	if strings.Count(r.stdout, "\"schemaVersion\"") != 1 {
		t.Fatalf("exactly one envelope expected:\n%s", r.stdout)
	}

	// In table mode the summary goes to stderr with the PARTIAL code.
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--node", "2:13", "--out", out, "-o", "table")
	if r.code != figctl.ExitPartial || !strings.Contains(r.stderr, "error: 1 of 2 renders failed (PARTIAL)") || !strings.Contains(r.stdout, "2:13    Debug") {
		t.Fatalf("table partial: exit = %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}

	// An unknown node is a per-node failure resolved locally: no image
	// request is wasted on it.
	api.Reset()
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:3", "--node", "9:9", "--out", out)
	if r.code != figctl.ExitPartial {
		t.Fatalf("unknown node: exit = %d, stdout = %s", r.code, r.stdout)
	}
	d = data(t, r, "render")
	list = items(t, d, "items")
	if list[1]["nodeId"] != "9:9" || !strings.Contains(list[1]["error"].(string), "node 9:9 not found") {
		t.Fatalf("unknown node item: %v", list)
	}
	for _, req := range api.Requests() {
		if req.Path == imagesAPIPath() && req.Query.Get("ids") != "2:3" {
			t.Fatalf("unknown node must not be requested: %v", req.Query)
		}
	}

	// Every node failing is RENDER_FAILED with the failures in details.
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:13", "--out", out)
	if r.code != figctl.ExitError || errorCode(t, r) != "RENDER_FAILED" {
		t.Fatalf("all failed: exit = %d, stdout = %s", r.code, r.stdout)
	}
	env = decodeEnvelope(t, r.stdout)
	details := env["error"].(map[string]any)["details"].(map[string]any)
	if f := details["failures"].([]any); len(f) != 1 || f[0].(map[string]any)["nodeId"] != "2:13" {
		t.Fatalf("details: %v", details)
	}

	// Usage errors.
	r = execute(t, "", "render", figmatest.FileKey)
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "at least one node") {
		t.Fatalf("no node: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "-f", "gif")
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "invalid format") {
		t.Fatalf("bad format: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--scale", "5")
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "invalid scale") {
		t.Fatalf("bad scale: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:2", "--name-by", "hash")
	if r.code != figctl.ExitUsage {
		t.Fatalf("bad name-by: exit = %d", r.code)
	}
	r = execute(t, "", "render", figmatest.FileKey, "--node", "nope")
	if r.code != figctl.ExitUsage {
		t.Fatalf("bad node: exit = %d", r.code)
	}
	// An API failure on the images endpoint is a normal error envelope.
	api.Reset()
	api.Respond(http.MethodGet, imagesAPIPath(), figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Forbidden"}})
	r = execute(t, "", "render", figmatest.FileKey, "--node", "2:4", "--out", out)
	if r.code != figctl.ExitError || errorCode(t, r) != "FORBIDDEN" {
		t.Fatalf("forbidden: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestAssetsExport(t *testing.T) {
	api := setup(t)
	out := filepath.Join(t.TempDir(), "assets")

	r := execute(t, "", "assets", "export", figmatest.FileKey, "--out", out)
	ok(t, r)
	d := data(t, r, "assets.export")
	if d["outDir"] != out || d["dryRun"] != false || d["requests"] != float64(3) || d["manifestPath"] != filepath.Join(out, "manifest.json") {
		t.Fatalf("data: %v", d)
	}
	icons := items(t, d, "icons")
	if len(icons) != 1 || icons[0]["nodeId"] != "2:9" || icons[0]["format"] != "svg" || icons[0]["path"] != filepath.Join(out, "icon-arrow-right.svg") {
		t.Fatalf("icons: %v", icons)
	}
	exports := items(t, d, "exports")
	if len(exports) != 3 {
		t.Fatalf("exports: %v", exports)
	}
	if exports[0]["nodeId"] != "2:2" || exports[0]["path"] != filepath.Join(out, "login@2x.png") || exports[0]["scale"] != float64(2) {
		t.Fatalf("login export: %v", exports[0])
	}
	if exports[1]["path"] != icons[0]["path"] {
		t.Fatalf("an icon that is export-marked at the same format must share the file: %v vs %v", exports[1], icons[0])
	}
	if exports[2]["path"] != filepath.Join(out, "icon-arrow-right@2x.png") || exports[2]["suffix"] != "@2x" {
		t.Fatalf("arrow png export: %v", exports[2])
	}
	fills := items(t, d, "imageFills")
	if len(fills) != 1 || fills[0]["imageRef"] != "img1" || fills[0]["path"] != filepath.Join(out, "imageref-img1.png") || fills[0]["scaleMode"] != "FILL" {
		t.Fatalf("fills: %v", fills)
	}
	if nodes := fills[0]["nodes"].([]any); len(nodes) != 1 || nodes[0].(map[string]any)["nodeId"] != "2:8" {
		t.Fatalf("fill nodes: %v", fills[0]["nodes"])
	}
	if failures := d["failures"].([]any); len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}
	for _, path := range []string{"login@2x.png", "icon-arrow-right.svg", "icon-arrow-right@2x.png", "imageref-img1.png", "manifest.json"} {
		mustExist(t, filepath.Join(out, path))
	}
	var manifest struct {
		File      string                      `json:"file"`
		Version   string                      `json:"version"`
		Nodes     map[string][]map[string]any `json:"nodes"`
		ImageRefs map[string]string           `json:"imageRefs"`
	}
	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("manifest: %v\n%s", err, raw)
	}
	if manifest.File != figmatest.FileKey || manifest.Version != figmatest.Version || len(manifest.Nodes["2:9"]) != 2 || len(manifest.Nodes["2:2"]) != 1 || manifest.ImageRefs["img1"] != filepath.Join(out, "imageref-img1.png") {
		t.Fatalf("manifest: %s", raw)
	}
	if manifest.Nodes["2:9"][0]["category"] != "export" || manifest.Nodes["2:9"][0]["path"] != filepath.Join(out, "icon-arrow-right.svg") {
		t.Fatalf("manifest 2:9: %v", manifest.Nodes["2:9"])
	}
	if api.Count(http.MethodGet, imagesAPIPath()) != 2 || api.Count(http.MethodGet, fileKeyPath("/images")) != 1 {
		t.Fatalf("requests: images=%d fills=%d", api.Count(http.MethodGet, imagesAPIPath()), api.Count(http.MethodGet, fileKeyPath("/images")))
	}
	env := decodeEnvelope(t, r.stdout)
	if hints := env["hints"].([]any); len(hints) != 1 || !strings.Contains(hints[0].(string), "manifest.json") {
		t.Fatalf("hints: %v", hints)
	}

	// Repeating the export costs no image or fill requests.
	api.Reset()
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--out", out)
	ok(t, r)
	d = data(t, r, "assets.export")
	if d["requests"] != float64(0) || api.Count(http.MethodGet, imagesAPIPath()) != 0 || api.Count(http.MethodGet, fileKeyPath("/images")) != 0 || api.CountPrefix(http.MethodGet, "/img") != 0 {
		t.Fatalf("second export must be served from the cache: %v", api.Requests())
	}
	if icons := items(t, d, "icons"); icons[0]["cached"] != true {
		t.Fatalf("cached icon: %v", icons)
	}

	// Dry run: discovery only, zero image requests, nothing written.
	api.Reset()
	dry := filepath.Join(t.TempDir(), "dry")
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--dry-run", "--out", dry)
	ok(t, r)
	d = data(t, r, "assets.export")
	if d["dryRun"] != true || d["requests"] != float64(0) || d["manifestPath"] != nil {
		t.Fatalf("dry run data: %v", d)
	}
	if icons := items(t, d, "icons"); icons[0]["path"] != filepath.Join(dry, "icon-arrow-right.svg") || icons[0]["bytes"] != float64(0) {
		t.Fatalf("dry run icon: %v", icons)
	}
	if fills := items(t, d, "imageFills"); fills[0]["path"] != nil {
		t.Fatalf("dry run fill must not have a path: %v", fills)
	}
	if api.Count(http.MethodGet, imagesAPIPath()) != 0 || api.Count(http.MethodGet, fileKeyPath("/images")) != 0 {
		t.Fatalf("dry run made image requests: %v", api.Requests())
	}
	if _, err := os.Stat(dry); !os.IsNotExist(err) {
		t.Fatal("dry run must not create the output directory")
	}
	env = decodeEnvelope(t, r.stdout)
	if hints := env["hints"].([]any); !strings.Contains(hints[0].(string), "Dry run: 5 file(s)") {
		t.Fatalf("dry run hint: %v", hints)
	}

	// Category filters and icon options.
	api.Reset()
	only := filepath.Join(t.TempDir(), "icons")
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--icons-only", "--format", "png", "--out", only, "--svg-strip-ids", "--svg-strip-dimensions", "--svg-current-color")
	ok(t, r)
	d = data(t, r, "assets.export")
	icons = items(t, d, "icons")
	if len(icons) != 1 || icons[0]["path"] != filepath.Join(only, "icon-arrow-right@2x.png") || len(d["exports"].([]any)) != 0 || len(d["imageFills"].([]any)) != 0 {
		t.Fatalf("icons only: %v", d)
	}
	if api.Count(http.MethodGet, fileKeyPath("/images")) != 0 {
		t.Fatal("icons only must not fetch image fills")
	}
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--fills-only", "--out", only)
	ok(t, r)
	d = data(t, r, "assets.export")
	if len(d["icons"].([]any)) != 0 || len(d["exports"].([]any)) != 0 || len(items(t, d, "imageFills")) != 1 {
		t.Fatalf("fills only: %v", d)
	}
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--exports-only", "--node", "2:7", "--out", only, "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "category  id   name              format  scale  path") || !strings.Contains(r.stdout, "export    2:9  icon/arrow-right  svg     1      "+filepath.Join(only, "icon-arrow-right.svg")) || strings.Contains(r.stdout, "2:2") {
		t.Fatalf("exports only table:\n%s", r.stdout)
	}
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--icons-only", "--fills-only")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exclusive flags: exit = %d", r.code)
	}
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--format", "pdf")
	if r.code != figctl.ExitUsage {
		t.Fatalf("bad icon format: exit = %d", r.code)
	}
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--icon-pattern", "[bad")
	if r.code != figctl.ExitUsage {
		t.Fatalf("bad pattern: exit = %d", r.code)
	}

	// Nothing under a node without assets, and unknown nodes.
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--node", "2:14", "--out", filepath.Join(t.TempDir(), "none"))
	ok(t, r)
	d = data(t, r, "assets.export")
	if len(d["icons"].([]any))+len(d["exports"].([]any))+len(d["imageFills"].([]any)) != 0 || d["manifestPath"] != nil {
		t.Fatalf("empty export: %v", d)
	}
	env = decodeEnvelope(t, r.stdout)
	if hints := env["hints"].([]any); !strings.Contains(hints[0].(string), "No assets found") {
		t.Fatalf("empty hint: %v", hints)
	}
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("unknown node: exit = %d, stdout = %s", r.code, r.stdout)
	}

	// A hidden export-marked node is a partial failure with exit 6.
	api.Reset()
	partial := filepath.Join(t.TempDir(), "partial")
	r = execute(t, "", "assets", "export", figmatest.FileKey, "--node", "2:2", "--include-hidden", "--icon-pattern", "debug", "--out", partial)
	if r.code != figctl.ExitPartial {
		t.Fatalf("hidden icon: exit = %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
	d = data(t, r, "assets.export")
	failures := items(t, d, "failures")
	if len(failures) != 1 || failures[0]["category"] != "icon" || failures[0]["id"] != "2:13" || !strings.Contains(failures[0]["error"].(string), "visible") {
		t.Fatalf("failures: %v", failures)
	}
	if exports := items(t, d, "exports"); len(exports) != 3 || exports[0]["error"] != nil {
		t.Fatalf("exports still written: %v", exports)
	}
	mustExist(t, filepath.Join(partial, "manifest.json"))
}

func TestAssetsList(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "assets", "list", figmatest.FileKey)
	ok(t, r)
	rows := list(t, r, "assets.list")
	if len(rows) != 4 {
		t.Fatalf("candidates: %v", rows)
	}
	icon := rows[0].(map[string]any)
	if icon["category"] != "icon" || icon["id"] != "2:9" || icon["name"] != "icon/arrow-right" || icon["type"] != "VECTOR" || icon["formats"] != "svg" {
		t.Fatalf("icon: %v", icon)
	}
	login := rows[1].(map[string]any)
	if login["category"] != "export" || login["id"] != "2:2" || login["formats"] != "png@2x" {
		t.Fatalf("login: %v", login)
	}
	arrow := rows[2].(map[string]any)
	if arrow["category"] != "export" || arrow["id"] != "2:9" || arrow["formats"] != "svg, png@2x" {
		t.Fatalf("arrow: %v", arrow)
	}
	fill := rows[3].(map[string]any)
	if fill["category"] != "imageFill" || fill["id"] != "img1" || fill["name"] != "Avatar (2:8)" || fill["type"] != "IMAGE" || fill["formats"] != "fill" {
		t.Fatalf("fill: %v", fill)
	}
	if api.Count(http.MethodGet, imagesAPIPath()) != 0 || api.Count(http.MethodGet, fileKeyPath("/images")) != 0 || api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatalf("list must only read the document: %v", api.Requests())
	}

	r = execute(t, "", "assets", "list", figmatest.FileKey, "--node", "2-7", "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "category   id    name") || !strings.Contains(r.stdout, "icon       2:9   icon/arrow-right  VECTOR  svg") || !strings.Contains(r.stdout, "imageFill  img1  Avatar (2:8)") || strings.Contains(r.stdout, "2:2") {
		t.Fatalf("table:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "hint: Export them with figctl assets export") {
		t.Fatalf("stderr: %s", r.stderr)
	}
	r = execute(t, "", "assets", "list", figmatest.FileKey, "--node", "2:14", "--icon-pattern", "stat*")
	ok(t, r)
	if rows := list(t, r, "assets.list"); len(rows) != 1 || rows[0].(map[string]any)["id"] != "2:14" || rows[0].(map[string]any)["type"] != "FRAME" {
		t.Fatalf("custom pattern: %v", rows)
	}
	r = execute(t, "", "assets", "list", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown node: exit = %d", r.code)
	}
	r = execute(t, "", "assets", "list", figmatest.FileKey, "--max-file-mb", "0", "--refresh")
	ok(t, r)
	r = execute(t, "", "assets", "list", figmatest.FileKey, "--max-file-mb", "0")
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "max-file-mb") {
		t.Fatalf("too large without --node: exit = %d, stdout = %s", r.code, r.stdout)
	}
}
