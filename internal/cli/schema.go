package cli

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// envelopeTarget is the name of the pseudo command whose schema is the
// output envelope every command is wrapped in.
const envelopeTarget = "envelope"

// maxSchemaDepth bounds the field list rendered in markdown and table
// output; the JSON schema itself is always complete.
const maxSchemaDepth = 8

var schemaCmd = &cobra.Command{
	Use:   "schema [command]",
	Short: "JSON Schema of a command's data payload",
	Long: `Print the JSON Schema of the data payload of a command, so field names
never have to be guessed. Command names are the dotted names that appear
in the "command" field of every envelope, for example node.context or
file.tree. Run without an argument to list the commands that have a
schema.

"figctl schema envelope" prints the shape of the envelope itself: the
schemaVersion, command, file, profile, data, truncated, nextCursor, and
hints fields, plus the error envelope.

JSON output is the schema itself. Markdown and table output flatten it
into a field list (path, type, required, description).`,
	Example: `  figctl schema
  figctl schema node.context
  figctl schema envelope
  figctl schema file.tree -o table`,
	Args: cobra.MaximumNArgs(1),
}

// schemaTarget maps a command name to a zero value of its data payload.
// Note is set for commands whose payload has no static schema.
type schemaTarget struct {
	name  string
	value any
	note  string
}

// schemaTargets is the registry of command payloads, in the order the
// listing prints them.
var schemaTargets = []schemaTarget{
	{name: envelopeTarget, value: output.Envelope{}},
	{name: "auth.login", value: profileView{}},
	{name: "auth.logout", value: logoutResult{}},
	{name: "auth.scopes", value: scopeList{}},
	{name: "auth.status", value: authStatus{}},
	{name: "assets.export", value: assetsData{}},
	{name: "assets.list", value: candidateList{}},
	{name: "cache.clear", value: cacheClearResult{}},
	{name: "cache.status", value: cacheStatusView{}},
	{name: "comments.add", value: commentRow{}},
	{name: "comments.list", value: commentList{}},
	{name: "components.get", value: componentDetail{}},
	{name: "components.list", value: componentList{}},
	{name: "devresources.list", value: devResourceList{}},
	{name: "file.find", value: findHits{}},
	{name: "file.get", note: "file get returns the raw Figma file or nodes JSON, whose shape is Figma's own file schema, not a figctl type."},
	{name: "file.info", value: fileInfo{}},
	{name: "file.tree", value: treeRows{}},
	{name: "folders.files", value: fileList{}},
	{name: "folders.list", value: folderList{}},
	{name: "init", value: initResult{}},
	{name: "me", value: meInfo{}},
	{name: "node.context", value: contextData{}},
	{name: "node.inspect", value: inspectResult{}},
	{name: "profile.add", value: profileView{}},
	{name: "profile.list", value: profileList{}},
	{name: "profile.remove", value: removeResult{}},
	{name: "profile.show", value: profileView{}},
	{name: "profile.use", value: profileView{}},
	{name: "projects.files", value: fileList{}},
	{name: "projects.list", value: projectList{}},
	{name: "render", value: renderData{}},
	{name: "schema", value: schemaList{}},
	{name: "skill.install", value: skillResult{}},
	{name: "skill.print", value: skillPrintResult{}},
	{name: "skill.uninstall", value: skillResult{}},
	{name: "styles.get", value: styleDetail{}},
	{name: "styles.list", value: styleList{}},
	{name: "tokens.export", value: tokensExportData{}},
	{name: "tokens.resolve", value: tokenResolution{}},
	{name: "variables.get", value: variableDetail{}},
	{name: "variables.list", value: variableList{}},
	{name: "version", value: versionInfo{}},
	{name: "versions.list", value: versionList{}},
}

// errorEnvelopeValue is the payload the error envelope schema reflects.
func errorEnvelopeValue() any { return output.ErrorEnvelope{} }

// schemaRow is one line of the schema listing.
type schemaRow struct {
	Command string `json:"command"`
	Type    string `json:"type"`
	Schema  bool   `json:"schema"`
	Note    string `json:"note,omitempty"`
}

type schemaList []schemaRow

// Columns implements output.Tabular.
func (l schemaList) Columns() []string { return []string{"command", "type", "schema", "note"} }

// Rows implements output.Tabular.
func (l schemaList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, r := range l {
		rows = append(rows, []string{r.Command, r.Type, yesNo(r.Schema), r.Note})
	}
	return rows
}

// schemaField is one flattened entry of a schema.
type schemaField struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// schemaDoc is the JSON Schema of one command's data payload. It encodes
// as the schema itself so JSON output is the schema, and renders as a
// field list in markdown and table output.
type schemaDoc struct {
	schema *jsonschema.Schema
}

// MarshalJSON implements json.Marshaler: the data payload is the schema.
func (d schemaDoc) MarshalJSON() ([]byte, error) { return json.Marshal(d.schema) }

// Columns implements output.Tabular.
func (d schemaDoc) Columns() []string { return []string{"path", "type", "required", "description"} }

// Rows implements output.Tabular.
func (d schemaDoc) Rows() [][]string {
	fields := d.Fields()
	rows := make([][]string, 0, len(fields))
	for _, f := range fields {
		rows = append(rows, []string{f.Path, f.Type, yesNo(f.Required), f.Description})
	}
	return rows
}

// Fields flattens the schema into a readable list of paths.
func (d schemaDoc) Fields() []schemaField {
	var out []schemaField
	appendSchemaFields(&out, d.schema, d.schema, "", map[string]bool{}, 0)
	return out
}

// resolveSchemaRef follows a $ref into $defs and returns the resolved
// schema together with the definition name it came from.
func resolveSchemaRef(root, s *jsonschema.Schema) (*jsonschema.Schema, string) {
	name := ""
	for i := 0; s != nil && s.Ref != "" && i < maxSchemaDepth; i++ {
		name = strings.TrimPrefix(s.Ref, "#/$defs/")
		def, ok := root.Definitions[name]
		if !ok {
			return s, name
		}
		s = def
	}
	return s, name
}

// schemaTypeName describes a property's type: the definition name for a
// referenced object, "array of X" for a list, or the JSON type.
func schemaTypeName(root, s *jsonschema.Schema) string {
	resolved, name := resolveSchemaRef(root, s)
	if resolved == nil {
		return ""
	}
	if resolved.Type == "array" && resolved.Items != nil {
		return "array of " + schemaTypeName(root, resolved.Items)
	}
	if name != "" {
		return name
	}
	if resolved.Type == "" {
		if len(resolved.OneOf) > 0 || len(resolved.AnyOf) > 0 {
			return "any"
		}
		return "object"
	}
	return resolved.Type
}

// appendSchemaFields walks properties depth first, following $refs and
// stopping when a definition repeats so recursive types terminate.
func appendSchemaFields(out *[]schemaField, root, s *jsonschema.Schema, prefix string, seen map[string]bool, depth int) {
	if s == nil || depth > maxSchemaDepth {
		return
	}
	resolved, name := resolveSchemaRef(root, s)
	if resolved == nil {
		return
	}
	if name != "" {
		if seen[name] {
			return
		}
		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}
		next[name] = true
		seen = next
	}
	if resolved.Type == "array" && resolved.Items != nil {
		appendSchemaFields(out, root, resolved.Items, prefix+"[]", seen, depth+1)
		return
	}
	if resolved.Properties == nil {
		return
	}
	required := map[string]bool{}
	for _, r := range resolved.Required {
		required[r] = true
	}
	for key, child := range resolved.Properties.FromOldest() {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		description := child.Description
		if description == "" {
			if target, _ := resolveSchemaRef(root, child); target != nil {
				description = target.Description
			}
		}
		*out = append(*out, schemaField{Path: path, Type: schemaTypeName(root, child), Required: required[key], Description: description})
		appendSchemaFields(out, root, child, path, seen, depth+1)
	}
}

// reflectSchema builds the JSON Schema of a payload value.
func reflectSchema(value any) *jsonschema.Schema {
	r := &jsonschema.Reflector{
		Anonymous:      true,
		ExpandedStruct: true,
	}
	return r.ReflectFromType(reflect.TypeOf(value))
}

// schemaTargetNames lists the registered command names.
func schemaTargetNames() []string {
	names := make([]string, 0, len(schemaTargets))
	for _, t := range schemaTargets {
		names = append(names, t.name)
	}
	sort.Strings(names)
	return names
}

func findSchemaTarget(name string) (schemaTarget, bool) {
	for _, t := range schemaTargets {
		if t.name == name {
			return t, true
		}
	}
	return schemaTarget{}, false
}

// normalizeSchemaName accepts "node context" and "node/context" as well
// as the dotted "node.context".
func normalizeSchemaName(arg string) string {
	name := strings.TrimSpace(arg)
	name = strings.ReplaceAll(name, " ", ".")
	name = strings.ReplaceAll(name, "/", ".")
	return strings.TrimPrefix(name, "figctl.")
}

func init() {
	schemaCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return printSchemaList(ctx)
		}
		name := normalizeSchemaName(args[0])
		target, ok := findSchemaTarget(name)
		if !ok {
			return figctl.Newf(figctl.CodeUsage, "no schema for command %q", args[0]).
				WithHint("Valid names: %s. Run figctl schema with no argument for the same list.", strings.Join(schemaTargetNames(), ", "))
		}
		if target.value == nil {
			return figctl.Newf(figctl.CodeUsage, "command %s has no static schema", name).
				WithHint("%s", target.note)
		}
		doc := schemaDoc{schema: reflectSchema(target.value)}
		env := ctx.Envelope(doc)
		env.AddHint("This is the shape of the data field of a " + name + " envelope; figctl schema envelope describes the envelope around it.")
		env.AddHint(strconv.Itoa(len(doc.Fields())) + " fields; fields whose JSON tag has omitempty are absent when empty and are not listed as required.")
		return ctx.Printer.Print(env)
	})
	rootCmd.AddCommand(schemaCmd)
}

func printSchemaList(ctx *Context) error {
	list := make(schemaList, 0, len(schemaTargets))
	for _, t := range schemaTargets {
		row := schemaRow{Command: t.name, Schema: t.value != nil, Note: t.note}
		if t.value != nil {
			row.Type = reflect.TypeOf(t.value).String()
		}
		list = append(list, row)
	}
	env := ctx.Envelope(list)
	env.AddHint("Print one with figctl schema <command>, for example figctl schema node.context.")
	env.AddHint("figctl schema envelope describes the wrapper every command shares: schemaVersion, command, file, profile, data, truncated, nextCursor, and hints.")
	return ctx.Printer.Print(env)
}
