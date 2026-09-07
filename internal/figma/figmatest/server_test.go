package figmatest

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func get(t *testing.T, s *Server, path string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, s.URL+path, nil)
	req.Header.Set("X-Figma-Token", Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// TestFixturesConsistent checks that ids referenced across fixture files
// exist in the document so later golden tests can rely on them.
func TestFixturesConsistent(t *testing.T) {
	var file map[string]any
	Decode(t, "file.json", &file)
	ids := map[string]bool{}
	for _, id := range NodeIDs(t) {
		ids[id] = true
	}
	for _, want := range []string{"0:1", "1:2", "2:1", "2:2", "2:3", "2:4", "2:5", "2:6", "2:7", "2:8", "2:9", "2:10", "2:11", "2:12", "2:13", "2:14", "3:10", "3:11", "3:20", "4:0", "I2:6;3:12", "I2:5;3:21"} {
		if !ids[want] {
			t.Errorf("document is missing node %s", want)
		}
	}
	components := file["components"].(map[string]any)
	for id := range components {
		if !ids[id] {
			t.Errorf("component %s is not in the document", id)
		}
	}

	var comps struct {
		Meta struct {
			Components []struct {
				Key    string `json:"key"`
				NodeID string `json:"node_id"`
			} `json:"components"`
		} `json:"meta"`
	}
	Decode(t, "components.json", &comps)
	for _, c := range comps.Meta.Components {
		entry, ok := components[c.NodeID].(map[string]any)
		if !ok || entry["key"] != c.Key {
			t.Errorf("components.json entry %s does not match file.json", c.NodeID)
		}
	}

	var styleNodes map[string]any
	Decode(t, "nodes.json", &styleNodes)
	styles := file["styles"].(map[string]any)
	for id := range styles {
		if _, ok := styleNodes["nodes"].(map[string]any)[id]; !ok {
			t.Errorf("style %s has no node in nodes.json", id)
		}
	}

	var vars struct {
		Meta struct {
			Variables map[string]struct {
				VariableCollectionID string `json:"variableCollectionId"`
			} `json:"variables"`
			Collections map[string]struct {
				VariableIDs []string `json:"variableIds"`
			} `json:"variableCollections"`
		} `json:"meta"`
	}
	Decode(t, "variables_local.json", &vars)
	for id, v := range vars.Meta.Variables {
		if _, ok := vars.Meta.Collections[v.VariableCollectionID]; !ok {
			t.Errorf("variable %s references unknown collection %s", id, v.VariableCollectionID)
		}
	}
	for cid, c := range vars.Meta.Collections {
		for _, id := range c.VariableIDs {
			if _, ok := vars.Meta.Variables[id]; !ok {
				t.Errorf("collection %s lists unknown variable %s", cid, id)
			}
		}
	}
	bound := map[string]bool{}
	raw := Fixture(t, "file.json")
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	collectAliases(generic, bound)
	for id := range bound {
		if _, ok := vars.Meta.Variables[id]; !ok {
			t.Errorf("document binds unknown variable %s", id)
		}
	}
}

func collectAliases(v any, out map[string]bool) {
	switch val := v.(type) {
	case map[string]any:
		if val["type"] == "VARIABLE_ALIAS" {
			if id, ok := val["id"].(string); ok {
				out[id] = true
			}
		}
		for _, child := range val {
			collectAliases(child, out)
		}
	case []any:
		for _, child := range val {
			collectAliases(child, out)
		}
	}
}

func TestServerRoutes(t *testing.T) {
	s := NewServer(t)

	status, body := get(t, s, "/v1/files/"+FileKey+"?depth=1")
	if status != 200 {
		t.Fatalf("file: %d %s", status, body)
	}
	var file map[string]any
	_ = json.Unmarshal(body, &file)
	pages := file["document"].(map[string]any)["children"].([]any)
	if len(pages) != 2 || pages[0].(map[string]any)["children"] != nil {
		t.Fatalf("depth=1 should return pages without children: %v", pages)
	}
	if _, ok := file["branches"]; ok {
		t.Fatal("branches should be absent without branch_data")
	}

	status, body = get(t, s, "/v1/files/"+FileKey+"/nodes?ids=2:6,5:1,9:9&depth=1")
	if status != 200 {
		t.Fatalf("nodes: %d %s", status, body)
	}
	var nodes struct {
		Nodes map[string]json.RawMessage `json:"nodes"`
	}
	_ = json.Unmarshal(body, &nodes)
	if string(nodes.Nodes["9:9"]) != "null" {
		t.Fatalf("unknown node should be null, got %s", nodes.Nodes["9:9"])
	}
	var entry map[string]any
	_ = json.Unmarshal(nodes.Nodes["2:6"], &entry)
	if _, ok := entry["components"].(map[string]any)["3:11"]; !ok {
		t.Fatalf("instance entry should carry its component: %v", entry["components"])
	}
	if _, ok := entry["componentSets"].(map[string]any)["3:10"]; !ok {
		t.Fatalf("instance entry should carry its component set: %v", entry["componentSets"])
	}
	if _, ok := entry["styles"].(map[string]any)["5:1"]; !ok {
		t.Fatalf("instance entry should carry its fill style: %v", entry["styles"])
	}
	_ = json.Unmarshal(nodes.Nodes["5:1"], &entry)
	if entry["document"].(map[string]any)["name"] != "color/brand/500" {
		t.Fatalf("style node not served: %v", entry)
	}

	status, body = get(t, s, "/v1/images/"+FileKey+"?ids=2:2,2:13&format=svg")
	if status != 200 {
		t.Fatalf("images: %d %s", status, body)
	}
	var images struct {
		Images map[string]*string `json:"images"`
	}
	_ = json.Unmarshal(body, &images)
	if images.Images["2:13"] != nil || images.Images["2:2"] == nil {
		t.Fatalf("unexpected images: %v", images.Images)
	}
	status, _ = get(t, s, "/images/2-2.svg")
	if status != 200 {
		t.Fatalf("blob download: %d", status)
	}

	status, _ = get(t, s, "/v1/files/NoSuchFileKey0000/meta")
	if status != 404 {
		t.Fatalf("unknown file: %d", status)
	}
	req, _ := http.NewRequest(http.MethodGet, s.URL+"/v1/me", nil)
	req.Header.Set("X-Figma-Token", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}

	s.Enqueue(http.MethodGet, "/v1/me", Response{Status: 500, Body: map[string]any{"status": 500, "err": "boom"}})
	status, _ = get(t, s, "/v1/me")
	if status != 500 {
		t.Fatalf("queued response not used: %d", status)
	}
	status, _ = get(t, s, "/v1/me")
	if status != 200 {
		t.Fatalf("queue should be consumed: %d", status)
	}
	if s.Count(http.MethodGet, "/v1/me") != 3 {
		t.Fatalf("count = %d", s.Count(http.MethodGet, "/v1/me"))
	}

	status, body = get(t, s, "/v1/teams/"+TeamID+"/components?page_size=2&after=2")
	if status != 200 {
		t.Fatalf("team components: %d %s", status, body)
	}
	var team struct {
		Meta struct {
			Components []any          `json:"components"`
			Cursor     map[string]int `json:"cursor"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(body, &team)
	if len(team.Meta.Components) != 2 || team.Meta.Cursor["after"] != 4 || team.Meta.Cursor["before"] != 2 {
		t.Fatalf("unexpected page: %+v", team.Meta)
	}

	status, body = get(t, s, "/v1/files/"+FileKey+"/versions?page_size=1")
	if status != 200 {
		t.Fatalf("versions: %d %s", status, body)
	}
	var versions struct {
		Versions   []any             `json:"versions"`
		Pagination map[string]string `json:"pagination"`
	}
	_ = json.Unmarshal(body, &versions)
	if len(versions.Versions) != 1 || versions.Pagination["next_page"] == "" {
		t.Fatalf("unexpected versions page: %+v", versions)
	}
}
