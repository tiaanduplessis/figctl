package inspect

import (
	"sort"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// sideAxis maps the side a measurement is pinned to onto the axis it measures
// along. A measurement runs between two parallel sides, so a pair that does
// not share an axis has no single distance.
var sideAxis = map[string]string{
	"TOP":    "vertical",
	"BOTTOM": "vertical",
	"LEFT":   "horizontal",
	"RIGHT":  "horizontal",
}

// Measurements returns the Dev Mode measurements that touch the given
// subtrees, with each gap resolved to pixels.
//
// Measurements are pinned on the page rather than on the nodes they measure,
// so the whole document is needed to find them: doc should be the file's
// DOCUMENT node. Passing no roots returns every measurement in the document.
func Measurements(doc *figma.Node, roots ...*figma.Node) []model.Measurement {
	if doc == nil {
		return nil
	}
	boxes, names, types := indexNodes(doc)

	// Only measurements with an end inside one of the requested subtrees are
	// relevant; a screen's measurements are noise when inspecting a
	// different screen on the same page.
	var wanted map[string]bool
	if len(roots) > 0 {
		wanted = map[string]bool{}
		for _, root := range roots {
			if root == nil {
				continue
			}
			root.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
				wanted[n.ID] = true
				return true
			})
		}
	}

	out := []model.Measurement{}
	doc.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
		for _, m := range n.Measurements {
			if wanted != nil && !wanted[m.Start.NodeID] && !wanted[m.End.NodeID] {
				continue
			}
			out = append(out, resolveMeasurement(m, boxes, names, types))
		}
		return true
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// indexNodes collects the bounding box, name, and type of every node once, so
// resolving many measurements does not walk the document repeatedly.
func indexNodes(doc *figma.Node) (map[string]*figma.Rectangle, map[string]string, map[string]string) {
	boxes := map[string]*figma.Rectangle{}
	names := map[string]string{}
	types := map[string]string{}
	doc.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
		if n.AbsoluteBoundingBox != nil {
			boxes[n.ID] = n.AbsoluteBoundingBox
		}
		names[n.ID] = n.Name
		types[n.ID] = n.Type
		return true
	})
	return boxes, names, types
}

func resolveMeasurement(m figma.Measurement, boxes map[string]*figma.Rectangle, names, types map[string]string) model.Measurement {
	out := model.Measurement{
		ID:       m.ID,
		FreeText: m.FreeText,
		Start:    pinOf(m.Start, names, types),
		End:      pinOf(m.End, names, types),
	}
	if m.Offset.Type != "" {
		out.Offset = &model.MeasurementOffset{
			Type: m.Offset.Type, Relative: m.Offset.Relative, Fixed: m.Offset.Fixed,
		}
	}

	startAxis, endAxis := sideAxis[m.Start.Side], sideAxis[m.End.Side]
	switch {
	case startAxis == "" || endAxis == "":
		out.Unresolved = "the measurement is pinned to an unrecognized side"
	case startAxis != endAxis:
		out.Unresolved = "the two ends are pinned to sides on different axes, so there is no single distance"
	default:
		out.Axis = startAxis
		start, startOK := boxes[m.Start.NodeID]
		end, endOK := boxes[m.End.NodeID]
		switch {
		case !startOK:
			out.Unresolved = "node " + m.Start.NodeID + " is not in the document, so the distance cannot be measured"
		case !endOK:
			out.Unresolved = "node " + m.End.NodeID + " is not in the document, so the distance cannot be measured"
		default:
			d := gap(sideCoordinate(start, m.Start.Side), sideCoordinate(end, m.End.Side))
			out.Distance = &d
			out.CSS = resolve.Px(d)
		}
	}

	// The designer's own text wins: they overrode the measured value on
	// purpose, and that override is the specification.
	switch {
	case strings.TrimSpace(m.FreeText) != "":
		out.Label = strings.TrimSpace(m.FreeText)
	case out.CSS != "":
		out.Label = out.CSS
	}
	return out
}

func pinOf(p figma.MeasurementStartEnd, names, types map[string]string) model.MeasurementPin {
	return model.MeasurementPin{
		NodeID: p.NodeID,
		Name:   names[p.NodeID],
		Type:   types[p.NodeID],
		Side:   p.Side,
	}
}

// sideCoordinate is the absolute position of one side of a box.
func sideCoordinate(b *figma.Rectangle, side string) float64 {
	switch side {
	case "TOP":
		return b.Y
	case "BOTTOM":
		return b.Y + b.Height
	case "LEFT":
		return b.X
	case "RIGHT":
		return b.X + b.Width
	}
	return 0
}

// gap is the distance between two coordinates, rounded the way the rest of
// the model rounds lengths. Direction does not matter: a measurement is a
// distance, not a vector.
func gap(a, b float64) float64 {
	d := b - a
	if d < 0 {
		d = -d
	}
	return resolve.Round(d, 2)
}
