package cli

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

func diffInputs(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "expected.png"), filepath.Join(dir, "actual.png")
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	for _, path := range []string{a, b} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		img.Pix[0], img.Pix[1], img.Pix[2] = 255, 255, 255
	}
	return a, b
}

func TestDiffContractAndExit(t *testing.T) {
	isolate(t)
	// An unusable endpoint ensures comparison needs neither API nor credentials.
	t.Setenv("FIGMA_API_BASE_URL", "http://127.0.0.1:1")
	a, b := diffInputs(t)
	out := filepath.Join(t.TempDir(), "diff.png")
	for _, tc := range []struct {
		name   string
		args   []string
		exit   int
		passed bool
	}{
		{"identical", []string{a, a}, 0, true},
		{"different", []string{a, b, "--out", out}, 7, false},
		{"accepted", []string{a, b, "--max-diff-ratio", "0.25"}, 0, true},
		{"reset flags", []string{a, b}, 7, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"diff", "--json"}, tc.args...)
			r := execute(t, "", args...)
			if r.code != tc.exit || r.stderr != "" {
				t.Fatalf("%+v", r)
			}
			d := data(t, r, "diff")
			if d["passed"] != tc.passed {
				t.Fatalf("%v", d)
			}
			env := decodeJSON(t, r.stdout)
			if env["error"] != nil || env["profile"] != nil || env["file"] != nil {
				t.Fatalf("unexpected envelope: %v", env)
			}
			validateAgainstSchema(t, "diff", envelopeTarget, "envelope", env)
			validateAgainstSchema(t, "diff", "diff", "data payload", env["data"])
		})
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestDiffOutputModes(t *testing.T) {
	isolate(t)
	a, b := diffInputs(t)
	for _, mode := range []string{"table", "md", "plain"} {
		r := execute(t, "", "diff", a, b, "-o", mode)
		if r.code != figctl.ExitImageMismatch || !strings.Contains(r.stdout, "false") || !strings.Contains(r.stdout, "0.25") || !strings.Contains(r.stderr, "exceeds") {
			t.Fatalf("%s: %+v", mode, r)
		}
	}
	r := execute(t, "", "diff", a, b, "--fields", "passed,mismatchRatio")
	d := data(t, r, "diff")
	if r.code != 7 || len(d) != 2 || d["mismatchRatio"] != 0.25 {
		t.Fatalf("%+v", r)
	}
}

func TestDiffUsage(t *testing.T) {
	isolate(t)
	a, b := diffInputs(t)
	for _, args := range [][]string{
		{}, {a}, {a, b, a}, {a, b, "--threshold", "NaN"}, {a, b, "--threshold", "+Inf"},
		{a, b, "--threshold", "-0.1"}, {a, b, "--max-diff-ratio", "1.01"},
		{a, b, "--max-diff-ratio", "NaN"}, {a, b, "--out", ""}, {a, b, "--out", a},
	} {
		r := execute(t, "", append([]string{"diff"}, args...)...)
		if r.code != 2 || errorCode(t, r) != "USAGE" {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	r := execute(t, "", "diff", a, "missing.png")
	if r.code != 4 || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("%+v", r)
	}
	r = execute(t, "", "diff", a, a, "--threshold", "0", "--include-aa")
	d := data(t, r, "diff")
	if r.code != 0 || d["threshold"] != float64(0) || d["includeAA"] != true {
		t.Fatalf("%+v", r)
	}
}
