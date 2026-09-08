package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/resolve"
	"github.com/tiaanduplessis/figctl/internal/tokens"
)

// Export formats accepted by tokens export.
const (
	formatDTCG            = "dtcg"
	formatStyleDictionary = "style-dictionary"
	formatCSS             = "css"
	formatTailwind        = "tailwind"
	formatJSON            = "json"
)

// exportFormats lists the accepted --format values.
var exportFormats = []string{formatDTCG, formatCSS, formatTailwind, formatStyleDictionary, formatJSON}

// namePreview is how many token names table and markdown output list.
const namePreview = 20

// tokenFile is one file written by --out.
type tokenFile struct {
	Path   string `json:"path"`
	Mode   string `json:"mode,omitempty"`
	Bytes  int    `json:"bytes"`
	Tokens int    `json:"tokens"`
}

// tokensExportData is the data of tokens export.
type tokensExportData struct {
	Format  string         `json:"format"`
	Summary tokens.Summary `json:"summary"`
	// Tokens holds the rendered document for the structured formats: the
	// DTCG tree (or the flat token list for --format json), or a map
	// keyed by mode when --mode-strategy separate produced several.
	Tokens any `json:"tokens,omitempty"`
	// Content is the rendered text for css and tailwind.
	Content string `json:"content,omitempty"`
	// Files is the manifest of what --out wrote.
	Files []tokenFile `json:"files,omitempty"`
	// Names previews the first tokens by name and type.
	Names []tokens.Preview `json:"names,omitempty"`
}

// Columns implements output.Tabular.
func (d tokensExportData) Columns() []string { return []string{"collection", "tokens", "modes"} }

// Rows implements output.Tabular: one row per collection, a total, then
// one row per previewed token labelled "token" in the first column.
func (d tokensExportData) Rows() [][]string {
	rows := make([][]string, 0, len(d.Summary.Collections)+len(d.Names)+1)
	for _, c := range d.Summary.Collections {
		rows = append(rows, []string{c.Name, strconv.Itoa(c.Tokens), strings.Join(c.Modes, ", ")})
	}
	rows = append(rows, []string{"total", strconv.Itoa(d.Summary.Tokens), ""})
	for _, p := range d.Names {
		rows = append(rows, []string{"token", p.Name, p.Type})
	}
	return rows
}

// Markdown implements output.Markdowner: the summary table, a preview of
// the token names, and the rendered text when the format produces one.
func (d tokensExportData) Markdown() string {
	var b strings.Builder
	b.WriteString("| collection | tokens | modes |\n| --- | --- | --- |\n")
	for _, c := range d.Summary.Collections {
		fmt.Fprintf(&b, "| %s | %d | %s |\n", c.Name, c.Tokens, strings.Join(c.Modes, ", "))
	}
	fmt.Fprintf(&b, "| total | %d | |\n", d.Summary.Tokens)
	if len(d.Names) > 0 {
		fmt.Fprintf(&b, "\nTokens (first %d):\n\n", len(d.Names))
		for _, p := range d.Names {
			b.WriteString("- " + p.Name + " (" + p.Type + ")\n")
		}
	}
	if len(d.Files) > 0 {
		b.WriteString("\nFiles:\n\n")
		for _, f := range d.Files {
			fmt.Fprintf(&b, "- %s (%d bytes, %d tokens)\n", f.Path, f.Bytes, f.Tokens)
		}
	}
	if d.Content != "" {
		lang := d.Format
		if lang == formatTailwind {
			lang = "js"
		}
		fmt.Fprintf(&b, "\n```%s\n%s```\n", lang, d.Content)
	}
	return b.String()
}

var tokensExportCmd = &cobra.Command{
	Use:   "export <ref>",
	Short: "Export variables and styles as DTCG, CSS custom properties, or a Tailwind theme",
	Long: `Export the variables and styles of a file as design tokens.

The default format is DTCG (Design Tokens Format Module 2025.10): groups
nest on the "/" in Figma names, aliases become {group.token} references,
and everything Figma expresses but DTCG does not (modes, scopes, layout
grids, gradient angles) is kept under $extensions.com.figma.
--format style-dictionary is the same bytes: Style Dictionary v4 reads
DTCG unchanged.

--format css writes custom properties, a :root block for the selected
modes and one block per remaining mode. --format tailwind writes a theme
module. --format json writes figctl's flat token list with the Figma ids.

Collections with several modes cannot be flattened once, so pick a
strategy: default keeps the default mode and records the rest under
$extensions, separate writes one document per mode, and select takes the
modes named by --mode (which also switches the strategy on its own).

Without variables access (any plan below Enterprise, or a token without
the file_variables:read scope) the command still exports the styles and
says so in hints.`,
	Example: `  figctl tokens export KEY > tokens.json
  figctl tokens export KEY --format css --out ./src/styles
  figctl tokens export KEY --mode Dark --format css
  figctl tokens export KEY --mode-strategy separate --out ./tokens
  figctl tokens export KEY --format tailwind --tailwind-vars --tailwind-format cjs`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var format, out, strategy, nameCase, include, modeSelector, tailwindFormat string
	var modes, collections []string
	var excludeHidden, excludeRemote, tailwindVars bool
	f := tokensExportCmd.Flags()
	f.StringVarP(&format, "format", "f", formatDTCG, "dtcg, css, tailwind, style-dictionary, or json")
	f.StringVar(&out, "out", "", "write files to this directory (or file) instead of the envelope")
	f.StringVar(&strategy, "mode-strategy", tokens.StrategyDefault, "how to flatten collections with several modes: default, separate, or select")
	f.StringArrayVar(&modes, "mode", nil, "mode name to select per collection; repeatable, implies --mode-strategy select")
	f.StringVar(&nameCase, "name-case", tokens.CaseKebab, "normalize name segments: kebab, camel, snake, or none")
	f.StringVar(&include, "include", "variables,styles", "sources to export: variables, styles, or both")
	f.StringArrayVar(&collections, "collection", nil, "only this variable collection (name or id); repeatable")
	f.BoolVar(&excludeHidden, "exclude-hidden", false, "drop variables marked hiddenFromPublishing")
	f.BoolVar(&excludeRemote, "exclude-remote", false, "drop variables and styles from remote libraries")
	f.StringVar(&modeSelector, "mode-selector", tokens.DefaultModeSelector, "css: selector for a mode block; <mode> is replaced with the kebab cased mode name")
	f.StringVar(&tailwindFormat, "tailwind-format", tokens.TailwindESM, "tailwind: esm, cjs, or json")
	f.BoolVar(&tailwindVars, "tailwind-vars", false, "tailwind: emit var(--name) values instead of literals")
	tokensExportCmd.RunE = run(func(ctx *Context, cmd *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		format = strings.ToLower(strings.TrimSpace(format))
		if !contains(exportFormats, format) {
			return figctl.Newf(figctl.CodeUsage, "invalid --format %q", format).
				WithHint("Use one of: %s.", strings.Join(exportFormats, ", "))
		}
		if !tokens.ValidNameCase(nameCase) {
			return figctl.Newf(figctl.CodeUsage, "invalid --name-case %q", nameCase).
				WithHint("Use one of: %s.", strings.Join(tokens.NameCases, ", "))
		}
		if len(modes) > 0 && !cmd.Flags().Changed("mode-strategy") {
			strategy = tokens.StrategySelect
		}
		if !contains(tokens.Strategies, strategy) {
			return figctl.Newf(figctl.CodeUsage, "invalid --mode-strategy %q", strategy).
				WithHint("Use one of: %s.", strings.Join(tokens.Strategies, ", "))
		}
		if !contains(tokens.TailwindFormats, tailwindFormat) {
			return figctl.Newf(figctl.CodeUsage, "invalid --tailwind-format %q", tailwindFormat).
				WithHint("Use one of: %s.", strings.Join(tokens.TailwindFormats, ", "))
		}
		wantVariables, wantStyles, err := parseInclude(include)
		if err != nil {
			return err
		}

		rctx, cancel := session.Context()
		defer cancel()
		resolver, hints, err := tokensResolver(rctx, session, r.FileKey)
		if err != nil {
			return err
		}
		if err := checkModes(resolver, strategy, modes); err != nil {
			return err
		}
		if !resolver.HasVariables() && len(resolver.Styles()) == 0 {
			return figctl.Newf(figctl.CodeNotFound, "file %s has no variables or styles to export", r.FileKey).
				WithHint("Check the file with figctl file info %s; a file with no design system only carries raw values, which figctl node inspect shows.", r.FileKey)
		}
		fileName, fileVersion := ctx.fileNameVersion()
		doc, err := tokens.Build(resolver, tokens.BuildOptions{
			File:          fileName,
			Version:       fileVersion,
			NameCase:      nameCase,
			Strategy:      strategy,
			Modes:         modes,
			Variables:     wantVariables,
			Styles:        wantStyles,
			Collections:   collections,
			ExcludeHidden: excludeHidden,
			ExcludeRemote: excludeRemote,
		})
		if err != nil {
			return err
		}
		outputs, err := render(doc, format, modeSelector, tailwindFormat, tailwindVars)
		if err != nil {
			return err
		}

		data := tokensExportData{Format: format, Summary: doc.Summary, Names: doc.Preview(namePreview)}
		if out == "" {
			if err := fillContent(&data, format, outputs); err != nil {
				return err
			}
		} else {
			data.Files, err = writeOutputs(out, outputs)
			if err != nil {
				return err
			}
		}

		env := ctx.Envelope(data)
		for _, hint := range hints {
			env.AddHint(hint)
		}
		for _, w := range doc.Warnings {
			env.AddHint(w)
		}
		for _, o := range outputs {
			for _, w := range o.Warnings {
				env.AddHint(w)
			}
		}
		if doc.Empty() {
			env.AddHint("No tokens matched the filters; drop --collection, --include, --exclude-hidden, or --exclude-remote.")
		}
		if len(data.Names) < doc.Summary.Tokens {
			env.AddHint(fmt.Sprintf("Showing the first %d of %d token names; the full document is in data.", len(data.Names), doc.Summary.Tokens))
		}
		if len(data.Files) > 0 {
			env.AddHint("Files written: " + strings.Join(paths(data.Files), ", "))
		}
		return ctx.Printer.Print(env)
	})
	tokensCmd.AddCommand(tokensExportCmd)
}

// fileNameVersion returns the name and version of the file recorded on
// the envelope, which the generated headers name.
func (c *Context) fileNameVersion() (string, string) {
	if c.file == nil {
		return "", ""
	}
	return c.file.Name, c.file.Version
}

// tokensResolver builds a resolver holding every style of the file with
// the value read from its style node, plus the variables when the plan
// and token allow them. The hints explain whatever was not available.
func tokensResolver(ctx context.Context, session *Session, key string) (*resolve.Resolver, []string, error) {
	var hints []string
	catalog, styleHint, err := session.StyleCatalog(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if styleHint != "" {
		hints = append(hints, styleHint)
	}
	ids := make([]string, 0, len(catalog))
	for id := range catalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	styleNodes, err := session.StyleNodes(ctx, key, ids)
	if err != nil {
		return nil, nil, err
	}
	file, err := session.fileMetadata(ctx, key, &figma.GetFileNodesResponse{})
	if err != nil {
		return nil, nil, err
	}
	vars, varsHint, err := session.Variables(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if varsHint != "" {
		hints = append(hints, varsHint)
	}
	resolver := resolve.New(resolve.Options{File: file, Variables: vars, StyleNodes: styleNodes})
	for id, s := range catalog {
		resolver.AddStyle(id, s)
	}
	return resolver, hints, nil
}

// checkModes rejects a --mode that names no mode of any collection, so a
// typo does not silently export the default modes.
func checkModes(r *resolve.Resolver, strategy string, modes []string) error {
	if strategy != tokens.StrategySelect || len(modes) == 0 {
		return nil
	}
	known := tokens.ModeNames(r)
	for _, want := range modes {
		found := false
		for _, name := range known {
			if strings.EqualFold(name, want) {
				found = true
				break
			}
		}
		if !found {
			return figctl.Newf(figctl.CodeUsage, "no collection has a mode named %q", want).
				WithHint("Modes in this file: %s.", strings.Join(known, ", "))
		}
	}
	return nil
}

// render dispatches to the writer of the chosen format. style-dictionary
// is an alias of dtcg because Style Dictionary v4 consumes DTCG as is.
func render(doc *tokens.Document, format, modeSelector, tailwindFormat string, tailwindVars bool) ([]tokens.Output, error) {
	switch format {
	case formatCSS:
		return doc.CSS(tokens.CSSOptions{ModeSelector: modeSelector})
	case formatTailwind:
		return doc.Tailwind(tokens.TailwindOptions{Format: tailwindFormat, Vars: tailwindVars})
	case formatJSON:
		return doc.JSON()
	default:
		return doc.DTCG()
	}
}

// fillContent puts the rendered output in the envelope: structured for
// the JSON formats, a string for the text ones.
func fillContent(data *tokensExportData, format string, outputs []tokens.Output) error {
	switch format {
	case formatCSS, formatTailwind:
		var b strings.Builder
		for _, o := range outputs {
			b.Write(o.Content)
		}
		data.Content = b.String()
		return nil
	default:
		return decodeInto(data, outputs)
	}
}

// decodeInto parses the rendered JSON back into the envelope so agents
// get one document rather than an escaped string.
func decodeInto(data *tokensExportData, outputs []tokens.Output) error {
	decode := func(raw []byte) (any, error) {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, figctl.Wrap(figctl.CodeInternal, err, "decoding rendered tokens: "+err.Error())
		}
		return v, nil
	}
	if len(outputs) == 1 && outputs[0].Mode == "" {
		v, err := decode(outputs[0].Content)
		if err != nil {
			return err
		}
		data.Tokens = v
		return nil
	}
	byMode := map[string]any{}
	for _, o := range outputs {
		v, err := decode(o.Content)
		if err != nil {
			return err
		}
		byMode[o.Mode] = v
	}
	data.Tokens = byMode
	return nil
}

// writeOutputs writes every rendered file under --out and returns the
// manifest. A path with a file extension names a file, which only works
// when the format produced exactly one.
func writeOutputs(out string, outputs []tokens.Output) ([]tokenFile, error) {
	abs, err := filepath.Abs(out)
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "resolving --out: "+err.Error())
	}
	asDir := isDirTarget(out, abs)
	if !asDir && len(outputs) > 1 {
		return nil, figctl.Newf(figctl.CodeUsage, "--out %s names one file but the export produced %d", out, len(outputs)).
			WithHint("Pass a directory so each mode gets its own file, or use --mode-strategy default.")
	}
	dir := abs
	if !asDir {
		dir = filepath.Dir(abs)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "creating "+dir+": "+err.Error())
	}
	files := make([]tokenFile, 0, len(outputs))
	for _, o := range outputs {
		path := abs
		if asDir {
			path = filepath.Join(abs, o.Name)
		}
		if err := os.WriteFile(path, o.Content, 0o600); err != nil {
			return nil, figctl.Wrap(figctl.CodeInternal, err, "writing "+path+": "+err.Error())
		}
		files = append(files, tokenFile{Path: path, Mode: o.Mode, Bytes: len(o.Content), Tokens: o.Tokens})
	}
	return files, nil
}

// isDirTarget reports whether --out names a directory: an existing one,
// a trailing separator, or a path with no file extension.
func isDirTarget(flag, abs string) bool {
	if info, err := os.Stat(abs); err == nil {
		return info.IsDir()
	}
	if strings.HasSuffix(flag, string(os.PathSeparator)) || strings.HasSuffix(flag, "/") {
		return true
	}
	return filepath.Ext(abs) == ""
}

// parseInclude reads the --include list.
func parseInclude(include string) (bool, bool, error) {
	var variables, styles bool
	for _, part := range strings.Split(include, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "":
		case "variables", "variable":
			variables = true
		case "styles", "style":
			styles = true
		default:
			return false, false, figctl.Newf(figctl.CodeUsage, "invalid --include value %q", strings.TrimSpace(part)).
				WithHint("Use variables, styles, or variables,styles.")
		}
	}
	if !variables && !styles {
		return false, false, figctl.New(figctl.CodeUsage, "--include selected nothing").
			WithHint("Use --include variables, --include styles, or both.")
	}
	return variables, styles, nil
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func paths(files []tokenFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}
