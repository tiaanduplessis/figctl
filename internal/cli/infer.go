package cli

import (
	"context"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/infer"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/ref"
)

type inferData struct {
	Variables []infer.Variable `json:"variables"`
	Sites     int              `json:"sites"`
	Nodes     int              `json:"nodes"`
	Resolved  bool             `json:"resolved"`
}

func (d inferData) Columns() []string {
	return []string{"name", "category", "values", "usages", "modes", "variableId"}
}

func (d inferData) Rows() [][]string {
	rows := make([][]string, 0, len(d.Variables))
	for _, v := range d.Variables {
		values := make([]string, 0, len(v.Values))
		for _, o := range v.Values {
			values = append(values, o.Value+" ("+strconv.Itoa(o.Count)+")")
		}
		modes := ""
		if v.LikelyModes {
			modes = "likely"
		}
		rows = append(rows, []string{
			v.Name, v.Category, strings.Join(values, ", "),
			strconv.Itoa(v.Usages), modes, v.ID,
		})
	}
	return rows
}

var variablesInferCmd = &cobra.Command{
	Use:   "infer [ref]",
	Short: "Reconstruct the variables a file uses from how they are used",
	Long: `Report the variables a file uses by walking its nodes, for accounts that
cannot read the variables endpoint.

Reading variables needs an Enterprise plan, but the bindings do not: every node
says which variable governs each of its properties, and the value that variable
resolved to sits on the same node. Walking the file therefore recovers which
values are governed, which places share one, and what each resolves to.

What it cannot recover is the designer's name for a variable and the collection
it belongs to. Names here are derived from the category and the most common
observed value, and every entry says so, because a plausible invented name
reads as authoritative and cannot be checked.

A variable seen resolving to more than one value is reported as likely having
modes, which is what light and dark look like from the outside.

On a plan that can read variables, prefer figctl variables list: it has the
real names, collections, and every mode.`,
	Example: `  figctl variables infer design-system
  figctl variables infer KEY --node 2:2
  figctl variables infer KEY --json > variables.json`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var (
		nodes         []string
		includeHidden bool
		samples       int
		minUsages     int
	)
	variablesInferCmd.Flags().StringArrayVar(&nodes, "node", nil, "limit the walk to these nodes; repeatable")
	variablesInferCmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "walk nodes the designer turned off")
	variablesInferCmd.Flags().IntVar(&samples, "samples", 3, "node ids kept per observed value")
	variablesInferCmd.Flags().IntVar(&minUsages, "min-usages", 0, "drop variables bound fewer times than this")
	variablesInferCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()

		roots, hints, err := inferRoots(rctx, session, r, nodes)
		if err != nil {
			return err
		}

		opts := infer.Options{IncludeHidden: includeHidden, MaxSamples: samples, MinUsages: minUsages}
		merged := &infer.Inventory{}
		for _, root := range roots {
			part := infer.Infer(root, opts)
			merged.Variables = append(merged.Variables, part.Variables...)
			merged.Sites += part.Sites
			merged.Nodes += part.Nodes
		}
		data := inferData{Variables: merged.Variables, Sites: merged.Sites, Nodes: merged.Nodes}
		if data.Variables == nil {
			data.Variables = []infer.Variable{}
		}

		// When the real variables are readable, say so rather than letting a
		// derived inventory pass for the authoritative one.
		if vars, _, verr := session.Variables(rctx, r.FileKey); verr == nil && vars != nil {
			data.Resolved = true
		}

		env := ctx.Envelope(data)
		for _, h := range hints {
			env.AddHint(h)
		}
		return ctx.Printer.Print(inferEnvelope(env, data))
	})
	variablesCmd.AddCommand(variablesInferCmd)
}

// inferEnvelope adds the hints that say how far the result can be trusted.
func inferEnvelope(env *output.Envelope, data inferData) *output.Envelope {
	switch {
	case len(data.Variables) == 0:
		env.AddHint("No variable binding was found. Either the file uses styles rather than variables, or the walked nodes do not bind any; try a wider --node or --include-hidden.")
		return env
	case data.Resolved:
		env.AddHint("This account can read the variables endpoint, so figctl variables list gives the real names, collections, and modes. Prefer it over these derived names.")
	default:
		env.AddHint("Names here are derived from the observed values, not the designer's names, which need an Enterprise plan to read. Treat them as identifiers to map onto your own tokens.")
	}
	var modes int
	for _, v := range data.Variables {
		if v.LikelyModes {
			modes++
		}
	}
	if modes > 0 {
		env.AddHint(strconv.Itoa(modes) + " variable(s) resolve to more than one value, which is what light and dark modes look like from the outside. Check the values before treating them as a theme.")
	}
	env.AddHint("Bindings seen: " + strconv.Itoa(data.Sites) + " across " + strconv.Itoa(data.Nodes) + " nodes. A value with few usages is weaker evidence than one used throughout.")
	return env
}

// inferRoots picks what to walk: the requested nodes, or the whole document.
func inferRoots(rctx context.Context, session *Session, r ref.Ref, nodes []string) ([]*figma.Node, []string, error) {
	ids, err := parseNodeFlags(nodes, r)
	if err != nil {
		return nil, nil, err
	}
	if len(ids) > 0 {
		resp, err := session.Nodes(rctx, r.FileKey, ids, figma.NodesOptions{})
		if err != nil {
			return nil, nil, err
		}
		var roots []*figma.Node
		for _, entry := range resp.Nodes {
			if entry != nil && entry.Document != nil {
				roots = append(roots, entry.Document)
			}
		}
		if len(roots) == 0 {
			return nil, nil, nodeNotFound(strings.Join(ids, ", "), r.FileKey)
		}
		session.Touch(r.FileKey)
		return roots, nil, nil
	}
	file, err := session.File(rctx, r.FileKey)
	if err != nil {
		return nil, nil, figctl.Wrap(figctl.CodeUsage, err, "the whole document is needed to walk every binding: "+figctl.From(err).Message).
			WithHint("Pass --node to walk one screen of a file this large, or run figctl variables infer %s --node <id>.", r.FileKey)
	}
	session.Touch(r.FileKey)
	return []*figma.Node{file.Document}, nil, nil
}
