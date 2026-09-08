package cli

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// The contract test is the guard the plan calls for: every payload a
// command emits must validate against the JSON Schema that
// "figctl schema <command>" publishes for it. Agents read that schema to
// learn field names, so a payload that grows, loses, or renames a field
// without the schema following is a defect, not a detail.

// outDirToken stands in for a temporary directory in a case's arguments.
// It is replaced with a fresh directory when the case runs.
const outDirToken = "<outdir>"

// contractCase runs one command against the fake API and validates the
// data payload of its envelope against the published schema. Adding a
// command to the contract is one line in contractCases.
type contractCase struct {
	// command is the dotted name that appears in the envelope's command
	// field and is also the argument to figctl schema.
	command string
	// args is the full argument list, with outDirToken standing in for a
	// writable output directory.
	args []string
}

// contractCases covers the commands an agent relies on. Commands whose
// payload is a raw pass-through of Figma's own JSON are excluded and
// covered by TestSchemaContractExcludesFileGet instead.
var contractCases = []contractCase{
	{command: "node.inspect", args: []string{"node", "inspect", figmatest.FileKey, "--node", "2:2", "--depth", "1", "--css", "--interactions"}},
	{command: "node.context", args: []string{"node", "context", figmatest.FileKey, "--node", "2:2", "--depth", "1", "--out", outDirToken}},
	{command: "file.tree", args: []string{"file", "tree", figmatest.FileKey, "--depth", "1"}},
	{command: "file.find", args: []string{"file", "find", figmatest.FileKey, "--name", "*"}},
	{command: "file.info", args: []string{"file", "info", figmatest.FileKey}},
	{command: "components.list", args: []string{"components", "list", figmatest.FileKey}},
	{command: "styles.list", args: []string{"styles", "list", figmatest.FileKey}},
	{command: "variables.list", args: []string{"variables", "list", figmatest.FileKey}},
	{command: "variables.infer", args: []string{"variables", "infer", figmatest.FileKey}},
	{command: "devresources.add", args: []string{"devresources", "add", figmatest.FileKey,
		"--node", "3:11", "--url", "https://storybook.example.com/button", "--yes"}},
	{command: "devresources.update", args: []string{"devresources", "update", figmatest.FileKey,
		"--id", "dr-1", "--url", "https://storybook.example.com/renamed", "--yes"}},
	{command: "devresources.remove", args: []string{"devresources", "remove", figmatest.FileKey,
		"--id", "dr-1", "--yes"}},
	{command: "render", args: []string{"render", figmatest.FileKey, "--node", "2:2", "--out", outDirToken}},
	{command: "assets.export", args: []string{"assets", "export", figmatest.FileKey, "--node", "2:7", "--out", outDirToken}},
	{command: "assets.list", args: []string{"assets", "list", figmatest.FileKey}},
	{command: "tokens.export", args: []string{"tokens", "export", figmatest.FileKey}},
	{command: "tokens.resolve", args: []string{"tokens", "resolve", figmatest.FileKey, "--name", "bg/surface"}},
	{command: "versions.list", args: []string{"versions", "list", figmatest.FileKey}},
	{command: "comments.list", args: []string{"comments", "list", figmatest.FileKey}},
	{command: "devresources.list", args: []string{"devresources", "list", figmatest.FileKey}},
	{command: "me", args: []string{"me"}},
	{command: "auth.status", args: []string{"auth", "status"}},
	{command: "profile.list", args: []string{"profile", "list"}},
	{command: "cache.status", args: []string{"cache", "status"}},
	{command: "schema", args: []string{"schema"}},
}

func TestSchemaContract(t *testing.T) {
	for _, tc := range contractCases {
		t.Run(tc.command, func(t *testing.T) {
			setup(t)
			r := execute(t, "", expandArgs(t, tc.args)...)
			ok(t, r)
			env := decodeJSON(t, r.stdout)
			if env["command"] != tc.command {
				t.Fatalf("envelope command = %v, want %s (the table name must match the envelope)", env["command"], tc.command)
			}
			validateAgainstSchema(t, tc.command, envelopeTarget, "envelope", env)
			payload, present := env["data"]
			if !present {
				t.Fatalf("%s: envelope has no data field:\n%s", tc.command, r.stdout)
			}
			validateAgainstSchema(t, tc.command, tc.command, "data payload", payload)
		})
	}
}

// TestSchemaContractWithoutVariables covers the degraded path. When the
// variables endpoint is not available to the plan, the commands that
// fall back to styles and raw values emit a smaller payload, and that
// payload must match the same published schema.
func TestSchemaContractWithoutVariables(t *testing.T) {
	for _, tc := range []contractCase{
		{command: "node.context", args: []string{"node", "context", figmatest.FileKey, "--node", "2:2", "--depth", "1", "--out", outDirToken}},
		{command: "node.inspect", args: []string{"node", "inspect", figmatest.FileKey, "--node", "2:2", "--depth", "1"}},
		{command: "tokens.export", args: []string{"tokens", "export", figmatest.FileKey}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			api := setup(t)
			api.Respond(http.MethodGet, fileKeyPath("/variables/local"), figmatest.Response{
				Status: 403,
				Body:   map[string]any{"status": 403, "err": "This endpoint is only available to Enterprise plan users."},
			})
			r := execute(t, "", expandArgs(t, tc.args)...)
			ok(t, r)
			env := decodeJSON(t, r.stdout)
			validateAgainstSchema(t, tc.command, envelopeTarget, "envelope", env)
			validateAgainstSchema(t, tc.command, tc.command, "data payload", env["data"])
		})
	}
}

// TestSchemaContractErrorEnvelope validates a real error envelope
// against the error schema the skill reference files publish.
func TestSchemaContractErrorEnvelope(t *testing.T) {
	setup(t)
	r := execute(t, "", "file", "info", "NoSuchFile000000")
	if r.code != figctl.ExitNotFound {
		t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitNotFound, r.stdout)
	}
	env := decodeJSON(t, r.stdout)
	validateAgainstDoc(t, "file.info", "error", "error envelope", decodeJSON(t, ErrorEnvelopeEntry().JSON), env)
}

// TestSchemaContractExcludesFileGet pins the one deliberate exclusion:
// file get passes Figma's own file JSON through untouched, so it has no
// figctl schema and says so instead of printing a misleading one.
func TestSchemaContractExcludesFileGet(t *testing.T) {
	setup(t)
	r := execute(t, "", "schema", "file.get")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exit = %d, want %d\n%s", r.code, figctl.ExitUsage, r.stdout)
	}
	if code := errorCode(t, r); code != string(figctl.CodeUsage) {
		t.Fatalf("error code = %s, want %s", code, figctl.CodeUsage)
	}
	if !strings.Contains(r.stdout, "no static schema") || !strings.Contains(r.stdout, "raw Figma file") {
		t.Fatalf("figctl schema file.get should explain the exclusion:\n%s", r.stdout)
	}
}

// expandArgs replaces outDirToken with a fresh temporary directory.
func expandArgs(t *testing.T, args []string) []string {
	t.Helper()
	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		if a == outDirToken {
			out[i] = filepath.Join(t.TempDir(), "out")
		}
	}
	return out
}

// decodeJSON parses JSON the way the validator wants it, with numbers
// kept as json.Number so numeric keywords compare exactly.
func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(s))
	if err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
	object, isObject := value.(map[string]any)
	if !isObject {
		t.Fatalf("output is %T, want a JSON object:\n%s", value, s)
	}
	return object
}

// fetchSchema runs figctl schema <name> and returns the schema document.
func fetchSchema(t *testing.T, name string) any {
	t.Helper()
	r := execute(t, "", "schema", name)
	ok(t, r)
	return decodeJSON(t, r.stdout)["data"]
}

// validateAgainstSchema checks an instance against the schema that
// figctl schema publishes under schemaName.
func validateAgainstSchema(t *testing.T, command, schemaName, what string, instance any) {
	t.Helper()
	validateAgainstDoc(t, command, schemaName, what, fetchSchema(t, schemaName), instance)
}

// validateAgainstDoc compiles doc and validates instance against it,
// reporting the command, the JSON path, and what the schema expected.
func validateAgainstDoc(t *testing.T, command, schemaName, what string, doc, instance any) {
	t.Helper()
	const resource = "https://figctl.invalid/schema.json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resource, doc); err != nil {
		t.Fatalf("%s: schema %q is not a usable JSON Schema document: %v", command, schemaName, err)
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		t.Fatalf("%s: schema %q does not compile: %v", command, schemaName, err)
	}
	err = schema.Validate(instance)
	if err == nil {
		return
	}
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("%s: validating the %s failed: %v", command, what, err)
	}
	t.Fatalf("%s: the %s does not match the schema published by `figctl schema %s`.\n"+
		"Fix the payload or the schema so agents reading the schema see the real fields.\n%s",
		command, what, schemaName, strings.Join(violations(invalid), "\n"))
}

// violations flattens a validation error into one actionable line per
// offending JSON path, each naming the path and what the schema wanted.
func violations(e *jsonschema.ValidationError) []string {
	if len(e.Causes) == 0 {
		leaf := &jsonschema.ValidationError{
			SchemaURL:        e.SchemaURL,
			InstanceLocation: e.InstanceLocation,
			ErrorKind:        e.ErrorKind,
		}
		return []string{"  " + strings.ReplaceAll(leaf.GoString(), "\n", "\n  ")}
	}
	var out []string
	for _, cause := range e.Causes {
		out = append(out, violations(cause)...)
	}
	return out
}
