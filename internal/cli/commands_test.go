package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// setup isolates the environment, starts the fake API, and selects the
// implicit env profile with the fixture token.
func setup(t *testing.T) *figmatest.Server {
	t.Helper()
	isolate(t)
	api := fakeAPI(t)
	t.Setenv(config.TokenEnv, figmatest.Token)
	return api
}

func ok(t *testing.T, r result) {
	t.Helper()
	if r.code != figctl.ExitOK {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
}

func list(t *testing.T, r result, command string) []any {
	t.Helper()
	env := decodeEnvelope(t, r.stdout)
	if env["command"] != command {
		t.Fatalf("command = %v, want %s", env["command"], command)
	}
	items, isList := env["data"].([]any)
	if !isList {
		t.Fatalf("data is %T, want list: %s", env["data"], r.stdout)
	}
	return items
}

func fileKeyPath(suffix string) string {
	return "/v1/files/" + figmatest.FileKey + suffix
}

func TestMe(t *testing.T) {
	setup(t)
	r := execute(t, "", "me")
	ok(t, r)
	d := data(t, r, "me")
	if d["handle"] != figmatest.UserHandle || d["email"] != figmatest.UserEmail || d["id"] != "1234567890" {
		t.Fatalf("me: %v", d)
	}
	env := decodeEnvelope(t, r.stdout)
	if env["profile"].(map[string]any)["name"] != "env" {
		t.Fatalf("profile block: %v", env["profile"])
	}
	r = execute(t, "", "me", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "handle  "+figmatest.UserHandle) || !strings.HasPrefix(r.stdout, "field   value\n") {
		t.Fatalf("table:\n%s", r.stdout)
	}

	t.Setenv(config.TokenEnv, "figd_wrong")
	r = execute(t, "", "me")
	if r.code != figctl.ExitAuth || errorCode(t, r) != "AUTH_INVALID" {
		t.Fatalf("wrong token: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestFileInfo(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "file", "info", "https://www.figma.com/design/"+figmatest.FileKey+"/Fixture?node-id=2-2")
	ok(t, r)
	d := data(t, r, "file.info")
	if d["name"] != figmatest.FileName || d["version"] != figmatest.Version || d["editorType"] != "figma" || d["role"] != "editor" || d["linkAccess"] != "org_view" {
		t.Fatalf("info: %v", d)
	}
	pages := d["pages"].([]any)
	if len(pages) != 2 || pages[0].(map[string]any)["name"] != "Screens" || pages[0].(map[string]any)["children"] != float64(1) || pages[1].(map[string]any)["children"] != float64(3) {
		t.Fatalf("pages: %v", pages)
	}
	branches := d["branches"].([]any)
	if len(branches) != 1 || branches[0].(map[string]any)["key"] != figmatest.BranchKey {
		t.Fatalf("branches: %v", branches)
	}
	if d["components"] != float64(5) || d["styles"] != float64(4) {
		t.Fatalf("counts: %v", d)
	}
	env := decodeEnvelope(t, r.stdout)
	file := env["file"].(map[string]any)
	if file["key"] != figmatest.FileKey || file["name"] != figmatest.FileName || file["version"] != figmatest.Version || file["lastModified"] != "2026-09-01T10:15:00Z" {
		t.Fatalf("envelope file: %v", file)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, fileKeyPath("/meta")) != 1 {
		t.Fatalf("expected one file fetch and one meta check, got %d and %d", api.Count(http.MethodGet, fileKeyPath("")), api.Count(http.MethodGet, fileKeyPath("/meta")))
	}
	// file info reports pages and metadata, so it must stay on the shallow
	// path. Pulling the whole document for an outline takes minutes on a
	// large file and Figma refuses it outright past a certain size.
	for _, req := range api.Requests() {
		if req.Path == fileKeyPath("") && (req.Query.Get("branch_data") != "true" || req.Query.Get("depth") != "2") {
			t.Fatalf("file info should fetch depth 2 with branch_data: %v", req.Query)
		}
	}

	r = execute(t, "", "file", "info", figmatest.FileKey, "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "pages          0:1 Screens (1 children); 1:2 Design System (3 children)") || !strings.Contains(r.stdout, "branches       feature/dark-mode") {
		t.Fatalf("table:\n%s", r.stdout)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatal("second file info should be served from the cache")
	}
	r = execute(t, "", "file", "info", figmatest.FileKey, "-o", "md", "--fields", "name,version")
	ok(t, r)
	if !strings.Contains(r.stdout, "File: Fixture Design System ("+figmatest.FileKey+"), version "+figmatest.Version) || !strings.Contains(r.stdout, "| name | Fixture Design System |") || strings.Contains(r.stdout, "| pages |") {
		t.Fatalf("markdown:\n%s", r.stdout)
	}

	r = execute(t, "", "file", "info", "NoSuchFile000000")
	if r.code != figctl.ExitNotFound || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("unknown file: exit = %d, stdout = %s", r.code, r.stdout)
	}
	if !strings.Contains(r.stdout, "file NoSuchFile000000 not found") {
		t.Fatalf("message: %s", r.stdout)
	}
	r = execute(t, "", "file", "info", "not a ref")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("bad ref: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestFileGetWholeFileReuse(t *testing.T) {
	api := setup(t)
	nodesPath := fileKeyPath("/nodes")

	r := execute(t, "", "file", "get", figmatest.FileKey, "--node", "2-6", "--node", "9:9", "--depth", "1")
	ok(t, r)
	d := data(t, r, "file.get")
	entry, isEntry := d["2:6"].(map[string]any)
	if !isEntry || d["9:9"] != nil {
		t.Fatalf("nodes: %v", d)
	}
	doc := entry["document"].(map[string]any)
	if doc["name"] != "Button/Primary" || doc["componentId"] != "3:11" {
		t.Fatalf("node document: %v", doc)
	}
	if children := doc["children"].([]any); len(children) != 1 || children[0].(map[string]any)["characters"] != "Sign in" {
		t.Fatalf("children: %v", children)
	}
	if _, has := entry["components"].(map[string]any)["3:11"]; !has {
		t.Fatalf("referenced component missing: %v", entry["components"])
	}
	if _, has := entry["componentSets"].(map[string]any)["3:10"]; !has {
		t.Fatalf("referenced component set missing: %v", entry["componentSets"])
	}
	if _, has := entry["styles"].(map[string]any)["5:1"]; !has {
		t.Fatalf("referenced style missing: %v", entry["styles"])
	}
	env := decodeEnvelope(t, r.stdout)
	if hints := env["hints"].([]any); len(hints) != 1 || !strings.Contains(hints[0].(string), "Node 9:9 does not exist") {
		t.Fatalf("hints: %v", hints)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, nodesPath) != 0 {
		t.Fatalf("first call: file=%d nodes=%d", api.Count(http.MethodGet, fileKeyPath("")), api.Count(http.MethodGet, nodesPath))
	}

	// A second process reuses the cached document: zero Tier 1 requests and
	// no meta check within the TTL.
	r = execute(t, "", "file", "get", "https://www.figma.com/design/"+figmatest.FileKey+"/App?node-id=2-3")
	ok(t, r)
	d = data(t, r, "file.get")
	if d["2:3"].(map[string]any)["document"].(map[string]any)["characters"] != "Welcome back" {
		t.Fatalf("node from URL: %v", d)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 1 || api.Count(http.MethodGet, nodesPath) != 0 || api.Count(http.MethodGet, fileKeyPath("/meta")) != 1 {
		t.Fatalf("second call must be served locally: file=%d nodes=%d meta=%d", api.Count(http.MethodGet, fileKeyPath("")), api.Count(http.MethodGet, nodesPath), api.Count(http.MethodGet, fileKeyPath("/meta")))
	}

	// Geometry is not part of the cached document, so it goes to GET nodes.
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:9", "--geometry", "paths")
	ok(t, r)
	if api.Count(http.MethodGet, nodesPath) != 1 {
		t.Fatalf("geometry should call GET nodes once, got %d", api.Count(http.MethodGet, nodesPath))
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:9", "--geometry", "paths")
	ok(t, r)
	if api.Count(http.MethodGet, nodesPath) != 1 {
		t.Fatal("repeated geometry request should hit the cache")
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:9", "--geometry", "bogus")
	if r.code != figctl.ExitUsage {
		t.Fatalf("bad geometry: exit = %d", r.code)
	}

	// --refresh refetches and rewrites; --no-cache bypasses reads and writes.
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3", "--refresh")
	ok(t, r)
	if api.Count(http.MethodGet, fileKeyPath("")) != 2 || api.Count(http.MethodGet, fileKeyPath("/meta")) != 2 {
		t.Fatalf("--refresh: file=%d meta=%d", api.Count(http.MethodGet, fileKeyPath("")), api.Count(http.MethodGet, fileKeyPath("/meta")))
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3", "--no-cache")
	ok(t, r)
	if api.Count(http.MethodGet, fileKeyPath("")) != 3 || api.Count(http.MethodGet, fileKeyPath("/meta")) != 2 {
		t.Fatalf("--no-cache: file=%d meta=%d", api.Count(http.MethodGet, fileKeyPath("")), api.Count(http.MethodGet, fileKeyPath("/meta")))
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3")
	ok(t, r)
	if api.Count(http.MethodGet, fileKeyPath("")) != 3 {
		t.Fatal("cache written by --refresh should still serve")
	}

	// Whole document without --node, and depth limited fetches.
	r = execute(t, "", "file", "get", figmatest.FileKey, "--fields", "name,document")
	ok(t, r)
	d = data(t, r, "file.get")
	if d["name"] != figmatest.FileName || len(d["document"].(map[string]any)["children"].([]any)) != 2 {
		t.Fatalf("whole document: %v", d["name"])
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--depth", "1", "--fields", "document")
	ok(t, r)
	d = data(t, r, "file.get")
	if pages := d["document"].(map[string]any)["children"].([]any); pages[0].(map[string]any)["children"] != nil {
		t.Fatalf("depth 1 should drop page children: %v", pages)
	}
	if api.Count(http.MethodGet, fileKeyPath("")) != 4 {
		t.Fatalf("depth fetch should be its own request, got %d", api.Count(http.MethodGet, fileKeyPath("")))
	}

	// A pinned version bypasses meta validation and is sent to Figma.
	api.Reset()
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3", "--file-version", "2100120000")
	ok(t, r)
	if api.Count(http.MethodGet, fileKeyPath("/meta")) != 0 || api.Count(http.MethodGet, fileKeyPath("")) != 1 {
		t.Fatalf("pinned version: meta=%d file=%d", api.Count(http.MethodGet, fileKeyPath("/meta")), api.Count(http.MethodGet, fileKeyPath("")))
	}
	if reqs := api.Requests(); reqs[0].Query.Get("version") != "2100120000" {
		t.Fatalf("version not sent: %v", reqs[0].Query)
	}

	// A tiny --max-file-mb stops whole-file caching and falls back to GET nodes.
	api.Reset()
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3", "--max-file-mb", "0", "--refresh")
	ok(t, r)
	r = execute(t, "", "file", "get", figmatest.FileKey, "--node", "2:3", "--max-file-mb", "0")
	ok(t, r)
	if api.Count(http.MethodGet, nodesPath) != 1 {
		t.Fatalf("too large file should fall back to GET nodes, got %d", api.Count(http.MethodGet, nodesPath))
	}
	r = execute(t, "", "file", "get", figmatest.FileKey, "--max-file-mb", "0")
	if r.code != figctl.ExitUsage || !strings.Contains(r.stdout, "max-file-mb") {
		t.Fatalf("whole document over the limit: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestVersionsList(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "versions", "list", figmatest.FileKey, "--limit", "1")
	ok(t, r)
	items := list(t, r, "versions.list")
	if len(items) != 1 || items[0].(map[string]any)["id"] != figmatest.Version || items[0].(map[string]any)["label"] != "Login v2" || items[0].(map[string]any)["user"] != figmatest.UserHandle {
		t.Fatalf("versions: %v", items)
	}
	env := decodeEnvelope(t, r.stdout)
	if env["nextCursor"] != figmatest.Version {
		t.Fatalf("nextCursor = %v", env["nextCursor"])
	}
	if env["file"].(map[string]any)["key"] != figmatest.FileKey {
		t.Fatalf("file block: %v", env["file"])
	}
	r = execute(t, "", "versions", "list", figmatest.FileKey, "--limit", "1", "--cursor", figmatest.Version, "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "2100120000  2026-08-20T09:00:00Z") || strings.Contains(r.stdout, "Login v2") {
		t.Fatalf("second page table:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "hint: More results available") && strings.Contains(r.stdout, "2100120000") {
		// The last page has no cursor; nothing else to check.
		_ = r
	}
	if api.Count(http.MethodGet, fileKeyPath("/versions")) != 2 {
		t.Fatalf("versions requests = %d", api.Count(http.MethodGet, fileKeyPath("/versions")))
	}
	for _, req := range api.Requests() {
		if req.Path == fileKeyPath("/versions") && req.Query.Get("page_size") != "1" {
			t.Fatalf("page_size missing: %v", req.Query)
		}
	}
}

func TestComments(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "comments", "list", figmatest.FileKey, "--md")
	ok(t, r)
	items := list(t, r, "comments.list")
	if len(items) != 3 || items[0].(map[string]any)["nodeId"] != "2:2" || items[1].(map[string]any)["parentId"] != "9001" || items[2].(map[string]any)["resolvedAt"] == nil {
		t.Fatalf("comments: %v", items)
	}
	if items[0].(map[string]any)["reactions"] != float64(1) || items[2].(map[string]any)["x"] != float64(120) {
		t.Fatalf("comment details: %v", items)
	}
	if reqs := api.Requests(); reqs[0].Query.Get("as_md") != "true" {
		t.Fatalf("as_md not sent: %v", reqs[0].Query)
	}

	r = execute(t, "", "comments", "list", figmatest.FileKey, "--node", "2-2")
	ok(t, r)
	items = list(t, r, "comments.list")
	if len(items) != 2 || items[0].(map[string]any)["id"] != "9001" || items[1].(map[string]any)["id"] != "9002" {
		t.Fatalf("filtered comments should include the pinned comment and its reply: %v", items)
	}
	r = execute(t, "", "comments", "list", figmatest.FileKey, "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "id    parentId  user") || !strings.Contains(r.stdout, "9003            Fixture Designer  2026-08-25T09:10:00Z  yes") {
		t.Fatalf("table:\n%s", r.stdout)
	}

	r = execute(t, "", "comments", "add", figmatest.FileKey, "Implemented", "--node", "2:2", "--x", "10", "--y", "20")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("add without --yes must fail when not a TTY: exit = %d, stdout = %s", r.code, r.stdout)
	}
	if api.Count(http.MethodPost, fileKeyPath("/comments")) != 0 {
		t.Fatal("nothing should be posted without confirmation")
	}
	r = execute(t, "", "comments", "add", figmatest.FileKey, "Implemented", "--node", "2:2", "--x", "10", "--y", "20", "--yes")
	ok(t, r)
	d := data(t, r, "comments.add")
	if d["id"] != "9100" || d["message"] != "Implemented" || d["nodeId"] != "2:2" || d["x"] != float64(10) || d["y"] != float64(20) {
		t.Fatalf("posted: %v", d)
	}
	reqs := api.Requests()
	post := reqs[len(reqs)-1]
	if post.Method != http.MethodPost || !strings.Contains(string(post.Body), `"node_id":"2:2"`) || !strings.Contains(string(post.Body), `"node_offset":{"x":10,"y":20}`) {
		t.Fatalf("post body: %s", post.Body)
	}

	r = execute(t, "From stdin\n", "comments", "add", figmatest.FileKey, "-", "--x", "5", "--y", "6", "--yes")
	ok(t, r)
	reqs = api.Requests()
	post = reqs[len(reqs)-1]
	if !strings.Contains(string(post.Body), `"message":"From stdin"`) || !strings.Contains(string(post.Body), `"client_meta":{"x":5,"y":6}`) {
		t.Fatalf("stdin post body: %s", post.Body)
	}
	r = execute(t, "", "comments", "add", figmatest.FileKey, "", "--yes")
	if r.code != figctl.ExitUsage {
		t.Fatalf("empty message: exit = %d", r.code)
	}
}

func TestDevResources(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "devresources", "list", figmatest.FileKey)
	ok(t, r)
	if items := list(t, r, "devresources.list"); len(items) != 2 || items[0].(map[string]any)["nodeId"] != "3:11" {
		t.Fatalf("dev resources: %v", items)
	}
	r = execute(t, "", "devresources", "list", figmatest.FileKey, "--node", "3-20", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "dr-2  3:20    Input.tsx on GitHub") || strings.Contains(r.stdout, "dr-1") {
		t.Fatalf("table:\n%s", r.stdout)
	}
	if reqs := api.Requests(); reqs[1].Query.Get("node_ids") != "3:20" {
		t.Fatalf("node_ids: %v", reqs[1].Query)
	}
}

func TestProjectsAndFolders(t *testing.T) {
	setup(t)
	r := execute(t, "", "projects", "list", figmatest.TeamID)
	ok(t, r)
	if items := list(t, r, "projects.list"); len(items) != 2 || items[0].(map[string]any)["name"] != "Web" {
		t.Fatalf("projects: %v", items)
	}
	r = execute(t, "", "projects", "list")
	if r.code != figctl.ExitUsage {
		t.Fatalf("projects list without a team must fail: exit = %d", r.code)
	}
	r = execute(t, "", "projects", "files", figmatest.ProjectID, "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, figmatest.FileKey+"  "+figmatest.FileName) {
		t.Fatalf("project files table:\n%s", r.stdout)
	}
	r = execute(t, "", "projects", "files", "9999")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("unknown project: exit = %d", r.code)
	}

	r = execute(t, "", "folders", "list", figmatest.TeamID)
	ok(t, r)
	items := list(t, r, "folders.list")
	if len(items) != 2 || items[0].(map[string]any)["id"] != figmatest.FolderID || items[0].(map[string]any)["parentFolderId"] != nil {
		t.Fatalf("team folders: %v", items)
	}
	r = execute(t, "", "folders", "list", figmatest.FolderID, "--folder", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "2003  Archive  2001") {
		t.Fatalf("subfolders table:\n%s", r.stdout)
	}
	r = execute(t, "", "folders", "files", figmatest.FolderID, "--branches")
	ok(t, r)
	items = list(t, r, "folders.files")
	if len(items) != 1 || len(items[0].(map[string]any)["branches"].([]any)) != 1 {
		t.Fatalf("folder files: %v", items)
	}
	r = execute(t, "", "folders", "list", "--folder")
	if r.code != figctl.ExitUsage {
		t.Fatalf("--folder without id: exit = %d", r.code)
	}
}

func TestCacheCommands(t *testing.T) {
	api := setup(t)
	r := execute(t, "", "cache", "status")
	ok(t, r)
	d := data(t, r, "cache.status")
	if d["bytes"] != float64(0) || len(d["profiles"].([]any)) != 0 {
		t.Fatalf("empty status: %v", d)
	}
	ok(t, execute(t, "", "file", "info", figmatest.FileKey))
	r = execute(t, "", "cache", "status")
	ok(t, r)
	d = data(t, r, "cache.status")
	profiles := d["profiles"].([]any)
	if len(profiles) != 1 || profiles[0].(map[string]any)["name"] != "env" {
		t.Fatalf("profiles: %v", profiles)
	}
	files := profiles[0].(map[string]any)["files"].([]any)
	entry := files[0].(map[string]any)
	if entry["key"] != figmatest.FileKey || entry["version"] != figmatest.Version || len(entry["entries"].([]any)) != 1 || entry["checkedAge"] == "" {
		t.Fatalf("file status: %v", entry)
	}
	if entry["entries"].([]any)[0].(map[string]any)["key"] != "file?branch_data=true&depth=2" {
		t.Fatalf("entry key: %v", entry["entries"])
	}
	r = execute(t, "", "cache", "status", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "env      "+figmatest.FileKey+"  1        0") {
		t.Fatalf("table:\n%s", r.stdout)
	}

	r = execute(t, "", "cache", "clear")
	if r.code != figctl.ExitUsage {
		t.Fatalf("clear without --yes: exit = %d", r.code)
	}
	r = execute(t, "", "cache", "clear", "--profile", "other", "--yes")
	ok(t, r)
	if d := data(t, r, "cache.clear"); d["removedBytes"] != float64(0) {
		t.Fatalf("clearing another profile removed data: %v", d)
	}
	r = execute(t, "", "cache", "clear", "--profile", "env", "--file", "https://www.figma.com/design/"+figmatest.FileKey+"/X", "--yes")
	ok(t, r)
	if d := data(t, r, "cache.clear"); d["removedBytes"] == float64(0) || d["file"] != figmatest.FileKey {
		t.Fatalf("clear file: %v", d)
	}
	ok(t, execute(t, "", "file", "info", figmatest.FileKey))
	if api.Count(http.MethodGet, fileKeyPath("")) != 2 {
		t.Fatal("file should be refetched after cache clear")
	}
	r = execute(t, "", "cache", "clear", "--yes")
	ok(t, r)
	r = execute(t, "", "cache", "status")
	if d := data(t, r, "cache.status"); d["bytes"] != float64(0) {
		t.Fatalf("status after clear all: %v", d)
	}
}

func TestAuthStatusRateLimitHeaders(t *testing.T) {
	api := setup(t)
	api.Respond(http.MethodGet, "/v1/me", figmatest.Response{
		Status: 200,
		Body:   map[string]any{"id": "1", "handle": "H", "img_url": "", "email": "h@example.com"},
		Header: map[string]string{"X-Figma-Plan-Tier": "pro", "X-Figma-Rate-Limit-Type": "high"},
	})
	r := execute(t, "", "auth", "status")
	ok(t, r)
	d := data(t, r, "auth.status")
	if d["validated"] != true || d["handle"] != "H" || d["email"] != "h@example.com" {
		t.Fatalf("status: %v", d)
	}
	rl := d["rateLimit"].(map[string]any)
	if rl["planTier"] != "pro" || rl["type"] != "high" {
		t.Fatalf("rate limit: %v", rl)
	}
	env := decodeEnvelope(t, r.stdout)
	if env["profile"].(map[string]any)["handle"] != "H" {
		t.Fatalf("profile handle: %v", env["profile"])
	}
	r = execute(t, "", "auth", "status", "-o", "table")
	ok(t, r)
	if !strings.Contains(r.stdout, "rateLimit    plan pro, limit high") {
		t.Fatalf("table:\n%s", r.stdout)
	}
}

func TestVerboseTraceRedacts(t *testing.T) {
	setup(t)
	r := execute(t, "", "me", "--verbose")
	ok(t, r)
	if strings.Contains(r.stderr, figmatest.Token) || !strings.Contains(r.stderr, "X-Figma-Token: [redacted]") {
		t.Fatalf("trace:\n%s", r.stderr)
	}
}

func TestProfileHintOnNotFound(t *testing.T) {
	setup(t)
	execute(t, figmatest.Token+"\n", "profile", "add", "acme")
	execute(t, figmatest.Token+"\n", "profile", "add", "other")
	t.Setenv(config.TokenEnv, "")
	r := execute(t, "", "file", "info", "NoSuchFile000000", "--profile", "acme")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
	env := decodeEnvelope(t, r.stdout)
	hint := env["error"].(map[string]any)["hint"].(string)
	if !strings.Contains(hint, "Other configured profiles: other") || strings.Contains(hint, "acme,") {
		t.Fatalf("hint = %q", hint)
	}
}
