package cli

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// renderCount counts image render requests for one format.
func renderCount(api *figmatest.Server, format string) int {
	n := 0
	for _, req := range api.Requests() {
		if req.Path == imagesAPIPath() && req.Query.Get("format") == format {
			n++
		}
	}
	return n
}

// contextRun runs node context for one node into dir and returns the data.
func contextRun(t *testing.T, dir string, args ...string) (result, map[string]any) {
	t.Helper()
	full := append([]string{"node", "context", figmatest.FileKey, "--node", "2:2", "--out", dir}, args...)
	r := execute(t, "", full...)
	ok(t, r)
	return r, data(t, r, "node.context")
}

func nodeEntry(t *testing.T, d map[string]any, i int) map[string]any {
	t.Helper()
	nodes := items(t, d, "nodes")
	if len(nodes) <= i {
		t.Fatalf("nodes has %d entries, want more than %d", len(nodes), i)
	}
	return nodes[i]
}

func summaryOf(t *testing.T, d map[string]any) map[string]any {
	t.Helper()
	s, isObject := d["summary"].(map[string]any)
	if !isObject {
		t.Fatalf("summary is %T", d["summary"])
	}
	return s
}

func TestNodeContext(t *testing.T) {
	api := setup(t)
	out := filepath.Join(t.TempDir(), "context")
	r, d := contextRun(t, out)

	if d["outDir"] != out {
		t.Fatalf("outDir = %v, want %s", d["outDir"], out)
	}

	// The screenshot is a PNG at 2x on disk, named after the node.
	node := nodeEntry(t, d, 0)
	if node["id"] != "2:2" || node["name"] != "Login" || node["type"] != "FRAME" || node["page"] != "Screens" {
		t.Fatalf("node: %v", node)
	}
	if path := node["path"].([]any); len(path) != 3 || path[0] != "Screens" || path[2] != "Login" {
		t.Fatalf("path: %v", node["path"])
	}
	shot, isObject := node["screenshot"].(map[string]any)
	if !isObject {
		t.Fatalf("screenshot is %T: %v", node["screenshot"], node)
	}
	want := filepath.Join(out, "login@2x.png")
	if shot["path"] != want || shot["format"] != "png" || shot["scale"] != float64(2) || shot["cached"] != false {
		t.Fatalf("screenshot: %v", shot)
	}
	mustExist(t, want)

	// The inspected model is there with CSS and interactions.
	inspected, isObject := node["inspect"].(map[string]any)
	if !isObject {
		t.Fatalf("inspect is %T", node["inspect"])
	}
	if css := inspected["css"].(map[string]any); css["gap"] != "var(--space-4)" || css["display"] != "flex" {
		t.Fatalf("css: %v", inspected["css"])
	}
	if len(inspected["children"].([]any)) == 0 {
		t.Fatal("inspect should include children at depth 4")
	}

	// The icon 2:9 is exported as SVG and the image fill img1 downloaded.
	assetsBlock, isObject := d["assets"].(map[string]any)
	if !isObject {
		t.Fatalf("assets is %T", d["assets"])
	}
	icons := items(t, assetsBlock, "icons")
	if len(icons) != 1 || icons[0]["nodeId"] != "2:9" || icons[0]["format"] != "svg" {
		t.Fatalf("icons: %v", icons)
	}
	iconPath := filepath.Join(out, "icon-arrow-right.svg")
	if icons[0]["path"] != iconPath {
		t.Fatalf("icon path = %v, want %s", icons[0]["path"], iconPath)
	}
	mustExist(t, iconPath)
	fills := items(t, assetsBlock, "imageFills")
	if len(fills) != 1 || fills[0]["imageRef"] != "img1" || fills[0]["error"] != nil {
		t.Fatalf("imageFills: %v", fills)
	}
	mustExist(t, fills[0]["path"].(string))
	if users := fills[0]["nodes"].([]any); len(users) != 1 || users[0].(map[string]any)["nodeId"] != "2:8" {
		t.Fatalf("fill users: %v", fills[0]["nodes"])
	}

	// Only the tokens the subtree uses, with values per mode.
	tokens := items(t, d, "tokens")
	byName := map[string]map[string]any{}
	for _, tok := range tokens {
		byName[tok["name"].(string)] = tok
	}
	for _, name := range []string{"space/4", "bg/surface", "text/primary", "brand/500", "neutral/0", "font/family/sans"} {
		if _, found := byName[name]; !found {
			t.Fatalf("token %s missing from %v", name, figmatest.SortedKeys(byName))
		}
	}
	space := byName["space/4"]
	if space["collection"] != "Primitives" || space["resolvedType"] != "FLOAT" {
		t.Fatalf("space/4: %v", space)
	}
	if space["values"].(map[string]any)["Default"] != float64(16) {
		t.Fatalf("space/4 values: %v", space["values"])
	}
	if space["codeSyntax"].(map[string]any)["WEB"] != "--space-4" {
		t.Fatalf("space/4 codeSyntax: %v", space["codeSyntax"])
	}
	surface := byName["bg/surface"]
	if surface["collection"] != "Semantic" {
		t.Fatalf("bg/surface: %v", surface)
	}
	modes := surface["values"].(map[string]any)
	if modes["Light"] != "#ffffff" || modes["Dark"] != "#111827" {
		t.Fatalf("bg/surface values: %v", modes)
	}
	// Variables not bound anywhere in the subtree stay out of the bundle.
	if _, leaked := byName["feature/show-helper"]; leaked {
		t.Fatalf("unused variables leaked into the bundle: %v", figmatest.SortedKeys(byName))
	}

	styles := items(t, d, "styles")
	styleNames := map[string]bool{}
	for _, s := range styles {
		styleNames[s["name"].(string)] = true
	}
	for _, name := range []string{"color/brand/500", "heading/lg", "shadow/md", "grid/12"} {
		if !styleNames[name] {
			t.Fatalf("style %s missing from %v", name, styles)
		}
	}

	// Every instance in the subtree brings its component definition.
	components := items(t, d, "components")
	byComponent := map[string]map[string]any{}
	for _, c := range components {
		byComponent[c["name"].(string)] = c
	}
	button := byComponent["Variant=Primary, Size=md"]
	if button == nil {
		t.Fatalf("Button variant missing from components: %v", components)
	}
	if button["setName"] != "Button" || button["setNodeId"] != "3:10" || button["nodeId"] != "3:11" {
		t.Fatalf("button: %v", button)
	}
	if variants := button["variantProperties"].(map[string]any); variants["Variant"] != "Primary" || variants["Size"] != "md" {
		t.Fatalf("button variantProperties: %v", button["variantProperties"])
	}
	defs, isObject := button["propertyDefinitions"].(map[string]any)
	if !isObject {
		t.Fatalf("button propertyDefinitions: %v", button["propertyDefinitions"])
	}
	variant, isObject := defs["Variant"].(map[string]any)
	if !isObject || variant["type"] != "VARIANT" {
		t.Fatalf("Variant definition: %v", defs)
	}
	options := variant["variantOptions"].([]any)
	if len(options) != 2 || options[0] != "Primary" || options[1] != "Secondary" {
		t.Fatalf("Variant options: %v", options)
	}
	if size := defs["Size"].(map[string]any); size["type"] != "VARIANT" {
		t.Fatalf("Size definition: %v", defs)
	}
	if _, hasLabel := defs["Label#5:0"]; !hasLabel {
		t.Fatalf("Label property definition missing: %v", figmatest.SortedKeys(defs))
	}
	if instances := button["instances"].([]any); len(instances) != 1 || instances[0] != "2:6" {
		t.Fatalf("button instances: %v", button["instances"])
	}
	// The Storybook dev resource on the main component reaches the agent
	// even though it is not attached to the instance.
	drs := button["devResources"].([]any)
	if len(drs) != 1 || drs[0].(map[string]any)["id"] != "dr-1" {
		t.Fatalf("button devResources: %v", drs)
	}
	input := byComponent["Input/Text"]
	if input == nil || input["nodeId"] != "3:20" {
		t.Fatalf("Input/Text missing: %v", components)
	}
	if _, hasHelper := input["propertyDefinitions"].(map[string]any)["Show helper#12:1"]; !hasHelper {
		t.Fatalf("Input/Text definitions: %v", input["propertyDefinitions"])
	}

	// Comments pinned on the node or a descendant, plus their replies.
	comments := items(t, d, "comments")
	if len(comments) != 2 || comments[0]["id"] != "9001" || comments[1]["id"] != "9002" {
		t.Fatalf("comments: %v", comments)
	}
	if comments[0]["nodeId"] != "2:2" || comments[0]["user"] != figmatest.UserHandle || comments[0]["createdAt"] == "" {
		t.Fatalf("comment 9001: %v", comments[0])
	}
	if comments[0]["resolvedAt"] != nil {
		t.Fatalf("comment 9001 should be open: %v", comments[0])
	}

	// Dev resources on the subtree itself: the fixture attaches none.
	if resources := items(t, d, "devResources"); len(resources) != 0 {
		t.Fatalf("devResources: %v", resources)
	}

	summary := summaryOf(t, d)
	if summary["page"] != "Screens" || summary["tokens"] != float64(len(tokens)) || summary["components"] != float64(2) {
		t.Fatalf("summary: %v", summary)
	}
	if summary["assets"] != float64(2) || summary["comments"] != float64(2) {
		t.Fatalf("summary counts: %v", summary)
	}
	if summary["nodes"].(float64) < 10 {
		t.Fatalf("summary nodes = %v, want the whole subtree", summary["nodes"])
	}
	if failures := d["failures"].([]any); len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}

	joinedHints := strings.Join(hints(t, r), "\n")
	for _, want := range []string{want, "vision tool", "\"token\": null", "figctl render " + figmatest.FileKey} {
		if !strings.Contains(joinedHints, want) {
			t.Fatalf("hints missing %q:\n%s", want, joinedHints)
		}
	}

	// A second run is served from the blob cache: no render and no image
	// fill request at all.
	api.Reset()
	_, d = contextRun(t, out)
	if api.CountPrefix(http.MethodGet, "/v1/images/") != 0 || api.Count(http.MethodGet, fileKeyPath("/images")) != 0 {
		t.Fatalf("second run made image requests: renders=%d fills=%d", api.CountPrefix(http.MethodGet, "/v1/images/"), api.Count(http.MethodGet, fileKeyPath("/images")))
	}
	if d["requests"] != float64(0) {
		t.Fatalf("requests = %v, want 0 on a cached run", d["requests"])
	}
	if nodeEntry(t, d, 0)["screenshot"].(map[string]any)["cached"] != true {
		t.Fatal("the second screenshot should come from the cache")
	}
}

func TestNodeContextMarkdownGolden(t *testing.T) {
	setup(t)
	out := filepath.Join(t.TempDir(), "context")
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:2", "--out", out, "--depth", "2", "-o", "md")
	ok(t, r)
	// Output directories differ per run, so the golden holds a placeholder.
	got := strings.ReplaceAll(r.stdout, out, "<OUT>")
	checkGolden(t, "node_context_2-2.md", got)
}

func TestNodeContextBudgetFlags(t *testing.T) {
	api := setup(t)
	root := t.TempDir()

	out := filepath.Join(root, "no-screenshot")
	r, d := contextRun(t, out, "--no-screenshot")
	if _, present := nodeEntry(t, d, 0)["screenshot"]; present {
		t.Fatalf("--no-screenshot still rendered: %v", d["nodes"])
	}
	if renderCount(api, "png") != 0 || renderCount(api, "svg") != 1 {
		t.Fatalf("--no-screenshot requests: png=%d svg=%d", renderCount(api, "png"), renderCount(api, "svg"))
	}
	if len(items(t, d["assets"].(map[string]any), "icons")) != 1 {
		t.Fatal("--no-screenshot should keep the assets")
	}
	if !strings.Contains(strings.Join(hints(t, r), "\n"), "figctl render") {
		t.Fatalf("--no-screenshot hints: %v", hints(t, r))
	}

	api.Reset()
	out = filepath.Join(root, "no-assets")
	r, d = contextRun(t, out, "--no-assets")
	if renderCount(api, "png") != 1 || renderCount(api, "svg") != 0 {
		t.Fatalf("--no-assets requests: png=%d svg=%d", renderCount(api, "png"), renderCount(api, "svg"))
	}
	if api.Count(http.MethodGet, fileKeyPath("/images")) != 0 {
		t.Fatal("--no-assets should not download image fills")
	}
	assetsBlock := d["assets"].(map[string]any)
	if len(items(t, assetsBlock, "icons")) != 0 || len(items(t, assetsBlock, "imageFills")) != 0 {
		t.Fatalf("--no-assets assets: %v", assetsBlock)
	}
	if !strings.Contains(strings.Join(hints(t, r), "\n"), "figctl assets export") {
		t.Fatalf("--no-assets hints: %v", hints(t, r))
	}

	api.Reset()
	out = filepath.Join(root, "no-comments")
	_, d = contextRun(t, out, "--no-comments")
	if api.Count(http.MethodGet, fileKeyPath("/comments")) != 0 || api.Count(http.MethodGet, fileKeyPath("/dev_resources")) != 0 {
		t.Fatal("--no-comments should read neither comments nor dev resources")
	}
	if len(items(t, d, "comments")) != 0 {
		t.Fatalf("comments: %v", d["comments"])
	}

	out = filepath.Join(root, "budget")
	r, d = contextRun(t, out, "--max-nodes", "3", "--no-css", "--no-screenshot", "--no-assets")
	if summaryOf(t, d)["nodes"] != float64(3) {
		t.Fatalf("--max-nodes 3 inspected %v nodes", summaryOf(t, d)["nodes"])
	}
	if _, hasCSS := nodeEntry(t, d, 0)["inspect"].(map[string]any)["css"]; hasCSS {
		t.Fatal("--no-css should drop the css map")
	}
	if decodeEnvelope(t, r.stdout)["truncated"] != true {
		t.Fatalf("truncated = %v", decodeEnvelope(t, r.stdout)["truncated"])
	}
	if !strings.Contains(strings.Join(hints(t, r), "\n"), "--max-nodes") {
		t.Fatalf("hints: %v", hints(t, r))
	}
}

func mustRun(t *testing.T, args ...string) result {
	t.Helper()
	r := execute(t, "", args...)
	ok(t, r)
	return r
}

func TestNodeContextWithoutVariables(t *testing.T) {
	api := setup(t)
	api.Respond(http.MethodGet, fileKeyPath("/variables/local"), figmatest.Response{
		Status: 403,
		Body:   map[string]any{"status": 403, "err": "This endpoint is only available to Enterprise plan users."},
	})
	out := filepath.Join(t.TempDir(), "context")
	r, d := contextRun(t, out)

	mustExist(t, filepath.Join(out, "login@2x.png"))
	if len(items(t, d, "components")) != 2 {
		t.Fatalf("components should still resolve: %v", d["components"])
	}
	styles := items(t, d, "styles")
	if len(styles) == 0 {
		t.Fatal("styles should still resolve without variables")
	}
	// Bound values are still reported, by id, so it is clear they exist.
	tokens := items(t, d, "tokens")
	if len(tokens) == 0 {
		t.Fatal("bound variables should be listed as unresolved references")
	}
	for _, tok := range tokens {
		if tok["unresolved"] != true || tok["variableId"] == "" {
			t.Fatalf("token should be an unresolved reference: %v", tok)
		}
	}
	joinedHints := strings.Join(hints(t, r), "\n")
	if !strings.Contains(joinedHints, "Enterprise plan") {
		t.Fatalf("hints should explain the missing variables:\n%s", joinedHints)
	}
}

func TestNodeContextErrors(t *testing.T) {
	setup(t)
	r := execute(t, "", "node", "context", figmatest.FileKey)
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "node", "context", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestNodeContextPartialOnRenderFailure(t *testing.T) {
	setup(t)
	out := filepath.Join(t.TempDir(), "context")
	// 2:13 is hidden in the fixture, so Figma renders nothing for it.
	r := execute(t, "", "node", "context", figmatest.FileKey, "--node", "2:13", "--out", out, "--no-assets", "--no-comments")
	if r.code != figctl.ExitPartial {
		t.Fatalf("exit = %d, want %d\nstdout: %s", r.code, figctl.ExitPartial, r.stdout)
	}
	d := data(t, r, "node.context")
	failures := items(t, d, "failures")
	if len(failures) != 1 || failures[0]["kind"] != failureScreenshot || failures[0]["id"] != "2:13" {
		t.Fatalf("failures: %v", failures)
	}
	if len(items(t, d, "nodes")) != 1 {
		t.Fatal("the rest of the bundle should survive a screenshot failure")
	}
}

func TestSchemaCommand(t *testing.T) {
	setup(t)

	r := mustRun(t, "schema", "node.inspect")
	env := decodeEnvelope(t, r.stdout)
	schema, isObject := env["data"].(map[string]any)
	if !isObject {
		t.Fatalf("data is %T", env["data"])
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %v", schema["$schema"])
	}
	// node inspect returns a list of nodes, so the object shape lives in
	// the item definition the array references.
	if schema["type"] != "array" || schema["items"] == nil {
		t.Fatalf("node.inspect schema should be an array: %v", schema["type"])
	}
	defs, isObject := schema["$defs"].(map[string]any)
	if !isObject {
		t.Fatalf("$defs is %T", schema["$defs"])
	}
	node, isObject := defs["Node"].(map[string]any)
	if !isObject {
		t.Fatalf("$defs.Node is %T", defs["Node"])
	}
	props, isObject := node["properties"].(map[string]any)
	if !isObject {
		t.Fatalf("Node has no properties: %v", node)
	}
	for _, field := range []string{"id", "name", "type", "layout", "visual", "tokens", "children"} {
		if _, present := props[field]; !present {
			t.Fatalf("Node.properties is missing %s: %v", field, figmatest.SortedKeys(props))
		}
	}
	// The schema round-trips as JSON Schema, so an agent can feed it to a
	// validator without further processing.
	raw, err := json.Marshal(schema)
	if err != nil || len(raw) == 0 {
		t.Fatalf("schema does not re-encode: %v", err)
	}

	r = mustRun(t, "schema", "node.context")
	schema = decodeEnvelope(t, r.stdout)["data"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("node.context schema type = %v", schema["type"])
	}
	props = schema["properties"].(map[string]any)
	for _, field := range []string{"outDir", "nodes", "tokens", "styles", "components", "assets", "comments", "devResources", "summary", "failures"} {
		if _, present := props[field]; !present {
			t.Fatalf("node.context properties missing %s: %v", field, figmatest.SortedKeys(props))
		}
	}

	r = mustRun(t, "schema", "envelope")
	props = decodeEnvelope(t, r.stdout)["data"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"schemaVersion", "command", "data", "hints", "truncated"} {
		if _, present := props[field]; !present {
			t.Fatalf("envelope properties missing %s: %v", field, figmatest.SortedKeys(props))
		}
	}

	// The field list is what markdown and table output render.
	r = mustRun(t, "schema", "file.tree", "-o", "table")
	if !strings.Contains(r.stdout, "path") || !strings.Contains(r.stdout, "[].id") {
		t.Fatalf("table output:\n%s", r.stdout)
	}
}

func TestSchemaList(t *testing.T) {
	setup(t)
	r := mustRun(t, "schema")
	entries := rows(t, r, "schema")
	byName := map[string]map[string]any{}
	for _, row := range entries {
		byName[row["command"].(string)] = row
	}
	for _, name := range []string{"envelope", "node.context", "node.inspect", "file.tree", "file.find", "components.list", "render", "assets.export", "tokens.export", "variables.list", "styles.list"} {
		row, present := byName[name]
		if !present {
			t.Fatalf("%s is not listed: %v", name, figmatest.SortedKeys(byName))
		}
		if row["schema"] != true {
			t.Fatalf("%s should have a schema: %v", name, row)
		}
	}
	// Payloads that are not a figctl type say so instead of pretending.
	for _, name := range []string{"file.get"} {
		row := byName[name]
		if row == nil || row["schema"] != false || row["note"] == "" {
			t.Fatalf("%s should be listed without a schema and with a note: %v", name, row)
		}
	}
	if !strings.Contains(strings.Join(hints(t, r), "\n"), "figctl schema node.context") {
		t.Fatalf("hints: %v", hints(t, r))
	}
}

func TestSchemaUnknownCommand(t *testing.T) {
	setup(t)
	r := execute(t, "", "schema", "node.contextual")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
	e := decodeEnvelope(t, r.stdout)["error"].(map[string]any)
	hint, _ := e["hint"].(string)
	for _, want := range []string{"node.context", "file.tree", "envelope"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint should list valid names, got %q", hint)
		}
	}

	// A command whose payload has no schema explains why.
	r = execute(t, "", "schema", "file.get")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
	e = decodeEnvelope(t, r.stdout)["error"].(map[string]any)
	if !strings.Contains(e["hint"].(string), "raw Figma") {
		t.Fatalf("hint: %v", e["hint"])
	}

	// Spaces and slashes are accepted for the dotted name.
	mustRun(t, "schema", "node context")
	mustRun(t, "schema", "node/context")
}
