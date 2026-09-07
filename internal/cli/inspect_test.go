package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func TestNodeInspectGolden(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2")
	ok(t, r)
	checkGolden(t, "node_inspect_2-2.json", r.stdout)
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, fileKeyPath("/variables/local")) != 1 || api.Count(http.MethodGet, fileKeyPath("/nodes")) != 1 {
		t.Fatalf("requests: file=%d variables=%d nodes=%d", api.Count(http.MethodGet, fileKeyPath("")), api.Count(http.MethodGet, fileKeyPath("/variables/local")), api.Count(http.MethodGet, fileKeyPath("/nodes")))
	}
	for _, req := range api.Requests() {
		if req.Path == fileKeyPath("/nodes") && req.Query.Get("ids") != "5:1,5:2,5:3,5:4" {
			t.Fatalf("style nodes should be fetched in one batched request, got ids=%q", req.Query.Get("ids"))
		}
	}

	r = execute(t, "", "node", "inspect", "https://www.figma.com/design/"+figmatest.FileKey+"/App?node-id=2-2", "--depth", "1", "-o", "md")
	ok(t, r)
	checkGolden(t, "node_inspect_2-2_depth1.md", r.stdout)
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, fileKeyPath("/variables/local")) != 1 || api.Count(http.MethodGet, fileKeyPath("/nodes")) != 1 {
		t.Fatal("second inspect must be served from the cache")
	}

	r = execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:3", "--css")
	ok(t, r)
	checkGolden(t, "node_inspect_2-3_css.json", r.stdout)
}

func TestNodeInspectModel(t *testing.T) {
	setup(t)
	r := execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2", "--node", "9:9", "--depth", "2", "--css", "--web", "--interactions")
	ok(t, r)
	items := list(t, r, "node.inspect")
	if len(items) != 1 {
		t.Fatalf("expected one node, got %d", len(items))
	}
	env := decodeEnvelope(t, r.stdout)
	hints := env["hints"].([]any)
	if len(hints) != 1 || !strings.Contains(hints[0].(string), "Node 9:9 does not exist") {
		t.Fatalf("hints: %v", hints)
	}
	login := items[0].(map[string]any)
	layout := login["layout"].(map[string]any)
	if layout["mode"] != "column" || layout["gap"] != float64(16) || layout["justify"] != "flex-start" || layout["position"] != "absolute" {
		t.Fatalf("layout: %v", layout)
	}
	if layout["padding"].(map[string]any)["shorthand"] != "24px" {
		t.Fatalf("padding: %v", layout["padding"])
	}
	tokens := layout["tokens"].(map[string]any)
	gap := tokens["gap"].(map[string]any)
	if gap["name"] != "space/4" || gap["web"] != "var(--space-4)" || gap["values"].(map[string]any)["Default"] != float64(16) {
		t.Fatalf("gap token: %v", gap)
	}
	if v, present := tokens["paddingTop"]; !present || v != nil {
		t.Fatalf("paddingTop should be present and null: %v", tokens)
	}
	fill := login["visual"].(map[string]any)["fills"].([]any)[0].(map[string]any)
	token := fill["token"].(map[string]any)
	if fill["hex"] != "#ffffff" || token["name"] != "bg/surface" || token["values"].(map[string]any)["Dark"] != "#111827" || token["chain"].([]any)[1] != "neutral/0" {
		t.Fatalf("fill: %v", fill)
	}
	effect := login["visual"].(map[string]any)["effects"].([]any)[0].(map[string]any)
	if effect["css"] != "0px 4px 12px rgba(0, 0, 0, 0.1)" || effect["style"].(map[string]any)["name"] != "shadow/md" {
		t.Fatalf("effect: %v", effect)
	}
	css := login["css"].(map[string]any)
	if css["gap"] != "var(--space-4)" || css["background"] != "var(--bg-surface)" || css["display"] != "flex" || css["flex-direction"] != "column" || css["width"] != "390px" {
		t.Fatalf("css: %v", css)
	}
	children := login["children"].([]any)
	byID := map[string]map[string]any{}
	for _, c := range children {
		n := c.(map[string]any)
		byID[n["id"].(string)] = n
	}
	if _, hidden := byID["2:13"]; hidden {
		t.Fatal("hidden node should be skipped")
	}
	button := byID["2:6"]
	comp := button["component"].(map[string]any)
	if comp["mainComponentName"] != "Variant=Primary, Size=md" || comp["setName"] != "Button" || comp["variantProperties"].(map[string]any)["Size"] != "md" {
		t.Fatalf("component: %v", comp)
	}
	if comp["properties"].(map[string]any)["Label"].(map[string]any)["value"] != "Sign in" {
		t.Fatalf("properties: %v", comp["properties"])
	}
	proto := button["prototype"].(map[string]any)["interactions"].([]any)[0].(map[string]any)
	if proto["trigger"] != "ON_CLICK" || proto["destinationId"] != "2:14" || proto["transition"].(map[string]any)["type"] != "SMART_ANIMATE" {
		t.Fatalf("prototype: %v", proto)
	}
	body := byID["2:4"]
	runs := body["text"].(map[string]any)["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["text"] != "continue" || runs[0].(map[string]any)["overrides"].(map[string]any)["weight"] != float64(600) {
		t.Fatalf("runs: %v", runs)
	}
	stats := byID["2:14"]
	if stats["layout"].(map[string]any)["mode"] != "grid" || stats["css"].(map[string]any)["grid-template-columns"] != "minmax(0, 1fr) minmax(0, 1fr)" {
		t.Fatalf("grid: %v", stats["layout"])
	}
	stat := stats["children"].([]any)[0].(map[string]any)
	if stat["layout"].(map[string]any)["gridChild"].(map[string]any)["rowSpan"] != float64(2) || stat["css"].(map[string]any)["grid-row"] != "1 / span 2" {
		t.Fatalf("grid child: %v", stat["layout"])
	}
	if stat["childCount"] != nil || stats["childCount"] != float64(3) {
		t.Fatalf("child counts: stat=%v stats=%v", stat["childCount"], stats["childCount"])
	}
	field := byID["2:5"]["children"].([]any)[1].(map[string]any)
	if field["id"] != "I2:5;3:22" || field["children"] != nil || field["childCount"] != float64(1) {
		t.Fatalf("depth 2 should not expand grandchildren: %v", field)
	}

	r = execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2", "--include-hidden", "--depth", "1")
	ok(t, r)
	items = list(t, r, "node.inspect")
	found := false
	for _, c := range items[0].(map[string]any)["children"].([]any) {
		if c.(map[string]any)["id"] == "2:13" && c.(map[string]any)["visible"] == false {
			found = true
		}
	}
	if !found {
		t.Fatal("--include-hidden should keep 2:13")
	}

	r = execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2", "--max-nodes", "3", "--depth", "-1")
	ok(t, r)
	env = decodeEnvelope(t, r.stdout)
	if env["truncated"] != true {
		t.Fatalf("truncated: %v", env)
	}
	hints = env["hints"].([]any)
	if len(hints) == 0 || !strings.Contains(hints[len(hints)-1].(string), "--max-nodes") || !strings.Contains(hints[len(hints)-1].(string), "--depth") {
		t.Fatalf("truncation hint: %v", hints)
	}
	root := env["data"].([]any)[0].(map[string]any)
	if root["truncated"] != true || root["omitted"].(float64) <= 0 || len(root["children"].([]any)) != 2 {
		t.Fatalf("root truncation: truncated=%v omitted=%v children=%d", root["truncated"], root["omitted"], len(root["children"].([]any)))
	}

	r = execute(t, "", "node", "inspect", figmatest.FileKey)
	if r.code != figctl.ExitUsage {
		t.Fatalf("missing node: exit = %d", r.code)
	}
	r = execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("unknown node: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2", "--depth", "1", "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "id    type      name") || !strings.Contains(r.stdout, "2:3   TEXT        Heading") || !strings.Contains(r.stdout, "#ffffff [bg/surface]") {
		t.Fatalf("table:\n%s", r.stdout)
	}
}

func TestNodeInspectWithoutVariables(t *testing.T) {
	api := setup(t)
	api.Respond(http.MethodGet, fileKeyPath("/variables/local"), figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "This endpoint is only available to Enterprise plan users."}})
	r := execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:3")
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	hints := env["hints"].([]any)
	if len(hints) != 1 || !strings.Contains(hints[0].(string), "Enterprise") || !strings.Contains(hints[0].(string), "styles and raw values") {
		t.Fatalf("hints: %v", hints)
	}
	if !strings.Contains(r.stderr, "warning: Variables need an Enterprise plan") {
		t.Fatalf("stderr: %s", r.stderr)
	}
	heading := env["data"].([]any)[0].(map[string]any)
	fill := heading["visual"].(map[string]any)["fills"].([]any)[0].(map[string]any)
	token := fill["token"].(map[string]any)
	if fill["hex"] != "#111827" || token["variableId"] != "VariableID:1:202" || token["unresolved"] != true || token["name"] != nil {
		t.Fatalf("fill without variables: %v", fill)
	}
	if heading["text"].(map[string]any)["style"].(map[string]any)["name"] != "heading/lg" {
		t.Fatalf("style ref should still resolve: %v", heading["text"])
	}

	api.Respond(http.MethodGet, fileKeyPath("/variables/local"), figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Invalid scope(s): file_variables:read required"}})
	r = execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:3")
	ok(t, r)
	env = decodeEnvelope(t, r.stdout)
	hints = env["hints"].([]any)
	if len(hints) != 1 || !strings.Contains(hints[0].(string), "file_variables:read") {
		t.Fatalf("scope hint: %v", hints)
	}

	r = execute(t, "", "variables", "list", figmatest.FileKey)
	if r.code != figctl.ExitAuth || errorCode(t, r) != "AUTH_SCOPE" {
		t.Fatalf("variables list without scope: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestVariablesCommands(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "variables", "list", figmatest.FileKey)
	ok(t, r)
	items := list(t, r, "variables.list")
	if len(items) != 8 {
		t.Fatalf("expected 8 variables, got %d", len(items))
	}
	byName := map[string]map[string]any{}
	for _, it := range items {
		v := it.(map[string]any)
		byName[v["name"].(string)] = v
	}
	surface := byName["bg/surface"]
	if surface["collection"] != "Semantic" || surface["type"] != "COLOR" || surface["values"].(map[string]any)["Light"] != "#ffffff" || surface["values"].(map[string]any)["Dark"] != "#111827" {
		t.Fatalf("bg/surface: %v", surface)
	}
	if surface["codeSyntax"].(map[string]any)["WEB"] != "--bg-surface" || surface["hiddenFromPublishing"] != false {
		t.Fatalf("bg/surface metadata: %v", surface)
	}
	if byName["feature/show-helper"]["hiddenFromPublishing"] != true || byName["feature/show-helper"]["values"].(map[string]any)["Default"] != true {
		t.Fatalf("show-helper: %v", byName["feature/show-helper"])
	}
	if byName["space/4"]["values"].(map[string]any)["Default"] != float64(16) || byName["font/family/sans"]["values"].(map[string]any)["Default"] != "Inter" {
		t.Fatalf("scalar values: %v %v", byName["space/4"], byName["font/family/sans"])
	}

	r = execute(t, "", "variables", "list", figmatest.FileKey, "--collection", "Semantic", "--mode", "Dark", "--type", "color", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "bg/surface    Semantic    COLOR  Dark=#111827") || strings.Contains(r.stdout, "Light=") || strings.Contains(r.stdout, "brand/500") {
		t.Fatalf("filtered table:\n%s", r.stdout)
	}
	r = execute(t, "", "variables", "list", figmatest.FileKey, "--type", "bogus")
	if r.code != figctl.ExitUsage {
		t.Fatalf("bad type: exit = %d", r.code)
	}

	r = execute(t, "", "variables", "list", figmatest.FileKey, "--published")
	ok(t, r)
	items = list(t, r, "variables.list")
	if len(items) != 7 || items[0].(map[string]any)["subscribedId"] == "" {
		t.Fatalf("published: %v", items)
	}

	r = execute(t, "", "variables", "get", figmatest.FileKey, "--name", "text/primary")
	ok(t, r)
	d := data(t, r, "variables.get")
	modes := d["modes"].([]any)
	if d["id"] != "VariableID:1:202" || len(modes) != 2 {
		t.Fatalf("detail: %v", d)
	}
	light := modes[0].(map[string]any)
	if light["mode"] != "Light" || light["default"] != true || light["value"] != "#111827" || light["chain"].([]any)[1] != "neutral/900" {
		t.Fatalf("light mode: %v", light)
	}
	r = execute(t, "", "variables", "get", figmatest.FileKey, "--id", "VariableID:1:201/1", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "mode Light            #ffffff (bg/surface -> neutral/0)") || !strings.Contains(r.stdout, "mode Dark             #111827 (bg/surface -> neutral/900)") {
		t.Fatalf("detail table:\n%s", r.stdout)
	}
	r = execute(t, "", "variables", "get", figmatest.FileKey, "--name", "nope")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown name: exit = %d", r.code)
	}
	r = execute(t, "", "variables", "get", figmatest.FileKey)
	if r.code != figctl.ExitUsage {
		t.Fatalf("missing selector: exit = %d", r.code)
	}
	if api.Count(http.MethodGet, fileKeyPath("/variables/local")) != 1 {
		t.Fatalf("variables should be cached across runs, got %d fetches", api.Count(http.MethodGet, fileKeyPath("/variables/local")))
	}

	api.Respond(http.MethodGet, fileKeyPath("/variables/local"), figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Enterprise plan required"}})
	r = execute(t, "", "variables", "list", figmatest.FileKey, "--refresh")
	if r.code != figctl.ExitError || errorCode(t, r) != "PLAN_REQUIRED" {
		t.Fatalf("plan gate: exit = %d, stdout = %s", r.code, r.stdout)
	}
	if !strings.Contains(r.stdout, "figctl styles list") {
		t.Fatalf("plan gate hint should name the styles fallback: %s", r.stdout)
	}
}

func TestTokensResolve(t *testing.T) {
	setup(t)
	r := execute(t, "", "tokens", "resolve", figmatest.FileKey, "--name", "bg/surface")
	ok(t, r)
	d := data(t, r, "tokens.resolve")
	if d["kind"] != "variable" {
		t.Fatalf("kind: %v", d)
	}
	v := d["variable"].(map[string]any)
	if v["name"] != "bg/surface" || len(v["modes"].([]any)) != 2 {
		t.Fatalf("variable: %v", v)
	}
	r = execute(t, "", "tokens", "resolve", figmatest.FileKey, "--id", "VariableID:1:201", "--mode", "dark")
	ok(t, r)
	d = data(t, r, "tokens.resolve")
	modes := d["variable"].(map[string]any)["modes"].([]any)
	if len(modes) != 1 || modes[0].(map[string]any)["mode"] != "Dark" || modes[0].(map[string]any)["value"] != "#111827" {
		t.Fatalf("mode filter: %v", modes)
	}
	r = execute(t, "", "tokens", "resolve", figmatest.FileKey, "--id", "VariableID:1:201", "--mode", "Sepia")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown mode: exit = %d", r.code)
	}

	r = execute(t, "", "tokens", "resolve", figmatest.FileKey, "--id", "5-2")
	ok(t, r)
	d = data(t, r, "tokens.resolve")
	if d["kind"] != "style" {
		t.Fatalf("kind: %v", d)
	}
	style := d["style"].(map[string]any)
	typo := style["value"].(map[string]any)["typography"].(map[string]any)
	if style["name"] != "heading/lg" || typo["fontFamily"] != "Inter" || typo["weight"] != float64(700) || typo["lineHeight"].(map[string]any)["unitless"] != 1.21 {
		t.Fatalf("style: %v", style)
	}
	if typo["tokens"].(map[string]any)["fontFamily"].(map[string]any)["name"] != "font/family/sans" {
		t.Fatalf("style token: %v", typo["tokens"])
	}
	r = execute(t, "", "tokens", "resolve", figmatest.FileKey, "--name", "shadow/md", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "value        box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1)") {
		t.Fatalf("style table:\n%s", r.stdout)
	}
	r = execute(t, "", "tokens", "resolve", figmatest.FileKey, "--name", "missing/token")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("missing: exit = %d", r.code)
	}
	r = execute(t, "", "tokens", "resolve", figmatest.FileKey)
	if r.code != figctl.ExitUsage {
		t.Fatalf("no selector: exit = %d", r.code)
	}
}

func TestStylesCommands(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "styles", "list", figmatest.FileKey)
	ok(t, r)
	items := list(t, r, "styles.list")
	if len(items) != 4 {
		t.Fatalf("expected 4 styles, got %d", len(items))
	}
	byName := map[string]map[string]any{}
	for _, it := range items {
		s := it.(map[string]any)
		byName[s["name"].(string)] = s
	}
	fill := byName["color/brand/500"]
	if fill["type"] != "FILL" || fill["id"] != "5:1" || fill["value"].(map[string]any)["fills"].([]any)[0].(map[string]any)["hex"] != "#3366ff" {
		t.Fatalf("fill style: %v", fill)
	}
	if fill["value"].(map[string]any)["fills"].([]any)[0].(map[string]any)["token"].(map[string]any)["name"] != "brand/500" {
		t.Fatalf("fill style token: %v", fill)
	}
	grid := byName["grid/12"]["value"].(map[string]any)["grids"].([]any)[0].(map[string]any)
	if grid["pattern"] != "columns" || grid["count"] != float64(12) || grid["gutter"] != float64(16) {
		t.Fatalf("grid style: %v", grid)
	}
	if api.Count(http.MethodGet, fileKeyPath("/nodes")) != 1 {
		t.Fatalf("style nodes should be one batched request, got %d", api.Count(http.MethodGet, fileKeyPath("/nodes")))
	}

	r = execute(t, "", "styles", "list", figmatest.FileKey, "--type", "TEXT", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "5:2  TEXT  heading/lg  Inter [font/family/sans] 700 28px/34px tracking -0.5px") || strings.Contains(r.stdout, "shadow/md") {
		t.Fatalf("table:\n%s", r.stdout)
	}
	r = execute(t, "", "styles", "list", figmatest.FileKey, "--limit", "3")
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	if len(env["data"].([]any)) != 3 || env["nextCursor"] != "3" {
		t.Fatalf("pagination: %v", env["nextCursor"])
	}
	r = execute(t, "", "styles", "list", figmatest.FileKey, "--limit", "3", "--cursor", "3")
	ok(t, r)
	env = decodeEnvelope(t, r.stdout)
	if len(env["data"].([]any)) != 1 || env["nextCursor"] != nil {
		t.Fatalf("second page: %v", env)
	}

	r = execute(t, "", "styles", "list", "--team", figmatest.TeamID, "--limit", "2")
	ok(t, r)
	env = decodeEnvelope(t, r.stdout)
	items = env["data"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["value"] != nil || env["nextCursor"] != "2" {
		t.Fatalf("team styles: %v", env)
	}
	if hints := env["hints"].([]any); len(hints) != 1 || !strings.Contains(hints[0].(string), "owning file") {
		t.Fatalf("team hint: %v", hints)
	}
	r = execute(t, "", "styles", "list")
	if r.code != figctl.ExitUsage {
		t.Fatalf("no ref or team: exit = %d", r.code)
	}

	r = execute(t, "", "styles", "get", figmatest.FileKey, "--node", "5-3")
	ok(t, r)
	d := data(t, r, "styles.get")
	effects := d["value"].(map[string]any)["effects"].([]any)
	if d["name"] != "shadow/md" || effects[0].(map[string]any)["css"] != "0px 4px 12px rgba(0, 0, 0, 0.1)" || effects[0].(map[string]any)["property"] != "box-shadow" {
		t.Fatalf("effect style: %v", d)
	}
	r = execute(t, "", "styles", "get", figmatest.FileKey, "--key", "5f4e3d2c1b0a9f8e7d6c5b4a39281706f5e4d502", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "name         heading/lg") || !strings.Contains(r.stdout, "value        Inter [font/family/sans] 700 28px/34px") {
		t.Fatalf("get by key table:\n%s", r.stdout)
	}
	r = execute(t, "", "styles", "get", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown style: exit = %d", r.code)
	}
	r = execute(t, "", "styles", "get", figmatest.FileKey)
	if r.code != figctl.ExitUsage {
		t.Fatalf("no selector: exit = %d", r.code)
	}
}
