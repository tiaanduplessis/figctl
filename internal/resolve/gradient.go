package resolve

import (
	"math"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

// GradientAngle converts Figma gradient handles (start and end points in
// the node's unit square, y down) into the CSS angle where 0deg points
// up and 90deg points right. Missing handles give 180deg (top to
// bottom), Figma's default.
func GradientAngle(handles []figma.Vector) float64 {
	if len(handles) < 2 {
		return 180
	}
	dx := handles[1].X - handles[0].X
	dy := handles[1].Y - handles[0].Y
	if dx == 0 && dy == 0 {
		return 180
	}
	deg := math.Atan2(dx, -dy) * 180 / math.Pi
	deg = math.Mod(deg+360, 360)
	return Round(deg, 2)
}

// Gradient converts a gradient paint into CSS. Linear gradients are
// exact in angle and stops; radial, angular, and diamond gradients are
// approximations and flagged as such. Stops carry the token bound to
// their color when the resolver knows it.
func (r *Resolver) Gradient(p figma.Paint) *model.Gradient {
	opacity := 1.0
	if p.Opacity != nil {
		opacity = *p.Opacity
	}
	g := &model.Gradient{Stops: make([]model.GradientStop, 0, len(p.GradientStops))}
	for _, s := range p.GradientStops {
		c := FromFigma(s.Color).WithOpacity(opacity)
		stop := model.GradientStop{
			Position: Round(s.Position, 4),
			Hex:      c.Hex(),
			RGBA:     c.RGBA(),
			CSS:      c.CSS() + " " + Num(s.Position*100, 2) + "%",
		}
		if alias, ok := s.BoundVariables["color"]; ok {
			stop.Token = r.Token(alias.ID)
		}
		g.Stops = append(g.Stops, stop)
	}
	stops := make([]string, 0, len(g.Stops))
	for _, s := range g.Stops {
		stops = append(stops, s.CSS)
	}
	list := strings.Join(stops, ", ")
	h := p.GradientHandlePositions
	switch p.Type {
	case "GRADIENT_LINEAR":
		g.Type = "linear"
		g.Angle = GradientAngle(h)
		g.CSS = "linear-gradient(" + Num(g.Angle, 2) + "deg, " + list + ")"
	case "GRADIENT_RADIAL", "GRADIENT_DIAMOND":
		g.Type = "radial"
		if p.Type == "GRADIENT_DIAMOND" {
			g.Type = "diamond"
		}
		g.Approximate = true
		g.Angle = GradientAngle(h)
		cx, cy, rx, ry := 0.5, 0.5, 0.5, 0.5
		if len(h) >= 3 {
			cx, cy = h[0].X, h[0].Y
			rx = math.Hypot(h[1].X-h[0].X, h[1].Y-h[0].Y)
			ry = math.Hypot(h[2].X-h[0].X, h[2].Y-h[0].Y)
		}
		g.CSS = "radial-gradient(" + Num(rx*100, 2) + "% " + Num(ry*100, 2) + "% at " + Num(cx*100, 2) + "% " + Num(cy*100, 2) + "%, " + list + ")"
	case "GRADIENT_ANGULAR":
		g.Type = "angular"
		g.Approximate = true
		g.Angle = GradientAngle(h)
		cx, cy := 0.5, 0.5
		if len(h) >= 1 {
			cx, cy = h[0].X, h[0].Y
		}
		g.CSS = "conic-gradient(from " + Num(g.Angle, 2) + "deg at " + Num(cx*100, 2) + "% " + Num(cy*100, 2) + "%, " + list + ")"
	default:
		return nil
	}
	return g
}
