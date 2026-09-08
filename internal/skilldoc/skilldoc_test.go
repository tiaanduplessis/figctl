package skilldoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/skill"
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

// publishedDir is where the skill is published for installers that discover
// skills by walking a repository.
const publishedDir = "../../skills/figctl"

// TestPublishedSkillMatchesTheEmbeddedOne fails when the published copy no
// longer matches what the binary embeds. The copy exists because installers
// such as "npx skills add <owner>/<repo>" look for a SKILL.md at the
// repository root, under skills/, or under an agent directory, and the
// templates the binary embeds live in none of those. Run "make gen".
func TestPublishedSkillMatchesTheEmbeddedOne(t *testing.T) {
	for _, f := range skill.Files() {
		path := filepath.Join(publishedDir, filepath.FromSlash(f.Path))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if string(got) != f.Template() {
			t.Errorf("%s is out of date; run make gen", f.Path)
		}
		// The published copy is installed by another tool, so it must not
		// claim figctl owns it.
		if strings.Contains(string(got), skill.Marker) {
			t.Errorf("%s should not carry the figctl ownership marker", f.Path)
		}
	}
}

// TestPublishedSkillIsDiscoverable checks the two things a skill installer
// requires: a SKILL.md at a path it walks, carrying name and description
// front matter.
func TestPublishedSkillIsDiscoverable(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(publishedDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("the published SKILL.md is missing: %v", err)
	}
	head, _, found := strings.Cut(strings.TrimPrefix(string(body), "---\n"), "\n---")
	if !found {
		t.Fatal("the published SKILL.md has no YAML front matter")
	}
	for _, key := range []string{"name:", "description:"} {
		if !strings.Contains(head, key) {
			t.Errorf("front matter is missing %q", key)
		}
	}
	if !strings.Contains(head, "name: figctl") {
		t.Error("the skill name must be figctl, the identifier an installer uses")
	}
}
