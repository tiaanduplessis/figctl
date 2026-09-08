package cli

import (
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// TestVariablesInferOnTheFixture covers the command end to end. The fixture
// binds variables the way a real file does, and the point of the command is
// that this works without the variables endpoint.
func TestVariablesInferOnTheFixture(t *testing.T) {
	setup(t)
	r := execute(t, "", "variables", "infer", figmatest.FileKey)
	ok(t, r)
	d := data(t, r, "variables.infer")

	vars, _ := d["variables"].([]any)
	if len(vars) == 0 {
		t.Fatalf("the fixture binds variables; none were recovered: %v", d)
	}
	if sites, _ := d["sites"].(float64); sites == 0 {
		t.Error("no bindings were counted")
	}
	first := vars[0].(map[string]any)
	for _, key := range []string{"variableId", "name", "nameSource", "category", "values", "usages"} {
		if _, ok := first[key]; !ok {
			t.Errorf("a recovered variable is missing %q: %v", key, first)
		}
	}
	// The derived name must never pass for the designer's own.
	if src, _ := first["nameSource"].(string); !strings.Contains(src, "derived") {
		t.Errorf("nameSource should say the name was derived, got %q", src)
	}
}

// TestVariablesInferSaysWhenRealNamesAreAvailable keeps a derived inventory
// from being mistaken for the authoritative one on a plan that has both.
func TestVariablesInferSaysWhenRealNamesAreAvailable(t *testing.T) {
	setup(t)
	r := execute(t, "", "variables", "infer", figmatest.FileKey)
	ok(t, r)
	env := decodeEnvelope(t, r.stdout)
	hints, _ := env["hints"].([]any)
	var joined string
	for _, h := range hints {
		if s, isString := h.(string); isString {
			joined += s + " "
		}
	}
	// The fixture server serves variables, so the command should point at
	// the real ones rather than let the derived names stand.
	if !strings.Contains(joined, "figctl variables list") {
		t.Fatalf("expected a pointer to the real variables, got %v", hints)
	}
}

// TestVariablesInferWithoutTheVariablesEndpoint is the case the command
// exists for: no Enterprise plan, so no names, but the bindings still tell
// you what is governed.
func TestVariablesInferWithoutTheVariablesEndpoint(t *testing.T) {
	api := setup(t)
	api.Respond("GET", "/v1/files/"+figmatest.FileKey+"/variables/local", figmatest.Response{
		Status: 403,
		Body: map[string]any{"status": 403, "err": "Invalid scope(s): file_content:read. " +
			"This endpoint requires the file_variables:read scope"},
	})
	r := execute(t, "", "variables", "infer", figmatest.FileKey)
	ok(t, r)
	d := data(t, r, "variables.infer")
	if vars, _ := d["variables"].([]any); len(vars) == 0 {
		t.Fatal("bindings should still be recovered without the variables endpoint")
	}
	if resolved, _ := d["resolved"].(bool); resolved {
		t.Error("the real names are not available here")
	}
	env := decodeEnvelope(t, r.stdout)
	var joined string
	for _, h := range env["hints"].([]any) {
		joined += h.(string) + " "
	}
	if !strings.Contains(joined, "Enterprise") {
		t.Errorf("the hint should explain why the names are derived: %v", env["hints"])
	}
}

// TestVariablesInferNarrowing covers the flags that make a large file usable.
func TestVariablesInferNarrowing(t *testing.T) {
	setup(t)
	full := execute(t, "", "variables", "infer", figmatest.FileKey)
	ok(t, full)
	all, _ := data(t, full, "variables.infer")["variables"].([]any)

	narrowed := execute(t, "", "variables", "infer", figmatest.FileKey, "--min-usages", "1000")
	ok(t, narrowed)
	few, _ := data(t, narrowed, "variables.infer")["variables"].([]any)
	if len(few) >= len(all) || len(few) != 0 {
		t.Fatalf("--min-usages should drop everything here: %d of %d", len(few), len(all))
	}

	scoped := execute(t, "", "variables", "infer", figmatest.FileKey, "--node", "2:2")
	ok(t, scoped)
}

func TestVariablesInferUnknownNode(t *testing.T) {
	setup(t)
	r := execute(t, "", "variables", "infer", figmatest.FileKey, "--node", "9:99")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitNotFound, r.stdout)
	}
}
