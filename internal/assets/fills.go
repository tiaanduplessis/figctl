package assets

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// fillFormat is the blob cache format of a downloaded image fill.
const fillFormat = "fill"

// FillResult is the manifest entry of one downloaded image fill.
type FillResult struct {
	ImageRef    string `json:"imageRef"`
	Path        string `json:"path,omitempty"`
	Bytes       int64  `json:"bytes"`
	Cached      bool   `json:"cached"`
	ContentType string `json:"contentType,omitempty"`
	Error       string `json:"error,omitempty"`
}

// FillOutput is the result of DownloadFills.
type FillOutput struct {
	Items []FillResult `json:"items"`
	// Requests counts image fills API calls made (0 or 1).
	Requests   int `json:"requests"`
	Downloaded int `json:"downloaded"`
	Cached     int `json:"cached"`
	Failed     int `json:"failed"`
}

// FillFileName is the deterministic filename of an image fill:
// imageref-<first 12 characters of the ref>.<ext>.
func FillFileName(ref, ext string) string {
	short := ref
	if len(short) > 12 {
		short = short[:12]
	}
	return "imageref-" + Slug(short) + "." + ext
}

// SniffExtension returns png, jpg, gif, or webp from the content, or
// "bin" when the bytes are not a known raster image.
func SniffExtension(data []byte) string {
	switch http.DetectContentType(data) {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "bin"
	}
}

// DownloadFills downloads the given image refs into outDir. It calls the
// image fills endpoint only when at least one ref is not in the blob
// cache. Refs Figma does not return become per-item errors.
func DownloadFills(ctx context.Context, client *figma.Client, store BlobStore, fileKey, version string, refs []string, outDir string, concurrency int) (*FillOutput, error) {
	out := &FillOutput{Items: make([]FillResult, len(refs))}
	if len(refs) == 0 {
		return out, nil
	}
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "creating output directory: "+err.Error())
	}
	var misses []int
	for i, ref := range refs {
		out.Items[i] = FillResult{ImageRef: ref}
		if store != nil {
			data, hit, err := store.GetBlob(fileKey, cache.BlobKey(ref, fillFormat, 1, version))
			if err != nil {
				return nil, err
			}
			if hit {
				out.Items[i].Cached = true
				if err := writeFill(&out.Items[i], outDir, data); err != nil {
					return nil, err
				}
				out.Cached++
				continue
			}
		}
		misses = append(misses, i)
	}
	if len(misses) == 0 {
		return out, nil
	}
	resp, err := client.GetImageFills(ctx, fileKey)
	if err != nil {
		return nil, err
	}
	out.Requests++

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var fatal error
	for _, i := range misses {
		res := &out.Items[i]
		u, ok := resp.Meta.Images[res.ImageRef]
		if !ok || u == "" {
			res.Error = "Figma returned no download URL for image fill " + res.ImageRef
			continue
		}
		wg.Add(1)
		go func(res *FillResult, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var buf bytes.Buffer
			if err := client.Download(ctx, u, &buf); err != nil {
				res.Error = "download failed: " + figctl.From(err).Message
				return
			}
			data := buf.Bytes()
			if store != nil {
				if err := store.PutBlob(fileKey, cache.BlobKey(res.ImageRef, fillFormat, 1, version), data); err != nil {
					mu.Lock()
					if fatal == nil {
						fatal = err
					}
					mu.Unlock()
					return
				}
			}
			if err := writeFill(res, outDir, data); err != nil {
				mu.Lock()
				if fatal == nil {
					fatal = err
				}
				mu.Unlock()
			}
		}(res, u)
	}
	wg.Wait()
	if fatal != nil {
		return nil, fatal
	}
	for _, i := range misses {
		if out.Items[i].Error != "" {
			out.Failed++
		} else {
			out.Downloaded++
		}
	}
	return out, nil
}

func writeFill(res *FillResult, outDir string, data []byte) error {
	res.ContentType = http.DetectContentType(data)
	res.Path = filepath.Join(outDir, FillFileName(res.ImageRef, SniffExtension(data)))
	if err := os.WriteFile(res.Path, data, 0o644); err != nil { //nolint:gosec // exported assets are project files
		return figctl.Wrap(figctl.CodeInternal, err, "writing "+res.Path+": "+err.Error())
	}
	res.Bytes = int64(len(data))
	return nil
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
