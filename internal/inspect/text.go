package inspect

import (
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

func (in *inspector) text(n *figma.Node) *model.Text {
	t := in.r.Typography(n.Style, in.styleRef(n, "text"))
	if t == nil {
		t = &model.Text{Tokens: map[string]*model.TokenRef{}}
		for _, key := range []string{"fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing"} {
			t.Tokens[key] = nil
		}
	}
	t.Characters = n.Characters
	t.Runs = in.r.Runs(n)
	return t
}
