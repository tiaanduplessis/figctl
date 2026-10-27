package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
	"github.com/tiaanduplessis/figctl/internal/skill"
)

// The documented selectors must work on fixture output, so copied recipes do
// not send agents through schema guessing and failed parsing loops.
func TestWorkflowSelectorsMatchFixtureOutput(t *testing.T) {
	setup(t)
	workflow, err := skill.Content("workflows")
	if err != nil {
		t.Fatal(err)
	}
	examples := map[string]result{
		"frames.json":     execute(t, "", "file", "find", figmatest.FileKey, "--name", "Login*"),
		"discussion.json": execute(t, "", "comments", "list", figmatest.FileKey, "--thread", "9001", "--limit", "20"),
	}
	selectors := regexp.MustCompile(`jq -r '\.data\[\] \| \[([^\]]+)\] \| @tsv' ([a-z]+\.json)`).FindAllStringSubmatch(workflow, -1)
	if len(selectors) != len(examples) {
		t.Fatalf("expected selector coverage for each workflow fixture, got %d", len(selectors))
	}
	for _, match := range selectors {
		r, ok := examples[match[2]]
		if !ok {
			t.Fatalf("workflow example %s needs a fixture", match[2])
		}
		if r.code != 0 {
			t.Fatalf("example failed: %s", r.stdout)
		}
		rows := decodeEnvelope(t, r.stdout)["data"].([]any)
		if len(rows) == 0 {
			t.Fatalf("empty example %s", match[2])
		}
		for _, row := range rows {
			for _, selector := range strings.Split(match[1], ",") {
				field := strings.TrimPrefix(strings.TrimSpace(selector), ".")
				value, exists := row.(map[string]any)[field]
				if !exists {
					t.Fatalf("%s: documented field %q is absent from fixture output", match[2], field)
				}
				if _, ok := value.(string); !ok {
					t.Fatalf("%s: %s must be a scalar string for @tsv, got %T", match[2], field, value)
				}
			}
		}
	}
	// Validate normalized-model paths in the prose against the same schema that
	// validates actual context responses in the command contract tests.
	fields := map[string]bool{}
	for _, f := range (schemaDoc{schema: reflectSchema(contextData{})}).Fields() {
		fields[f.Path] = true
	}
	for _, path := range regexp.MustCompile(`nodes\[\](?:\.[A-Za-z][A-Za-z0-9]*(?:\[\])?)+`).FindAllString(workflow, -1) {
		if !fields[path] {
			t.Fatalf("documented context path %s is absent from its published schema", path)
		}
	}
}
