package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// variableRow is one variable in variables list.
type variableRow struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Collection           string            `json:"collection"`
	CollectionID         string            `json:"collectionId"`
	Type                 string            `json:"type"`
	Values               map[string]any    `json:"values"`
	CodeSyntax           map[string]string `json:"codeSyntax,omitempty"`
	Scopes               []string          `json:"scopes,omitempty"`
	Description          string            `json:"description,omitempty"`
	HiddenFromPublishing bool              `json:"hiddenFromPublishing"`
	Remote               bool              `json:"remote"`
}

type variableList []variableRow

func (l variableList) Columns() []string {
	return []string{"name", "collection", "type", "values", "codeSyntax", "scopes", "description", "hidden", "remote"}
}

func (l variableList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, v := range l {
		rows = append(rows, []string{v.Name, v.Collection, v.Type, anyMapSummary(v.Values), mapSummary(v.CodeSyntax), strings.Join(v.Scopes, ","), v.Description, boolText(v.HiddenFromPublishing), boolText(v.Remote)})
	}
	return rows
}

// publishedVariableRow is one variable in variables list --published.
type publishedVariableRow struct {
	ID           string `json:"id"`
	SubscribedID string `json:"subscribedId"`
	Name         string `json:"name"`
	Key          string `json:"key"`
	Collection   string `json:"collection"`
	CollectionID string `json:"collectionId"`
	Type         string `json:"type"`
	UpdatedAt    string `json:"updatedAt"`
}

type publishedVariableList []publishedVariableRow

func (l publishedVariableList) Columns() []string {
	return []string{"name", "collection", "type", "id", "subscribedId", "key", "updatedAt"}
}

func (l publishedVariableList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, v := range l {
		rows = append(rows, []string{v.Name, v.Collection, v.Type, v.ID, v.SubscribedID, v.Key, v.UpdatedAt})
	}
	return rows
}

// modeValue is one resolved mode in variables get and tokens resolve.
type modeValue struct {
	Mode       string   `json:"mode"`
	ModeID     string   `json:"modeId"`
	Collection string   `json:"collection,omitempty"`
	Default    bool     `json:"default,omitempty"`
	Value      any      `json:"value"`
	Chain      []string `json:"chain,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// variableDetail is the data of variables get.
type variableDetail struct {
	variableRow
	Key   string      `json:"key,omitempty"`
	Modes []modeValue `json:"modes"`
}

func (d variableDetail) Columns() []string { return []string{"field", "value"} }

func (d variableDetail) Rows() [][]string {
	kv := [][2]string{
		{"id", d.ID}, {"name", d.Name}, {"collection", d.Collection}, {"type", d.Type},
	}
	for _, m := range d.Modes {
		label := m.Mode
		if m.Collection != "" && m.Collection != d.Collection {
			label = m.Collection + "/" + m.Mode
		}
		val := valueText(m.Value)
		if len(m.Chain) > 1 {
			val += " (" + strings.Join(m.Chain, " -> ") + ")"
		}
		if m.Error != "" {
			val = "error: " + m.Error
		}
		kv = append(kv, [2]string{"mode " + label, val})
	}
	kv = append(kv,
		[2]string{"codeSyntax", mapSummary(d.CodeSyntax)},
		[2]string{"scopes", strings.Join(d.Scopes, ",")},
		[2]string{"description", d.Description},
		[2]string{"hiddenFromPublishing", boolText(d.HiddenFromPublishing)},
		[2]string{"remote", boolText(d.Remote)},
	)
	if d.Key != "" {
		kv = append(kv, [2]string{"key", d.Key})
	}
	rows := make([][]string, 0, len(kv))
	for _, pair := range kv {
		rows = append(rows, []string{pair[0], pair[1]})
	}
	return rows
}

// boolText renders a boolean as yes or no.
func boolText(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func valueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return resolve.Num(x, 4)
	case string:
		return x
	case bool:
		return boolText(x)
	}
	return fmt.Sprint(v)
}

func anyMapSummary(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+valueText(m[k]))
	}
	return strings.Join(parts, "; ")
}

// variableRowFrom builds a list row with values resolved per mode.
func variableRowFrom(r *resolve.Resolver, v resolve.Variable, mode string) (variableRow, bool) {
	row := variableRow{
		ID:                   v.ID,
		Name:                 v.Name,
		Collection:           v.Collection,
		CollectionID:         v.CollectionID,
		Type:                 v.ResolvedType,
		Values:               map[string]any{},
		CodeSyntax:           v.CodeSyntax,
		Scopes:               v.Scopes,
		Description:          v.Description,
		HiddenFromPublishing: v.HiddenFromPublishing,
		Remote:               v.Remote,
	}
	values, err := r.ResolveAll(v.ID)
	if err != nil {
		return row, mode == ""
	}
	matched := mode == ""
	for _, mv := range values {
		if mode != "" && !strings.EqualFold(mv.Mode, mode) && mv.ModeID != mode {
			continue
		}
		matched = true
		if mv.Error != "" {
			continue
		}
		row.Values[mv.Label(v.CollectionID)] = mv.Value.Any()
	}
	return row, matched
}

func modeValuesFrom(r *resolve.Resolver, v resolve.Variable) []modeValue {
	values, err := r.ResolveAll(v.ID)
	if err != nil {
		return []modeValue{{Error: err.Error()}}
	}
	out := make([]modeValue, 0, len(values))
	for _, mv := range values {
		m := modeValue{Mode: mv.Mode, ModeID: mv.ModeID, Default: mv.Default, Value: mv.Value.Any(), Chain: mv.Value.Chain, Error: mv.Error}
		if mv.CollectionID != v.CollectionID {
			m.Collection = mv.Collection
		}
		if len(m.Chain) < 2 {
			m.Chain = nil
		}
		out = append(out, m)
	}
	return out
}

// findVariable locates a variable by id or name, with usage errors that
// name the alternatives.
func findVariable(r *resolve.Resolver, id, name string) (*resolve.Variable, error) {
	switch {
	case id != "" && name != "":
		return nil, figctl.New(figctl.CodeUsage, "pass either --id or --name, not both")
	case id == "" && name == "":
		return nil, figctl.New(figctl.CodeUsage, "--id or --name is required").
			WithHint("Use --id VariableID:1:2 or --name collection/token; list them with figctl variables list <ref>.")
	case id != "":
		if v, ok := r.Variable(id); ok {
			return v, nil
		}
		return nil, figctl.Newf(figctl.CodeNotFound, "variable %s not found", id).
			WithHint("List variables with figctl variables list <ref>.")
	default:
		if v, ok := r.VariableByName(name); ok {
			return v, nil
		}
		return nil, figctl.Newf(figctl.CodeNotFound, "variable named %q not found", name).
			WithHint("Names are like color/brand/500; list them with figctl variables list <ref>.")
	}
}

// variablesResolver builds a resolver from the local variables of a file,
// failing with PLAN_REQUIRED or AUTH_SCOPE when they cannot be read.
func variablesResolver(session *Session, key string) (*resolve.Resolver, error) {
	rctx, cancel := session.Context()
	defer cancel()
	vars, err := session.localVariables(rctx, key)
	if err != nil {
		return nil, err
	}
	session.Touch(key)
	return resolve.New(resolve.Options{Variables: vars}), nil
}

var variablesCmd = &cobra.Command{
	Use:   "variables",
	Short: "Raw variables and collections with values resolved per mode (Enterprise)",
}

var variablesListCmd = &cobra.Command{
	Use:   "list <ref>",
	Short: "List variables with values per mode, code syntax, scopes, and descriptions",
	Long: `List the local variables of a file. Aliases are resolved to concrete
values per mode and colors are shown as hex. Filter by collection, type,
or mode. The variables API needs an Enterprise plan with a full seat and
the file_variables:read scope; on other plans the command fails with
PLAN_REQUIRED and figctl styles list is the fallback.`,
	Example: `  figctl variables list KEY
  figctl variables list KEY --collection Semantic --mode Dark
  figctl variables list KEY --type COLOR -o table
  figctl variables list KEY --published`,
	Args: cobra.MaximumNArgs(1),
}

var variablesGetCmd = &cobra.Command{
	Use:   "get <ref> --id ID | --name NAME",
	Short: "Show one variable with its alias chain per mode",
	Example: `  figctl variables get KEY --id VariableID:1:201
  figctl variables get KEY --name bg/surface`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	var collection, typ, mode string
	var published bool
	f := variablesListCmd.Flags()
	f.StringVar(&collection, "collection", "", "only variables in this collection (name or id)")
	f.StringVar(&typ, "type", "", "only this resolved type: COLOR, FLOAT, STRING, or BOOLEAN")
	f.StringVar(&mode, "mode", "", "only values in this mode (name or id)")
	f.BoolVar(&published, "published", false, "list published variables (metadata only, with subscribed ids)")
	variablesListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		if typ != "" {
			typ = strings.ToUpper(typ)
			switch typ {
			case "COLOR", "FLOAT", "STRING", "BOOLEAN":
			default:
				return figctl.Newf(figctl.CodeUsage, "invalid --type %q", typ).WithHint("Use COLOR, FLOAT, STRING, or BOOLEAN.")
			}
		}
		if published {
			rctx, cancel := session.Context()
			defer cancel()
			resp, err := session.PublishedVariables(rctx, r.FileKey)
			if err != nil {
				return err
			}
			session.Touch(r.FileKey)
			list := publishedVariableList{}
			for _, v := range resp.Meta.Variables {
				c := resp.Meta.VariableCollections[v.VariableCollectionID]
				if collection != "" && !strings.EqualFold(c.Name, collection) && c.ID != collection {
					continue
				}
				if typ != "" && v.ResolvedDataType != typ {
					continue
				}
				list = append(list, publishedVariableRow{ID: v.ID, SubscribedID: v.SubscribedID, Name: v.Name, Key: v.Key, Collection: c.Name, CollectionID: v.VariableCollectionID, Type: v.ResolvedDataType, UpdatedAt: v.UpdatedAt})
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].Collection != list[j].Collection {
					return list[i].Collection < list[j].Collection
				}
				return list[i].Name < list[j].Name
			})
			env := ctx.Envelope(list)
			env.AddHint("Published variables carry no values; use figctl variables list " + r.FileKey + " for values per mode.")
			return ctx.Printer.Print(env)
		}
		resolver, err := variablesResolver(session, r.FileKey)
		if err != nil {
			return err
		}
		list := variableList{}
		for _, v := range resolver.Variables() {
			if collection != "" && !strings.EqualFold(v.Collection, collection) && v.CollectionID != collection {
				continue
			}
			if typ != "" && v.ResolvedType != typ {
				continue
			}
			row, matched := variableRowFrom(resolver, v, mode)
			if !matched {
				continue
			}
			list = append(list, row)
		}
		env := ctx.Envelope(list)
		if len(list) == 0 {
			env.AddHint("No variables matched; drop --collection, --type, or --mode, or check figctl variables list " + r.FileKey + " without filters.")
		}
		return ctx.Printer.Print(env)
	})

	var id, name string
	variablesGetCmd.Flags().StringVar(&id, "id", "", "variable id (VariableID:1:2) or subscribed id")
	variablesGetCmd.Flags().StringVar(&name, "name", "", "variable name (for example color/brand/500)")
	variablesGetCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		resolver, err := variablesResolver(session, r.FileKey)
		if err != nil {
			return err
		}
		v, err := findVariable(resolver, id, name)
		if err != nil {
			return err
		}
		row, _ := variableRowFrom(resolver, *v, "")
		detail := variableDetail{variableRow: row, Key: v.Key, Modes: modeValuesFrom(resolver, *v)}
		return ctx.Print(detail)
	})

	variablesCmd.AddCommand(variablesListCmd, variablesGetCmd)
	rootCmd.AddCommand(variablesCmd)
}
