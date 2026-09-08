package imagediff

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

func save(t *testing.T, path string, img image.Image) {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) image.Image {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestFixtureComparison(t *testing.T) {
	a, b := "testdata/1a.png", "testdata/1b.png"
	out := filepath.Join(t.TempDir(), "diff.png")
	r, err := Compare(a, b, Options{Threshold: 0.1, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if r.MismatchedPixels != 106 || r.Passed || r.DiffPath != out {
		t.Fatalf("result: %+v", r)
	}
	got, want := read(t, out), read(t, "testdata/1diffdefaultthreshold.png")
	if got.Bounds() != want.Bounds() {
		t.Fatal("diff bounds differ")
	}
	for y := range got.Bounds().Dy() {
		for x := range got.Bounds().Dx() {
			if color.NRGBAModel.Convert(got.At(x, y)) != color.NRGBAModel.Convert(want.At(x, y)) {
				t.Fatalf("diff pixel differs at %d,%d", x, y)
			}
		}
	}
	r, err = Compare(a, b, Options{Threshold: 0.05})
	if err != nil || r.MismatchedPixels != 143 {
		t.Fatalf("sensitivity: %+v, %v", r, err)
	}
	r, err = Compare(a, b, Options{Threshold: 0.1, IncludeAA: true})
	if err != nil || r.MismatchedPixels <= 106 {
		t.Fatalf("anti-alias: %+v, %v", r, err)
	}
}

func TestPixelsAndAcceptance(t *testing.T) {
	dir := t.TempDir()
	a, b, out := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png"), filepath.Join(dir, "out.png")
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	save(t, a, img)
	r, err := Compare(a, a, Options{Threshold: 0.1, Out: out})
	if err != nil || !r.Passed || r.MismatchedPixels != 0 {
		t.Fatalf("identical: %+v %v", r, err)
	}
	if read(t, out).Bounds() != img.Bounds() {
		t.Fatal("missing identical diff")
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	save(t, b, img)
	for _, tc := range []struct {
		ratio float64
		pass  bool
	}{{0, false}, {0.249, false}, {0.25, true}, {1, true}} {
		r, err = Compare(a, b, Options{Threshold: 0.1, MaxDiffRatio: tc.ratio, Out: out})
		if err != nil || r.Passed != tc.pass || r.MismatchedPixels != 1 || r.TotalPixels != 4 || r.MismatchRatio != 0.25 {
			t.Fatalf("boundary: %+v %v", r, err)
		}
	}
	if color.NRGBAModel.Convert(read(t, out).At(0, 0)) != (color.NRGBA{R: 255, A: 255}) {
		t.Fatal("changed pixel not red")
	}
}

func TestTransparencyAndPNGModels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		a, b  image.Image
		count int
	}{
		{"transparent RGB", &image.NRGBA{Pix: []byte{255, 0, 0, 0}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)}, &image.NRGBA{Pix: []byte{0, 255, 0, 0}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)}, 0},
		{"alpha", &image.NRGBA{Pix: []byte{0, 0, 0, 128}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)}, &image.NRGBA{Pix: []byte{127, 127, 127, 255}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)}, 1},
		{"gray", image.NewGray(image.Rect(0, 0, 1, 1)), image.NewGray16(image.Rect(0, 0, 1, 1)), 0},
		{"palette", image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black}), &image.NRGBA{Pix: []byte{0, 0, 0, 255}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
			save(t, a, tc.a)
			save(t, b, tc.b)
			r, err := Compare(a, b, Options{Threshold: 0.1})
			if err != nil || r.MismatchedPixels != tc.count {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	save(t, a, image.NewGray(image.Rect(0, 0, 2, 2)))
	save(t, b, image.NewGray(image.Rect(0, 0, 3, 2)))
	for _, v := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		for _, opts := range []Options{{Threshold: v}, {MaxDiffRatio: v}} {
			if _, err := Compare(a, a, opts); err == nil || figctl.From(err).Code != figctl.CodeUsage {
				t.Fatalf("invalid %v: %v", v, err)
			}
		}
	}
	for _, tc := range []struct {
		a, b string
		opts Options
		code figctl.Code
	}{
		{a, b, Options{}, figctl.CodeUsage},
		{a, "missing.png", Options{}, figctl.CodeNotFound},
		{dir, a, Options{}, figctl.CodeUsage},
		{a, a, Options{Out: a}, figctl.CodeUsage},
		{a, a, Options{Out: dir}, figctl.CodeUsage},
		{a, a, Options{Out: filepath.Join(dir, "missing", "out.png")}, figctl.CodeNotFound},
	} {
		_, err := Compare(tc.a, tc.b, tc.opts)
		if err == nil || figctl.From(err).Code != tc.code {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	if err := os.WriteFile(b, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(a, b, Options{}); err == nil || figctl.From(err).Code != figctl.CodeUsage {
		t.Fatalf("invalid PNG: %v", err)
	}
	// A valid header with excessive dimensions must fail before pixel allocation.
	raw, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint32(raw[16:20], uint32(MaxPixels))
	binary.BigEndian.PutUint32(raw[29:33], crc32.ChecksumIEEE(raw[12:29]))
	if err := os.WriteFile(b, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(b, b, Options{}); err == nil || figctl.From(err).Code != figctl.CodeUsage {
		t.Fatalf("oversized PNG: %v", err)
	}
	// A valid header with truncated pixels must fail without touching the output.
	save(t, b, image.NewGray(image.Rect(0, 0, 2, 2)))
	raw, err = os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, raw[:33], 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.png")
	sentinel := []byte("existing output")
	if err := os.WriteFile(out, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(a, b, Options{Out: out}); err == nil {
		t.Fatal("accepted truncated PNG")
	}
	got, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatal("output changed on failure")
	}
}

func TestOutputAliases(t *testing.T) {
	for _, link := range []struct {
		name string
		fn   func(string, string) error
	}{{"symlink", os.Symlink}, {"hardlink", os.Link}} {
		t.Run(link.name, func(t *testing.T) {
			dir := t.TempDir()
			a, out := filepath.Join(dir, "a.png"), filepath.Join(dir, "out.png")
			save(t, a, image.NewGray(image.Rect(0, 0, 1, 1)))
			if err := link.fn(a, out); err != nil {
				t.Skip(err)
			}
			if _, err := Compare(a, a, Options{Out: out}); err == nil || figctl.From(err).Code != figctl.CodeUsage {
				t.Fatalf("input alias: %v", err)
			}
			read(t, a)
		})
	}
}
