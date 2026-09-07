package inspect

import (
	"strconv"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

func px(v float64) string { return resolve.Px(v) }

// tokenOrValue returns var(--name) when the token has WEB code syntax,
// otherwise the raw value.
func tokenOrValue(t *model.TokenRef, raw string) string {
	if v := resolve.CSSVar(t); v != "" {
		return v
	}
	return raw
}

// CSS builds a flat map of CSS declarations for a node from its model.
// It is a starting point, not framework code: sizing and positioning
// depend on the parent's layout mode.
func CSS(m *model.Node, parent parentInfo) map[string]string {
	css := map[string]string{}
	layoutCSS(css, m, parent)
	visualCSS(css, m)
	textCSS(css, m)
	if len(css) == 0 {
		return nil
	}
	return css
}

func layoutCSS(css map[string]string, m *model.Node, parent parentInfo) {
	l := m.Layout
	if l == nil {
		return
	}
	switch l.Mode {
	case "row", "column", "wrap":
		css["display"] = "flex"
		css["flex-direction"] = "row"
		if l.Mode == "column" {
			css["flex-direction"] = "column"
		}
		if l.Mode == "wrap" {
			css["flex-wrap"] = "wrap"
		}
		if l.Gap != nil {
			css["gap"] = tokenOrValue(l.Tokens["gap"], px(*l.Gap))
			if l.RowGap != nil && *l.RowGap != *l.Gap {
				css["row-gap"] = tokenOrValue(l.Tokens["rowGap"], px(*l.RowGap))
				css["column-gap"] = css["gap"]
				delete(css, "gap")
			}
		}
		if l.Justify != "" {
			css["justify-content"] = l.Justify
		}
		if l.Align != "" {
			css["align-items"] = l.Align
		}
		if l.AlignContent != "" {
			css["align-content"] = l.AlignContent
		}
	case "grid":
		css["display"] = "grid"
		if g := l.Grid; g != nil {
			css["grid-template-rows"] = gridTemplate(g.RowsSizing, g.Rows)
			css["grid-template-columns"] = gridTemplate(g.ColumnsSizing, g.Columns)
			css["row-gap"] = tokenOrValue(l.Tokens["rowGap"], px(g.RowGap))
			css["column-gap"] = tokenOrValue(l.Tokens["columnGap"], px(g.ColumnGap))
		}
	}
	if p := l.Padding; p != nil && (p.Top != 0 || p.Right != 0 || p.Bottom != 0 || p.Left != 0) {
		css["padding"] = paddingCSS(p, l.Tokens)
	}
	if s := l.Sizing; s != nil {
		axisCSS(css, "width", s.Horizontal, parent, "row")
		axisCSS(css, "height", s.Vertical, parent, "column")
	}
	if l.MinWidth != nil {
		css["min-width"] = px(*l.MinWidth)
	}
	if l.MaxWidth != nil {
		css["max-width"] = px(*l.MaxWidth)
	}
	if l.MinHeight != nil {
		css["min-height"] = px(*l.MinHeight)
	}
	if l.MaxHeight != nil {
		css["max-height"] = px(*l.MaxHeight)
	}
	if l.Position == "absolute" {
		css["position"] = "absolute"
		if l.Offset != nil {
			css["left"] = px(l.Offset.X)
			css["top"] = px(l.Offset.Y)
		}
	}
	if l.Clips {
		css["overflow"] = "hidden"
	}
	switch l.Overflow {
	case "scroll-x":
		css["overflow-x"] = "auto"
	case "scroll-y":
		css["overflow-y"] = "auto"
	case "scroll":
		css["overflow"] = "auto"
	}
	if g := l.GridChild; g != nil {
		css["grid-row"] = strconv.Itoa(g.Row+1) + " / span " + strconv.Itoa(g.RowSpan)
		css["grid-column"] = strconv.Itoa(g.Column+1) + " / span " + strconv.Itoa(g.ColumnSpan)
		if g.AlignH != "" {
			css["justify-self"] = g.AlignH
		}
		if g.AlignV != "" {
			css["align-self"] = g.AlignV
		}
	}
}

// axisCSS emits width/height for fixed sizing, flex or align-self for
// fill depending on the parent's direction, and nothing for hug.
func axisCSS(css map[string]string, prop string, axis model.Axis, parent parentInfo, primaryOf string) {
	switch axis.Mode {
	case "fixed":
		css[prop] = px(axis.Px)
	case "fill":
		parentDir := parent.mode
		if parentDir == "wrap" {
			parentDir = "row"
		}
		switch parentDir {
		case primaryOf:
			css["flex"] = "1 0 0"
		case "row", "column":
			css["align-self"] = "stretch"
		case "grid":
			return
		default:
			css[prop] = "100%"
		}
	}
}

func gridTemplate(sizing string, count int) string {
	if strings.TrimSpace(sizing) != "" {
		return sizing
	}
	if count <= 0 {
		return "none"
	}
	return "repeat(" + strconv.Itoa(count) + ", minmax(0, 1fr))"
}

func paddingCSS(p *model.Padding, tokens map[string]*model.TokenRef) string {
	t := tokenOrValue(tokens["paddingTop"], px(p.Top))
	r := tokenOrValue(tokens["paddingRight"], px(p.Right))
	b := tokenOrValue(tokens["paddingBottom"], px(p.Bottom))
	l := tokenOrValue(tokens["paddingLeft"], px(p.Left))
	switch {
	case t == r && r == b && b == l:
		return t
	case t == b && l == r:
		return t + " " + r
	case l == r:
		return t + " " + r + " " + b
	}
	return t + " " + r + " " + b + " " + l
}

func visualCSS(css map[string]string, m *model.Node) {
	v := m.Visual
	if v == nil {
		return
	}
	if len(v.Fills) > 0 {
		prop := "background"
		if m.Type == "TEXT" {
			prop = "color"
		}
		if val := fillsCSS(v.Fills, m.Type == "TEXT"); val != "" {
			if m.Type != "TEXT" && (strings.Contains(val, "gradient(") || strings.Contains(val, "url(")) {
				prop = "background-image"
			}
			css[prop] = val
			if m.Type != "TEXT" {
				if size := imageSize(v.Fills); size != "" {
					css["background-size"] = size
				}
			}
		}
	}
	if len(v.Strokes) > 0 && v.StrokeWeight != nil {
		color := fillCSS(v.Strokes[0])
		style := "solid"
		if len(v.StrokeDashes) > 0 {
			style = "dashed"
		}
		if v.StrokeWeight.Uniform() {
			css["border"] = px(v.StrokeWeight.Top) + " " + style + " " + color
		} else {
			for side, w := range map[string]float64{"top": v.StrokeWeight.Top, "right": v.StrokeWeight.Right, "bottom": v.StrokeWeight.Bottom, "left": v.StrokeWeight.Left} {
				if w > 0 {
					css["border-"+side] = px(w) + " " + style + " " + color
				}
			}
		}
		if v.StrokeAlign == "inside" {
			css["box-sizing"] = "border-box"
		}
	}
	if r := v.Radius; r != nil {
		if r.Uniform != nil {
			css["border-radius"] = tokenOrValue(r.Token, px(*r.Uniform))
		} else if r.Corners != nil {
			c := r.Corners
			css["border-radius"] = px(c[0]) + " " + px(c[1]) + " " + px(c[2]) + " " + px(c[3])
		}
	}
	byProp := map[string][]string{}
	for _, e := range v.Effects {
		val := e.CSS
		if e.Property == "box-shadow" && e.Token != nil {
			if variable := resolve.CSSVar(e.Token); variable != "" {
				val = strings.Replace(val, e.Hex, variable, 1)
				val = strings.Replace(val, e.RGBA, variable, 1)
			}
		}
		byProp[e.Property] = append(byProp[e.Property], val)
	}
	for prop, vals := range byProp {
		sep := ", "
		if prop != "box-shadow" {
			sep = " "
		}
		css[prop] = strings.Join(vals, sep)
	}
	if v.Opacity != nil {
		css["opacity"] = resolve.Num(*v.Opacity, 4)
	}
	if v.BlendMode != "" {
		css["mix-blend-mode"] = v.BlendMode
	}
}

// fillsCSS layers visible fills into one background value, top paint
// first as CSS expects. Text nodes use only the top solid color.
func fillsCSS(fills []model.Fill, textColor bool) string {
	if textColor {
		for i := len(fills) - 1; i >= 0; i-- {
			if fills[i].Type == "solid" {
				return fillCSS(fills[i])
			}
		}
		return ""
	}
	var layers []string
	for i := len(fills) - 1; i >= 0; i-- {
		if val := fillCSS(fills[i]); val != "" {
			layers = append(layers, val)
		}
	}
	return strings.Join(layers, ", ")
}

func fillCSS(f model.Fill) string {
	switch f.Type {
	case "solid":
		raw := f.Hex
		if f.RGBA != "" && !strings.HasSuffix(f.RGBA, ", 1)") {
			raw = f.RGBA
		}
		return tokenOrValue(f.Token, raw)
	case "gradient":
		if f.Gradient != nil {
			return f.Gradient.CSS
		}
	case "image":
		if f.ImageRef != "" {
			return "url(" + f.ImageRef + ")"
		}
	}
	return ""
}

func imageSize(fills []model.Fill) string {
	for _, f := range fills {
		if f.Type != "image" {
			continue
		}
		switch f.ScaleMode {
		case "fill":
			return "cover"
		case "fit":
			return "contain"
		case "tile":
			return "auto"
		}
	}
	return ""
}

func textCSS(css map[string]string, m *model.Node) {
	t := m.Text
	if t == nil {
		return
	}
	if t.FontFamily != "" {
		css["font-family"] = tokenOrValue(t.Tokens["fontFamily"], quoteFamily(t.FontFamily))
	}
	if t.Size != nil {
		css["font-size"] = tokenOrValue(t.Tokens["fontSize"], px(*t.Size))
	}
	if t.Weight != nil {
		css["font-weight"] = tokenOrValue(t.Tokens["fontWeight"], resolve.Num(*t.Weight, 0))
	}
	if t.Italic {
		css["font-style"] = "italic"
	}
	if lh := t.LineHeight; lh != nil {
		switch {
		case lh.Unit == "auto":
			css["line-height"] = "normal"
		case lh.Unit == "percent" && lh.Unitless != nil:
			css["line-height"] = tokenOrValue(t.Tokens["lineHeight"], resolve.Num(*lh.Unitless, 2))
		default:
			css["line-height"] = tokenOrValue(t.Tokens["lineHeight"], px(lh.Px))
		}
	}
	if ls := t.LetterSpacing; ls != nil && ls.Px != 0 {
		css["letter-spacing"] = tokenOrValue(t.Tokens["letterSpacing"], px(ls.Px))
	}
	switch t.Case {
	case "small-caps", "all-small-caps":
		css["font-variant-caps"] = t.Case
	case "":
	default:
		css["text-transform"] = t.Case
	}
	if t.Decoration != "" {
		css["text-decoration"] = t.Decoration
	}
	if t.AlignH != "" && t.AlignH != "left" {
		css["text-align"] = t.AlignH
	}
	if t.TabularNumbers {
		css["font-variant-numeric"] = "tabular-nums"
	}
	if t.Truncation == "ending" {
		css["overflow"] = "hidden"
		css["text-overflow"] = "ellipsis"
		if t.MaxLines != nil && *t.MaxLines > 1 {
			css["display"] = "-webkit-box"
			css["-webkit-box-orient"] = "vertical"
			css["-webkit-line-clamp"] = strconv.Itoa(*t.MaxLines)
		} else {
			css["white-space"] = "nowrap"
		}
	}
}

func quoteFamily(name string) string {
	if strings.ContainsAny(name, " ") {
		return `"` + name + `"`
	}
	return name
}
