package inspect

import (
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// hasAutoLayout reports whether a node lays out its children.
func hasAutoLayout(n *figma.Node) bool {
	return n != nil && n.LayoutMode != "" && n.LayoutMode != "NONE"
}

// layoutMode maps layoutMode and layoutWrap to none, row, column, wrap,
// or grid.
func layoutMode(n *figma.Node) string {
	switch n.LayoutMode {
	case "HORIZONTAL":
		if n.LayoutWrap == "WRAP" {
			return "wrap"
		}
		return "row"
	case "VERTICAL":
		return "column"
	case "GRID":
		return "grid"
	}
	return "none"
}

// parentLayout summarizes the parent for child sizing and CSS.
type parentInfo struct {
	mode string
}

func parentLayout(parent *figma.Node) parentInfo {
	if parent == nil {
		return parentInfo{mode: "none"}
	}
	return parentInfo{mode: layoutMode(parent)}
}

func alignValue(v string) string {
	switch v {
	case "MIN", "":
		return "flex-start"
	case "CENTER":
		return "center"
	case "MAX":
		return "flex-end"
	case "SPACE_BETWEEN":
		return "space-between"
	case "BASELINE":
		return "baseline"
	}
	return strings.ToLower(v)
}

func (in *inspector) layout(n, parent *figma.Node) *model.Layout {
	l := &model.Layout{Mode: layoutMode(n), Position: "auto"}
	if l.Mode != "none" {
		l.Tokens = map[string]*model.TokenRef{}
		if l.Mode == "grid" {
			l.Grid = &model.Grid{RowsSizing: n.GridRowsSizing, ColumnsSizing: n.GridColumnsSizing}
			if n.GridRowCount != nil {
				l.Grid.Rows = *n.GridRowCount
			}
			if n.GridColumnCount != nil {
				l.Grid.Columns = *n.GridColumnCount
			}
			if n.GridRowGap != nil {
				l.Grid.RowGap = *n.GridRowGap
			}
			if n.GridColumnGap != nil {
				l.Grid.ColumnGap = *n.GridColumnGap
			}
			l.Tokens["rowGap"] = in.r.Bound(n, "gridRowGap")
			l.Tokens["columnGap"] = in.r.Bound(n, "gridColumnGap")
		} else {
			gap := 0.0
			if n.ItemSpacing != nil {
				gap = *n.ItemSpacing
			}
			l.Gap = &gap
			l.Tokens["gap"] = in.r.Bound(n, "itemSpacing")
			if l.Mode == "wrap" {
				rowGap := gap
				if n.CounterAxisSpacing != nil {
					rowGap = *n.CounterAxisSpacing
				}
				l.RowGap = &rowGap
				l.Tokens["rowGap"] = in.r.Bound(n, "counterAxisSpacing")
			}
			l.Justify = alignValue(n.PrimaryAxisAlignItems)
			l.Align = alignValue(n.CounterAxisAlignItems)
			if n.CounterAxisAlignContent == "SPACE_BETWEEN" {
				l.AlignContent = "space-between"
			}
		}
		l.Padding = padding(n)
		for key, prop := range map[string]string{"paddingTop": "paddingTop", "paddingRight": "paddingRight", "paddingBottom": "paddingBottom", "paddingLeft": "paddingLeft"} {
			l.Tokens[key] = in.r.Bound(n, prop)
		}
		l.ReverseZIndex = n.ItemReverseZIndex != nil && *n.ItemReverseZIndex
		l.StrokesInLayout = n.StrokesIncludedInLayout != nil && *n.StrokesIncludedInLayout
	}
	l.Sizing = sizing(n, parent)
	l.MinWidth, l.MaxWidth, l.MinHeight, l.MaxHeight = n.MinWidth, n.MaxWidth, n.MinHeight, n.MaxHeight
	if n.LayoutGrow != nil && *n.LayoutGrow != 0 {
		g := *n.LayoutGrow
		l.Grow = &g
	}
	if n.LayoutAlign != "" && n.LayoutAlign != "INHERIT" {
		l.LayoutAlign = strings.ToLower(n.LayoutAlign)
	}
	if n.ClipsContent != nil && *n.ClipsContent {
		l.Clips = true
	}
	l.Overflow = overflow(n.OverflowDirection)
	if parent != nil && parent.Type != "CANVAS" && parent.Type != "DOCUMENT" {
		if !hasAutoLayout(parent) || n.LayoutPositioning == "ABSOLUTE" {
			l.Position = "absolute"
			if n.AbsoluteBoundingBox != nil && parent.AbsoluteBoundingBox != nil {
				l.Offset = &model.Point{
					X: resolve.Round(n.AbsoluteBoundingBox.X-parent.AbsoluteBoundingBox.X, 2),
					Y: resolve.Round(n.AbsoluteBoundingBox.Y-parent.AbsoluteBoundingBox.Y, 2),
				}
			}
			if n.Constraints != nil {
				l.Constraints = &model.Constraints{Horizontal: strings.ToLower(n.Constraints.Horizontal), Vertical: strings.ToLower(n.Constraints.Vertical)}
			}
		}
		if parent.LayoutMode == "GRID" && n.LayoutPositioning != "ABSOLUTE" {
			l.GridChild = gridChild(n)
		}
	}
	return l
}

func padding(n *figma.Node) *model.Padding {
	val := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}
	p := &model.Padding{Top: val(n.PaddingTop), Right: val(n.PaddingRight), Bottom: val(n.PaddingBottom), Left: val(n.PaddingLeft)}
	p.Shorthand = paddingShorthand(p.Top, p.Right, p.Bottom, p.Left)
	return p
}

func paddingShorthand(t, r, b, l float64) string {
	switch {
	case t == r && r == b && b == l:
		return resolve.Px(t)
	case t == b && l == r:
		return resolve.Px(t) + " " + resolve.Px(r)
	case l == r:
		return resolve.Px(t) + " " + resolve.Px(r) + " " + resolve.Px(b)
	}
	return resolve.Px(t) + " " + resolve.Px(r) + " " + resolve.Px(b) + " " + resolve.Px(l)
}

// sizing maps layoutSizingHorizontal/Vertical to fixed, hug, or fill,
// falling back to the axis sizing modes of the node's own auto layout and
// to layoutGrow and layoutAlign for children of older files.
func sizing(n, parent *figma.Node) *model.Sizing {
	w, h := 0.0, 0.0
	if n.AbsoluteBoundingBox != nil {
		w, h = n.AbsoluteBoundingBox.Width, n.AbsoluteBoundingBox.Height
	}
	s := &model.Sizing{
		Horizontal: model.Axis{Mode: sizingMode(n.LayoutSizingHorizontal), Px: w},
		Vertical:   model.Axis{Mode: sizingMode(n.LayoutSizingVertical), Px: h},
	}
	if s.Horizontal.Mode == "" || s.Vertical.Mode == "" {
		fh, fv := fallbackSizing(n, parent)
		if s.Horizontal.Mode == "" {
			s.Horizontal.Mode = fh
		}
		if s.Vertical.Mode == "" {
			s.Vertical.Mode = fv
		}
	}
	return s
}

func sizingMode(v string) string {
	switch v {
	case "FIXED":
		return "fixed"
	case "HUG":
		return "hug"
	case "FILL":
		return "fill"
	}
	return ""
}

func fallbackSizing(n, parent *figma.Node) (string, string) {
	h, v := "fixed", "fixed"
	// The node's own auto layout: AUTO sizing means hug on that axis.
	switch n.LayoutMode {
	case "HORIZONTAL":
		if n.PrimaryAxisSizingMode == "AUTO" {
			h = "hug"
		}
		if n.CounterAxisSizingMode == "AUTO" {
			v = "hug"
		}
	case "VERTICAL":
		if n.PrimaryAxisSizingMode == "AUTO" {
			v = "hug"
		}
		if n.CounterAxisSizingMode == "AUTO" {
			h = "hug"
		}
	}
	if n.Type == "TEXT" && n.Style != nil {
		switch n.Style.TextAutoResize {
		case "WIDTH_AND_HEIGHT":
			h, v = "hug", "hug"
		case "HEIGHT":
			v = "hug"
		}
	}
	// A child of an auto layout parent: grow fills the primary axis and
	// STRETCH fills the counter axis.
	if hasAutoLayout(parent) {
		grow := n.LayoutGrow != nil && *n.LayoutGrow > 0
		stretch := n.LayoutAlign == "STRETCH"
		switch parent.LayoutMode {
		case "HORIZONTAL":
			if grow {
				h = "fill"
			}
			if stretch {
				v = "fill"
			}
		case "VERTICAL":
			if grow {
				v = "fill"
			}
			if stretch {
				h = "fill"
			}
		}
	}
	return h, v
}

func overflow(direction string) string {
	switch direction {
	case "HORIZONTAL_SCROLLING":
		return "scroll-x"
	case "VERTICAL_SCROLLING":
		return "scroll-y"
	case "HORIZONTAL_AND_VERTICAL_SCROLLING":
		return "scroll"
	}
	return ""
}

func gridChild(n *figma.Node) *model.GridChild {
	g := &model.GridChild{RowSpan: 1, ColumnSpan: 1}
	if n.GridRowSpan != nil {
		g.RowSpan = *n.GridRowSpan
	}
	if n.GridColumnSpan != nil {
		g.ColumnSpan = *n.GridColumnSpan
	}
	if n.GridRowAnchorIndex != nil {
		g.Row = *n.GridRowAnchorIndex
	}
	if n.GridColumnAnchorIndex != nil {
		g.Column = *n.GridColumnAnchorIndex
	}
	g.AlignH = gridAlign(n.GridChildHorizontalAlign)
	g.AlignV = gridAlign(n.GridChildVerticalAlign)
	return g
}

func gridAlign(v string) string {
	switch v {
	case "MIN":
		return "start"
	case "CENTER":
		return "center"
	case "MAX":
		return "end"
	case "STRETCH":
		return "stretch"
	case "AUTO", "":
		return ""
	}
	return strings.ToLower(v)
}
