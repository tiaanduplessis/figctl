package figma_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func TestAPIRedirectDoesNotSendTokenToAnotherOrigin(t *testing.T) {
	var received string
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("X-Figma-Token")
		_, _ = w.Write([]byte(`{"id":"123","handle":"test"}`))
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()
	c := figma.New(figma.Options{BaseURL: source.URL, Token: figmatest.Token, MaxAttempts: 1})
	if _, err := c.GetMe(context.Background()); err == nil {
		t.Fatal("expected the cross-origin API redirect to fail")
	}
	if received != "" {
		t.Fatal("redirect forwarded the API token to another origin")
	}
}

func TestAPIRedirectWithinOriginPreservesAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			http.Redirect(w, r, "/moved", http.StatusFound)
			return
		}
		if r.Header.Get("X-Figma-Token") != figmatest.Token {
			t.Error("same-origin redirect lost authentication")
		}
		_, _ = w.Write([]byte(`{"id":"123","handle":"test"}`))
	}))
	defer server.Close()
	c := figma.New(figma.Options{BaseURL: server.URL, Token: figmatest.Token, MaxAttempts: 1})
	if _, err := c.GetMe(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadRedirectNeverSendsAPIToken(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Figma-Token") != "" {
			t.Error("download sent the API token")
		}
		_, _ = w.Write([]byte("image"))
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()
	c := figma.New(figma.Options{Token: figmatest.Token})
	var data bytes.Buffer
	if err := c.Download(context.Background(), source.URL, &data); err != nil {
		t.Fatal(err)
	}
	if data.String() != "image" {
		t.Fatal("redirected download returned the wrong content")
	}
}

type recorder struct {
	lines []string
}

func (r *recorder) Debugf(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

type fakeSleeper struct {
	waits []time.Duration
}

func (f *fakeSleeper) sleep(_ context.Context, d time.Duration) error {
	f.waits = append(f.waits, d)
	return nil
}

func newClient(t *testing.T, s *figmatest.Server, mutate func(*figma.Options)) (*figma.Client, *fakeSleeper, *recorder) {
	t.Helper()
	sleeper := &fakeSleeper{}
	log := &recorder{}
	opts := figma.Options{
		Token:   figmatest.Token,
		BaseURL: s.URL,
		Version: "test",
		Timeout: 10 * time.Second,
		Logger:  log,
		Sleep:   sleeper.sleep,
	}
	if mutate != nil {
		mutate(&opts)
	}
	return figma.New(opts), sleeper, log
}

func code(t *testing.T, err error) *figctl.Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var typed *figctl.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error %v is not a figctl error", err)
	}
	return typed
}

func TestGetFileDecodes(t *testing.T) {
	s := figmatest.NewServer(t)
	c, _, _ := newClient(t, s, nil)
	ctx := context.Background()

	file, err := c.GetFile(ctx, figmatest.FileKey, figma.FileOptions{BranchData: true})
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != figmatest.FileName || file.Version != figmatest.Version || file.Role != "editor" || file.EditorType != "figma" {
		t.Fatalf("unexpected header: %+v", file)
	}
	if len(file.Branches) != 1 || file.Branches[0].Key != figmatest.BranchKey {
		t.Fatalf("branches: %+v", file.Branches)
	}
	if len(file.Document.Children) != 2 || file.Document.Children[0].Name != "Screens" {
		t.Fatalf("pages: %+v", file.Document.Children)
	}
	login := file.Document.Find("2:2")
	if login == nil || login.Type != "FRAME" || login.LayoutMode != "VERTICAL" || *login.PaddingLeft != 24 || *login.ItemSpacing != 16 {
		t.Fatalf("login frame: %+v", login)
	}
	if login.AbsoluteBoundingBox.Width != 390 || login.AbsoluteBoundingBox.Height != 844 {
		t.Fatalf("login bounds: %+v", login.AbsoluteBoundingBox)
	}
	if b := login.BoundVariables["fills"]; len(b.Aliases) != 1 || b.Aliases[0].ID != "VariableID:1:201" {
		t.Fatalf("fills binding: %+v", b)
	}
	if b := login.BoundVariables["itemSpacing"]; b.Alias == nil || b.Alias.ID != "VariableID:1:104" {
		t.Fatalf("itemSpacing binding: %+v", b)
	}
	if login.Styles["effect"] != "5:3" || login.Effects[0].Type != "DROP_SHADOW" || *login.Effects[0].Radius != 12 {
		t.Fatalf("effects: %+v %+v", login.Styles, login.Effects)
	}
	if len(login.LayoutGrids) != 1 || login.LayoutGrids[0].Count != 12 {
		t.Fatalf("layout grids: %+v", login.LayoutGrids)
	}
	heading := file.Document.Find("2:3")
	if heading.Characters != "Welcome back" || *heading.Style.FontWeight != 700 || *heading.Style.LineHeightPx != 34 || firstID(heading.Style.BoundVariables["fontFamily"]) != "VariableID:1:105" {
		t.Fatalf("heading: %+v", heading.Style)
	}
	if firstID(heading.Fills[0].BoundVariables["color"]) != "VariableID:1:202" {
		t.Fatalf("heading fill binding: %+v", heading.Fills[0])
	}
	body := file.Document.Find("2:4")
	if len(body.CharacterStyleOverrides) != 36 || *body.StyleOverrideTable["1"].FontWeight != 600 || body.StyleOverrideTable["1"].Hyperlink.URL == "" {
		t.Fatalf("body overrides: %+v", body.StyleOverrideTable)
	}
	if body.Style.FontPostScriptName != nil {
		t.Fatalf("null fontPostScriptName should decode to nil, got %v", *body.Style.FontPostScriptName)
	}
	button := file.Document.Find("2:6")
	if button.ComponentID != "3:11" || button.ComponentProperties["Variant"].Value != "Primary" || button.ComponentProperties["Label#5:0"].Type != "TEXT" {
		t.Fatalf("button instance: %+v", button.ComponentProperties)
	}
	if len(button.Overrides) != 2 || button.Overrides[1].ID != "I2:6;3:12" || button.TransitionNodeID != "2:14" || button.Interactions[0].Trigger.Type != "ON_CLICK" || *button.Interactions[0].Actions[0].DestinationID != "2:14" {
		t.Fatalf("button prototype data: %+v", button.Interactions)
	}
	social := file.Document.Find("2:7")
	if social.LayoutWrap != "WRAP" || *social.CounterAxisSpacing != 8 {
		t.Fatalf("social: %+v", social)
	}
	avatar := file.Document.Find("2:8")
	if avatar.Fills[0].Type != "IMAGE" || avatar.Fills[0].ImageRef != "img1" || *avatar.CornerRadius != 20 {
		t.Fatalf("avatar: %+v", avatar.Fills)
	}
	icon := file.Document.Find("2:9")
	if len(icon.ExportSettings) != 2 || icon.ExportSettings[0].Format != "SVG" || len(icon.FillGeometry) != 1 {
		t.Fatalf("icon: %+v", icon)
	}
	blob := file.Document.Find("2:11")
	if blob.Fills[0].Type != "GRADIENT_LINEAR" || len(blob.Fills[0].GradientStops) != 2 || firstID(blob.Fills[0].GradientStops[0].BoundVariables["color"]) != "VariableID:1:101" {
		t.Fatalf("gradient: %+v", blob.Fills[0])
	}
	if debug := file.Document.Find("2:13"); debug.IsVisible() || debug.AbsoluteRenderBounds != nil {
		t.Fatalf("debug frame should be hidden: %+v", debug)
	}
	stats := file.Document.Find("2:14")
	if stats.LayoutMode != "GRID" || *stats.GridRowCount != 2 || *stats.GridColumnGap != 8 || *file.Document.Find("2:15").GridRowSpan != 2 {
		t.Fatalf("grid: %+v", stats)
	}
	set := file.Document.Find("3:10")
	if set.Type != "COMPONENT_SET" || len(set.ComponentPropertyDefinitions["Variant"].VariantOptions) != 2 || set.ComponentPropertyDefinitions["Label#5:0"].DefaultValue != "Button" {
		t.Fatalf("component set: %+v", set.ComponentPropertyDefinitions)
	}
	if file.Components["3:11"].ComponentSetID != "3:10" || file.ComponentSets["3:10"].Name != "Button" || file.Styles["5:2"].StyleType != "TEXT" {
		t.Fatalf("maps: %+v %+v %+v", file.Components, file.ComponentSets, file.Styles)
	}
	if section := file.Document.Find("2:1"); section.DevStatus == nil || section.DevStatus.Type != "READY_FOR_DEV" {
		t.Fatalf("section: %+v", section)
	}

	// Re-encoding keeps the explicit false and the bindings.
	raw, err := json.Marshal(file.Document.Find("2:13"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"visible":false`)) {
		t.Fatalf("visible false lost: %s", raw)
	}
	raw, _ = json.Marshal(login.BoundVariables)
	if string(raw) != `{"fills":[{"type":"VARIABLE_ALIAS","id":"VariableID:1:201"}],"itemSpacing":{"type":"VARIABLE_ALIAS","id":"VariableID:1:104"}}` {
		t.Fatalf("bindings round trip: %s", raw)
	}
}

func TestNodeHelpers(t *testing.T) {
	var file figma.GetFileResponse
	figmatest.Decode(t, "file.json", &file)
	doc := file.Document
	var visited []string
	depths := map[string]int{}
	doc.Walk(func(n *figma.Node, parent *figma.Node, depth int) bool {
		visited = append(visited, n.ID)
		depths[n.ID] = depth
		if parent != nil && parent.ID == "2:2" && n.ID == "2:5" {
			return false
		}
		return true
	})
	if depths["0:0"] != 0 || depths["0:1"] != 1 || depths["2:2"] != 3 || depths["2:3"] != 4 {
		t.Fatalf("depths: %v", depths)
	}
	for _, id := range visited {
		if id == "I2:5;3:21" {
			t.Fatal("children of 2:5 should have been skipped")
		}
	}
	if doc.Find("I2:6;3:12") == nil || doc.Find("nope") != nil {
		t.Fatal("Find failed")
	}
	pruned := doc.Prune(1)
	if len(pruned.Children) != 2 || pruned.Children[0].Children != nil {
		t.Fatalf("Prune(1): %+v", pruned)
	}
	if len(doc.Children[0].Children) == 0 {
		t.Fatal("Prune must not modify the original")
	}
	if doc.Prune(0).Children != nil || len(doc.Prune(-1).Children[0].Children) == 0 {
		t.Fatal("Prune edge cases")
	}

	var binding figma.VariableBinding
	if err := json.Unmarshal([]byte(`{"x":{"type":"VARIABLE_ALIAS","id":"VariableID:1:1"},"y":{"type":"VARIABLE_ALIAS","id":"VariableID:1:2"}}`), &binding); err != nil {
		t.Fatal(err)
	}
	if len(binding.Fields) != 2 || len(binding.All()) != 2 {
		t.Fatalf("compound binding: %+v", binding)
	}
	var value figma.VariableValue
	for raw, check := range map[string]func(figma.VariableValue) bool{
		`true`:                          func(v figma.VariableValue) bool { return v.Bool != nil && *v.Bool },
		`16`:                            func(v figma.VariableValue) bool { return v.Number != nil && *v.Number == 16 },
		`"Inter"`:                       func(v figma.VariableValue) bool { return v.String != nil && *v.String == "Inter" },
		`{"r":0.2,"g":0.4,"b":1,"a":1}`: func(v figma.VariableValue) bool { return v.Color != nil && v.Color.B == 1 },
		`{"type":"VARIABLE_ALIAS","id":"VariableID:1:103"}`: func(v figma.VariableValue) bool { return v.Alias != nil && v.Alias.ID == "VariableID:1:103" },
	} {
		value = figma.VariableValue{}
		if err := json.Unmarshal([]byte(raw), &value); err != nil || !check(value) {
			t.Fatalf("value %s: %+v %v", raw, value, err)
		}
		out, _ := json.Marshal(value)
		if string(out) != raw {
			t.Fatalf("value %s round trip: %s", raw, out)
		}
	}
}

func TestEndpointsDecode(t *testing.T) {
	s := figmatest.NewServer(t)
	c, _, _ := newClient(t, s, nil)
	ctx := context.Background()
	key := figmatest.FileKey

	nodes, err := c.GetFileNodes(ctx, key, []string{"2:2", "5:2", "9:9"}, figma.NodesOptions{Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if nodes.Version != figmatest.Version || len(nodes.Nodes) != 3 || nodes.Nodes["9:9"] != nil {
		t.Fatalf("nodes: %+v", nodes)
	}
	if login := nodes.Nodes["2:2"].Document; len(login.Children) != 8 || login.Children[2].Children != nil {
		t.Fatalf("depth 1 login: %d children", len(login.Children))
	}
	if nodes.Nodes["5:2"].Document.Style == nil || *nodes.Nodes["5:2"].Document.Style.FontSize != 28 {
		t.Fatalf("style node: %+v", nodes.Nodes["5:2"].Document)
	}
	if nodes.Nodes["2:2"].Styles["5:3"].Name != "shadow/md" {
		t.Fatalf("referenced styles: %+v", nodes.Nodes["2:2"].Styles)
	}

	meta, err := c.GetFileMeta(ctx, key)
	if err != nil || meta.File.LastTouchedAt != "2026-09-01T10:15:00Z" || meta.File.Version != figmatest.Version || meta.File.Creator.Handle != figmatest.UserHandle {
		t.Fatalf("meta: %+v %v", meta, err)
	}

	images, err := c.GetImages(ctx, key, []string{"2:2", "2:13"}, figma.ImageOptions{Format: "svg", Scale: 2})
	if err != nil || images.Err != nil || images.Images["2:13"] != nil || !strings.HasSuffix(*images.Images["2:2"], "/images/2-2.svg") {
		t.Fatalf("images: %+v %v", images, err)
	}
	var buf bytes.Buffer
	if err := c.Download(ctx, *images.Images["2:2"], &buf); err != nil || !strings.Contains(buf.String(), "<svg") {
		t.Fatalf("download: %q %v", buf.String(), err)
	}
	for _, req := range s.Requests() {
		if strings.HasPrefix(req.Path, "/images/") && req.Header.Get("X-Figma-Token") != "" {
			t.Fatal("download must not send the token")
		}
		if req.Path == "/v1/images/"+key {
			if req.Query.Get("scale") != "2" || req.Query.Get("format") != "svg" || req.Query.Get("svg_outline_text") != "" {
				t.Fatalf("image query: %v", req.Query)
			}
		}
	}

	fills, err := c.GetImageFills(ctx, key)
	if err != nil || !strings.HasPrefix(fills.Meta.Images["img1"], s.URL) {
		t.Fatalf("image fills: %+v %v", fills, err)
	}

	local, err := c.GetLocalVariables(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	semantic := local.Meta.VariableCollections["VariableCollectionId:1:200"]
	if len(semantic.Modes) != 2 || semantic.Modes[1].Name != "Dark" || semantic.DefaultModeID != "2:0" {
		t.Fatalf("semantic collection: %+v", semantic)
	}
	surface := local.Meta.Variables["VariableID:1:201"]
	if surface.ResolvedType != "COLOR" || surface.ValuesByMode["2:1"].Alias.ID != "VariableID:1:102" || surface.CodeSyntax.Web != "--bg-surface" {
		t.Fatalf("bg/surface: %+v", surface)
	}
	space := local.Meta.Variables["VariableID:1:104"]
	if *space.ValuesByMode["1:0"].Number != 16 || space.Scopes[0] != "GAP" || space.CodeSyntax.Web != "--space-4" {
		t.Fatalf("space/4: %+v", space)
	}
	if brand := local.Meta.Variables["VariableID:1:101"]; brand.ValuesByMode["1:0"].Color.B != 1 {
		t.Fatalf("brand/500: %+v", brand)
	}
	if *local.Meta.Variables["VariableID:1:105"].ValuesByMode["1:0"].String != "Inter" || !*local.Meta.Variables["VariableID:1:106"].ValuesByMode["1:0"].Bool {
		t.Fatal("string or boolean variable")
	}

	published, err := c.GetPublishedVariables(ctx, key)
	if err != nil || published.Meta.Variables["VariableID:1:201"].SubscribedID != "VariableID:1:201/1" || published.Meta.VariableCollections["VariableCollectionId:1:100"].Name != "Primitives" {
		t.Fatalf("published: %+v %v", published, err)
	}

	comps, err := c.GetFileComponents(ctx, key)
	if err != nil || len(comps.Meta.Components) != 5 || comps.Meta.Components[0].ContainingFrame.ContainingStateGroup.Name != "Button" {
		t.Fatalf("components: %+v %v", comps, err)
	}
	sets, err := c.GetFileComponentSets(ctx, key)
	if err != nil || len(sets.Meta.ComponentSets) != 1 || sets.Meta.ComponentSets[0].NodeID != "3:10" {
		t.Fatalf("component sets: %+v %v", sets, err)
	}
	styles, err := c.GetFileStyles(ctx, key)
	if err != nil || len(styles.Meta.Styles) != 4 || styles.Meta.Styles[1].StyleType != "TEXT" {
		t.Fatalf("styles: %+v %v", styles, err)
	}
	teamComps, err := c.GetTeamComponents(ctx, figmatest.TeamID, figma.PageOptions{PageSize: 2})
	if err != nil || len(teamComps.Meta.Components) != 2 || teamComps.Meta.Cursor == nil || *teamComps.Meta.Cursor.After != 2 {
		t.Fatalf("team components: %+v %v", teamComps.Meta, err)
	}
	if _, err := c.GetTeamComponentSets(ctx, figmatest.TeamID, figma.PageOptions{}); err != nil {
		t.Fatal(err)
	}
	teamStyles, err := c.GetTeamStyles(ctx, figmatest.TeamID, figma.PageOptions{After: "3"})
	if err != nil || len(teamStyles.Meta.Styles) != 1 || *teamStyles.Meta.Cursor.Before != 3 {
		t.Fatalf("team styles: %+v %v", teamStyles.Meta, err)
	}
	comp, err := c.GetComponent(ctx, comps.Meta.Components[4].Key)
	if err != nil || comp.Meta.Name != "Input/Text" {
		t.Fatalf("component by key: %+v %v", comp, err)
	}
	set, err := c.GetComponentSet(ctx, sets.Meta.ComponentSets[0].Key)
	if err != nil || set.Meta.Name != "Button" {
		t.Fatalf("component set by key: %+v %v", set, err)
	}
	style, err := c.GetStyle(ctx, styles.Meta.Styles[2].Key)
	if err != nil || style.Meta.Name != "shadow/md" {
		t.Fatalf("style by key: %+v %v", style, err)
	}
	if _, err := c.GetStyle(ctx, "0000000000000000000000000000000000000000"); code(t, err).Code != figctl.CodeNotFound {
		t.Fatalf("unknown style: %v", err)
	}

	versions, err := c.GetVersions(ctx, key, figma.PageOptions{PageSize: 1})
	if err != nil || len(versions.Versions) != 1 || *versions.Versions[0].Label != "Login v2" || !strings.Contains(versions.Pagination.NextPage, "before=2100123456") {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	comments, err := c.GetComments(ctx, key, true)
	if err != nil || len(comments.Comments) != 3 || comments.Comments[0].ClientMeta.NodeID != "2:2" || comments.Comments[1].ParentID != "9001" || comments.Comments[2].ResolvedAt == nil || comments.Comments[1].OrderID != nil {
		t.Fatalf("comments: %+v %v", comments, err)
	}
	if comments.Comments[2].ClientMeta.NodeID != "" || *comments.Comments[2].ClientMeta.X != 120 {
		t.Fatalf("canvas pin: %+v", comments.Comments[2].ClientMeta)
	}
	posted, err := c.PostComment(ctx, key, figma.PostCommentRequest{Message: "Implemented", ClientMeta: &figma.CommentPin{NodeID: "2:2", NodeOffset: &figma.Vector{X: 1, Y: 2}}})
	if err != nil || posted.ID != "9100" || posted.Message != "Implemented" || posted.ClientMeta.NodeID != "2:2" {
		t.Fatalf("post comment: %+v %v", posted, err)
	}
	dev, err := c.GetDevResources(ctx, key, []string{"3:20"})
	if err != nil || len(dev.DevResources) != 1 || dev.DevResources[0].ID != "dr-2" {
		t.Fatalf("dev resources: %+v %v", dev, err)
	}
	me, err := c.GetMe(ctx)
	if err != nil || me.Handle != figmatest.UserHandle || me.Email != figmatest.UserEmail || me.ID != "1234567890" {
		t.Fatalf("me: %+v %v", me, err)
	}
	projects, err := c.GetTeamProjects(ctx, figmatest.TeamID)
	if err != nil || len(projects.Projects) != 2 || projects.Name != "Fixture Team" {
		t.Fatalf("projects: %+v %v", projects, err)
	}
	files, err := c.GetProjectFiles(ctx, figmatest.ProjectID, false)
	if err != nil || len(files.Files) != 1 || files.Files[0].Key != key {
		t.Fatalf("project files: %+v %v", files, err)
	}
	folders, err := c.GetTeamFolders(ctx, figmatest.TeamID)
	if err != nil || len(folders.Folders) != 2 || folders.Folders[0].ParentFolderID != nil {
		t.Fatalf("team folders: %+v %v", folders, err)
	}
	sub, err := c.GetFolderFolders(ctx, figmatest.FolderID)
	if err != nil || len(sub.Folders) != 1 || *sub.Folders[0].ParentFolderID != figmatest.FolderID {
		t.Fatalf("subfolders: %+v %v", sub, err)
	}
	folderFiles, err := c.GetFolderFiles(ctx, figmatest.FolderID, true)
	if err != nil || len(folderFiles.Files) != 1 || len(folderFiles.Files[0].Branches) != 1 {
		t.Fatalf("folder files: %+v %v", folderFiles, err)
	}
	folderMeta, err := c.GetFolderMeta(ctx, figmatest.FolderID)
	if err != nil || folderMeta.FileCount != 1 || folderMeta.ThumbnailURL != nil {
		t.Fatalf("folder meta: %+v %v", folderMeta, err)
	}
	if _, err := c.GetFolderFiles(ctx, "9999", false); code(t, err).Code != figctl.CodeNotFound {
		t.Fatalf("unknown folder: %v", err)
	}
}

func TestSplitIDs(t *testing.T) {
	if got := figma.SplitIDs(nil, 100); got != nil {
		t.Fatalf("empty: %v", got)
	}
	ids := []string{"1:2", "1:3", "1:2", "10:20"}
	batches := figma.SplitIDs(ids, 12)
	// "1%3A2" is 5 bytes, "%2C" is 3, so two ids fit in 13 bytes, not 12.
	if len(batches) != 3 || batches[0][0] != "1:2" || batches[1][0] != "1:3" || batches[2][0] != "10:20" {
		t.Fatalf("batches: %v", batches)
	}
	if got := figma.SplitIDs([]string{"1:2", "1:3"}, 13); len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("fit: %v", got)
	}
	if got := figma.SplitIDs([]string{strings.Repeat("1", 50)}, 10); len(got) != 1 {
		t.Fatalf("oversized id: %v", got)
	}
}

func TestBatchingKeepsURLsShort(t *testing.T) {
	s := figmatest.NewServer(t)
	c, _, _ := newClient(t, s, nil)
	ids := make([]string, 0, 3000)
	for i := 0; i < 3000; i++ {
		ids = append(ids, fmt.Sprintf("77:%d", i))
	}
	nodes, err := c.GetFileNodes(context.Background(), figmatest.FileKey, ids, figma.NodesOptions{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes.Nodes) != 3000 || nodes.Name != figmatest.FileName {
		t.Fatalf("merged %d nodes, name %q", len(nodes.Nodes), nodes.Name)
	}
	count := s.Count(http.MethodGet, "/v1/files/"+figmatest.FileKey+"/nodes")
	if count < 2 {
		t.Fatalf("expected several batches, got %d requests", count)
	}
	seen := 0
	for _, req := range s.Requests() {
		if req.Path != "/v1/files/"+figmatest.FileKey+"/nodes" {
			continue
		}
		full := s.URL + req.Path + "?" + req.Query.Encode()
		if len(full) > 8*1024 {
			t.Fatalf("URL too long: %d bytes", len(full))
		}
		if req.Query.Get("depth") != "2" {
			t.Fatalf("depth lost on batch: %v", req.Query)
		}
		seen += len(strings.Split(req.Query.Get("ids"), ","))
	}
	if seen != 3000 {
		t.Fatalf("ids sent = %d", seen)
	}

	s.Reset()
	images, err := c.GetImages(context.Background(), figmatest.FileKey, append([]string{"2:2"}, ids[:2500]...), figma.ImageOptions{Format: "png"})
	if err == nil {
		t.Fatalf("unknown ids should fail rendering, got %d images", len(images.Images))
	}
	if code(t, err).Code != figctl.CodeUsage {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRetries(t *testing.T) {
	s := figmatest.NewServer(t)
	c, sleeper, _ := newClient(t, s, nil)
	ctx := context.Background()

	s.Enqueue(http.MethodGet, "/v1/me", figmatest.Response{
		Status: 429,
		Body:   map[string]any{"status": 429, "err": "Rate limit exceeded"},
		Header: map[string]string{"Retry-After": "7", "X-Figma-Plan-Tier": "pro", "X-Figma-Rate-Limit-Type": "low"},
	})
	me, err := c.GetMe(ctx)
	if err != nil || me.Handle != figmatest.UserHandle {
		t.Fatalf("retry after 429 failed: %v", err)
	}
	if len(sleeper.waits) != 1 || sleeper.waits[0] != 7*time.Second {
		t.Fatalf("waits = %v", sleeper.waits)
	}
	if s.Count(http.MethodGet, "/v1/me") != 2 {
		t.Fatalf("requests = %d", s.Count(http.MethodGet, "/v1/me"))
	}
	rl := c.LastRateLimit()
	if !rl.Seen || rl.PlanTier != "pro" || rl.Type != "low" || rl.RetryAfterSeconds != 7 || rl.Status != 429 {
		t.Fatalf("rate limit not captured: %+v", rl)
	}

	s.Reset()
	sleeper.waits = nil
	s.Enqueue(http.MethodGet, "/v1/me",
		figmatest.Response{Status: 500, Body: "boom"},
		figmatest.Response{Status: 503, Body: map[string]any{"status": 503, "err": "unavailable"}},
	)
	if _, err := c.GetMe(ctx); err != nil {
		t.Fatalf("retry after 5xx failed: %v", err)
	}
	if len(sleeper.waits) != 2 || sleeper.waits[0] < 500*time.Millisecond || sleeper.waits[0] > 750*time.Millisecond || sleeper.waits[1] < time.Second || sleeper.waits[1] > 1500*time.Millisecond {
		t.Fatalf("backoff waits = %v", sleeper.waits)
	}

	s.Reset()
	sleeper.waits = nil
	s.Enqueue(http.MethodGet, "/v1/me",
		figmatest.Response{Status: 500, Body: "1"},
		figmatest.Response{Status: 500, Body: "2"},
		figmatest.Response{Status: 500, Body: "3"},
	)
	_, err = c.GetMe(ctx)
	if e := code(t, err); e.Code != figctl.CodeNetwork || e.HTTPStatus != 500 {
		t.Fatalf("exhausted retries: %+v", e)
	}
	if s.Count(http.MethodGet, "/v1/me") != 3 || len(sleeper.waits) != 2 {
		t.Fatalf("attempts = %d, waits = %v", s.Count(http.MethodGet, "/v1/me"), sleeper.waits)
	}

	for _, status := range []int{400, 401} {
		s.Reset()
		sleeper.waits = nil
		s.Enqueue(http.MethodGet, "/v1/me", figmatest.Response{Status: status, Body: map[string]any{"status": status, "err": "nope"}})
		if _, err := c.GetMe(ctx); err == nil {
			t.Fatalf("%d should fail", status)
		}
		if s.Count(http.MethodGet, "/v1/me") != 1 || len(sleeper.waits) != 0 {
			t.Fatalf("%d must not retry: attempts = %d", status, s.Count(http.MethodGet, "/v1/me"))
		}
	}

	// A Retry-After beyond the deadline gives up without sleeping.
	s.Reset()
	sleeper.waits = nil
	s.Enqueue(http.MethodGet, "/v1/me", figmatest.Response{Status: 429, Body: "", Header: map[string]string{"Retry-After": "30"}})
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = c.GetMe(short)
	if e := code(t, err); e.Code != figctl.CodeRateLimited || e.RetryAfterSeconds != 30 {
		t.Fatalf("deadline bound: %+v", e)
	}
	if len(sleeper.waits) != 0 {
		t.Fatalf("should not sleep past the deadline: %v", sleeper.waits)
	}
}

func TestErrorMapping(t *testing.T) {
	s := figmatest.NewServer(t)
	c, _, _ := newClient(t, s, func(o *figma.Options) {
		o.MaxAttempts = 1
		o.ProfileNames = func() []string { return []string{"acme", "other"} }
	})
	ctx := context.Background()
	me := "/v1/me"
	vars := "/v1/files/" + figmatest.FileKey + "/variables/local"

	cases := []struct {
		name    string
		path    string
		resp    figmatest.Response
		call    func() error
		code    figctl.Code
		message string
		hint    string
	}{
		{"400 usage", me, figmatest.Response{Status: 400, Body: map[string]any{"status": 400, "err": "Invalid depth"}}, func() error { _, err := c.GetMe(ctx); return err }, figctl.CodeUsage, "Invalid depth", "Check"},
		{"401", me, figmatest.Response{Status: 401, Body: map[string]any{"error": true, "status": 401, "message": "Token expired"}}, func() error { _, err := c.GetMe(ctx); return err }, figctl.CodeAuthInvalid, "Token expired", "90 days"},
		{"403 invalid token", me, figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Invalid token"}}, func() error { _, err := c.GetMe(ctx); return err }, figctl.CodeAuthInvalid, "Invalid token", "wrong profile"},
		{"403 scope", "/v1/files/" + figmatest.FileKey + "/comments", figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Invalid scope(s): file_comments:read"}}, func() error { _, err := c.GetComments(ctx, figmatest.FileKey, false); return err }, figctl.CodeAuthScope, "scope", "file_comments:read scope"},
		{"403 forbidden", me, figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Access denied"}}, func() error { _, err := c.GetMe(ctx); return err }, figctl.CodeForbidden, "Access denied", "Other configured profiles: acme, other"},
		{"403 plan", vars, figmatest.Response{Status: 403, Body: map[string]any{"error": true, "status": 403, "message": "Incorrect access level. This endpoint requires an Enterprise plan."}}, func() error { _, err := c.GetLocalVariables(ctx, figmatest.FileKey); return err }, figctl.CodePlanRequired, "Enterprise", "styles"},
		{"400 plan", vars, figmatest.Response{Status: 400, Body: map[string]any{"error": true, "status": 400, "message": "Enterprise plan required"}}, func() error { _, err := c.GetLocalVariables(ctx, figmatest.FileKey); return err }, figctl.CodePlanRequired, "Enterprise", "styles list"},
		{"404 file", "/v1/files/NoSuchFile0000", figmatest.Response{Status: 404, Body: map[string]any{"status": 404, "err": "Not found"}}, func() error { _, err := c.GetFile(ctx, "NoSuchFile0000", figma.FileOptions{}); return err }, figctl.CodeNotFound, "file NoSuchFile0000 not found", "--profile <name>"},
		{"404 nodes", "/v1/files/" + figmatest.FileKey + "/nodes", figmatest.Response{Status: 404, Body: map[string]any{"status": 404, "err": "Not found"}}, func() error {
			_, err := c.GetFileNodes(ctx, figmatest.FileKey, []string{"1:1"}, figma.NodesOptions{})
			return err
		}, figctl.CodeNotFound, "nodes 1:1", "Other configured profiles"},
		{"429", me, figmatest.Response{Status: 429, Body: map[string]any{"status": 429, "err": "Rate limit exceeded"}, Header: map[string]string{"Retry-After": "42", "X-Figma-Plan-Tier": "starter", "X-Figma-Rate-Limit-Type": "low", "X-Figma-Upgrade-Link": "https://figma.com/upgrade"}}, func() error { _, err := c.GetMe(ctx); return err }, figctl.CodeRateLimited, "plan: starter, seat limit: low", "Retry after 42s"},
		{"502", me, figmatest.Response{Status: 502, Body: "<html>bad gateway</html>"}, func() error { _, err := c.GetMe(ctx); return err }, figctl.CodeNetwork, "HTTP 502", "retry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s.Enqueue(http.MethodGet, tc.path, tc.resp)
			e := code(t, tc.call())
			if e.Code != tc.code {
				t.Fatalf("code = %s, want %s (%s)", e.Code, tc.code, e.Message)
			}
			if !strings.Contains(e.Message, tc.message) {
				t.Fatalf("message %q lacks %q", e.Message, tc.message)
			}
			if !strings.Contains(e.Hint, tc.hint) {
				t.Fatalf("hint %q lacks %q", e.Hint, tc.hint)
			}
			if e.HTTPStatus != tc.resp.Status {
				t.Fatalf("httpStatus = %d", e.HTTPStatus)
			}
			if tc.code == figctl.CodeRateLimited {
				if e.RetryAfterSeconds != 42 || e.Details["planTier"] != "starter" || e.Details["rateLimitType"] != "low" || e.Details["upgradeLink"] != "https://figma.com/upgrade" {
					t.Fatalf("rate limit details: %+v", e)
				}
			}
			if tc.code == figctl.CodeAuthScope && e.Details["scope"] != figma.ScopeFileCommentsRead {
				t.Fatalf("scope details: %+v", e.Details)
			}
		})
	}

	s.Close()
	_, err := c.GetMe(ctx)
	if e := code(t, err); e.Code != figctl.CodeNetwork {
		t.Fatalf("transport error: %+v", e)
	}
}

func TestVerboseRedaction(t *testing.T) {
	s := figmatest.NewServer(t)
	c, _, log := newClient(t, s, nil)
	if _, err := c.GetFileMeta(context.Background(), figmatest.FileKey); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log.lines, "\n")
	if strings.Contains(joined, figmatest.Token) {
		t.Fatalf("token leaked into trace:\n%s", joined)
	}
	if !strings.Contains(joined, "X-Figma-Token: [redacted]") || !strings.Contains(joined, "GET "+s.URL+"/v1/files/"+figmatest.FileKey+"/meta") || !strings.Contains(joined, "200 GET") {
		t.Fatalf("trace incomplete:\n%s", joined)
	}
	if !strings.Contains(joined, "User-Agent: figctl/test") {
		t.Fatalf("user agent missing:\n%s", joined)
	}
}

func TestBaseURLDefault(t *testing.T) {
	c := figma.New(figma.Options{})
	if c.BaseURL() != figma.DefaultBaseURL {
		t.Fatalf("base = %s", c.BaseURL())
	}
	if c.LastRateLimit().Seen {
		t.Fatal("no rate limit should be seen yet")
	}
}

// firstID is the variable id a binding points at, for assertions.
func firstID(b figma.VariableBinding) string {
	if a, ok := b.First(); ok {
		return a.ID
	}
	return ""
}
