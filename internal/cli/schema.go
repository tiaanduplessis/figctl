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
	{name: "devresources.add", value: devResourceWriteData{}},
	{name: "devresources.update", value: devResourceWriteData{}},
	{name: "devresources.remove", value: devResourceWriteData{}},
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
	{name: "diff", value: diffData{}},
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
	{name: "variables.infer", value: inferData{}},
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
	s = unwrapNullable(s)
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
	if inner := unwrapNullable(s); inner != s {
		return schemaTypeName(root, inner) + " or null"
	}
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
	t := reflect.TypeOf(value)
	schema := r.ReflectFromType(t)
	applyNullability(schema, schema, t, map[string]bool{})
	return schema
}

// nullableSchema wraps s so it also accepts JSON null, keeping the
// description on the wrapper so field listings still read well.
func nullableSchema(s *jsonschema.Schema) *jsonschema.Schema {
	if s == nil || unwrapNullable(s) != s {
		return s
	}
	return &jsonschema.Schema{
		OneOf:       []*jsonschema.Schema{s, {Type: "null"}},
		Description: s.Description,
	}
}

// unwrapNullable returns the value branch of a schema widened by
// applyNullability, and s itself for every other schema.
func unwrapNullable(s *jsonschema.Schema) *jsonschema.Schema {
	if s == nil || len(s.OneOf) != 2 || s.OneOf[1] == nil || s.OneOf[1].Type != "null" {
		return s
	}
	return s.OneOf[0]
}

// applyNullability widens every part of the reflected schema that the
// CLI really prints as null. Reflection maps a pointer to its element
// type alone, but a nil pointer without an omitempty tag is emitted as
// null rather than omitted, so without this pass figctl schema would
// advertise a shape the CLI never produces. seen keys the definitions
// already walked so recursive types terminate.
func applyNullability(root, s *jsonschema.Schema, t reflect.Type, seen map[string]bool) {
	target, name := resolveSchemaRef(root, s)
	if target == nil {
		return
	}
	if name != "" {
		if seen[name] {
			return
		}
		seen[name] = true
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		if items := target.Items; items != nil {
			if t.Elem().Kind() == reflect.Pointer {
				target.Items = nullableSchema(items)
			}
			applyNullability(root, items, t.Elem(), seen)
		}
	case reflect.Map:
		if values := target.AdditionalProperties; values != nil {
			if t.Elem().Kind() == reflect.Pointer {
				target.AdditionalProperties = nullableSchema(values)
			}
			applyNullability(root, values, t.Elem(), seen)
		}
	case reflect.Struct:
		applyStructNullability(root, target, t, seen)
	}
}

// applyStructNullability widens the properties of one struct, following
// embedded structs whose fields JSON inlines into the same object.
func applyStructNullability(root, s *jsonschema.Schema, t reflect.Type, seen map[string]bool) {
	if s.Properties == nil {
		return
	}
	for i := range t.NumField() {
		field := t.Field(i)
		name, omitempty, encoded := jsonFieldName(field)
		if !encoded {
			continue
		}
		if name == "" {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			applyStructNullability(root, s, embedded, seen)
			continue
		}
		property, present := s.Properties.Get(name)
		if !present {
			continue
		}
		if field.Type.Kind() == reflect.Pointer && !omitempty {
			s.Properties.Set(name, nullableSchema(property))
		}
		applyNullability(root, property, field.Type, seen)
	}
}

// jsonFieldName reports the JSON name of a struct field, whether it has
// the omitempty option, and whether encoding/json emits it at all. An
// embedded struct whose fields are inlined reports an empty name.
func jsonFieldName(field reflect.StructField) (name string, omitempty, encoded bool) {
	if field.PkgPath != "" && !field.Anonymous {
		return "", false, false
	}
	tag := field.Tag.Get("json")
	parts := strings.Split(tag, ",")
	if parts[0] == "-" && len(parts) == 1 {
		return "", false, false
	}
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}
	name = parts[0]
	if field.Anonymous && name == "" {
		kind := field.Type.Kind()
		if kind == reflect.Pointer {
			kind = field.Type.Elem().Kind()
		}
		if kind == reflect.Struct {
			return "", omitempty, true
		}
	}
	if name == "" {
		name = field.Name
	}
	return name, omitempty, true
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
