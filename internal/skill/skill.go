// Package skill ships the figctl agent skill: the SKILL.md onboarding
// document, its generated reference files, and the installers that write
// them into the directories coding agents read.
//
// The reference files are generated from the cobra command tree and the
// JSON Schema registry by cmd/gen-skill-docs, so they cannot drift from
// the CLI they describe.
package skill

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed templates/SKILL.md templates/reference/commands.md templates/reference/output-schemas.md templates/reference/workflows.md
var templates embed.FS

// Marker is written into every whole file figctl owns. Installs replace a
// file that carries it and refuse to touch one that does not unless
// forced, so a hand written document is never clobbered.
const Marker = "<!-- figctl-skill: written by figctl skill install; local edits are replaced -->"

// BlockBegin and BlockEnd delimit the figctl section inside a shared
// instruction file such as AGENTS.md. Re-installing replaces what is
// between them and leaves the rest of the file alone.
const (
	BlockBegin = "<!-- BEGIN figctl -->"
	BlockEnd   = "<!-- END figctl -->"
)

// Description is the one line trigger sentence shared by the SKILL.md
// front matter and the Cursor rule front matter.
const Description = "Read Figma designs from the shell with figctl - outline a file, get one node's layout, styles and design tokens, render screenshots, export icons, and export the design system. Use when a task mentions Figma, a figma.com URL, a design file, design tokens, or implementing a screen from a design."

// File is one document of the skill.
type File struct {
	// Name is the identifier accepted by skill print --file.
	Name string
	// Path is the location inside a skill directory, always relative and
	// always with forward slashes.
	Path string

	source string
}

// files lists the skill documents in the order they are installed.
var files = []File{
	{Name: "SKILL", Path: "SKILL.md", source: "templates/SKILL.md"},
	{Name: "commands", Path: "reference/commands.md", source: "templates/reference/commands.md"},
	{Name: "schemas", Path: "reference/output-schemas.md", source: "templates/reference/output-schemas.md"},
	{Name: "workflows", Path: "reference/workflows.md", source: "templates/reference/workflows.md"},
}

// Files returns the skill documents.
func Files() []File {
	out := make([]File, len(files))
	copy(out, files)
	return out
}

// FileNames returns the identifiers accepted by skill print --file.
func FileNames() []string {
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

// Lookup finds a document by its identifier or by its relative path.
func Lookup(name string) (File, bool) {
	name = strings.TrimSpace(name)
	for _, f := range files {
		if strings.EqualFold(f.Name, name) || strings.EqualFold(f.Path, name) {
			return f, true
		}
	}
	return File{}, false
}

// Content returns the document with the ownership marker inserted, which
// is exactly what an install writes.
func (f File) Content() string {
	return withMarker(f.raw())
}

// raw returns the embedded template unchanged.
func (f File) raw() string {
	data, err := templates.ReadFile(f.source)
	if err != nil {
		// The templates are embedded at build time, so a missing one is a
		// build fault rather than a runtime condition.
		panic(fmt.Sprintf("skill: embedded template %s: %v", f.source, err))
	}
	return string(data)
}

// Content returns the content of a document by identifier.
func Content(name string) (string, error) {
	f, ok := Lookup(name)
	if !ok {
		return "", fmt.Errorf("unknown skill file %q, want one of %s", name, strings.Join(FileNames(), ", "))
	}
	return f.Content(), nil
}

// withMarker inserts the ownership marker after the YAML front matter, or
// at the top when there is none.
func withMarker(content string) string {
	if strings.Contains(content, Marker) {
		return content
	}
	if head, rest, ok := splitFrontMatter(content); ok {
		return head + Marker + "\n\n" + strings.TrimLeft(rest, "\n")
	}
	return Marker + "\n\n" + content
}

// splitFrontMatter separates a leading YAML front matter block (including
// its closing delimiter and newline) from the body.
func splitFrontMatter(content string) (head, body string, ok bool) {
	const fence = "---\n"
	if !strings.HasPrefix(content, fence) {
		return "", content, false
	}
	end := strings.Index(content[len(fence):], "\n"+fence)
	if end < 0 {
		return "", content, false
	}
	cut := len(fence) + end + 1 + len(fence)
	return content[:cut], content[cut:], true
}

// Body returns the SKILL.md document without its front matter and without
// the marker, for embedding in a rule file or a marked block.
func Body() string {
	f, _ := Lookup("SKILL")
	_, body, _ := splitFrontMatter(f.raw())
	return strings.TrimLeft(body, "\n")
}
