package figma

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Node is the document node model. Figma describes nodes as a discriminated
// union over the "type" property, with each type carrying a subset of
// traits. This CLI uses one struct with every property it reads, so a node
// of any type decodes without a type switch. Properties absent from the
// response are nil or zero and are omitted again when re-encoded.
type Node struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Visible  *bool    `json:"visible,omitempty"`
	Locked   *bool    `json:"locked,omitempty"`
	IsFixed  *bool    `json:"isFixed,omitempty"`
	Rotation *float64 `json:"rotation,omitempty"`

	ScrollBehavior              string                     `json:"scrollBehavior,omitempty"`
	ComponentPropertyReferences map[string]string          `json:"componentPropertyReferences,omitempty"`
	PluginData                  json.RawMessage            `json:"pluginData,omitempty"`
	SharedPluginData            json.RawMessage            `json:"sharedPluginData,omitempty"`
	BoundVariables              map[string]VariableBinding `json:"boundVariables,omitempty"`
	ExplicitVariableModes       map[string]string          `json:"explicitVariableModes,omitempty"`

	Children []*Node `json:"children,omitempty"`

	// Layout (HasLayoutTrait).
	AbsoluteBoundingBox      *Rectangle        `json:"absoluteBoundingBox,omitempty"`
	AbsoluteRenderBounds     *Rectangle        `json:"absoluteRenderBounds,omitempty"`
	PreserveRatio            *bool             `json:"preserveRatio,omitempty"`
	Constraints              *LayoutConstraint `json:"constraints,omitempty"`
	RelativeTransform        Transform         `json:"relativeTransform,omitempty"`
	Size                     *Vector           `json:"size,omitempty"`
	LayoutAlign              string            `json:"layoutAlign,omitempty"`
	LayoutGrow               *float64          `json:"layoutGrow,omitempty"`
	LayoutPositioning        string            `json:"layoutPositioning,omitempty"`
	MinWidth                 *float64          `json:"minWidth,omitempty"`
	MaxWidth                 *float64          `json:"maxWidth,omitempty"`
	MinHeight                *float64          `json:"minHeight,omitempty"`
	MaxHeight                *float64          `json:"maxHeight,omitempty"`
	LayoutSizingHorizontal   string            `json:"layoutSizingHorizontal,omitempty"`
	LayoutSizingVertical     string            `json:"layoutSizingVertical,omitempty"`
	GridRowCount             *int              `json:"gridRowCount,omitempty"`
	GridColumnCount          *int              `json:"gridColumnCount,omitempty"`
	GridRowGap               *float64          `json:"gridRowGap,omitempty"`
	GridColumnGap            *float64          `json:"gridColumnGap,omitempty"`
	GridColumnsSizing        string            `json:"gridColumnsSizing,omitempty"`
	GridRowsSizing           string            `json:"gridRowsSizing,omitempty"`
	GridChildHorizontalAlign string            `json:"gridChildHorizontalAlign,omitempty"`
	GridChildVerticalAlign   string            `json:"gridChildVerticalAlign,omitempty"`
	GridRowSpan              *int              `json:"gridRowSpan,omitempty"`
	GridColumnSpan           *int              `json:"gridColumnSpan,omitempty"`
	GridRowAnchorIndex       *int              `json:"gridRowAnchorIndex,omitempty"`
	GridColumnAnchorIndex    *int              `json:"gridColumnAnchorIndex,omitempty"`

	// Frame properties (HasFramePropertiesTrait), including auto layout.
	ClipsContent            *bool        `json:"clipsContent,omitempty"`
	Background              []Paint      `json:"background,omitempty"`
	BackgroundColor         *Color       `json:"backgroundColor,omitempty"`
	LayoutGrids             []LayoutGrid `json:"layoutGrids,omitempty"`
	OverflowDirection       string       `json:"overflowDirection,omitempty"`
	LayoutMode              string       `json:"layoutMode,omitempty"`
	PrimaryAxisSizingMode   string       `json:"primaryAxisSizingMode,omitempty"`
	CounterAxisSizingMode   string       `json:"counterAxisSizingMode,omitempty"`
	PrimaryAxisAlignItems   string       `json:"primaryAxisAlignItems,omitempty"`
	CounterAxisAlignItems   string       `json:"counterAxisAlignItems,omitempty"`
	CounterAxisAlignContent string       `json:"counterAxisAlignContent,omitempty"`
	PaddingLeft             *float64     `json:"paddingLeft,omitempty"`
	PaddingRight            *float64     `json:"paddingRight,omitempty"`
	PaddingTop              *float64     `json:"paddingTop,omitempty"`
	PaddingBottom           *float64     `json:"paddingBottom,omitempty"`
	ItemSpacing             *float64     `json:"itemSpacing,omitempty"`
	CounterAxisSpacing      *float64     `json:"counterAxisSpacing,omitempty"`
	ItemReverseZIndex       *bool        `json:"itemReverseZIndex,omitempty"`
	StrokesIncludedInLayout *bool        `json:"strokesIncludedInLayout,omitempty"`
	LayoutWrap              string       `json:"layoutWrap,omitempty"`

	// Blend mode and opacity.
	BlendMode string   `json:"blendMode,omitempty"`
	Opacity   *float64 `json:"opacity,omitempty"`

	ExportSettings []ExportSetting `json:"exportSettings,omitempty"`

	// Geometry (HasGeometryTrait), present with geometry=paths.
	FillGeometry      []Path                    `json:"fillGeometry,omitempty"`
	StrokeGeometry    []Path                    `json:"strokeGeometry,omitempty"`
	FillOverrideTable map[string]*PaintOverride `json:"fillOverrideTable,omitempty"`
	StrokeCap         string                    `json:"strokeCap,omitempty"`
	StrokeJoin        string                    `json:"strokeJoin,omitempty"`
	StrokeMiterAngle  *float64                  `json:"strokeMiterAngle,omitempty"`

	// Fills and strokes.
	Fills                   []Paint           `json:"fills,omitempty"`
	Styles                  map[string]string `json:"styles,omitempty"`
	Strokes                 []Paint           `json:"strokes,omitempty"`
	StrokeWeight            *float64          `json:"strokeWeight,omitempty"`
	StrokeAlign             string            `json:"strokeAlign,omitempty"`
	StrokeDashes            []float64         `json:"strokeDashes,omitempty"`
	IndividualStrokeWeights *StrokeWeights    `json:"individualStrokeWeights,omitempty"`

	// Corners.
	CornerRadius         *float64  `json:"cornerRadius,omitempty"`
	CornerSmoothing      *float64  `json:"cornerSmoothing,omitempty"`
	RectangleCornerRadii []float64 `json:"rectangleCornerRadii,omitempty"`

	Effects []Effect `json:"effects,omitempty"`

	// Masks.
	IsMask        *bool  `json:"isMask,omitempty"`
	MaskType      string `json:"maskType,omitempty"`
	IsMaskOutline *bool  `json:"isMaskOutline,omitempty"`

	// Components and instances.
	ComponentPropertyDefinitions map[string]ComponentPropertyDefinition `json:"componentPropertyDefinitions,omitempty"`
	ComponentID                  string                                 `json:"componentId,omitempty"`
	IsExposedInstance            *bool                                  `json:"isExposedInstance,omitempty"`
	ExposedInstances             []string                               `json:"exposedInstances,omitempty"`
	ComponentProperties          map[string]ComponentProperty           `json:"componentProperties,omitempty"`
	Overrides                    []Override                             `json:"overrides,omitempty"`

	// Text (TypePropertiesTrait).
	Characters              string               `json:"characters,omitempty"`
	Style                   *TypeStyle           `json:"style,omitempty"`
	CharacterStyleOverrides []int                `json:"characterStyleOverrides,omitempty"`
	LayoutVersion           *int                 `json:"layoutVersion,omitempty"`
	StyleOverrideTable      map[string]TypeStyle `json:"styleOverrideTable,omitempty"`
	LineTypes               []string             `json:"lineTypes,omitempty"`
	LineIndentations        []float64            `json:"lineIndentations,omitempty"`

	// Prototyping (TransitionSourceTrait) and canvas flow data.
	TransitionNodeID     string              `json:"transitionNodeID,omitempty"`
	TransitionDuration   *float64            `json:"transitionDuration,omitempty"`
	TransitionEasing     string              `json:"transitionEasing,omitempty"`
	Interactions         []Interaction       `json:"interactions,omitempty"`
	PrototypeStartNodeID *string             `json:"prototypeStartNodeID,omitempty"`
	FlowStartingPoints   []FlowStartingPoint `json:"flowStartingPoints,omitempty"`
	PrototypeDevice      json.RawMessage     `json:"prototypeDevice,omitempty"`

	// Dev Mode. Measurements are pinned on the page, not on the nodes they
	// measure, so they appear on CANVAS nodes only.
	DevStatus    *DevStatus    `json:"devStatus,omitempty"`
	Measurements []Measurement `json:"measurements,omitempty"`

	// Type specific extras.
	SectionContentsHidden *bool    `json:"sectionContentsHidden,omitempty"`
	BooleanOperation      string   `json:"booleanOperation,omitempty"`
	ArcData               *ArcData `json:"arcData,omitempty"`
}

// IsVisible reports whether the node is visible. Figma omits the property
// when it is true.
func (n *Node) IsVisible() bool {
	return n.Visible == nil || *n.Visible
}

// Walk visits the node and its descendants depth first. fn receives the
// node, its parent (nil for the root), and the depth (0 for the root). It
// returns false to skip the node's children.
func (n *Node) Walk(fn func(n *Node, parent *Node, depth int) bool) {
	n.walk(nil, 0, fn)
}

func (n *Node) walk(parent *Node, depth int, fn func(*Node, *Node, int) bool) {
	if n == nil || !fn(n, parent, depth) {
		return
	}
	for _, child := range n.Children {
		child.walk(n, depth+1, fn)
	}
}

// Find returns the descendant (or the node itself) with the given ID, or
// nil.
func (n *Node) Find(id string) *Node {
	var found *Node
	n.Walk(func(node *Node, _ *Node, _ int) bool {
		if found != nil {
			return false
		}
		if node.ID == id {
			found = node
			return false
		}
		return true
	})
	return found
}

// Prune returns a shallow copy of the node with children removed below
// depth. Depth 0 keeps the node only; a negative depth keeps everything.
func (n *Node) Prune(depth int) *Node {
	if n == nil {
		return nil
	}
	copied := *n
	if depth < 0 {
		return &copied
	}
	if depth == 0 {
		copied.Children = nil
		return &copied
	}
	copied.Children = make([]*Node, 0, len(n.Children))
	for _, child := range n.Children {
		copied.Children = append(copied.Children, child.Prune(depth-1))
	}
	return &copied
}

// Color is an RGBA color with channels in 0..1.
type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

// Vector is a 2D point or size.
type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rectangle is an axis aligned box.
type Rectangle struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Transform is a 2x3 affine transform matrix.
type Transform [][]float64

// LayoutConstraint describes how a node responds to its parent's resize.
type LayoutConstraint struct {
	Vertical   string `json:"vertical"`
	Horizontal string `json:"horizontal"`
}

// StrokeWeights are per-side stroke weights.
type StrokeWeights struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Path is a vector path in SVG path syntax.
type Path struct {
	Path        string `json:"path"`
	WindingRule string `json:"windingRule"`
	OverrideID  *int   `json:"overrideID,omitempty"`
}

// PaintOverride is an entry in fillOverrideTable.
type PaintOverride struct {
	Fills              []Paint `json:"fills,omitempty"`
	InheritFillStyleID string  `json:"inheritFillStyleId,omitempty"`
}

// ExportSetting is one export preset on a node.
type ExportSetting struct {
	Suffix     string     `json:"suffix"`
	Format     string     `json:"format"`
	Constraint Constraint `json:"constraint"`
}

// Constraint sizes an export.
type Constraint struct {
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}

// VariableAlias references a variable by ID.
type VariableAlias struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// VariableBinding is one entry of a boundVariables map. Figma uses a single
// alias for scalar properties, an array of aliases for list properties
// (fills, strokes, effects, layoutGrids, and text ranges), and a nested
// object for compound properties (size, individualStrokeWeights,
// rectangleCornerRadii, componentProperties). Exactly one field is set.
type VariableBinding struct {
	Alias   *VariableAlias
	Aliases []VariableAlias
	Fields  map[string]VariableAlias
}

// First returns the single alias a binding carries, or the first of a list.
// Figma sends a bare object for some properties and an array for others, so a
// caller that wants "the variable bound here" should not have to care which.
func (b VariableBinding) First() (VariableAlias, bool) {
	all := b.All()
	if len(all) == 0 {
		return VariableAlias{}, false
	}
	return all[0], true
}

// All returns every alias in the binding.
func (b VariableBinding) All() []VariableAlias {
	switch {
	case b.Alias != nil:
		return []VariableAlias{*b.Alias}
	case b.Aliases != nil:
		return b.Aliases
	case b.Fields != nil:
		out := make([]VariableAlias, 0, len(b.Fields))
		for _, a := range b.Fields {
			out = append(out, a)
		}
		return out
	}
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *VariableBinding) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	switch trimmed[0] {
	case '[':
		return json.Unmarshal(trimmed, &b.Aliases)
	case '{':
		var probe struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(trimmed, &probe); err == nil && probe.Type == "VARIABLE_ALIAS" {
			b.Alias = &VariableAlias{Type: probe.Type, ID: probe.ID}
			return nil
		}
		return json.Unmarshal(trimmed, &b.Fields)
	}
	return fmt.Errorf("boundVariables entry: unexpected JSON %q", string(trimmed))
}

// MarshalJSON implements json.Marshaler.
func (b VariableBinding) MarshalJSON() ([]byte, error) {
	switch {
	case b.Alias != nil:
		return json.Marshal(b.Alias)
	case b.Aliases != nil:
		return json.Marshal(b.Aliases)
	case b.Fields != nil:
		return json.Marshal(b.Fields)
	}
	return []byte("null"), nil
}

// Paint is a fill or stroke.
type Paint struct {
	Type           string                     `json:"type"`
	Visible        *bool                      `json:"visible,omitempty"`
	Opacity        *float64                   `json:"opacity,omitempty"`
	BlendMode      string                     `json:"blendMode,omitempty"`
	Color          *Color                     `json:"color,omitempty"`
	BoundVariables map[string]VariableBinding `json:"boundVariables,omitempty"`

	// Gradient paints.
	GradientHandlePositions []Vector    `json:"gradientHandlePositions,omitempty"`
	GradientStops           []ColorStop `json:"gradientStops,omitempty"`

	// Image paints.
	ScaleMode      string          `json:"scaleMode,omitempty"`
	ImageRef       string          `json:"imageRef,omitempty"`
	ImageTransform Transform       `json:"imageTransform,omitempty"`
	ScalingFactor  *float64        `json:"scalingFactor,omitempty"`
	Rotation       *float64        `json:"rotation,omitempty"`
	Filters        json.RawMessage `json:"filters,omitempty"`
	GifRef         string          `json:"gifRef,omitempty"`

	// Pattern paints.
	SourceNodeID        string  `json:"sourceNodeId,omitempty"`
	TileType            string  `json:"tileType,omitempty"`
	Spacing             *Vector `json:"spacing,omitempty"`
	HorizontalAlignment string  `json:"horizontalAlignment,omitempty"`
	VerticalAlignment   string  `json:"verticalAlignment,omitempty"`
}

// IsVisible reports whether the paint is visible. Figma omits the property
// when it is true.
func (p Paint) IsVisible() bool {
	return p.Visible == nil || *p.Visible
}

// ColorStop is one stop of a gradient.
type ColorStop struct {
	Position       float64                    `json:"position"`
	Color          Color                      `json:"color"`
	BoundVariables map[string]VariableBinding `json:"boundVariables,omitempty"`
}

// LayoutGrid is a layout grid on a frame.
type LayoutGrid struct {
	Pattern        string                     `json:"pattern"`
	SectionSize    float64                    `json:"sectionSize"`
	Visible        bool                       `json:"visible"`
	Color          Color                      `json:"color"`
	Alignment      string                     `json:"alignment,omitempty"`
	GutterSize     float64                    `json:"gutterSize"`
	Offset         float64                    `json:"offset"`
	Count          float64                    `json:"count"`
	BoundVariables map[string]VariableBinding `json:"boundVariables,omitempty"`
}

// Effect is a shadow, blur, noise, or texture effect.
type Effect struct {
	Type                 string                     `json:"type"`
	Visible              *bool                      `json:"visible,omitempty"`
	Color                *Color                     `json:"color,omitempty"`
	BlendMode            string                     `json:"blendMode,omitempty"`
	Offset               *Vector                    `json:"offset,omitempty"`
	Radius               *float64                   `json:"radius,omitempty"`
	Spread               *float64                   `json:"spread,omitempty"`
	ShowShadowBehindNode *bool                      `json:"showShadowBehindNode,omitempty"`
	BlurType             string                     `json:"blurType,omitempty"`
	BoundVariables       map[string]VariableBinding `json:"boundVariables,omitempty"`
}

// IsVisible reports whether the effect is visible. Figma omits the property
// when it is true.
func (e Effect) IsVisible() bool {
	return e.Visible == nil || *e.Visible
}

// TypeStyle is the typography of a text node or a text run.
type TypeStyle struct {
	FontFamily                string                     `json:"fontFamily,omitempty"`
	FontPostScriptName        *string                    `json:"fontPostScriptName,omitempty"`
	FontStyle                 string                     `json:"fontStyle,omitempty"`
	Italic                    *bool                      `json:"italic,omitempty"`
	FontWeight                *float64                   `json:"fontWeight,omitempty"`
	FontSize                  *float64                   `json:"fontSize,omitempty"`
	TextCase                  string                     `json:"textCase,omitempty"`
	TextDecoration            string                     `json:"textDecoration,omitempty"`
	TextAutoResize            string                     `json:"textAutoResize,omitempty"`
	TextTruncation            string                     `json:"textTruncation,omitempty"`
	MaxLines                  *int                       `json:"maxLines,omitempty"`
	TextAlignHorizontal       string                     `json:"textAlignHorizontal,omitempty"`
	TextAlignVertical         string                     `json:"textAlignVertical,omitempty"`
	LetterSpacing             *float64                   `json:"letterSpacing,omitempty"`
	Fills                     []Paint                    `json:"fills,omitempty"`
	Hyperlink                 *Hyperlink                 `json:"hyperlink,omitempty"`
	OpentypeFlags             map[string]float64         `json:"opentypeFlags,omitempty"`
	SemanticWeight            string                     `json:"semanticWeight,omitempty"`
	SemanticItalic            string                     `json:"semanticItalic,omitempty"`
	ParagraphSpacing          *float64                   `json:"paragraphSpacing,omitempty"`
	ParagraphIndent           *float64                   `json:"paragraphIndent,omitempty"`
	ListSpacing               *float64                   `json:"listSpacing,omitempty"`
	LineHeightPx              *float64                   `json:"lineHeightPx,omitempty"`
	LineHeightPercent         *float64                   `json:"lineHeightPercent,omitempty"`
	LineHeightPercentFontSize *float64                   `json:"lineHeightPercentFontSize,omitempty"`
	LineHeightUnit            string                     `json:"lineHeightUnit,omitempty"`
	IsOverrideOverTextStyle   *bool                      `json:"isOverrideOverTextStyle,omitempty"`
	BoundVariables            map[string]VariableBinding `json:"boundVariables,omitempty"`
}

// Hyperlink is a link on a text run.
type Hyperlink struct {
	Type   string `json:"type"`
	URL    string `json:"url,omitempty"`
	NodeID string `json:"nodeID,omitempty"`
}

// ComponentPropertyDefinition is a property declared on a component or
// component set.
type ComponentPropertyDefinition struct {
	Type            string                       `json:"type"`
	DefaultValue    any                          `json:"defaultValue"`
	VariantOptions  []string                     `json:"variantOptions,omitempty"`
	PreferredValues []InstanceSwapPreferredValue `json:"preferredValues,omitempty"`
}

// ComponentProperty is a property value on an instance.
type ComponentProperty struct {
	Type            string                       `json:"type"`
	Value           any                          `json:"value"`
	PreferredValues []InstanceSwapPreferredValue `json:"preferredValues,omitempty"`
	BoundVariables  map[string]VariableBinding   `json:"boundVariables,omitempty"`
}

// InstanceSwapPreferredValue is an allowed value of an INSTANCE_SWAP
// property.
type InstanceSwapPreferredValue struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// Override lists the fields an instance overrides on one of its nodes.
type Override struct {
	ID               string   `json:"id"`
	OverriddenFields []string `json:"overriddenFields"`
}

// Interaction is a prototype interaction: a trigger and its actions.
type Interaction struct {
	Trigger *Trigger `json:"trigger"`
	Actions []Action `json:"actions,omitempty"`
}

// Trigger starts a prototype interaction.
type Trigger struct {
	Type         string   `json:"type"`
	Delay        *float64 `json:"delay,omitempty"`
	Timeout      *float64 `json:"timeout,omitempty"`
	DeviceType   string   `json:"deviceType,omitempty"`
	KeyCodes     []int    `json:"keyCodes,omitempty"`
	MediaHitTime *float64 `json:"mediaHitTime,omitempty"`
}

// Action is one effect of a prototype interaction. Conditional and
// variable actions keep their payload as raw JSON.
type Action struct {
	Type                       string          `json:"type"`
	URL                        string          `json:"url,omitempty"`
	Navigation                 string          `json:"navigation,omitempty"`
	DestinationID              *string         `json:"destinationId,omitempty"`
	Transition                 *Transition     `json:"transition,omitempty"`
	PreserveScrollPosition     *bool           `json:"preserveScrollPosition,omitempty"`
	OverlayRelativePosition    *Vector         `json:"overlayRelativePosition,omitempty"`
	ResetVideoPosition         *bool           `json:"resetVideoPosition,omitempty"`
	ResetScrollPosition        *bool           `json:"resetScrollPosition,omitempty"`
	ResetInteractiveComponents *bool           `json:"resetInteractiveComponents,omitempty"`
	MediaAction                string          `json:"mediaAction,omitempty"`
	VariableID                 string          `json:"variableId,omitempty"`
	VariableValue              json.RawMessage `json:"variableValue,omitempty"`
	ConditionalBlocks          json.RawMessage `json:"conditionalBlocks,omitempty"`
}

// Transition animates a navigation action.
type Transition struct {
	Type        string  `json:"type"`
	Direction   string  `json:"direction,omitempty"`
	Duration    float64 `json:"duration"`
	Easing      *Easing `json:"easing,omitempty"`
	MatchLayers *bool   `json:"matchLayers,omitempty"`
}

// Easing is a transition easing curve.
type Easing struct {
	Type                      string          `json:"type"`
	EasingFunctionCubicBezier json.RawMessage `json:"easingFunctionCubicBezier,omitempty"`
	EasingFunctionSpring      json.RawMessage `json:"easingFunctionSpring,omitempty"`
}

// FlowStartingPoint is a prototype flow entry on a canvas.
type FlowStartingPoint struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
}

// DevStatus is the Dev Mode status of a node.
type DevStatus struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// ArcData describes the sweep of an ellipse.
type ArcData struct {
	StartingAngle float64 `json:"startingAngle"`
	EndingAngle   float64 `json:"endingAngle"`
	InnerRadius   float64 `json:"innerRadius"`
}

// Measurement is a distance a designer pinned between two nodes in Dev Mode.
// It is spacing the design depends on that auto layout does not always
// express, so it is intent worth reading rather than inferring.
//
// The API reports what the measurement is pinned to but not how far apart the
// two sides actually are; that has to be computed from the nodes' bounding
// boxes.
type Measurement struct {
	ID     string              `json:"id"`
	Start  MeasurementStartEnd `json:"start"`
	End    MeasurementStartEnd `json:"end"`
	Offset MeasurementOffset   `json:"offset"`
	// FreeText is the label the designer typed instead of the measured
	// value. When it is set, it is what the design actually specifies.
	FreeText string `json:"freeText,omitempty"`
}

// MeasurementStartEnd is the node and the side a measurement is pinned to.
type MeasurementStartEnd struct {
	NodeID string `json:"nodeId"`
	// Side is TOP, RIGHT, BOTTOM, or LEFT.
	Side string `json:"side"`
}

// MeasurementOffset is where along the side the measurement sits. INNER
// offsets are a fraction of the start node's side; OUTER offsets are a fixed
// distance from it.
type MeasurementOffset struct {
	Type     string   `json:"type"`
	Relative *float64 `json:"relative,omitempty"`
	Fixed    *float64 `json:"fixed,omitempty"`
}
