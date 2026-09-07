package inspect

import (
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

func (in *inspector) styleRef(n *figma.Node, keys ...string) *model.StyleRef {
	for _, key := range keys {
		if id, ok := n.Styles[key]; ok && id != "" {
			return in.r.StyleRef(id)
		}
	}
	return nil
}

func (in *inspector) visual(n *figma.Node) *model.Visual {
	if n.Type == "CANVAS" || n.Type == "DOCUMENT" {
		return nil
	}
	v := &model.Visual{}
	v.Fills = in.r.Fills(n, "fills", n.Fills, in.styleRef(n, "fill", "fills"))
	if v.Fills == nil {
		v.Fills = []model.Fill{}
	}
	v.Strokes = in.r.Fills(n, "strokes", n.Strokes, in.styleRef(n, "stroke", "strokes"))
	if len(v.Strokes) > 0 {
		v.StrokeWeight = strokeWeight(n)
		v.StrokeAlign = strings.ToLower(n.StrokeAlign)
		v.StrokeDashes = n.StrokeDashes
	}
	v.Radius = in.radius(n)
	v.Effects = in.r.Effects(n.Effects, in.styleRef(n, "effect", "effects"))
	if n.Opacity != nil && *n.Opacity != 1 {
		o := resolve.Round(*n.Opacity, 4)
		v.Opacity = &o
	}
	v.BlendMode = resolve.BlendMode(n.BlendMode)
	v.LayoutGrids = in.r.Grids(n.LayoutGrids, in.styleRef(n, "grid", "grids"))
	v.IsMask = n.IsMask != nil && *n.IsMask
	return v
}

func strokeWeight(n *figma.Node) *model.Sides {
	if w := n.IndividualStrokeWeights; w != nil {
		return &model.Sides{Top: w.Top, Right: w.Right, Bottom: w.Bottom, Left: w.Left}
	}
	weight := 1.0
	if n.StrokeWeight != nil {
		weight = *n.StrokeWeight
	}
	return &model.Sides{Top: weight, Right: weight, Bottom: weight, Left: weight}
}

// radius uses rectangleCornerRadii when present, else cornerRadius. It is
// omitted when every corner is zero and nothing is bound.
func (in *inspector) radius(n *figma.Node) *model.Radius {
	var token *model.TokenRef
	for _, key := range []string{"topLeftRadius", "topRightRadius", "bottomRightRadius", "bottomLeftRadius", "cornerRadius"} {
		if t := in.r.Bound(n, key); t != nil {
			token = t
			break
		}
	}
	if len(n.RectangleCornerRadii) == 4 {
		var corners [4]float64
		copy(corners[:], n.RectangleCornerRadii)
		all := corners[0] == corners[1] && corners[1] == corners[2] && corners[2] == corners[3]
		if all {
			if corners[0] == 0 && token == nil {
				return nil
			}
			u := corners[0]
			return &model.Radius{Uniform: &u, Smoothing: smoothing(n), Token: token}
		}
		return &model.Radius{Corners: &corners, Smoothing: smoothing(n), Token: token}
	}
	if n.CornerRadius != nil && (*n.CornerRadius != 0 || token != nil) {
		u := *n.CornerRadius
		return &model.Radius{Uniform: &u, Smoothing: smoothing(n), Token: token}
	}
	if token != nil {
		u := 0.0
		return &model.Radius{Uniform: &u, Token: token}
	}
	return nil
}

func smoothing(n *figma.Node) float64 {
	if n.CornerSmoothing == nil {
		return 0
	}
	return resolve.Round(*n.CornerSmoothing, 4)
}
