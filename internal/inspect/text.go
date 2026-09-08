package inspect

import (
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

// typographyKeys are the text properties reported with a token, present in
// the map whether or not anything is bound so a reader can see the
// difference between "no token" and "not looked at".
var typographyKeys = []string{"fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing"}

func (in *inspector) text(n *figma.Node) *model.Text {
	t := in.r.Typography(n.Style, in.styleRef(n, "text"))
	if t == nil {
		t = &model.Text{Tokens: map[string]*model.TokenRef{}}
		for _, key := range typographyKeys {
			t.Tokens[key] = nil
		}
	}
	in.bindNodeTypography(n, t)
	t.Characters = n.Characters
	t.Runs = in.r.Runs(n)
	return t
}

// bindNodeTypography fills in typography tokens that Figma records on the
// node rather than inside its text style. Both placements occur, and a text
// style that binds nothing while the node binds everything is the common
// case in a real design system. Reading only the style reports every one of
// those values as having no token, which reads as "the designer typed this
// by hand" and invites an agent to hardcode a governed value.
func (in *inspector) bindNodeTypography(n *figma.Node, t *model.Text) {
	if n == nil || len(n.BoundVariables) == 0 {
		return
	}
	if t.Tokens == nil {
		t.Tokens = map[string]*model.TokenRef{}
	}
	for key, binding := range n.BoundVariables {
		if !isTypographyKey(key) {
			continue
		}
		if existing := t.Tokens[key]; existing != nil {
			continue
		}
		if a, ok := binding.First(); ok {
			t.Tokens[key] = in.r.Token(a.ID)
		}
	}
}

func isTypographyKey(key string) bool {
	for _, k := range typographyKeys {
		if k == key {
			return true
		}
	}
	// Figma also binds these, and they belong with the text properties
	// rather than with layout or fills.
	switch key {
	case "fontStyle", "paragraphSpacing", "paragraphIndent", "textCase", "textDecoration":
		return true
	}
	return false
}
