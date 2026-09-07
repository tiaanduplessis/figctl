package assets

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

func fillsPath() string { return "/v1/files/" + figmatest.FileKey + "/images" }

func TestSniffExtension(t *testing.T) {
	cases := map[string][]byte{
		"png":  {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0},
		"jpg":  {0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'},
		"gif":  []byte("GIF89a\x00\x00"),
		"webp": append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 8)...),
		"bin":  []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"),
	}
	for want, data := range cases {
		if got := SniffExtension(data); got != want {
			t.Errorf("SniffExtension(%s) = %s, want %s", want, got, want)
		}
	}
	if FillFileName("img1", "png") != "imageref-img1.png" || FillFileName("0123456789abcdefXYZ", "jpg") != "imageref-0123456789ab.jpg" {
		t.Fatal("fill file names")
	}
}

func TestDownloadFills(t *testing.T) {
	api := figmatest.NewServer(t)
	client := newClient(t, api, nil)
	store := newCache(t)
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "assets")

	res, err := DownloadFills(ctx, client, store, figmatest.FileKey, figmatest.Version, nil, out, 0)
	if err != nil || len(res.Items) != 0 || res.Requests != 0 {
		t.Fatalf("no refs: %+v %v", res, err)
	}
	res, err = DownloadFills(ctx, client, store, figmatest.FileKey, figmatest.Version, []string{"img1", "missing"}, out, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Requests != 1 || api.Count(http.MethodGet, fillsPath()) != 1 || res.Downloaded != 1 || res.Failed != 1 {
		t.Fatalf("counts: %+v", res)
	}
	img := res.Items[0]
	if img.ImageRef != "img1" || img.Path != filepath.Join(out, "imageref-img1.png") || img.ContentType != "image/png" || img.Cached || img.Bytes == 0 {
		t.Fatalf("img1: %+v", img)
	}
	if data, err := os.ReadFile(img.Path); err != nil || int64(len(data)) != img.Bytes {
		t.Fatalf("file: %v", err)
	}
	if res.Items[1].Error == "" || !strings.Contains(res.Items[1].Error, "missing") || res.Items[1].Path != "" {
		t.Fatalf("missing ref: %+v", res.Items[1])
	}

	// Cached: no API call, no download.
	api.Reset()
	out2 := filepath.Join(t.TempDir(), "again")
	res, err = DownloadFills(ctx, client, store, figmatest.FileKey, figmatest.Version, []string{"img1"}, out2, 0)
	if err != nil || res.Requests != 0 || res.Cached != 1 || !res.Items[0].Cached || res.Items[0].Path != filepath.Join(out2, "imageref-img1.png") {
		t.Fatalf("cached: %+v %v", res, err)
	}
	if len(api.Requests()) != 0 {
		t.Fatalf("cached fills must not hit the server: %v", api.Requests())
	}
	if _, err := os.Stat(res.Items[0].Path); err != nil {
		t.Fatal(err)
	}

	// Without a store every run downloads; a failed download is per-item.
	api.Reset()
	api.Respond(http.MethodGet, "/img/img1", figmatest.Response{Status: 404, Body: "gone"})
	res, err = DownloadFills(ctx, client, nil, figmatest.FileKey, figmatest.Version, []string{"img1"}, out, 0)
	if err != nil || res.Failed != 1 || !strings.Contains(res.Items[0].Error, "download failed") {
		t.Fatalf("download failure: %+v %v", res, err)
	}
	api.Reset()
	api.Respond(http.MethodGet, fillsPath(), figmatest.Response{Status: 500, Body: map[string]any{"status": 500, "err": "boom"}})
	if _, err := DownloadFills(ctx, client, nil, figmatest.FileKey, figmatest.Version, []string{"img1"}, out, 0); err == nil {
		t.Fatal("API failure must abort")
	}
}
