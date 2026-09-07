package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

var tokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Design tokens: resolve variables and styles, export as DTCG, CSS, or Tailwind",
}

var tokensResolveCmd = &cobra.Command{
	Use:   "resolve <ref> --id ID | --name NAME",
	Short: "Resolve one variable or style to concrete values across modes",
	Long: `Resolve a variable (by id or name) or a style (by node id, key, or name) to
its concrete values. Variables show the value per mode with the alias
chain that produced it; --mode narrows to one mode. Styles show the
resolved fills, typography, effects, or layout grids. Without variables
access (non Enterprise plans) only styles resolve, with a hint.`,
	Example: `  figctl tokens resolve KEY --id VariableID:1:201
  figctl tokens resolve KEY --name text/primary --mode Dark
  figctl tokens resolve KEY --id 5:2
  figctl tokens resolve KEY --name shadow/md`,
	Args: cobra.ExactArgs(1),
}

// tokenResolution is the data of tokens resolve.
type tokenResolution struct {
	Kind     string          `json:"kind"`
	Variable *variableDetail `json:"variable,omitempty"`
	Style    *styleDetail    `json:"style,omitempty"`
}

func (t tokenResolution) Columns() []string { return []string{"field", "value"} }

func (t tokenResolution) Rows() [][]string {
	if t.Variable != nil {
		return t.Variable.Rows()
	}
	if t.Style != nil {
		return t.Style.Rows()
	}
	return nil
}

func init() {
	var id, name, mode string
	f := tokensResolveCmd.Flags()
	f.StringVar(&id, "id", "", "variable id (VariableID:1:2), style node id (5:1), or style key")
	f.StringVar(&name, "name", "", "variable or style name")
	f.StringVar(&mode, "mode", "", "only this mode (name or id) for variables")
	tokensResolveCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(args[0])
		if err != nil {
			return err
		}
		if id == "" && name == "" {
			return figctl.New(figctl.CodeUsage, "--id or --name is required").
				WithHint("Use --id VariableID:1:2, --id 5:1 for a style node, or --name collection/token.")
		}
		if id != "" && name != "" {
			return figctl.New(figctl.CodeUsage, "pass either --id or --name, not both")
		}
		rctx, cancel := session.Context()
		defer cancel()
		styles, styleHint, err := session.StyleCatalog(rctx, r.FileKey)
		if err != nil {
			return err
		}
		file, err := session.fileMetadata(rctx, r.FileKey, nil)
		if err != nil {
			return err
		}
		vars, varsHint, err := session.Variables(rctx, r.FileKey)
		if err != nil {
			return err
		}
		resolver := resolve.New(resolve.Options{File: file, Variables: vars})
		for sid, st := range styles {
			resolver.AddStyle(sid, st)
		}
		env := ctx.Envelope(nil)
		if styleHint != "" {
			env.AddHint(styleHint)
		}

		var v *resolve.Variable
		if id != "" {
			if found, ok := resolver.Variable(id); ok {
				v = found
			}
		} else if found, ok := resolver.VariableByName(name); ok {
			v = found
		}
		if v != nil {
			row, matched := variableRowFrom(resolver, *v, mode)
			if !matched {
				return figctl.Newf(figctl.CodeNotFound, "variable %s has no mode %q", v.Name, mode).
					WithHint("Modes are listed by figctl variables get %s --id %s.", r.FileKey, v.ID)
			}
			modes := modeValuesFrom(resolver, *v)
			if mode != "" {
				kept := modes[:0]
				for _, m := range modes {
					if strings.EqualFold(m.Mode, mode) || m.ModeID == mode {
						kept = append(kept, m)
					}
				}
				modes = kept
			}
			env.Data = tokenResolution{Kind: "variable", Variable: &variableDetail{variableRow: row, Key: v.Key, Modes: modes}}
			return ctx.Printer.Print(env)
		}

		var style *resolve.Style
		var ok bool
		if id != "" {
			if style, ok = resolver.Style(id); !ok {
				if style, ok = resolver.StyleByKey(id); !ok {
					if nid, err := parseNodeArg(id); err == nil {
						style, ok = resolver.Style(nid)
					}
				}
			}
		} else {
			style, ok = resolver.StyleByName(name)
		}
		if !ok {
			what := id
			if what == "" {
				what = name
			}
			hint := "List variables with figctl variables list " + r.FileKey + " and styles with figctl styles list " + r.FileKey + "."
			if varsHint != "" {
				hint = varsHint + " " + hint
			}
			return figctl.Newf(figctl.CodeNotFound, "no variable or style matches %q", what).WithHint("%s", hint)
		}
		styleNodes, err := session.StyleNodes(rctx, r.FileKey, []string{style.ID})
		if err != nil {
			return err
		}
		resolver.AddStyleNode(style.ID, styleNodes[style.ID])
		style, _ = resolver.Style(style.ID)
		if varsHint != "" {
			env.AddHint(varsHint)
		}
		env.Data = tokenResolution{Kind: "style", Style: styleDetailFrom(style)}
		return ctx.Printer.Print(env)
	})
	tokensCmd.AddCommand(tokensResolveCmd)
	rootCmd.AddCommand(tokensCmd)
}
