package tokens

import (
	"strconv"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// styleToken converts one style into a token. The mapping is exact for
// the style types DTCG covers and explicit about the ones it does not:
//
//   - FILL with a single solid paint becomes color.
//   - FILL with a linear gradient becomes the DTCG gradient type. The
//     gradient angle is not part of that type, so it is kept under
//     $extensions.com.figma.gradient together with the CSS declaration.
//   - FILL with a radial, angular, or diamond gradient is not
//     representable: those are approximations even in CSS, and a colour
//     fallback would silently change the design, so the token is emitted
//     as a string holding the CSS gradient with a $description saying so.
//   - FILL with an image or pattern paint is emitted as a string holding
//     the image reference, which is all the REST API exposes.
//   - TEXT becomes the typography composite.
//   - EFFECT with shadows becomes shadow, an array when there are
//     several. Blur effects have no DTCG type, so a blur only style is
//     emitted as a string holding the CSS filter value with a
//     $description; blurs alongside shadows are kept under
//     $extensions.com.figma.filters.
//   - GRID has no DTCG equivalent at all. Rather than dropping layout
//     grids the style is emitted as a string summary with the full grid
//     definition under $extensions.com.figma.grid, so a build step can
//     still read the column count, gutter, and margin.
func (b *builder) styleToken(s resolve.Style) *Token {
	path := NormalizePath(s.Name, b.opts.NameCase)
	t := &Token{
		Name:        strings.Join(path, "."),
		Path:        path,
		Source:      s.Name,
		Description: s.Description,
		Kind:        KindStyle,
		Collection:  StyleCollection,
		ID:          s.ID,
		CSSName:     cssName(path),
	}
	ok := false
	switch s.Type {
	case "FILL":
		ok = b.fillToken(t, s)
	case "TEXT":
		ok = b.textToken(t, s)
	case "EFFECT":
		ok = b.effectToken(t, s)
	case "GRID":
		ok = b.gridToken(t, s)
	default:
		b.warn("style %s has unsupported type %s", s.Name, s.Type)
	}
	if !ok {
		return nil
	}
	b.extend(t, "styleId", s.ID)
	b.extend(t, "styleType", s.Type)
	if s.Key != "" {
		b.extend(t, "styleKey", s.Key)
	}
	t.Category = category(t)
	return t
}

func (b *builder) fillToken(t *Token, s resolve.Style) bool {
	fills := s.Value.Fills
	if len(fills) == 0 {
		b.warn("fill style %s has no visible paint", s.Name)
		return false
	}
	if len(fills) > 1 {
		extra := make([]string, 0, len(fills)-1)
		for _, f := range fills[1:] {
			extra = append(extra, f.Type)
		}
		b.extend(t, "additionalPaints", extra)
		b.warn("fill style %s has %d paints; only the first is exported", s.Name, len(fills))
	}
	f := fills[0]
	switch f.Type {
	case "solid":
		t.Type = TypeColor
		t.Value = solidHex(f)
		t.CSS = t.Value.(string)
		return true
	case "gradient":
		return b.gradientToken(t, s, f)
	default:
		t.Type = TypeString
		t.Value = f.ImageRef
		t.CSS = f.ImageRef
		t.Description = joinDescription(t.Description, "Figma "+f.Type+" paint; DTCG has no image paint type, so the value is the Figma image reference.")
		return f.ImageRef != ""
	}
}

func (b *builder) gradientToken(t *Token, s resolve.Style, f model.Fill) bool {
	g := f.Gradient
	if g == nil {
		b.warn("gradient style %s could not be converted", s.Name)
		return false
	}
	b.extend(t, "gradient", map[string]any{"type": g.Type, "angle": g.Angle, "css": g.CSS})
	if g.Type != "linear" || g.Approximate {
		t.Type = TypeString
		t.Value = g.CSS
		t.CSS = g.CSS
		t.Description = joinDescription(t.Description,
			"A "+g.Type+" gradient is not representable as a DTCG gradient, and a flat colour would change the design, so the value is the approximate CSS gradient.")
		return true
	}
	stops := make([]GradientStop, 0, len(g.Stops))
	for _, st := range g.Stops {
		stops = append(stops, GradientStop{Color: hexWithAlpha(st.Hex, st.RGBA), Position: resolve.Round(st.Position, 4)})
	}
	t.Type = TypeGradient
	t.Value = stops
	t.CSS = g.CSS
	t.Description = joinDescription(t.Description, "The DTCG gradient type carries stops only; the angle is under $extensions.")
	return true
}

func (b *builder) textToken(t *Token, s resolve.Style) bool {
	if s.Value.Typography == nil {
		b.warn("text style %s has no typography", s.Name)
		return false
	}
	typo := typographyOf(s.Value.Typography)
	t.Type = TypeTypography
	t.Value = typo
	t.Parts = typographyParts(typo)
	return len(t.Parts) > 0
}

func (b *builder) effectToken(t *Token, s resolve.Style) bool {
	var shadows []Shadow
	var filters []string
	var css []string
	// filterValues keeps blur values grouped by the CSS property they apply
	// to, so the CSS custom property can hold a usable value rather than a
	// whole declaration.
	filterValues := map[string][]string{}
	var filterProps []string
	for _, e := range s.Value.Effects {
		switch e.Property {
		case "box-shadow":
			shadows = append(shadows, shadowOf(e))
			css = append(css, e.CSS)
		default:
			filters = append(filters, e.Property+": "+e.CSS)
			if _, ok := filterValues[e.Property]; !ok {
				filterProps = append(filterProps, e.Property)
			}
			filterValues[e.Property] = append(filterValues[e.Property], e.CSS)
		}
	}
	switch {
	case len(shadows) == 1:
		t.Type = TypeShadow
		t.Value = shadows[0]
	case len(shadows) > 1:
		t.Type = TypeShadow
		t.Value = shadows
	case len(filters) > 0:
		t.Type = TypeString
		t.Value = strings.Join(filters, "; ")
		t.Description = joinDescription(t.Description, "Blur effects have no DTCG type, so the value is the CSS declaration.")
		// A custom property can only carry a value, never a property name, so
		// emit one only when every blur targets the same CSS property. The
		// declaration form stays in $value for DTCG consumers.
		if len(filterProps) == 1 {
			prop := filterProps[0]
			t.CSS = strings.Join(filterValues[prop], " ")
			b.extend(t, "cssProperty", prop)
		} else {
			b.warn("effect style %s mixes %s, which cannot be one CSS custom property; it is exported to DTCG only",
				s.Name, strings.Join(filterProps, " and "))
		}
		return true
	default:
		b.warn("effect style %s has no visible effect", s.Name)
		return false
	}
	t.CSS = strings.Join(css, ", ")
	if len(filters) > 0 {
		b.extend(t, "filters", filters)
	}
	return true
}

func (b *builder) gridToken(t *Token, s resolve.Style) bool {
	if len(s.Value.Grids) == 0 {
		b.warn("grid style %s has no layout grid", s.Name)
		return false
	}
	grids := make([]map[string]any, 0, len(s.Value.Grids))
	for _, g := range s.Value.Grids {
		grids = append(grids, map[string]any{
			"pattern":     g.Pattern,
			"count":       g.Count,
			"gutter":      g.Gutter,
			"offset":      g.Offset,
			"sectionSize": g.SectionSize,
			"alignment":   g.Alignment,
		})
	}
	b.extend(t, "grid", grids)
	g := s.Value.Grids[0]
	summary := g.Pattern
	if g.Count > 0 {
		summary += " " + resolve.Num(g.Count, 0)
	}
	summary += " gutter " + resolve.Px(g.Gutter) + " margin " + resolve.Px(g.Offset)
	t.Type = TypeString
	t.Value = summary
	t.CSS = summary
	t.Description = joinDescription(t.Description, "DTCG has no layout grid type; the grid definition is under $extensions.com.figma.grid.")
	if g.Count > 0 {
		t.Parts = append(t.Parts, Part{Suffix: "columns", Value: strconv.Itoa(int(g.Count))})
	}
	t.Parts = append(t.Parts,
		Part{Suffix: "gutter", Value: resolve.Px(g.Gutter)},
		Part{Suffix: "margin", Value: resolve.Px(g.Offset)},
	)
	return true
}

func shadowOf(e model.Effect) Shadow {
	x, y := 0.0, 0.0
	if e.Offset != nil {
		x, y = e.Offset.X, e.Offset.Y
	}
	return Shadow{
		Color:   hexWithAlpha(e.Hex, e.RGBA),
		OffsetX: Dimension{Value: resolve.Round(x, 4), Unit: "px"},
		OffsetY: Dimension{Value: resolve.Round(y, 4), Unit: "px"},
		Blur:    Dimension{Value: resolve.Round(e.Radius, 4), Unit: "px"},
		Spread:  Dimension{Value: resolve.Round(e.Spread, 4), Unit: "px"},
		Inset:   e.Type == "inner-shadow",
	}
}

func solidHex(f model.Fill) string {
	if f.Hex8 != "" {
		return f.Hex8
	}
	return f.Hex
}

// hexWithAlpha rebuilds #rrggbbaa from the opaque hex and the rgba text
// the resolver produces, so a translucent paint keeps its alpha.
func hexWithAlpha(hex, rgba string) string {
	c, ok := resolve.ParseHex(hex)
	if !ok {
		return hex
	}
	if a, found := alphaOf(rgba); found {
		c.A = a
	}
	if h := c.Hex8(); h != "" {
		return h
	}
	return c.Hex()
}

// alphaOf reads the alpha channel out of an "rgba(r, g, b, a)" string.
func alphaOf(rgba string) (float64, bool) {
	open := strings.LastIndex(rgba, "(")
	closing := strings.LastIndex(rgba, ")")
	if open < 0 || closing <= open {
		return 0, false
	}
	parts := strings.Split(rgba[open+1:closing], ",")
	if len(parts) != 4 {
		return 0, false
	}
	a, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err != nil {
		return 0, false
	}
	return a, true
}

func joinDescription(existing, added string) string {
	if existing == "" {
		return added
	}
	return existing + " " + added
}
