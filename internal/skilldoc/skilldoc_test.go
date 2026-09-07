package skilldoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// templateDir is the directory the generated documents are committed to.
const templateDir = "../skill/templates"

// TestGeneratedDocumentsAreUpToDate fails when the committed reference
// files no longer match the command tree and the schema registry. Run
// "make gen" to refresh them.
func TestGeneratedDocumentsAreUpToDate(t *testing.T) {
	for rel, want := range Documents() {
		path := filepath.Join(templateDir, filepath.FromSlash(rel))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s is out of date; run make gen", rel)
		}
	}
}

func TestCommandsReferenceCoversTheSurface(t *testing.T) {
	doc := Commands()
	for _, want := range []string{
		"### figctl file tree",
		"### figctl node context",
		"### figctl node inspect",
		"### figctl render",
		"### figctl assets export",
		"### figctl tokens export",
		"### figctl components list",
		"### figctl skill install",
		"### figctl schema",
		"## Global flags",
		"| `--profile string` |",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("command reference is missing %q", want)
		}
	}
	if strings.Contains(doc, "### figctl help") {
		t.Error("command reference should skip the help command")
	}
	// Hidden flags stay out of the reference.
	if strings.Contains(doc, "--cache-ttl") {
		t.Error("command reference should skip hidden flags")
	}
}

func TestOutputSchemasCoverTheEnvelopeAndPayloads(t *testing.T) {
	doc := OutputSchemas()
	for _, want := range []string{
		"## The success envelope",
		"## The error envelope",
		"| `schemaVersion` | integer | yes |",
		"| `error.code` | string | yes |",
		"### node.context",
		"### node.inspect",
		"### file.tree",
		"### tokens.export",
		"### render",
		"### assets.export",
		"figctl schema <command>",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("output schema reference is missing %q", want)
		}
	}
}
