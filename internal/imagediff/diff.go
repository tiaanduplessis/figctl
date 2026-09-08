// Package imagediff compares local PNG screenshots and writes visual differences.
package imagediff

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/raf555/pixelmatch"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// MaxPixels bounds decoded image memory before PNG decoding allocates pixels.
const MaxPixels = 16_000_000

// Options controls color sensitivity, acceptance, and output.
type Options struct {
	Threshold    float64
	MaxDiffRatio float64
	IncludeAA    bool
	Out          string
}

// Result is a completed comparison, including comparisons outside tolerance.
type Result struct {
	Expected         string  `json:"expected"`
	Actual           string  `json:"actual"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	TotalPixels      int     `json:"totalPixels"`
	MismatchedPixels int     `json:"mismatchedPixels"`
	MismatchRatio    float64 `json:"mismatchRatio"`
	Threshold        float64 `json:"threshold"`
	MaxDiffRatio     float64 `json:"maxDiffRatio"`
	IncludeAA        bool    `json:"includeAA"`
	Passed           bool    `json:"passed"`
	DiffPath         string  `json:"diffPath,omitempty"`
}

// Compare reads two PNGs of equal dimensions. Out is optional; when supplied,
// it is written through a temporary file after comparison, even when it fails
// the acceptance limit. Input files cannot be used as the output destination.
func Compare(expected, actual string, opts Options) (*Result, error) {
	for _, v := range []struct {
		name  string
		value float64
	}{
		{"threshold", opts.Threshold}, {"max-diff-ratio", opts.MaxDiffRatio},
	} {
		if math.IsNaN(v.value) || math.IsInf(v.value, 0) || v.value < 0 || v.value > 1 {
			return nil, figctl.Newf(figctl.CodeUsage, "--%s must be a finite number between 0 and 1", v.name)
		}
	}
	a, ac, err := openPNG(expected)
	if err != nil {
		return nil, err
	}
	defer func() { _ = a.Close() }()
	b, bc, err := openPNG(actual)
	if err != nil {
		return nil, err
	}
	defer func() { _ = b.Close() }()
	if ac.Width != bc.Width || ac.Height != bc.Height {
		return nil, figctl.Newf(figctl.CodeUsage, "image dimensions differ: expected %dx%d, actual %dx%d", ac.Width, ac.Height, bc.Width, bc.Height).
			WithHint("Capture the same viewport and crop at the same scale. figctl render defaults to 2x; use --scale 1 for a 1x screenshot.")
	}
	out := ""
	if opts.Out != "" {
		out, err = filepath.Abs(opts.Out)
		if err != nil {
			return nil, ioError(err)
		}
		if err := checkOutput(out, a, b); err != nil {
			return nil, err
		}
	}
	ai, err := png.Decode(a)
	if err != nil {
		return nil, invalidPNG(expected, err)
	}
	bi, err := png.Decode(b)
	if err != nil {
		return nil, invalidPNG(actual, err)
	}
	options := []pixelmatch.Option{pixelmatch.WithThreshold(opts.Threshold), pixelmatch.WithIncludeAA(opts.IncludeAA)}
	var diff *image.NRGBA
	if out != "" {
		diff = image.NewNRGBA(image.Rect(0, 0, ac.Width, ac.Height))
		options = append(options, pixelmatch.WithOutput(diff))
	}
	n, err := pixelmatch.Compare(ai, bi, options...)
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "comparing PNG images: "+err.Error())
	}
	if out != "" {
		if err := writePNG(out, diff); err != nil {
			return nil, ioError(err)
		}
	}
	expectedPath, err := filepath.Abs(expected)
	if err != nil {
		return nil, ioError(err)
	}
	actualPath, err := filepath.Abs(actual)
	if err != nil {
		return nil, ioError(err)
	}
	total := ac.Width * ac.Height
	ratio := float64(n) / float64(total)
	return &Result{Expected: expectedPath, Actual: actualPath, Width: ac.Width, Height: ac.Height,
		TotalPixels: total, MismatchedPixels: n, MismatchRatio: ratio, Threshold: opts.Threshold,
		MaxDiffRatio: opts.MaxDiffRatio, IncludeAA: opts.IncludeAA, Passed: ratio <= opts.MaxDiffRatio, DiffPath: out}, nil
}

func openPNG(path string) (*os.File, image.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, image.Config{}, ioError(err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, image.Config{}, ioError(err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, image.Config{}, figctl.Newf(figctl.CodeUsage, "%s is not a regular PNG file", path)
	}
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		_ = f.Close()
		return nil, cfg, invalidPNG(path, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxPixels/cfg.Height {
		_ = f.Close()
		return nil, cfg, figctl.Newf(figctl.CodeUsage, "%s exceeds the %d pixel limit", path, MaxPixels).WithHint("Capture a smaller region or reduce the render scale.")
	}
	if _, err := f.Seek(0, 0); err != nil {
		_ = f.Close()
		return nil, cfg, ioError(err)
	}
	return f, cfg, nil
}

func invalidPNG(path string, err error) error {
	return figctl.Wrap(figctl.CodeUsage, err, fmt.Sprintf("invalid PNG %s: %v", path, err))
}

func ioError(err error) error {
	code := figctl.CodeImageIO
	if os.IsNotExist(err) {
		code = figctl.CodeNotFound
	}
	return figctl.Wrap(code, err, err.Error())
}

func checkOutput(path string, inputs ...*os.File) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return ioError(err)
	}
	if !info.Mode().IsRegular() {
		return figctl.Newf(figctl.CodeUsage, "output %s is not a regular file", path)
	}
	for _, input := range inputs {
		src, err := input.Stat()
		if err != nil {
			return ioError(err)
		}
		if os.SameFile(info, src) {
			return figctl.New(figctl.CodeUsage, "diff output must not overwrite either input image")
		}
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".figctl-diff-*.png")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
