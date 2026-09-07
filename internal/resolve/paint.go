package resolve

import (
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

// Fills converts the visible paints of a node property (fills or strokes)
// into the model, attaching the bound token per paint and the style
// reference when the node uses a style for that property.
func (r *Resolver) Fills(node *figma.Node, property string, paints []figma.Paint, style *model.StyleRef) []model.Fill {
	out := make([]model.Fill, 0, len(paints))
	for i, p := range paints {
		if !p.IsVisible() {
			continue
		}
		f := r.Paint(p)
		if f == nil {
			continue
		}
		f.Token = r.PaintToken(node, property, i, p)
		f.Style = style
		out = append(out, *f)
	}
	return out
}

// Paint converts one paint without node context: no style reference and
// only the token bound on the paint itself.
func (r *Resolver) Paint(p figma.Paint) *model.Fill {
	f := &model.Fill{}
	if p.Opacity != nil && *p.Opacity != 1 {
		o := Round(*p.Opacity, 4)
		f.Opacity = &o
	}
	if p.BlendMode != "" && p.BlendMode != "NORMAL" && p.BlendMode != "PASS_THROUGH" {
		f.BlendMode = BlendMode(p.BlendMode)
	}
	opacity := 1.0
	if p.Opacity != nil {
		opacity = *p.Opacity
	}
	switch {
	case p.Type == "SOLID":
		f.Type = "solid"
		c := Color{A: 1}
		if p.Color != nil {
			c = FromFigma(*p.Color)
		}
		c = c.WithOpacity(opacity)
		f.Hex = c.Hex()
		f.Hex8 = c.Hex8()
		f.RGBA = c.RGBA()
	case strings.HasPrefix(p.Type, "GRADIENT_"):
		f.Type = "gradient"
		f.Gradient = r.Gradient(p)
		if f.Gradient == nil {
			return nil
		}
	case p.Type == "IMAGE", p.Type == "VIDEO":
		f.Type = "image"
		f.ImageRef = p.ImageRef
		if p.ImageRef == "" {
			f.ImageRef = p.GifRef
		}
		f.ScaleMode = strings.ToLower(p.ScaleMode)
	case p.Type == "PATTERN":
		f.Type = "pattern"
		f.ImageRef = p.SourceNodeID
	default:
		f.Type = strings.ToLower(p.Type)
	}
	if a, ok := p.BoundVariables["color"]; ok {
		f.Token = r.Token(a.ID)
	}
	return f
}

// BlendMode converts a Figma blend mode to the CSS mix-blend-mode value.
func BlendMode(mode string) string {
	switch mode {
	case "", "NORMAL", "PASS_THROUGH":
		return ""
	case "LINEAR_BURN":
		return "plus-darker"
	case "LINEAR_DODGE":
		return "plus-lighter"
	}
	return strings.ToLower(strings.ReplaceAll(mode, "_", "-"))
}

// Effects converts the visible effects of a node into the model with CSS
// declarations: box-shadow for shadows (inset for inner shadows), filter
// blur for layer blurs, and backdrop-filter blur for background blurs.
// Figma's blur radius is twice the CSS blur value.
func (r *Resolver) Effects(effects []figma.Effect, style *model.StyleRef) []model.Effect {
	out := make([]model.Effect, 0, len(effects))
	for _, e := range effects {
		if !e.IsVisible() {
			continue
		}
		m := r.Effect(e)
		if m == nil {
			continue
		}
		m.Style = style
		out = append(out, *m)
	}
	return out
}

// Effect converts one effect.
func (r *Resolver) Effect(e figma.Effect) *model.Effect {
	m := &model.Effect{}
	if e.Radius != nil {
		m.Radius = *e.Radius
	}
	if a, ok := e.BoundVariables["color"]; ok {
		m.Token = r.Token(a.ID)
	}
	switch e.Type {
	case "DROP_SHADOW", "INNER_SHADOW":
		m.Type = "drop-shadow"
		if e.Type == "INNER_SHADOW" {
			m.Type = "inner-shadow"
		}
		m.Property = "box-shadow"
		c := Color{A: 1}
		if e.Color != nil {
			c = FromFigma(*e.Color)
		}
		m.Hex = c.Hex()
		m.RGBA = c.RGBA()
		x, y := 0.0, 0.0
		if e.Offset != nil {
			x, y = e.Offset.X, e.Offset.Y
		}
		m.Offset = &model.Point{X: x, Y: y}
		if e.Spread != nil {
			m.Spread = *e.Spread
		}
		parts := []string{}
		if e.Type == "INNER_SHADOW" {
			parts = append(parts, "inset")
		}
		parts = append(parts, Px(x), Px(y), Px(m.Radius))
		if m.Spread != 0 {
			parts = append(parts, Px(m.Spread))
		}
		parts = append(parts, c.CSS())
		m.CSS = strings.Join(parts, " ")
	case "LAYER_BLUR", "BACKGROUND_BLUR":
		m.Type = "layer-blur"
		m.Property = "filter"
		if e.Type == "BACKGROUND_BLUR" {
			m.Type = "background-blur"
			m.Property = "backdrop-filter"
		}
		m.CSS = "blur(" + Px(m.Radius/2) + ")"
	default:
		return nil
	}
	return m
}

// Grids converts layout grids.
func (r *Resolver) Grids(grids []figma.LayoutGrid, style *model.StyleRef) []model.LayoutGrid {
	out := make([]model.LayoutGrid, 0, len(grids))
	for _, g := range grids {
		out = append(out, model.LayoutGrid{
			Pattern:     strings.ToLower(g.Pattern),
			Count:       g.Count,
			Gutter:      g.GutterSize,
			Offset:      g.Offset,
			SectionSize: g.SectionSize,
			Alignment:   strings.ToLower(g.Alignment),
			Visible:     g.Visible,
			Style:       style,
		})
	}
	return out
}
