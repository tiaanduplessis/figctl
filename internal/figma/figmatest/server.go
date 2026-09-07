// Package figmatest is a fake Figma API server for tests. It serves the
// synthetic "Fixture Design System" file from testdata and lets tests
// override routes, queue failure responses, and count requests.
package figmatest

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

//go:embed testdata/*.json
var fixtures embed.FS

// Identifiers used by the fixture. Every fixture file agrees on them.
const (
	FileKey    = "FixTuReDeSiGnSySt3m01"
	BranchKey  = "BrAnChKeY0000000000001"
	FileName   = "Fixture Design System"
	Version    = "2100123456"
	Token      = "figd_fixture_token" //nolint:gosec // fake token for tests
	TeamID     = "555000111"
	ProjectID  = "1001"
	FolderID   = "2001"
	UserHandle = "Fixture Designer"
	UserEmail  = "designer@example.com"
	// CDNHost is the host used by image URLs inside the fixtures. The
	// server rewrites it to its own address so downloads work in tests.
	CDNHost = "https://figma-alpha-api.s3.us-west-2.amazonaws.com"
)

// Response is a canned HTTP response.
type Response struct {
	Status int
	// Body is encoded as JSON unless it is a []byte or string.
	Body   any
	Header map[string]string
}

// Request records one request the server received.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// Server is a fake Figma API.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	token    string
	handlers map[string]http.HandlerFunc
	queue    map[string][]Response
	counts   map[string]int
	requests []Request
}

// NewServer starts a fake Figma API that is closed when the test ends.
func NewServer(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{
		token:    Token,
		handlers: map[string]http.HandlerFunc{},
		queue:    map[string][]Response{},
		counts:   map[string]int{},
	}
	// The embedded server must be assigned before anything can serve a
	// request, because handlers read s.URL. httptest.NewServer starts
	// accepting immediately, which leaves the assignment racing against the
	// handler goroutines, so start the server only once the field is set.
	s.Server = httptest.NewUnstartedServer(s)
	s.Start()
	tb.Cleanup(s.Close)
	return s
}

// Fixture returns the raw bytes of a testdata file, for example
// "file.json".
func Fixture(tb testing.TB, name string) []byte {
	tb.Helper()
	raw, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		tb.Fatalf("fixture %s: %v", name, err)
	}
	return raw
}

// Decode unmarshals a testdata file into v.
func Decode(tb testing.TB, name string, v any) {
	tb.Helper()
	if err := json.Unmarshal(Fixture(tb, name), v); err != nil {
		tb.Fatalf("decoding fixture %s: %v", name, err)
	}
}

func key(method, p string) string { return method + " " + p }

// Handle overrides one exact path with a handler.
func (s *Server) Handle(method, p string, fn http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[key(method, p)] = fn
}

// Respond overrides one exact path with a canned response.
func (s *Server) Respond(method, p string, resp Response) {
	s.Handle(method, p, func(w http.ResponseWriter, _ *http.Request) { writeResponse(w, resp) })
}

// Enqueue queues responses for a path. Each request consumes one; when the
// queue is empty the default handling resumes.
func (s *Server) Enqueue(method, p string, responses ...Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue[key(method, p)] = append(s.queue[key(method, p)], responses...)
}

// Count returns how many requests hit an exact path.
func (s *Server) Count(method, p string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[key(method, p)]
}

// CountPrefix returns how many requests hit paths starting with prefix.
func (s *Server) CountPrefix(method, prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for k, v := range s.counts {
		if strings.HasPrefix(k, key(method, prefix)) {
			n += v
		}
	}
	return n
}

// Requests returns every request received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Reset clears counts, recorded requests, queued responses, and overrides.
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = map[string]http.HandlerFunc{}
	s.queue = map[string][]Response{}
	s.counts = map[string]int{}
	s.requests = nil
}

// SetToken changes the token the server accepts.
func (s *Server) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	k := key(r.Method, r.URL.Path)

	s.mu.Lock()
	s.counts[k]++
	s.requests = append(s.requests, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body})
	var queued *Response
	if q := s.queue[k]; len(q) > 0 {
		queued = &q[0]
		s.queue[k] = q[1:]
	}
	handler := s.handlers[k]
	token := s.token
	s.mu.Unlock()

	if queued != nil {
		writeResponse(w, *queued)
		return
	}
	if handler != nil {
		handler(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/images/") || strings.HasPrefix(r.URL.Path, "/img/") {
		s.serveBlob(w, r)
		return
	}
	if r.Header.Get("X-Figma-Token") != token {
		writeJSON(w, http.StatusForbidden, map[string]any{"status": 403, "err": "Invalid token"})
		return
	}
	s.route(w, r, body)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request, body []byte) {
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	q := r.URL.Query()
	get := r.Method == http.MethodGet
	switch {
	case get && match(segs, "v1", "me"):
		s.fixture(w, "me.json")
	case get && match(segs, "v1", "files", "*"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.serveFile(w, q)
	case get && match(segs, "v1", "files", "*", "nodes"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.serveNodes(w, q)
	case get && match(segs, "v1", "files", "*", "meta"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.fixture(w, "meta.json")
	case get && match(segs, "v1", "files", "*", "images"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.fixtureRewritten(w, "image_fills.json")
	case get && match(segs, "v1", "images", "*"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.serveImages(w, q)
	case get && match(segs, "v1", "files", "*", "variables", "local"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.fixture(w, "variables_local.json")
	case get && match(segs, "v1", "files", "*", "variables", "published"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.fixture(w, "variables_published.json")
	case get && match(segs, "v1", "files", "*", "components"), get && match(segs, "v1", "files", "*", "component_sets"), get && match(segs, "v1", "files", "*", "styles"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.fixture(w, segs[3]+".json")
	case get && match(segs, "v1", "teams", "*", "components"), get && match(segs, "v1", "teams", "*", "component_sets"), get && match(segs, "v1", "teams", "*", "styles"):
		if !s.knownTeam(w, segs[2]) {
			return
		}
		s.serveTeamLibrary(w, segs[3]+".json", segs[3], q)
	case get && match(segs, "v1", "components", "*"):
		s.serveByKey(w, "components.json", "components", segs[2], "component")
	case get && match(segs, "v1", "component_sets", "*"):
		s.serveByKey(w, "component_sets.json", "component_sets", segs[2], "component set")
	case get && match(segs, "v1", "styles", "*"):
		s.serveByKey(w, "styles.json", "styles", segs[2], "style")
	case get && match(segs, "v1", "files", "*", "versions"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.serveVersions(w, q)
	case get && match(segs, "v1", "files", "*", "comments"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.fixture(w, "comments.json")
	case r.Method == http.MethodPost && match(segs, "v1", "files", "*", "comments"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.postComment(w, body)
	case get && match(segs, "v1", "files", "*", "dev_resources"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.serveDevResources(w, q)
	case r.Method == http.MethodPost && match(segs, "v1", "dev_resources"):
		s.createDevResources(w, body)
	case r.Method == http.MethodPut && match(segs, "v1", "dev_resources"):
		s.updateDevResources(w, body)
	case r.Method == http.MethodDelete && match(segs, "v1", "files", "*", "dev_resources", "*"):
		if !s.knownFile(w, segs[2]) {
			return
		}
		s.deleteDevResource(w, segs[4])
	case get && match(segs, "v1", "teams", "*", "projects"):
		if !s.knownTeam(w, segs[2]) {
			return
		}
		s.fixture(w, "projects.json")
	case get && match(segs, "v1", "projects", "*", "files"):
		if segs[2] != ProjectID {
			notFound(w, "Not found")
			return
		}
		s.fixture(w, "project_files.json")
	case get && match(segs, "v2", "teams", "*", "folders"):
		if !s.knownTeam(w, segs[2]) {
			return
		}
		s.fixture(w, "folders.json")
	case get && match(segs, "v2", "folders", "*", "folders"), get && match(segs, "v2", "folders", "*", "files"), get && match(segs, "v2", "folders", "*", "meta"):
		if segs[2] != FolderID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": true, "status": 404, "message": "Folder not found"})
			return
		}
		s.fixture(w, "folder_"+segs[3]+".json")
	default:
		notFound(w, "Not found")
	}
}

func match(segs []string, pattern ...string) bool {
	if len(segs) != len(pattern) {
		return false
	}
	for i, p := range pattern {
		if p != "*" && segs[i] != p {
			return false
		}
	}
	return true
}

func (s *Server) knownFile(w http.ResponseWriter, k string) bool {
	if k == FileKey || k == BranchKey {
		return true
	}
	notFound(w, "Not found")
	return false
}

func (s *Server) knownTeam(w http.ResponseWriter, id string) bool {
	if id == TeamID {
		return true
	}
	notFound(w, "Team not found")
	return false
}

func notFound(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"status": 404, "err": msg})
}

func (s *Server) fixture(w http.ResponseWriter, name string) {
	raw, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": 500, "err": "missing fixture " + name})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw) //nolint:gosec // embedded test fixture served to tests only
}

func (s *Server) fixtureRewritten(w http.ResponseWriter, name string) {
	raw, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": 500, "err": "missing fixture " + name})
		return
	}
	raw = bytes.ReplaceAll(raw, []byte(CDNHost), []byte(s.URL))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw) //nolint:gosec // embedded test fixture served to tests only
}

func (s *Server) decodeFixture(name string) map[string]any {
	raw, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	return m
}

func (s *Server) serveFile(w http.ResponseWriter, q url.Values) {
	file := s.decodeFixture("file.json")
	if q.Get("branch_data") != "true" {
		delete(file, "branches")
	}
	if d := q.Get("depth"); d != "" {
		depth, err := strconv.Atoi(d)
		if err != nil || depth < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "err": "Invalid depth"})
			return
		}
		prune(file["document"].(map[string]any), depth)
	}
	writeJSON(w, http.StatusOK, file)
}

func (s *Server) serveNodes(w http.ResponseWriter, q url.Values) {
	idsParam := q.Get("ids")
	if idsParam == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "err": "Invalid ids: must be a comma-separated list of node IDs"})
		return
	}
	file := s.decodeFixture("file.json")
	styleNodes := s.decodeFixture("nodes.json")["nodes"].(map[string]any)
	depth := 0
	if d := q.Get("depth"); d != "" {
		depth, _ = strconv.Atoi(d)
	}
	nodes := map[string]any{}
	for _, id := range strings.Split(idsParam, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		node := findNode(file["document"].(map[string]any), id)
		if node == nil {
			if entry, ok := styleNodes[id]; ok {
				nodes[id] = entry
			} else {
				nodes[id] = nil
			}
			continue
		}
		if depth > 0 {
			prune(node, depth)
		}
		nodes[id] = nodeEntry(file, node)
	}
	out := map[string]any{
		"name":         file["name"],
		"role":         file["role"],
		"lastModified": file["lastModified"],
		"editorType":   file["editorType"],
		"thumbnailUrl": file["thumbnailUrl"],
		"version":      file["version"],
		"nodes":        nodes,
	}
	writeJSON(w, http.StatusOK, out)
}

// nodeEntry wraps a node with the components, component sets, and styles
// its subtree references, as the real endpoint does.
func nodeEntry(file map[string]any, node map[string]any) map[string]any {
	components := map[string]any{}
	componentSets := map[string]any{}
	styles := map[string]any{}
	allComponents := file["components"].(map[string]any)
	allSets := file["componentSets"].(map[string]any)
	allStyles := file["styles"].(map[string]any)
	walk(node, func(n map[string]any) {
		if id, ok := n["componentId"].(string); ok {
			if c, ok := allComponents[id]; ok {
				components[id] = c
				if setID, ok := c.(map[string]any)["componentSetId"].(string); ok {
					if set, ok := allSets[setID]; ok {
						componentSets[setID] = set
					}
				}
			}
		}
		if refs, ok := n["styles"].(map[string]any); ok {
			for _, v := range refs {
				if id, ok := v.(string); ok {
					if st, ok := allStyles[id]; ok {
						styles[id] = st
					}
				}
			}
		}
	})
	return map[string]any{
		"document":      node,
		"components":    components,
		"componentSets": componentSets,
		"schemaVersion": file["schemaVersion"],
		"styles":        styles,
	}
}

func walk(node map[string]any, fn func(map[string]any)) {
	fn(node)
	children, _ := node["children"].([]any)
	for _, c := range children {
		if m, ok := c.(map[string]any); ok {
			walk(m, fn)
		}
	}
}

func findNode(node map[string]any, id string) map[string]any {
	if node["id"] == id {
		return node
	}
	children, _ := node["children"].([]any)
	for _, c := range children {
		if m, ok := c.(map[string]any); ok {
			if found := findNode(m, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// prune keeps depth levels of children below node.
func prune(node map[string]any, depth int) {
	children, ok := node["children"].([]any)
	if !ok {
		return
	}
	if depth <= 0 {
		delete(node, "children")
		return
	}
	for _, c := range children {
		if m, ok := c.(map[string]any); ok {
			prune(m, depth-1)
		}
	}
}

func (s *Server) serveImages(w http.ResponseWriter, q url.Values) {
	idsParam := q.Get("ids")
	if idsParam == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "err": "Invalid ids: must be a comma-separated list of node IDs"})
		return
	}
	format := q.Get("format")
	if format == "" {
		format = "png"
	}
	file := s.decodeFixture("file.json")
	styleNodes := s.decodeFixture("nodes.json")["nodes"].(map[string]any)
	images := map[string]any{}
	for _, id := range strings.Split(idsParam, ",") {
		node := findNode(file["document"].(map[string]any), id)
		if node == nil {
			if _, ok := styleNodes[id]; !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "err": "Invalid node id " + id})
				return
			}
		}
		if node != nil && node["visible"] == false {
			images[id] = nil
			continue
		}
		images[id] = fmt.Sprintf("%s/images/%s.%s", s.URL, strings.ReplaceAll(id, ":", "-"), format)
	}
	writeJSON(w, http.StatusOK, map[string]any{"err": nil, "images": images})
}

func (s *Server) serveBlob(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.URL.Path)
	var content []byte
	var ctype string
	switch {
	case strings.HasSuffix(name, ".svg"):
		ctype = "image/svg+xml"
		content = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M4 11h12.17l-5.58-5.59L12 4l8 8-8 8-1.41-1.41L16.17 13H4z"/></svg>`)
	case strings.HasSuffix(name, ".pdf"):
		ctype = "application/pdf"
		content = []byte("%PDF-1.4\n% figmatest\n")
	default:
		ctype = "image/png"
		content = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
		content = append(content, []byte(name)...)
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content) //nolint:gosec // synthetic asset bytes served to tests only
}

func (s *Server) serveTeamLibrary(w http.ResponseWriter, name, listKey string, q url.Values) {
	data := s.decodeFixture(name)
	meta := data["meta"].(map[string]any)
	items := meta[listKey].([]any)
	pageSize := len(items)
	if ps := q.Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 && n < pageSize {
			pageSize = n
		}
	}
	start := 0
	if after := q.Get("after"); after != "" {
		if n, err := strconv.Atoi(after); err == nil {
			start = n
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	meta[listKey] = items[start:end]
	cursor := map[string]any{}
	if end < len(items) {
		cursor["after"] = end
	}
	if start > 0 {
		cursor["before"] = start
	}
	meta["cursor"] = cursor
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) serveByKey(w http.ResponseWriter, name, listKey, k, what string) {
	data := s.decodeFixture(name)
	for _, item := range data["meta"].(map[string]any)[listKey].([]any) {
		m := item.(map[string]any)
		if m["key"] == k {
			writeJSON(w, http.StatusOK, map[string]any{"status": 200, "error": false, "meta": m})
			return
		}
	}
	notFound(w, "No "+what+" with key "+k)
}

func (s *Server) serveVersions(w http.ResponseWriter, q url.Values) {
	data := s.decodeFixture("versions.json")
	versions := data["versions"].([]any)
	before := q.Get("before")
	var kept []any
	for _, v := range versions {
		id := v.(map[string]any)["id"].(string)
		if before != "" && id >= before {
			continue
		}
		kept = append(kept, v)
	}
	pageSize := len(kept)
	if ps := q.Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 && n < pageSize {
			pageSize = n
		}
	}
	pagination := map[string]any{}
	if pageSize < len(kept) {
		last := kept[pageSize-1].(map[string]any)["id"].(string)
		pagination["next_page"] = fmt.Sprintf("https://api.figma.com/v1/files/%s/versions?before=%s&page_size=%d", FileKey, last, pageSize)
		kept = kept[:pageSize]
	}
	if kept == nil {
		kept = []any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": kept, "pagination": pagination})
}

func (s *Server) postComment(w http.ResponseWriter, body []byte) {
	var in struct {
		Message    string          `json:"message"`
		CommentID  string          `json:"comment_id"`
		ClientMeta json.RawMessage `json:"client_meta"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": true, "status": 400, "message": "message is required"})
		return
	}
	me := s.decodeFixture("me.json")
	delete(me, "email")
	var clientMeta any
	if len(in.ClientMeta) > 0 {
		_ = json.Unmarshal(in.ClientMeta, &clientMeta)
	}
	out := map[string]any{
		"id":          "9100",
		"client_meta": clientMeta,
		"file_key":    FileKey,
		"parent_id":   in.CommentID,
		"user":        me,
		"created_at":  "2026-09-07T12:00:00Z",
		"resolved_at": nil,
		"message":     in.Message,
		"order_id":    "3",
		"reactions":   []any{},
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) serveDevResources(w http.ResponseWriter, q url.Values) {
	data := s.decodeFixture("dev_resources.json")
	filter := q.Get("node_ids")
	if filter == "" {
		writeJSON(w, http.StatusOK, data)
		return
	}
	wanted := map[string]bool{}
	for _, id := range strings.Split(filter, ",") {
		wanted[strings.TrimSpace(id)] = true
	}
	kept := []any{}
	for _, item := range data["dev_resources"].([]any) {
		if wanted[item.(map[string]any)["node_id"].(string)] {
			kept = append(kept, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"dev_resources": kept})
}

func writeResponse(w http.ResponseWriter, resp Response) {
	for k, v := range resp.Header {
		w.Header().Set(k, v)
	}
	switch body := resp.Body.(type) {
	case nil:
		w.WriteHeader(resp.Status)
	case []byte:
		w.WriteHeader(resp.Status)
		_, _ = w.Write(body)
	case string:
		w.WriteHeader(resp.Status)
		_, _ = io.WriteString(w, body)
	default:
		writeJSON(w, resp.Status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// NodeIDs returns every node id in the fixture document in document order.
func NodeIDs(tb testing.TB) []string {
	tb.Helper()
	var file map[string]any
	Decode(tb, "file.json", &file)
	var ids []string
	walk(file["document"].(map[string]any), func(n map[string]any) {
		ids = append(ids, n["id"].(string))
	})
	return ids
}

// SortedKeys returns the sorted keys of a map, for stable test output.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// createDevResources answers POST /v1/dev_resources. It mirrors the real
// endpoint's partial success: a 200 whose errors array carries the links that
// were rejected. A node here accepts at most 10 links and refuses a duplicate
// URL, which is what Figma does.
func (s *Server) createDevResources(w http.ResponseWriter, body []byte) {
	var req struct {
		DevResources []struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			FileKey string `json:"file_key"`
			NodeID  string `json:"node_id"`
		} `json:"dev_resources"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.DevResources) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "err": "Invalid dev_resources"})
		return
	}
	created := []any{}
	errs := []any{}
	for i, d := range req.DevResources {
		switch {
		case d.FileKey != FileKey && d.FileKey != BranchKey:
			errs = append(errs, map[string]any{"file_key": d.FileKey, "node_id": d.NodeID, "error": "File not found"})
		case strings.Contains(d.URL, "duplicate"):
			errs = append(errs, map[string]any{"file_key": d.FileKey, "node_id": d.NodeID, "error": "Another dev resource for the node has the same url"})
		case strings.Contains(d.NodeID, "9:99"):
			errs = append(errs, map[string]any{"file_key": d.FileKey, "node_id": d.NodeID, "error": "The node already has the maximum of 10 dev resources"})
		default:
			created = append(created, map[string]any{
				"id":       fmt.Sprintf("dr-new-%d", i+1),
				"name":     d.Name,
				"url":      d.URL,
				"file_key": d.FileKey,
				"node_id":  d.NodeID,
			})
		}
	}
	out := map[string]any{"links_created": created}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	writeJSON(w, http.StatusOK, out)
}

// updateDevResources answers PUT /v1/dev_resources with the same partial
// success shape as the create endpoint.
func (s *Server) updateDevResources(w http.ResponseWriter, body []byte) {
	var req struct {
		DevResources []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"dev_resources"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.DevResources) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "err": "Invalid dev_resources"})
		return
	}
	updated := []any{}
	errs := []any{}
	for _, d := range req.DevResources {
		if strings.HasPrefix(d.ID, "missing") {
			errs = append(errs, map[string]any{"id": d.ID, "error": "Dev resource not found"})
			continue
		}
		updated = append(updated, map[string]any{
			"id":       d.ID,
			"name":     d.Name,
			"url":      d.URL,
			"file_key": FileKey,
			"node_id":  "3:11",
		})
	}
	out := map[string]any{"links_updated": updated}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteDevResource answers DELETE /v1/files/:key/dev_resources/:id.
func (s *Server) deleteDevResource(w http.ResponseWriter, id string) {
	if strings.HasPrefix(id, "missing") {
		writeJSON(w, http.StatusNotFound, map[string]any{"status": 404, "err": "Dev resource not found"})
		return
	}
	w.WriteHeader(http.StatusOK)
}
