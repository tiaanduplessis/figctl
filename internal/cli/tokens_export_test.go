package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// planDenied is the Figma response for the variables endpoint on a plan
// below Enterprise.
var planDenied = figmatest.Response{
	Status: http.StatusForbidden,
	Body:   map[string]any{"status": 403, "err": "Variables are only available on the Enterprise plan"},
}

func exportData(t *testing.T, r result) map[string]any {
	t.Helper()
	return data(t, r, "tokens.export")
}

func TestTokensExportDTCGGolden(t *testing.T) {
	setup(t)
	r := execute(t, "", "tokens", "export", figmatest.FileKey)
	ok(t, r)
	checkGolden(t, "tokens_export_dtcg.golden", r.stdout)

	d := exportData(t, r)
	if d["format"] != "dtcg" {
		t.Fatalf("format = %v", d["format"])
	}
	summary := d["summary"].(map[string]any)
	if summary["tokens"] != float64(12) || summary["variables"] != float64(8) || summary["styles"] != float64(4) {
		t.Fatalf("summary: %v", summary)
	}
	tokens := d["tokens"].(map[string]any)
	surface := tokens["bg"].(map[string]any)["surface"].(map[string]any)
	if surface["$value"] != "{neutral.0}" || surface["$type"] != "color" {
		t.Fatalf("bg.surface: %v", surface)
	}
	modes := surface["$extensions"].(map[string]any)["com.figma"].(map[string]any)["modes"].(map[string]any)
	if modes["Dark"] != "#111827" || modes["Light"] != "#ffffff" {
		t.Fatalf("modes: %v", modes)
	}
}

func TestTokensExportStyleDictionaryMatchesDTCG(t *testing.T) {
	setup(t)
	dtcg := execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "dtcg", "--out", "dtcg.json")
	ok(t, dtcg)
	sd := execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "style-dictionary", "--out", "sd.json")
	ok(t, sd)
	a, err := os.ReadFile(writtenPath(t, dtcg))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(writtenPath(t, sd))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("style-dictionary output should be byte identical to dtcg")
	}
}

func TestTokensExportCSS(t *testing.T) {
	setup(t)
	r := execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "css")
	ok(t, r)
	d := exportData(t, r)
	if d["format"] != "css" || d["tokens"] != nil {
		t.Fatalf("css should use data.content only: %v", d)
	}
	css := d["content"].(string)
	for _, want := range []string{
		"/* Design tokens from Fixture Design System (version 2100123456)",
		":root {",
		"--color-brand-500: #3366ff;",
		"--space-4: 16px;",
		"--bg-surface: var(--neutral-0);",
		"--heading-lg-font-size: 28px;",
		"--shadow-md: 0px 4px 12px rgba(0, 0, 0, 0.1);",
		"[data-theme=\"dark\"] {",
		"--text-primary: #ffffff;",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("css is missing %q:\n%s", want, css)
		}
	}

	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "css", "--mode-selector", ".dark-<mode>")
	ok(t, r)
	if !strings.Contains(exportData(t, r)["content"].(string), ".dark-dark {") {
		t.Fatal("--mode-selector should be honored")
	}
}

func TestTokensExportTailwind(t *testing.T) {
	setup(t)
	r := execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "tailwind")
	ok(t, r)
	js := exportData(t, r)["content"].(string)
	for _, want := range []string{"const theme = {", "export default theme;", `"colors"`, `"spacing"`, `"boxShadow"`, `"extend"`} {
		if !strings.Contains(js, want) {
			t.Fatalf("tailwind is missing %q:\n%s", want, js)
		}
	}

	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "tailwind", "--tailwind-format", "cjs", "--tailwind-vars")
	ok(t, r)
	js = exportData(t, r)["content"].(string)
	if !strings.Contains(js, "module.exports = theme;") || !strings.Contains(js, "var(--color-brand-500)") {
		t.Fatalf("cjs with vars:\n%s", js)
	}

	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "tailwind", "--tailwind-format", "json")
	ok(t, r)
	var theme map[string]any
	if err := json.Unmarshal([]byte(exportData(t, r)["content"].(string)), &theme); err != nil {
		t.Fatalf("tailwind json is not JSON: %v", err)
	}
	if theme["colors"].(map[string]any)["brand"].(map[string]any)["500"] != "#3366ff" {
		t.Fatalf("theme: %v", theme)
	}
}

func TestTokensExportOut(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	r := execute(t, "", "tokens", "export", figmatest.FileKey, "--out", dir)
	ok(t, r)
	d := exportData(t, r)
	if d["tokens"] != nil || d["content"] != nil {
		t.Fatalf("--out should return a manifest only: %v", d)
	}
	files := d["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	file := files[0].(map[string]any)
	want := filepath.Join(dir, "tokens.json")
	if file["path"] != want || file["tokens"] != float64(12) {
		t.Fatalf("manifest: %v", file)
	}
	raw, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if int(file["bytes"].(float64)) != len(raw) {
		t.Fatalf("bytes = %v, file is %d", file["bytes"], len(raw))
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("written file is not JSON: %v", err)
	}
	if tree["space"].(map[string]any)["4"].(map[string]any)["$type"] != "dimension" {
		t.Fatalf("written tree: %v", tree["space"])
	}

	// One file per mode, and the manifest names the mode.
	modeDir := t.TempDir()
	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--mode-strategy", "separate", "--out", modeDir)
	ok(t, r)
	files = exportData(t, r)["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("expected one file per mode: %v", files)
	}
	for _, name := range []string{"tokens.dark.json", "tokens.light.json"} {
		if _, err := os.Stat(filepath.Join(modeDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if files[0].(map[string]any)["mode"] != "Dark" {
		t.Fatalf("manifest mode: %v", files[0])
	}

	// A file target only works when the export is a single document.
	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "css", "--out", filepath.Join(dir, "nested", "tokens.css"))
	ok(t, r)
	if _, err := os.Stat(filepath.Join(dir, "nested", "tokens.css")); err != nil {
		t.Fatalf("css file: %v", err)
	}
	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--mode-strategy", "separate", "--out", filepath.Join(dir, "one.json"))
	if r.code != figctl.ExitUsage || errorCode(t, r) != string(figctl.CodeUsage) {
		t.Fatalf("a file target with several documents should be a usage error: %d %s", r.code, r.stdout)
	}
}

func TestTokensExportVariablesUnavailable(t *testing.T) {
	api := setup(t)
	api.Respond(http.MethodGet, fileKeyPath("/variables/local"), planDenied)
	r := execute(t, "", "tokens", "export", figmatest.FileKey)
	ok(t, r)
	d := exportData(t, r)
	summary := d["summary"].(map[string]any)
	if summary["variables"] != float64(0) || summary["styles"] != float64(4) {
		t.Fatalf("styles only export: %v", summary)
	}
	collections := summary["collections"].([]any)
	if len(collections) != 1 || collections[0].(map[string]any)["name"] != "Styles" {
		t.Fatalf("collections: %v", collections)
	}
	tokens := d["tokens"].(map[string]any)
	if tokens["color"].(map[string]any)["brand"].(map[string]any)["500"].(map[string]any)["$value"] != "#3366ff" {
		t.Fatalf("styles should still resolve: %v", tokens)
	}
	if _, present := tokens["brand"]; present {
		t.Fatalf("no variables should be exported: %v", tokens)
	}
	env := decodeEnvelope(t, r.stdout)
	hints := env["hints"].([]any)
	found := false
	for _, h := range hints {
		if strings.Contains(h.(string), "Enterprise plan") {
			found = true
		}
	}
	if !found {
		t.Fatalf("hints should explain the missing variables: %v", hints)
	}
}

func TestTokensExportModeSelection(t *testing.T) {
	setup(t)
	// --mode switches the strategy on its own.
	r := execute(t, "", "tokens", "export", figmatest.FileKey, "--mode", "Dark")
	ok(t, r)
	tokens := exportData(t, r)["tokens"].(map[string]any)
	if tokens["bg"].(map[string]any)["surface"].(map[string]any)["$value"] != "{neutral.900}" {
		t.Fatalf("dark mode: %v", tokens["bg"])
	}

	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--mode", "Midnight")
	if r.code != figctl.ExitUsage {
		t.Fatalf("an unknown mode should be a usage error: %d", r.code)
	}
	env := decodeEnvelope(t, r.stdout)
	if hint := env["error"].(map[string]any)["hint"].(string); !strings.Contains(hint, "Dark") {
		t.Fatalf("hint should list the modes: %s", hint)
	}
}

func TestTokensExportFilters(t *testing.T) {
	setup(t)
	r := execute(t, "", "tokens", "export", figmatest.FileKey, "--include", "variables", "--collection", "Primitives", "--exclude-hidden")
	ok(t, r)
	summary := exportData(t, r)["summary"].(map[string]any)
	if summary["tokens"] != float64(5) || summary["styles"] != float64(0) {
		t.Fatalf("filters: %v", summary)
	}

	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--name-case", "snake")
	ok(t, r)
	names := exportData(t, r)["names"].([]any)
	found := false
	for _, n := range names {
		if n.(map[string]any)["name"] == "feature.show_helper" {
			found = true
		}
	}
	if !found {
		t.Fatalf("--name-case snake: %v", names)
	}
}

func TestTokensExportUsageErrors(t *testing.T) {
	setup(t)
	for _, args := range [][]string{
		{"tokens", "export", figmatest.FileKey, "--format", "scss"},
		{"tokens", "export", figmatest.FileKey, "--name-case", "pascal"},
		{"tokens", "export", figmatest.FileKey, "--mode-strategy", "every"},
		{"tokens", "export", figmatest.FileKey, "--tailwind-format", "umd"},
		{"tokens", "export", figmatest.FileKey, "--include", "colours"},
	} {
		r := execute(t, "", args...)
		if r.code != figctl.ExitUsage || errorCode(t, r) != string(figctl.CodeUsage) {
			t.Fatalf("%v: exit = %d, stdout = %s", args, r.code, r.stdout)
		}
	}
}

func TestTokensExportTableAndMarkdown(t *testing.T) {
	setup(t)
	r := execute(t, "", "tokens", "export", figmatest.FileKey, "-o", "table")
	ok(t, r)
	for _, want := range []string{"collection", "Primitives", "Semantic", "Light, Dark", "Styles", "total", "token       brand.500"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("table is missing %q:\n%s", want, r.stdout)
		}
	}

	r = execute(t, "", "tokens", "export", figmatest.FileKey, "--format", "css", "-o", "md")
	ok(t, r)
	for _, want := range []string{"# tokens.export", "| collection | tokens | modes |", "Tokens (first", "- brand.500 (color)", "```css", ":root {"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("markdown is missing %q:\n%s", want, r.stdout)
		}
	}
}

// writtenPath reads the single written path out of an export manifest.
func writtenPath(t *testing.T, r result) string {
	t.Helper()
	files := exportData(t, r)["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("expected one file, got %v", files)
	}
	return files[0].(map[string]any)["path"].(string)
}
