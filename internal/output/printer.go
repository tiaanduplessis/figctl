package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// Tabular is implemented by command results that can be rendered as rows in
// markdown, table, and plain output. Results that do not implement it fall
// back to JSON in those modes.
type Tabular interface {
	Columns() []string
	Rows() [][]string
}

// Markdowner is implemented by command results with a markdown shape that
// is not a table, for example an indented tree. The printer writes the
// returned body between the envelope header and the hints.
type Markdowner interface {
	Markdown() string
}

// KeyValues renders a single object as field and value rows.
type KeyValues [][2]string

// Columns implements Tabular.
func (kv KeyValues) Columns() []string { return []string{"field", "value"} }

// Rows implements Tabular.
func (kv KeyValues) Rows() [][]string {
	rows := make([][]string, 0, len(kv))
	for _, pair := range kv {
		rows = append(rows, []string{pair[0], pair[1]})
	}
	return rows
}

// Printer renders envelopes in the selected mode. Data goes to Out, human
// readable errors go to Err.
type Printer struct {
	Out    io.Writer
	Err    io.Writer
	Mode   Mode
	Fields []string
	Color  bool
	Log    *Logger
}

// Print renders a success envelope.
func (p *Printer) Print(env *Envelope) error {
	if env.Hints == nil {
		env.Hints = []string{}
	}
	switch p.Mode {
	case ModeJSON:
		return p.printJSON(env)
	case ModeMarkdown:
		return p.printMarkdown(env)
	case ModeTable:
		return p.printTable(env, false)
	case ModePlain:
		return p.printTable(env, true)
	}
	return figctl.Newf(figctl.CodeInternal, "unsupported output mode %q", p.Mode)
}

// PrintError renders an error: the JSON error envelope on Out in JSON mode,
// human readable text on Err otherwise.
func (p *Printer) PrintError(err error) error {
	if p.Mode == ModeJSON {
		return writeJSON(p.Out, NewErrorEnvelope(err))
	}
	e := figctl.From(err)
	fmt.Fprintf(p.Err, "error: %s (%s)\n", e.Message, e.Code)
	if e.Hint != "" {
		fmt.Fprintf(p.Err, "hint: %s\n", e.Hint)
	}
	return nil
}

func (p *Printer) printJSON(env *Envelope) error {
	if len(p.Fields) > 0 {
		projected, err := Project(env.Data, p.Fields)
		if err != nil {
			return err
		}
		copied := *env
		copied.Data = projected
		env = &copied
	}
	return writeJSON(p.Out, env)
}

func (p *Printer) printMarkdown(env *Envelope) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", env.Command)
	if env.Profile != nil {
		fmt.Fprintf(&b, "Profile: %s\n\n", env.Profile.Name)
	}
	if env.File != nil {
		b.WriteString("File: " + describeFile(env.File) + "\n\n")
	}
	if md, ok := env.Data.(Markdowner); ok {
		b.WriteString(md.Markdown())
	} else if columns, rows, ok := p.rows(env.Data); ok {
		writeMarkdownTable(&b, columns, rows)
	} else {
		data, err := p.projectedJSON(env.Data)
		if err != nil {
			return err
		}
		b.WriteString("```json\n")
		b.Write(data)
		b.WriteString("```\n")
	}
	hints := p.hints(env)
	if len(hints) > 0 {
		b.WriteString("\nHints:\n")
		for _, h := range hints {
			b.WriteString("- " + h + "\n")
		}
	}
	_, err := io.WriteString(p.Out, b.String())
	return err
}

func (p *Printer) printTable(env *Envelope, plain bool) error {
	columns, rows, ok := p.rows(env.Data)
	if !ok {
		data, err := p.projectedJSON(env.Data)
		if err != nil {
			return err
		}
		_, err = p.Out.Write(data)
		return err
	}
	var b strings.Builder
	if plain {
		for _, row := range rows {
			b.WriteString(strings.Join(row, "\t") + "\n")
		}
	} else {
		writeAlignedTable(&b, columns, rows, p.Color)
	}
	if _, err := io.WriteString(p.Out, b.String()); err != nil {
		return err
	}
	for _, h := range p.hints(env) {
		p.Log.Infof("hint: %s", h)
	}
	return nil
}

// rows extracts the tabular view of data, applying --fields as a column
// filter.
func (p *Printer) rows(data any) ([]string, [][]string, bool) {
	tab, ok := data.(Tabular)
	if !ok {
		return nil, nil, false
	}
	columns, rows := tab.Columns(), tab.Rows()
	if len(p.Fields) == 0 {
		return columns, rows, true
	}
	if isKeyValue(columns) {
		return columns, filterKeyValueRows(rows, p.Fields), true
	}
	var keep []int
	var keptColumns []string
	for _, field := range p.Fields {
		for i, col := range columns {
			if strings.EqualFold(field, col) {
				keep = append(keep, i)
				keptColumns = append(keptColumns, col)
			}
		}
	}
	filtered := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(keep))
		for _, i := range keep {
			if i < len(row) {
				cells = append(cells, row[i])
			} else {
				cells = append(cells, "")
			}
		}
		filtered = append(filtered, cells)
	}
	return keptColumns, filtered, true
}

// isKeyValue reports whether a table is the field/value shape produced by
// KeyValues, where --fields selects rows rather than columns.
func isKeyValue(columns []string) bool {
	return len(columns) == 2 && columns[0] == "field" && columns[1] == "value"
}

func filterKeyValueRows(rows [][]string, fields []string) [][]string {
	filtered := make([][]string, 0, len(fields))
	for _, field := range fields {
		for _, row := range rows {
			if len(row) == 2 && strings.EqualFold(row[0], field) {
				filtered = append(filtered, row)
			}
		}
	}
	return filtered
}

func (p *Printer) hints(env *Envelope) []string {
	hints := append([]string{}, env.Hints...)
	if env.Truncated {
		hints = append(hints, "Output was truncated; narrow the query or use --limit and --cursor.")
	}
	if env.NextCursor != nil && *env.NextCursor != "" {
		hints = append(hints, fmt.Sprintf("More results available: pass --cursor %s.", *env.NextCursor))
	}
	return hints
}

func (p *Printer) projectedJSON(data any) ([]byte, error) {
	if len(p.Fields) > 0 {
		projected, err := Project(data, p.Fields)
		if err != nil {
			return nil, err
		}
		data = projected
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func describeFile(f *FileInfo) string {
	s := f.Key
	if f.Name != "" {
		s = f.Name + " (" + f.Key + ")"
	}
	if f.Version != "" {
		s += ", version " + f.Version
	}
	return s
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "encoding output: "+err.Error())
	}
	return nil
}

func writeMarkdownTable(b *strings.Builder, columns []string, rows [][]string) {
	b.WriteString("| " + strings.Join(columns, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(columns)) + "\n")
	for _, row := range rows {
		cells := make([]string, len(columns))
		for i := range columns {
			if i < len(row) {
				cells[i] = markdownCell(row[i])
			}
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r\n", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func writeAlignedTable(b *strings.Builder, columns []string, rows [][]string, color bool) {
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range rows {
		for i := range columns {
			if i < len(row) {
				if n := utf8.RuneCountInString(row[i]); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}
	header := padCells(columns, widths)
	if color {
		header = "\x1b[1m" + header + "\x1b[0m"
	}
	b.WriteString(header + "\n")
	for _, row := range rows {
		cells := make([]string, len(columns))
		for i := range columns {
			if i < len(row) {
				cells[i] = row[i]
			}
		}
		b.WriteString(padCells(cells, widths) + "\n")
	}
}

func padCells(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, cell := range cells {
		if i == len(cells)-1 {
			parts[i] = cell
			continue
		}
		parts[i] = cell + strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell))
	}
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}
