package resolve

import (
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

// Token builds the token reference for a variable id (a local id, a
// subscribed id, or an id carrying a key). When the definition is not
// available the reference carries the id and is marked unresolved, so a
// caller still sees that the value is bound.
func (r *Resolver) Token(id string) *model.TokenRef {
	if id == "" {
		return nil
	}
	v, ok := r.Variable(id)
	if !ok {
		return &model.TokenRef{VariableID: id, Unresolved: true}
	}
	ref := &model.TokenRef{
		VariableID:   v.ID,
		Name:         v.Name,
		Collection:   v.Collection,
		ResolvedType: v.ResolvedType,
		CodeSyntax:   v.CodeSyntax,
	}
	if v.Published {
		ref.Unresolved = true
		return ref
	}
	values, err := r.ResolveAll(v.ID)
	if err != nil {
		ref.Unresolved = true
		return ref
	}
	ref.Values = map[string]any{}
	for _, mv := range values {
		if mv.Error != "" {
			continue
		}
		ref.Values[mv.Label(v.CollectionID)] = mv.Value.Any()
		if mv.Default && mv.CollectionID == v.CollectionID {
			ref.Chain = mv.Value.Chain
		}
	}
	if len(ref.Chain) < 2 {
		ref.Chain = nil
	}
	return ref
}

// Bound returns the token bound to a node property such as fills,
// strokes, itemSpacing, paddingLeft, or topLeftRadius. List properties
// return the first binding; use BoundAt for a specific index.
func (r *Resolver) Bound(node *figma.Node, property string) *model.TokenRef {
	return r.BoundAt(node, property, 0)
}

// BoundAt returns the token bound to index i of a list property such as
// fills or strokes, or the token of a scalar property when i is 0.
func (r *Resolver) BoundAt(node *figma.Node, property string, i int) *model.TokenRef {
	if node == nil {
		return nil
	}
	b, ok := node.BoundVariables[property]
	if !ok {
		return nil
	}
	switch {
	case b.Alias != nil:
		if i == 0 {
			return r.Token(b.Alias.ID)
		}
	case b.Aliases != nil:
		if i >= 0 && i < len(b.Aliases) {
			return r.Token(b.Aliases[i].ID)
		}
	case b.Fields != nil:
		if a, ok := b.Fields[property]; ok {
			return r.Token(a.ID)
		}
	}
	return nil
}

// PaintToken returns the token bound to a paint's color, falling back to
// the node level binding of the list property at the paint's index.
func (r *Resolver) PaintToken(node *figma.Node, property string, i int, p figma.Paint) *model.TokenRef {
	if a, ok := p.BoundVariables["color"]; ok {
		return r.Token(first(a))
	}
	return r.BoundAt(node, property, i)
}

// AliasToken returns the token for a bound variable alias map entry.
func (r *Resolver) AliasToken(bound map[string]figma.VariableBinding, field string) *model.TokenRef {
	if b, ok := bound[field]; ok {
		if a, ok := b.First(); ok {
			return r.Token(a.ID)
		}
	}
	return nil
}

// CSSVar returns var(--name) for a token with WEB code syntax, or the
// empty string. Names already starting with -- or var( are normalized.
func CSSVar(t *model.TokenRef) string {
	if t == nil || t.CodeSyntax == nil {
		return ""
	}
	name := strings.TrimSpace(t.CodeSyntax["WEB"])
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "var(") {
		return name
	}
	if !strings.HasPrefix(name, "--") {
		name = "--" + name
	}
	return "var(" + name + ")"
}

// first is the variable id a binding points at, or the empty string. Figma
// sends a bare alias for some properties and a list for others; callers that
// want "the variable bound here" should not have to branch on that.
func first(b figma.VariableBinding) string {
	if a, ok := b.First(); ok {
		return a.ID
	}
	return ""
}
