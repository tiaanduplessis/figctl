package output

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

var update = flag.Bool("update", false, "update golden files")

type node struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type nodeList []node

func (l nodeList) Columns() []string { return []string{"id", "name", "type"} }

func (l nodeList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, n := range l {
		rows = append(rows, []string{n.ID, n.Name, n.Type})
	}
	return rows
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func newPrinter(mode Mode, fields ...string) (*Printer, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := &Printer{Out: out, Err: errOut, Mode: mode, Fields: fields, Log: NewLogger(errOut, false, false)}
	return p, out, errOut
}

func TestEnvelopeGolden(t *testing.T) {
	cursor := "page2"
	env := NewEnvelope("node.inspect", node{ID: "12:34", Name: "Login", Type: "FRAME"})
	env.Profile = &ProfileInfo{Name: "acme", Handle: "tiaan"}
	env.File = &FileInfo{Key: "ABC", Name: "Web App", Version: "123", LastModified: "2026-09-01T10:00:00Z"}
	env.Truncated = true
	env.NextCursor = &cursor
	env.AddHint("Use --depth 2 to see children of frame 12:34")

	p, out, _ := newPrinter(ModeJSON)
	if err := p.Print(env); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "envelope_success.golden", out.Bytes())

	p, out, _ = newPrinter(ModeJSON)
	if err := p.Print(NewEnvelope("version", map[string]string{"version": "dev"})); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "envelope_minimal.golden", out.Bytes())
}

func TestErrorEnvelopeGolden(t *testing.T) {
	err := figctl.New(figctl.CodeRateLimited, "Figma rate limit hit for tier 1 endpoints (plan: pro, seat: high).").
		WithHint("Retry after 42s. Reuse the cached file with 'figctl file tree ABC' instead of re-fetching.")
	err.RetryAfterSeconds = 42
	err.HTTPStatus = 429

	p, out, _ := newPrinter(ModeJSON)
	if perr := p.PrintError(err); perr != nil {
		t.Fatal(perr)
	}
	checkGolden(t, "envelope_error.golden", out.Bytes())

	p, out, _ = newPrinter(ModeJSON)
	if perr := p.PrintError(errors.New("boom")); perr != nil {
		t.Fatal(perr)
	}
	checkGolden(t, "envelope_error_minimal.golden", out.Bytes())
}

func TestPrintErrorHuman(t *testing.T) {
	p, out, errOut := newPrinter(ModeTable)
	err := figctl.New(figctl.CodeNotFound, "file XYZ not found").WithHint("Check the key.")
	if perr := p.PrintError(err); perr != nil {
		t.Fatal(perr)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", out.String())
	}
	want := "error: file XYZ not found (NOT_FOUND)\nhint: Check the key.\n"
	if errOut.String() != want {
		t.Fatalf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestResolveMode(t *testing.T) {
	cases := []struct {
		name    string
		flag    string
		json    bool
		tty     bool
		want    Mode
		wantErr bool
	}{
		{"flag wins over tty", "md", false, true, ModeMarkdown, false},
		{"flag wins over json alias", "table", true, false, ModeTable, false},
		{"json alias", "", true, true, ModeJSON, false},
		{"tty default", "", false, true, ModeTable, false},
		{"pipe default", "", false, false, ModeJSON, false},
		{"plain", "plain", false, true, ModePlain, false},
		{"markdown long name", "markdown", false, true, ModeMarkdown, false},
		{"unknown", "xml", false, true, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveMode(tc.flag, tc.json, tc.tty)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if figctl.From(err).Code != figctl.CodeUsage {
					t.Fatalf("code = %s, want USAGE", figctl.From(err).Code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("mode = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestColorEnabled(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	cases := []struct {
		name    string
		noColor bool
		tty     bool
		vars    map[string]string
		want    bool
	}{
		{"tty", false, true, nil, true},
		{"not tty", false, false, nil, false},
		{"flag", true, true, nil, false},
		{"NO_COLOR", false, true, map[string]string{"NO_COLOR": "1"}, false},
		{"TERM dumb", false, true, map[string]string{"TERM": "dumb"}, false},
		{"TERM xterm", false, true, map[string]string{"TERM": "xterm-256color"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ColorEnabled(tc.noColor, tc.tty, env(tc.vars)); got != tc.want {
				t.Fatalf("ColorEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProjectFields(t *testing.T) {
	p, out, _ := newPrinter(ModeJSON, "id", "type")
	if err := p.Print(NewEnvelope("x", node{ID: "1:2", Name: "A", Type: "FRAME"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id": "1:2"`) || strings.Contains(out.String(), `"name"`) {
		t.Fatalf("unexpected projection: %s", out.String())
	}

	p, out, _ = newPrinter(ModeJSON, "name")
	list := nodeList{{ID: "1:2", Name: "A", Type: "FRAME"}, {ID: "1:3", Name: "B", Type: "TEXT"}}
	if err := p.Print(NewEnvelope("x", list)); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if strings.Contains(s, `"id"`) || !strings.Contains(s, `"name": "B"`) {
		t.Fatalf("unexpected array projection: %s", s)
	}

	projected, err := Project(42, []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if projected.(interface{ String() string }).String() != "42" {
		t.Fatalf("scalar should pass through, got %v", projected)
	}
}

func TestMarkdownAndTableRendering(t *testing.T) {
	list := nodeList{{ID: "1:2", Name: "Log|in", Type: "FRAME"}, {ID: "1:3", Name: "B", Type: "TEXT"}}
	env := NewEnvelope("file.tree", list).AddHint("Drill down with --node.")
	env.File = &FileInfo{Key: "ABC", Name: "Web App"}

	p, out, _ := newPrinter(ModeMarkdown)
	if err := p.Print(env); err != nil {
		t.Fatal(err)
	}
	md := out.String()
	for _, want := range []string{"# file.tree", "File: Web App (ABC)", "| id | name | type |", "| --- | --- | --- |", `| 1:2 | Log\|in | FRAME |`, "- Drill down with --node."} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}

	p, out, errOut := newPrinter(ModeTable, "name", "id")
	if err := p.Print(env); err != nil {
		t.Fatal(err)
	}
	table := out.String()
	if !strings.HasPrefix(table, "name    id\nLog|in  1:2\nB       1:3\n") {
		t.Fatalf("unexpected table:\n%s", table)
	}
	if !strings.Contains(errOut.String(), "hint: Drill down with --node.") {
		t.Fatalf("hints should go to stderr in table mode, got %q", errOut.String())
	}

	p, out, _ = newPrinter(ModePlain)
	if err := p.Print(env); err != nil {
		t.Fatal(err)
	}
	if out.String() != "1:2\tLog|in\tFRAME\n1:3\tB\tTEXT\n" {
		t.Fatalf("unexpected plain output: %q", out.String())
	}

	p, out, _ = newPrinter(ModeTable)
	if err := p.Print(NewEnvelope("x", node{ID: "1:2"})); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "{\n  \"id\": \"1:2\"") {
		t.Fatalf("non tabular data should fall back to JSON, got %q", out.String())
	}

	p, out, _ = newPrinter(ModeMarkdown, "arch", "version")
	if err := p.Print(NewEnvelope("x", KeyValues{{"version", "dev"}, {"commit", "none"}, {"arch", "arm64"}})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "| arch | arm64 |\n| version | dev |\n") || strings.Contains(out.String(), "commit") {
		t.Fatalf("--fields should select key/value rows, got:\n%s", out.String())
	}

	p, out, _ = newPrinter(ModeTable)
	p.Color = true
	if err := p.Print(NewEnvelope("x", KeyValues{{"name", "acme"}})); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "\x1b[1mfield  value\x1b[0m\nname   acme\n") {
		t.Fatalf("unexpected colored table: %q", out.String())
	}
}

func TestLoggerQuiet(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf, true, true)
	l.Infof("a")
	l.Warnf("b")
	l.Debugf("c")
	if buf.Len() != 0 {
		t.Fatalf("quiet logger wrote %q", buf.String())
	}
	l = NewLogger(&buf, false, false)
	l.Infof("a")
	l.Warnf("b")
	l.Debugf("c")
	if buf.String() != "a\nwarning: b\n" {
		t.Fatalf("unexpected log output %q", buf.String())
	}
	buf.Reset()
	NewLogger(&buf, false, true).Debugf("c")
	if buf.String() != "debug: c\n" {
		t.Fatalf("unexpected verbose output %q", buf.String())
	}
}
