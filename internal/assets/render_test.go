package assets

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func imagesPath() string { return "/v1/images/" + figmatest.FileKey }

func newClient(t *testing.T, api *figmatest.Server, transport http.RoundTripper) *figma.Client {
	t.Helper()
	return figma.New(figma.Options{Token: figmatest.Token, BaseURL: api.URL, Transport: transport, Timeout: 10 * time.Second})
}

func newCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.Open(cache.Options{Root: t.TempDir(), Profile: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRenderGroupsCachesAndFails(t *testing.T) {
	api := figmatest.NewServer(t)
	client := newClient(t, api, nil)
	store := newCache(t)
	out := filepath.Join(t.TempDir(), "renders")
	req := RenderRequest{
		Items: []Item{
			{NodeID: "2:2", Name: "Login", Format: "png", Scale: 2},
			{NodeID: "2:6", Name: "Button/Primary", Format: "png", Scale: 2},
			{NodeID: "2:9", Name: "icon/arrow-right", Format: "svg", Scale: 1},
			{NodeID: "2:9", Name: "icon/arrow-right", Format: "png", Scale: 2, Suffix: "@2x"},
			{NodeID: "2:13", Name: "Debug", Format: "png", Scale: 2},
		},
		OutDir: out,
	}
	res, err := Render(context.Background(), client, store, figmatest.FileKey, figmatest.Version, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Requests != 2 || api.Count(http.MethodGet, imagesPath()) != 2 {
		t.Fatalf("expected one images request per (format, scale), got %d (server %d)", res.Requests, api.Count(http.MethodGet, imagesPath()))
	}
	for _, r := range api.Requests() {
		if r.Path != imagesPath() {
			continue
		}
		ids := r.Query.Get("ids")
		switch r.Query.Get("format") {
		case "png":
			if ids != "2:2,2:6,2:9,2:13" || r.Query.Get("scale") != "2" || r.Query.Get("version") != figmatest.Version {
				t.Fatalf("png request: %v", r.Query)
			}
		case "svg":
			if ids != "2:9" || r.Query.Get("scale") != "1" {
				t.Fatalf("svg request: %v", r.Query)
			}
		default:
			t.Fatalf("unexpected format: %v", r.Query)
		}
	}
	if res.Downloaded != 4 || res.Failed != 1 || res.Cached != 0 || len(res.Items) != 5 {
		t.Fatalf("counts: %+v", res)
	}
	wantPaths := []string{"login@2x.png", "button-primary@2x.png", "icon-arrow-right.svg", "icon-arrow-right@2x.png", "debug@2x.png"}
	for i, want := range wantPaths {
		if res.Items[i].Path != filepath.Join(out, want) {
			t.Fatalf("item %d path = %s, want %s", i, res.Items[i].Path, want)
		}
	}
	for _, r := range res.Items[:4] {
		data, err := os.ReadFile(r.Path)
		if err != nil || int64(len(data)) != r.Bytes || r.Bytes == 0 || r.Cached || r.Error != "" {
			t.Fatalf("item %+v: %v", r, err)
		}
	}
	if !strings.HasPrefix(string(mustRead(t, res.Items[2].Path)), "<svg") || string(mustRead(t, res.Items[0].Path)[:4]) != "\x89PNG" {
		t.Fatal("content by format")
	}
	failed := res.Items[4]
	if failed.Bytes != 0 || !strings.Contains(failed.Error, "no image for node 2:13 (Debug)") || !strings.Contains(failed.Error, "visible") {
		t.Fatalf("null url: %+v", failed)
	}
	if _, err := os.Stat(failed.Path); !os.IsNotExist(err) {
		t.Fatal("failed render must not leave a file")
	}
	if f := res.Failures(); len(f) != 1 || f[0].NodeID != "2:13" {
		t.Fatalf("failures = %+v", f)
	}

	// Second run of the successful items: everything is served from the
	// blob cache with zero image requests.
	api.Reset()
	out2 := filepath.Join(t.TempDir(), "again")
	req.OutDir = out2
	req.Items = req.Items[:4]
	res, err = Render(context.Background(), client, store, figmatest.FileKey, figmatest.Version, req)
	if err != nil {
		t.Fatal(err)
	}
	if api.Count(http.MethodGet, imagesPath()) != 0 || api.CountPrefix(http.MethodGet, "/images/") != 0 || res.Requests != 0 {
		t.Fatalf("second render must make no requests, got images=%d blobs=%d", api.Count(http.MethodGet, imagesPath()), api.CountPrefix(http.MethodGet, "/images/"))
	}
	if res.Cached != 4 || res.Downloaded != 0 {
		t.Fatalf("counts: %+v", res)
	}
	for _, r := range res.Items {
		if !r.Cached || r.Bytes == 0 || !strings.HasPrefix(r.Path, out2) {
			t.Fatalf("cached item: %+v", r)
		}
		if _, err := os.Stat(r.Path); err != nil {
			t.Fatal(err)
		}
	}

	// A different version or option set is a cache miss.
	api.Reset()
	yes := true
	req.SVG.IncludeID = &yes
	res, err = Render(context.Background(), client, store, figmatest.FileKey, figmatest.Version, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached != 3 || res.Downloaded != 1 || api.Count(http.MethodGet, imagesPath()) != 1 {
		t.Fatalf("svg option change should re-render the svg only: %+v", res)
	}
	for _, r := range api.Requests() {
		if r.Path == imagesPath() && (r.Query.Get("format") != "svg" || r.Query.Get("svg_include_id") != "true") {
			t.Fatalf("svg request: %v", r.Query)
		}
	}
	api.Reset()
	res, err = Render(context.Background(), client, store, figmatest.FileKey, "other-version", req)
	if err != nil || res.Cached != 0 || res.Downloaded != 4 {
		t.Fatalf("version change: %+v %v", res, err)
	}

	// Dry run assigns names without touching the network or the disk.
	api.Reset()
	req.OutDir = filepath.Join(t.TempDir(), "dry")
	req.DryRun = true
	res, err = Render(context.Background(), client, store, figmatest.FileKey, figmatest.Version, req)
	if err != nil || len(api.Requests()) != 0 || res.Items[0].Path != filepath.Join(req.OutDir, "login@2x.png") || res.Items[0].Bytes != 0 {
		t.Fatalf("dry run: %+v %v", res, err)
	}
	if _, err := os.Stat(req.OutDir); !os.IsNotExist(err) {
		t.Fatal("dry run must not create the output directory")
	}
}

func TestRenderErrors(t *testing.T) {
	api := figmatest.NewServer(t)
	client := newClient(t, api, nil)
	ctx := context.Background()
	if res, err := Render(ctx, client, nil, figmatest.FileKey, figmatest.Version, RenderRequest{}); err != nil || len(res.Items) != 0 {
		t.Fatalf("empty request: %+v %v", res, err)
	}
	_, err := Render(ctx, client, nil, figmatest.FileKey, figmatest.Version, RenderRequest{Items: []Item{{NodeID: "2:2", Format: "gif", Scale: 1}}})
	if figctl.From(err).Code != figctl.CodeUsage {
		t.Fatalf("bad format: %v", err)
	}
	_, err = Render(ctx, client, nil, figmatest.FileKey, figmatest.Version, RenderRequest{Items: []Item{{NodeID: "2:2", Format: "png", Scale: 9}}})
	if figctl.From(err).Code != figctl.CodeUsage {
		t.Fatalf("bad scale: %v", err)
	}
	// An API failure aborts the run.
	api.Respond(http.MethodGet, imagesPath(), figmatest.Response{Status: 403, Body: map[string]any{"status": 403, "err": "Invalid token"}})
	_, err = Render(ctx, client, nil, figmatest.FileKey, figmatest.Version, RenderRequest{Items: []Item{{NodeID: "2:2", Format: "png", Scale: 1}}, OutDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error")
	}
	// A failed download is a per-item error without a cache.
	api.Reset()
	api.Respond(http.MethodGet, "/images/2-2.png", figmatest.Response{Status: 500, Body: "boom"})
	res, err := Render(ctx, client, nil, figmatest.FileKey, figmatest.Version, RenderRequest{Items: []Item{{NodeID: "2:2", Name: "Login", Format: "png", Scale: 1}, {NodeID: "2:3", Name: "Heading", Format: "png", Scale: 1}}, OutDir: t.TempDir()})
	if err != nil || res.Failed != 1 || res.Downloaded != 1 || !strings.Contains(res.Items[0].Error, "download failed") || res.Items[1].Error != "" {
		t.Fatalf("download failure: %+v %v", res, err)
	}
}

// countingTransport counts simultaneous blob downloads.
type countingTransport struct {
	base    http.RoundTripper
	active  atomic.Int32
	max     atomic.Int32
	mu      sync.Mutex
	started int
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.HasPrefix(r.URL.Path, "/images/") {
		return c.base.RoundTrip(r)
	}
	n := c.active.Add(1)
	for {
		m := c.max.Load()
		if n <= m || c.max.CompareAndSwap(m, n) {
			break
		}
	}
	c.mu.Lock()
	c.started++
	c.mu.Unlock()
	time.Sleep(15 * time.Millisecond)
	resp, err := c.base.RoundTrip(r)
	c.active.Add(-1)
	return resp, err
}

func TestRenderBoundedConcurrency(t *testing.T) {
	api := figmatest.NewServer(t)
	transport := &countingTransport{base: http.DefaultTransport}
	client := newClient(t, api, transport)
	ids := []string{"2:2", "2:3", "2:4", "2:5", "2:6", "2:7", "2:8", "2:9", "2:10", "2:11", "2:12", "2:14", "2:15", "2:16", "2:17"}
	var items []Item
	for _, id := range ids {
		items = append(items, Item{NodeID: id, Name: id, Format: "png", Scale: 1})
	}
	res, err := Render(context.Background(), client, nil, figmatest.FileKey, figmatest.Version, RenderRequest{Items: items, OutDir: t.TempDir(), Concurrency: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Downloaded != len(ids) || res.Failed != 0 || res.Requests != 1 {
		t.Fatalf("counts: %+v", res)
	}
	if transport.started != len(ids) {
		t.Fatalf("downloads = %d", transport.started)
	}
	if m := transport.max.Load(); m > 3 || m < 2 {
		t.Fatalf("max simultaneous downloads = %d, want 2..3", m)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
