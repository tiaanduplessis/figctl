package cli

import (
	"bytes"
	"encoding/json"
)

// This file exposes the command tree and the schema registry to the skill
// documentation generator (cmd/gen-skill-docs), so the reference files
// ship the same text as --help and figctl schema.

// SchemaFieldInfo is one flattened field of a JSON Schema.
type SchemaFieldInfo struct {
	Path        string
	Type        string
	Required    bool
	Description string
}

// SchemaEntry is the schema of one command's data payload. Fields and
// JSON are empty when the payload has no static schema, in which case
// Note says why.
type SchemaEntry struct {
	Command   string
	Note      string
	HasSchema bool
	JSON      string
	Fields    []SchemaFieldInfo
}

// SchemaEntries returns the registered command payload schemas, in the
// order figctl schema lists them.
func SchemaEntries() []SchemaEntry {
	out := make([]SchemaEntry, 0, len(schemaTargets))
	for _, t := range schemaTargets {
		out = append(out, schemaEntry(t.name, t.value, t.note))
	}
	return out
}

// ErrorEnvelopeEntry returns the schema of the error envelope, which has
// no command of its own.
func ErrorEnvelopeEntry() SchemaEntry {
	return schemaEntry("error", errorEnvelopeValue(), "")
}

func schemaEntry(name string, value any, note string) SchemaEntry {
	entry := SchemaEntry{Command: name, Note: note}
	if value == nil {
		return entry
	}
	doc := schemaDoc{schema: reflectSchema(value)}
	entry.HasSchema = true
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc.schema); err == nil {
		entry.JSON = buf.String()
	}
	for _, f := range doc.Fields() {
		entry.Fields = append(entry.Fields, SchemaFieldInfo(f))
	}
	return entry
}
