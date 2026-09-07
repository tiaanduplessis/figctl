package cli

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

var updateGolden = flag.Bool("update", false, "update golden files under testdata")

// testdataDir is resolved before any test changes the working directory.
var testdataDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(wd, "testdata")
}()

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(testdataDir, name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test ./internal/cli -update): %v", name, err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// rows decodes a list envelope into row objects.
func rows(t *testing.T, r result, command string) []map[string]any {
	t.Helper()
	items := list(t, r, command)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, item.(map[string]any))
	}
	return out
}

func ids(rows []map[string]any, field string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		s, _ := row[field].(string)
		out = append(out, s)
	}
	return out
}

func joined(ss []string) string { return strings.Join(ss, ",") }

func flagsOf(row map[string]any) map[string]any {
	f, _ := row["flags"].(map[string]any)
	return f
}

func hints(t *testing.T, r result) []string {
	t.Helper()
	env := decodeEnvelope(t, r.stdout)
	raw, _ := env["hints"].([]any)
	out := make([]string, 0, len(raw))
	for _, h := range raw {
		out = append(out, h.(string))
	}
	return out
}

func nextCursor(t *testing.T, r result) (string, bool) {
	t.Helper()
	env := decodeEnvelope(t, r.stdout)
	cursor, ok := env["nextCursor"].(string)
	truncated, _ := env["truncated"].(bool)
	if ok != truncated {
		t.Fatalf("nextCursor %v and truncated %v disagree", env["nextCursor"], truncated)
	}
	return cursor, ok
}

func TestFileTreeDefault(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "file", "tree", figmatest.FileKey)
	ok(t, r)
	checkGolden(t, "file_tree.golden", r.stdout)
	got := rows(t, r, "file.tree")
	if joined(ids(got, "id")) != "0:1,2:1,1:2,3:10,3:20,4:0" {
		t.Fatalf("default rows: %v", ids(got, "id"))
	}
	if got[0]["depth"] != float64(1) || got[1]["depth"] != float64(2) || got[0]["childCount"] != float64(1) {
		t.Fatalf("depth and child count: %v", got[0])
	}
	if _, has := got[0]["w"]; has {
		t.Fatalf("pages have no bounding box: %v", got[0])
	}
	if flagsOf(got[1])["devStatus"] != "READY_FOR_DEV" || got[1]["x"] != float64(-40) || got[1]["w"] != float64(470) {
		t.Fatalf("section row: %v", got[1])
	}
	if flagsOf(got[3])["component"] != true || flagsOf(got[4])["component"] != true || len(flagsOf(got[5])) != 1 || flagsOf(got[5])["autoLayout"] != "row" {
		t.Fatalf("component flags: %v %v %v", got[3], got[4], got[5])
	}
	if h := hints(t, r); len(h) != 1 || !strings.Contains(h[0], "node context") {
		t.Fatalf("hints: %v", h)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatalf("file fetched %d times", api.Count(http.MethodGet, fileKeyPath("")))
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "--node", "2:2", "--depth", "1")
	ok(t, r)
	got = rows(t, r, "file.tree")
	if joined(ids(got, "id")) != "2:2,2:3,2:4,2:5,2:6,2:7,2:10,2:13,2:14" {
		t.Fatalf("login rows: %v", ids(got, "id"))
	}
	login := got[0]
	if login["depth"] != float64(0) || login["x"] != float64(0) || login["w"] != float64(390) || login["h"] != float64(844) || login["childCount"] != float64(8) {
		t.Fatalf("login row: %v", login)
	}
	if f := flagsOf(login); f["autoLayout"] != "column" || f["hasExport"] != true {
		t.Fatalf("login flags: %v", f)
	}
	byID := map[string]map[string]any{}
	for _, row := range got {
		byID[row["id"].(string)] = row
	}
	if flagsOf(byID["2:5"])["instanceOf"] != "Input/Text" || flagsOf(byID["2:6"])["instanceOf"] != "Button / Variant=Primary, Size=md" {
		t.Fatalf("instanceOf: %v %v", flagsOf(byID["2:5"]), flagsOf(byID["2:6"]))
	}
	if flagsOf(byID["2:7"])["autoLayout"] != "wrap" || flagsOf(byID["2:14"])["autoLayout"] != "grid" || flagsOf(byID["2:13"])["hidden"] != true {
		t.Fatalf("layout flags: %v %v %v", flagsOf(byID["2:7"]), flagsOf(byID["2:14"]), flagsOf(byID["2:13"]))
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatal("second tree call must be served from the cache")
	}

	r = execute(t, "", "file", "tree", "https://www.figma.com/design/"+figmatest.FileKey+"/App?node-id=2-2", "--depth", "1")
	ok(t, r)
	if got = rows(t, r, "file.tree"); got[0]["id"] != "2:2" || len(got) != 9 {
		t.Fatalf("node from URL: %v", ids(got, "id"))
	}
}

func TestFileTreeFilters(t *testing.T) {
	setup(t)
	r := execute(t, "", "file", "tree", figmatest.FileKey, "--node", "2:2", "--depth", "1", "--visible-only")
	ok(t, r)
	if got := rows(t, r, "file.tree"); len(got) != 8 || strings.Contains(joined(ids(got, "id")), "2:13") {
		t.Fatalf("visible only: %v", ids(got, "id"))
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "--node", "2:2", "--depth", "0", "--type", "text")
	ok(t, r)
	if got := rows(t, r, "file.tree"); joined(ids(got, "id")) != "2:3,2:4,I2:5;3:21,I2:5;3:23,I2:6;3:12,2:16,2:17" {
		t.Fatalf("type filter: %v", ids(got, "id"))
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "--depth", "0", "--name", "icon/*")
	ok(t, r)
	if got := rows(t, r, "file.tree"); len(got) != 1 || got[0]["id"] != "2:9" || got[0]["depth"] != float64(5) {
		t.Fatalf("name filter: %v", got)
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "--page", "design system")
	ok(t, r)
	got := rows(t, r, "file.tree")
	if len(got) != 15 || got[0]["id"] != "1:2" || got[0]["depth"] != float64(0) || got[1]["id"] != "3:10" {
		t.Fatalf("page by name: %v", ids(got, "id"))
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "--page", "0:1", "--type", "FRAME,SECTION", "--depth", "0")
	ok(t, r)
	if got = rows(t, r, "file.tree"); joined(ids(got, "id")) != "2:1,2:2,I2:5;3:22,2:7,2:13,2:14,2:15" {
		t.Fatalf("page by id with types: %v", ids(got, "id"))
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "--page", "0:1", "--node", "3:10")
	if r.code != figctl.ExitNotFound || !strings.Contains(r.stdout, "not on page") {
		t.Fatalf("node outside page: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--page", "Nope")
	if r.code != figctl.ExitNotFound || !strings.Contains(r.stdout, "Pages: Screens (0:1), Design System (1:2)") {
		t.Fatalf("unknown page: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("unknown node: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--depth", "-1")
	if r.code != figctl.ExitUsage {
		t.Fatalf("negative depth: exit = %d", r.code)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--name", "zzz*")
	ok(t, r)
	if got = rows(t, r, "file.tree"); len(got) != 0 {
		t.Fatalf("no match should be empty: %v", got)
	}
	if h := hints(t, r); len(h) != 1 || !strings.Contains(h[0], "No nodes matched") {
		t.Fatalf("empty hints: %v", h)
	}
}

func TestFileTreePagination(t *testing.T) {
	setup(t)
	r := execute(t, "", "file", "tree", figmatest.FileKey, "--limit", "3")
	ok(t, r)
	got := rows(t, r, "file.tree")
	if joined(ids(got, "id")) != "0:1,2:1,1:2" {
		t.Fatalf("first page: %v", ids(got, "id"))
	}
	cursor, more := nextCursor(t, r)
	if !more || cursor == "" {
		t.Fatalf("expected a cursor: %s", r.stdout)
	}
	if h := hints(t, r); len(h) != 2 || !strings.Contains(h[0], "6 nodes matched") || !strings.Contains(h[0], "--type FRAME,SECTION") {
		t.Fatalf("truncated hints: %v", h)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--limit", "3", "--cursor", cursor)
	ok(t, r)
	got = rows(t, r, "file.tree")
	if joined(ids(got, "id")) != "3:10,3:20,4:0" {
		t.Fatalf("second page: %v", ids(got, "id"))
	}
	if _, more := nextCursor(t, r); more {
		t.Fatalf("last page should have no cursor: %s", r.stdout)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--cursor", "not-a-cursor")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("bad cursor: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestFileTreeRenderers(t *testing.T) {
	setup(t)
	r := execute(t, "", "file", "tree", figmatest.FileKey, "--node", "2:2", "--depth", "1", "-o", "md")
	ok(t, r)
	want := []string{
		"# file.tree\n",
		"File: Fixture Design System (" + figmatest.FileKey + "), version " + figmatest.Version + "\n",
		"- 2:2 FRAME \"Login\" 390x844 @0,0 (8 children) [layout=column, export]\n",
		"  - 2:3 TEXT \"Heading\" 342x34 @24,24\n",
		"  - 2:6 INSTANCE \"Button/Primary\" 342x48 @24,186 (1 children) [instance of Button / Variant=Primary, Size=md, layout=row]\n",
		"  - 2:7 FRAME \"Social\" 342x40 @24,250 (2 children) [layout=wrap]\n",
		"  - 2:13 FRAME \"Debug\" 342x40 @24,442 [hidden]\n",
		"  - 2:14 FRAME \"Stats\" 342x120 @24,498 (3 children) [layout=grid]\n",
		"Hints:\n- Next: figctl file tree",
	}
	for _, w := range want {
		if !strings.Contains(r.stdout, w) {
			t.Fatalf("markdown missing %q:\n%s", w, r.stdout)
		}
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "-o", "md", "--depth", "1")
	ok(t, r)
	if !strings.Contains(r.stdout, "\n- 0:1 CANVAS \"Screens\" (1 children)\n- 1:2 CANVAS \"Design System\" (3 children)\n") {
		t.Fatalf("pages markdown:\n%s", r.stdout)
	}

	r = execute(t, "", "file", "tree", figmatest.FileKey, "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "id    type           name           size     children  flags\n") || !strings.Contains(r.stdout, "2:1   SECTION        Onboarding     470x964  1         dev=READY_FOR_DEV\n") {
		t.Fatalf("table:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "hint: Next:") {
		t.Fatalf("table hints go to stderr: %s", r.stderr)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "-o", "plain", "--fields", "id,name")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "0:1\tScreens\n2:1\tOnboarding\n") {
		t.Fatalf("plain:\n%s", r.stdout)
	}
}

func TestFileTreeLargeFileFallback(t *testing.T) {
	api := setup(t)
	// The first over-limit fetch still serves the document it just
	// downloaded; the too-large mark applies from the next run on.
	ok(t, execute(t, "", "file", "tree", figmatest.FileKey, "--max-file-mb", "0"))
	r := execute(t, "", "file", "tree", figmatest.FileKey, "--max-file-mb", "0")
	ok(t, r)
	got := rows(t, r, "file.tree")
	if joined(ids(got, "id")) != "0:1,2:1,1:2,3:10,3:20,4:0" {
		t.Fatalf("fallback rows: %v", ids(got, "id"))
	}
	if h := hints(t, r); len(h) != 2 || !strings.Contains(h[0], "above --max-file-mb") {
		t.Fatalf("fallback hints: %v", h)
	}
	depthFetches := 0
	for _, req := range api.Requests() {
		if req.Path == fileKeyPath("") && req.Query.Get("depth") == "2" {
			depthFetches++
		}
	}
	if depthFetches != 1 {
		t.Fatalf("expected one depth 2 overview fetch, got %d", depthFetches)
	}
	r = execute(t, "", "file", "tree", figmatest.FileKey, "--max-file-mb", "0", "--node", "2:2", "--depth", "1")
	ok(t, r)
	if got = rows(t, r, "file.tree"); len(got) != 9 || got[3]["childCount"] != float64(2) {
		t.Fatalf("fallback subtree: %v", got)
	}
	if api.Count(http.MethodGet, fileKeyPath("/nodes")) != 1 {
		t.Fatalf("expected one GET nodes, got %d", api.Count(http.MethodGet, fileKeyPath("/nodes")))
	}
	for _, req := range api.Requests() {
		if req.Path == fileKeyPath("/nodes") && req.Query.Get("depth") != "2" {
			t.Fatalf("subtree fetch should ask one level more than --depth: %v", req.Query)
		}
	}
}

func TestFileFind(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "file", "find", figmatest.FileKey, "--name", "label")
	ok(t, r)
	got := rows(t, r, "file.find")
	if joined(ids(got, "id")) != "I2:5;3:21,I2:6;3:12,3:12,3:14,3:16,3:18,3:21" {
		t.Fatalf("name hits: %v", ids(got, "id"))
	}
	if got[1]["path"] != "Screens / Onboarding / Login / Button/Primary" || got[1]["page"] != "Screens" || got[1]["w"] != float64(52) || got[1]["h"] != float64(24) {
		t.Fatalf("hit shape: %v", got[1])
	}
	if _, has := got[1]["text"]; has {
		t.Fatalf("name hits carry no text: %v", got[1])
	}

	r = execute(t, "", "file", "find", figmatest.FileKey, "--type", "component_set")
	ok(t, r)
	if got = rows(t, r, "file.find"); len(got) != 1 || got[0]["id"] != "3:10" || got[0]["path"] != "Design System" || got[0]["type"] != "COMPONENT_SET" {
		t.Fatalf("type hits: %v", got)
	}

	r = execute(t, "", "file", "find", figmatest.FileKey, "--text", "SIGN IN")
	ok(t, r)
	got = rows(t, r, "file.find")
	texts := map[string]string{}
	for _, row := range got {
		texts[row["id"].(string)] = row["text"].(string)
	}
	if texts["2:4"] != "Sign in to continue to your account." || texts["I2:6;3:12"] != "Sign in" {
		t.Fatalf("text hits: %v", texts)
	}
	r = execute(t, "", "file", "find", figmatest.FileKey, "--text", "sign in", "--type", "FRAME")
	ok(t, r)
	if got = rows(t, r, "file.find"); len(got) != 0 {
		t.Fatalf("text with non text type: %v", got)
	}

	r = execute(t, "", "file", "find", figmatest.FileKey, "--node", "3:10", "--name", "*primary*")
	ok(t, r)
	if got = rows(t, r, "file.find"); joined(ids(got, "id")) != "3:11,3:15" || got[0]["path"] != "Design System / Button" {
		t.Fatalf("scoped hits: %v", got)
	}
	r = execute(t, "", "file", "find", figmatest.FileKey, "--page", "Screens", "--type", "INSTANCE")
	ok(t, r)
	if got = rows(t, r, "file.find"); joined(ids(got, "id")) != "2:5,2:6" {
		t.Fatalf("page hits: %v", ids(got, "id"))
	}

	r = execute(t, "", "file", "find", figmatest.FileKey, "--type", "TEXT", "--limit", "5")
	ok(t, r)
	got = rows(t, r, "file.find")
	cursor, more := nextCursor(t, r)
	if len(got) != 5 || !more {
		t.Fatalf("pagination: %d rows, more=%v", len(got), more)
	}
	r = execute(t, "", "file", "find", figmatest.FileKey, "--type", "TEXT", "--limit", "5", "--cursor", cursor)
	ok(t, r)
	if got = rows(t, r, "file.find"); got[0]["id"] != "2:16" {
		t.Fatalf("second page: %v", ids(got, "id"))
	}

	r = execute(t, "", "file", "find", figmatest.FileKey, "--name", "icon/*", "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "id   type    name              page     path                                   size   text\n2:9  VECTOR  icon/arrow-right  Screens  Screens / Onboarding / Login / Social  24x24\n") {
		t.Fatalf("table:\n%s", r.stdout)
	}

	r = execute(t, "", "file", "find", figmatest.FileKey)
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" || !strings.Contains(r.stdout, "--name") {
		t.Fatalf("missing filter: exit = %d, stdout = %s", r.code, r.stdout)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatalf("file fetched %d times", api.Count(http.MethodGet, fileKeyPath("")))
	}
}

func TestComponentsListFile(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "components", "list", figmatest.FileKey)
	ok(t, r)
	checkGolden(t, "components_list.golden", r.stdout)
	got := rows(t, r, "components.list")
	if joined(ids(got, "nodeId")) != "3:11,3:13,3:15,3:17,3:20" {
		t.Fatalf("rows: %v", ids(got, "nodeId"))
	}
	first := got[0]
	if first["key"] != "a1b2c3d4e5f60718293a4b5c6d7e8f9012345311" || first["setName"] != "Button" || first["setNodeId"] != "3:10" || first["page"] != "Design System" || first["frame"] != "Button" || first["published"] != true || first["remote"] != false {
		t.Fatalf("first row: %v", first)
	}
	if props := first["variantProperties"].(map[string]any); props["Variant"] != "Primary" || props["Size"] != "md" {
		t.Fatalf("variant properties: %v", props)
	}
	if links := first["documentationLinks"].([]any); len(links) != 1 || links[0] != "https://example.com/design-system/button" {
		t.Fatalf("links: %v", links)
	}
	if _, has := got[4]["variantProperties"]; has || got[4]["setName"] != nil {
		t.Fatalf("plain component: %v", got[4])
	}
	if api.Count(http.MethodGet, fileKeyPath("/components")) != 1 || api.Count(http.MethodGet, fileKeyPath("/component_sets")) != 1 || api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatalf("requests: %v", api.Requests())
	}

	r = execute(t, "", "components", "search", figmatest.FileKey, "--query", "INPUT")
	ok(t, r)
	if got = rows(t, r, "components.list"); len(got) != 1 || got[0]["nodeId"] != "3:20" {
		t.Fatalf("query: %v", got)
	}
	r = execute(t, "", "components", "list", figmatest.FileKey, "--query", "button")
	ok(t, r)
	if got = rows(t, r, "components.list"); len(got) != 4 {
		t.Fatalf("query on set name: %v", ids(got, "nodeId"))
	}
	if api.Count(http.MethodGet, fileKeyPath("/components")) != 1 {
		t.Fatal("component lists should be served from the cache")
	}

	r = execute(t, "", "components", "list", figmatest.FileKey, "--limit", "2")
	ok(t, r)
	got = rows(t, r, "components.list")
	cursor, more := nextCursor(t, r)
	if len(got) != 2 || !more {
		t.Fatalf("first page: %v", ids(got, "nodeId"))
	}
	r = execute(t, "", "components", "list", figmatest.FileKey, "--limit", "2", "--cursor", cursor)
	ok(t, r)
	if got = rows(t, r, "components.list"); joined(ids(got, "nodeId")) != "3:15,3:17" {
		t.Fatalf("second page: %v", ids(got, "nodeId"))
	}

	r = execute(t, "", "components", "list", figmatest.FileKey, "-o", "table")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "key                                       nodeId  name                        setName  variants                    page           description\n") || !strings.Contains(r.stdout, "3:11    Variant=Primary, Size=md    Button   Size=md, Variant=Primary    Design System  Primary call to action.\n") {
		t.Fatalf("table:\n%s", r.stdout)
	}

	r = execute(t, "", "components", "list")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("no ref: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "components", "list", figmatest.FileKey, "--team", figmatest.TeamID)
	if r.code != figctl.ExitUsage {
		t.Fatalf("ref and team: exit = %d", r.code)
	}
}

func TestComponentsListTeam(t *testing.T) {
	api := setup(t)
	teamPath := "/v1/teams/" + figmatest.TeamID + "/components"
	r := execute(t, "", "components", "list", "--team", figmatest.TeamID, "--limit", "2")
	ok(t, r)
	got := rows(t, r, "components.list")
	if joined(ids(got, "nodeId")) != "3:11,3:13" || got[0]["fileKey"] != figmatest.FileKey || got[0]["setName"] != "Button" {
		t.Fatalf("first page: %v", got)
	}
	cursor, more := nextCursor(t, r)
	if !more || cursor != "2" {
		t.Fatalf("cursor: %q", cursor)
	}
	r = execute(t, "", "components", "list", "--team", figmatest.TeamID, "--limit", "2", "--cursor", cursor)
	ok(t, r)
	if got = rows(t, r, "components.list"); joined(ids(got, "nodeId")) != "3:15,3:17" {
		t.Fatalf("second page: %v", ids(got, "nodeId"))
	}
	if cursor, _ = nextCursor(t, r); cursor != "4" {
		t.Fatalf("second cursor: %q", cursor)
	}
	r = execute(t, "", "components", "list", "--team", figmatest.TeamID, "--limit", "2", "--cursor", cursor)
	ok(t, r)
	if got = rows(t, r, "components.list"); joined(ids(got, "nodeId")) != "3:20" {
		t.Fatalf("last page: %v", ids(got, "nodeId"))
	}
	if _, more = nextCursor(t, r); more {
		t.Fatal("last page should have no cursor")
	}
	if api.Count(http.MethodGet, teamPath) != 3 {
		t.Fatalf("team requests: %d", api.Count(http.MethodGet, teamPath))
	}
	var sawPaged bool
	for _, req := range api.Requests() {
		if req.Path == teamPath && req.Query.Get("page_size") == "2" && req.Query.Get("after") == "2" {
			sawPaged = true
		}
	}
	if !sawPaged {
		t.Fatalf("page_size and after not sent: %v", api.Requests())
	}
	r = execute(t, "", "components", "list", "--team", figmatest.TeamID, "--query", "compact")
	ok(t, r)
	if got = rows(t, r, "components.list"); joined(ids(got, "nodeId")) != "3:15,3:17" {
		t.Fatalf("team query: %v", ids(got, "nodeId"))
	}
	if h := hints(t, r); len(h) != 2 || !strings.Contains(h[0], "current page only") {
		t.Fatalf("team hints: %v", h)
	}
	r = execute(t, "", "components", "list", "--team", "000")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown team: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestComponentsGet(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "components", "get", figmatest.FileKey, "--node", "3:10")
	ok(t, r)
	d := data(t, r, "components.get")
	if d["type"] != "COMPONENT_SET" || d["key"] != "a1b2c3d4e5f60718293a4b5c6d7e8f9012345310" || d["name"] != "Button" || d["w"] != float64(368) || d["h"] != float64(132) || d["page"] != "Design System" || d["published"] != true {
		t.Fatalf("set: %v", d)
	}
	defs := d["propertyDefinitions"].(map[string]any)
	variant := defs["Variant"].(map[string]any)
	if variant["type"] != "VARIANT" || variant["default"] != "Primary" || len(variant["variantOptions"].([]any)) != 2 {
		t.Fatalf("variant definition: %v", variant)
	}
	if label := defs["Label#5:0"].(map[string]any); label["type"] != "TEXT" || label["default"] != "Button" {
		t.Fatalf("label definition: %v", label)
	}
	variants := d["variants"].([]any)
	if len(variants) != 4 {
		t.Fatalf("variants: %v", variants)
	}
	v0 := variants[0].(map[string]any)
	if v0["nodeId"] != "3:11" || v0["key"] != "a1b2c3d4e5f60718293a4b5c6d7e8f9012345311" || v0["w"] != float64(160) || v0["variantProperties"].(map[string]any)["Size"] != "md" || v0["description"] != "Primary call to action." {
		t.Fatalf("first variant: %v", v0)
	}

	r = execute(t, "", "components", "get", figmatest.FileKey, "--node", "3-11")
	ok(t, r)
	d = data(t, r, "components.get")
	if d["type"] != "COMPONENT" || d["setName"] != "Button" || d["setNodeId"] != "3:10" || d["setKey"] != "a1b2c3d4e5f60718293a4b5c6d7e8f9012345310" || d["w"] != float64(160) || d["h"] != float64(48) {
		t.Fatalf("component: %v", d)
	}
	if _, has := d["propertyDefinitions"]; has {
		t.Fatalf("variant has no definitions of its own: %v", d)
	}
	if d["variantProperties"].(map[string]any)["Variant"] != "Primary" {
		t.Fatalf("variant properties: %v", d)
	}

	r = execute(t, "", "components", "get", figmatest.FileKey, "--node", "3:20")
	ok(t, r)
	d = data(t, r, "components.get")
	defs = d["propertyDefinitions"].(map[string]any)
	if len(defs) != 2 || defs["Show helper#12:1"].(map[string]any)["default"] != false || d["setName"] != nil {
		t.Fatalf("input definitions: %v", d)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, fileKeyPath("/nodes")) != 0 {
		t.Fatalf("gets by node must be served from the cached document: %v", api.Requests())
	}

	r = execute(t, "", "components", "get", figmatest.FileKey, "--key", "a1b2c3d4e5f60718293a4b5c6d7e8f9012345320")
	ok(t, r)
	if d = data(t, r, "components.get"); d["nodeId"] != "3:20" || d["type"] != "COMPONENT" {
		t.Fatalf("by key: %v", d)
	}
	r = execute(t, "", "components", "get", figmatest.FileKey, "--key", "a1b2c3d4e5f60718293a4b5c6d7e8f9012345310")
	ok(t, r)
	if d = data(t, r, "components.get"); d["nodeId"] != "3:10" || d["type"] != "COMPONENT_SET" || len(d["variants"].([]any)) != 4 {
		t.Fatalf("set by key: %v", d)
	}
	if api.Count(http.MethodGet, "/v1/components/a1b2c3d4e5f60718293a4b5c6d7e8f9012345310") != 1 || api.Count(http.MethodGet, "/v1/component_sets/a1b2c3d4e5f60718293a4b5c6d7e8f9012345310") != 1 {
		t.Fatalf("set key should fall back to the component set endpoint: %v", api.Requests())
	}
	r = execute(t, "", "components", "get", figmatest.FileKey, "--key", "nope")
	if r.code != figctl.ExitNotFound || !strings.Contains(r.stdout, "no published component or component set with key nope") {
		t.Fatalf("unknown key: exit = %d, stdout = %s", r.code, r.stdout)
	}

	r = execute(t, "", "components", "get", figmatest.FileKey, "--node", "3:10", "-o", "table")
	ok(t, r)
	for _, want := range []string{"type                 COMPONENT_SET", "size                 368x132", "Variant: VARIANT default Primary options Primary|Secondary", "variants             3:11 Variant=Primary, Size=md 160x48; "} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("table missing %q:\n%s", want, r.stdout)
		}
	}

	r = execute(t, "", "components", "get", figmatest.FileKey, "--node", "2:2")
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "node 2:2 is a FRAME") {
		t.Fatalf("frame: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "components", "get", figmatest.FileKey, "--node", "9:9")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown node: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "components", "get", figmatest.FileKey)
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "--node or --key") {
		t.Fatalf("no selector: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "components", "get", "https://www.figma.com/design/"+figmatest.FileKey+"/App?node-id=3-20")
	ok(t, r)
	if d = data(t, r, "components.get"); d["nodeId"] != "3:20" {
		t.Fatalf("node from URL: %v", d)
	}
}

func TestFileGetRawIsLossless(t *testing.T) {
	api := setup(t)
	var file map[string]any
	figmatest.Decode(t, "file.json", &file)
	file["figctlUnknown"] = map[string]any{"answer": 42, "list": []any{1, 2.5, "x"}}
	document := file["document"].(map[string]any)
	login := document["children"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)
	heading := login["children"].([]any)[0].(map[string]any)
	heading["unknownNodeProp"] = "keep me"
	file["components"].(map[string]any)["3:11"].(map[string]any)["unknownComponentProp"] = true
	api.Handle(http.MethodGet, fileKeyPath(""), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(file)
	})

	r := execute(t, "", "file", "get", figmatest.FileKey)
	ok(t, r)
	d := data(t, r, "file.get")
	unknown, _ := d["figctlUnknown"].(map[string]any)
	if unknown["answer"] != float64(42) || len(unknown["list"].([]any)) != 3 {
		t.Fatalf("unknown top level property lost: %v", d["figctlUnknown"])
	}
	if !strings.Contains(r.stdout, "\"answer\": 42,") || !strings.Contains(r.stdout, "2.5,") {
		t.Fatalf("numbers must not be reformatted:\n%s", r.stdout)
	}
	gotHeading := d["document"].(map[string]any)["children"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)
	if gotHeading["id"] != "2:3" || gotHeading["unknownNodeProp"] != "keep me" {
		t.Fatalf("unknown node property lost: %v", gotHeading)
	}
	env := decodeEnvelope(t, r.stdout)
	if env["file"].(map[string]any)["name"] != figmatest.FileName || env["file"].(map[string]any)["version"] != figmatest.Version {
		t.Fatalf("envelope file block: %v", env["file"])
	}

	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3", "--node", "2:6", "--depth", "1")
	ok(t, r)
	d = data(t, r, "file.get")
	entry := d["2:3"].(map[string]any)
	if entry["document"].(map[string]any)["unknownNodeProp"] != "keep me" {
		t.Fatalf("unknown property lost on node path: %v", entry)
	}
	button := d["2:6"].(map[string]any)
	if button["components"].(map[string]any)["3:11"].(map[string]any)["unknownComponentProp"] != true {
		t.Fatalf("unknown component property lost: %v", button["components"])
	}
	if _, has := button["document"].(map[string]any)["children"].([]any)[0].(map[string]any)["children"]; has {
		t.Fatalf("depth 1 should cut grandchildren: %v", button["document"])
	}
	if button["schemaVersion"] != float64(0) || len(button["styles"].(map[string]any)) != 1 {
		t.Fatalf("entry shape: %v", button)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, fileKeyPath("/nodes")) != 0 {
		t.Fatalf("node get must be cut from the cached document: %v", api.Requests())
	}

	r = execute(t, "", "file", "get", figmatest.FileKey, "--fields", "name,figctlUnknown")
	ok(t, r)
	d = data(t, r, "file.get")
	if len(d) != 2 || d["name"] != figmatest.FileName {
		t.Fatalf("fields on raw output: %v", d)
	}

	api.Handle(http.MethodGet, fileKeyPath("/nodes"), func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": figmatest.FileName, "version": figmatest.Version, "lastModified": "2026-09-01T10:15:00Z", "futureField": []int{1},
			"nodes": map[string]any{
				"2:9": map[string]any{"document": map[string]any{"id": "2:9", "type": "VECTOR", "name": "icon/arrow-right", "unknownGeometryProp": req.URL.Query().Get("geometry")}, "components": map[string]any{}, "styles": map[string]any{}},
				"9:9": nil,
			},
		})
	})
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:9", "--node", "9:9", "--geometry", "paths")
	ok(t, r)
	d = data(t, r, "file.get")
	if d["2:9"].(map[string]any)["document"].(map[string]any)["unknownGeometryProp"] != "paths" || d["9:9"] != nil {
		t.Fatalf("raw nodes response: %v", d)
	}
	if h := hints(t, r); len(h) != 1 || !strings.Contains(h[0], "Node 9:9 does not exist") {
		t.Fatalf("hints: %v", h)
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:9", "--node", "9:9", "--geometry", "paths")
	ok(t, r)
	if api.Count(http.MethodGet, fileKeyPath("/nodes")) != 1 {
		t.Fatalf("raw nodes should be cached, got %d requests", api.Count(http.MethodGet, fileKeyPath("/nodes")))
	}
}
