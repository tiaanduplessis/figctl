// Package model is the normalized node model figctl emits: layout, visual,
// text, component, and prototype information with resolved tokens and
// CSS-like values. It is plain data with no behavior so other packages
// (inspect, tokens export, node context) can build on it.
package model

// Node is one inspected node.
type Node struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Visible    bool              `json:"visible"`
	Locked     bool              `json:"locked,omitempty"`
	Path       []string          `json:"path,omitempty"`
	Box        *Box              `json:"box,omitempty"`
	Layout     *Layout           `json:"layout,omitempty"`
	Visual     *Visual           `json:"visual,omitempty"`
	Text       *Text             `json:"text,omitempty"`
	Component  *Component        `json:"component,omitempty"`
	Prototype  *Prototype        `json:"prototype,omitempty"`
	Tokens     *Tokens           `json:"tokens,omitempty"`
	CSS        map[string]string `json:"css,omitempty"`
	ChildCount int               `json:"childCount,omitempty"`
	Children   []*Node           `json:"children,omitempty"`
	// Truncated is set when the node budget stopped the walk below this
	// node; Omitted counts the nodes that were left out.
	Truncated bool `json:"truncated,omitempty"`
	Omitted   int  `json:"omitted,omitempty"`
}

// Tokens indexes the variables and styles a node uses, keyed by property
// (for example fills[0], gap, paddingTop, fontFamily, effect). Values are
// token and style names; the full references live on the properties.
type Tokens struct {
	Variables map[string]string `json:"variables,omitempty"`
	Styles    map[string]string `json:"styles,omitempty"`
}

// Box is the absolute bounding box and the position relative to the
// parent's box.
type Box struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Relative *Point  `json:"relative,omitempty"`
}

// Point is a 2D offset.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Layout is the auto layout, grid, and positioning of a node in CSS terms.
type Layout struct {
	// Mode is none, row, column, wrap, or grid.
	Mode         string   `json:"mode"`
	Gap          *float64 `json:"gap,omitempty"`
	RowGap       *float64 `json:"rowGap,omitempty"`
	Padding      *Padding `json:"padding,omitempty"`
	Justify      string   `json:"justify,omitempty"`
	Align        string   `json:"align,omitempty"`
	AlignContent string   `json:"alignContent,omitempty"`
	Sizing       *Sizing  `json:"sizing,omitempty"`
	MinWidth     *float64 `json:"minWidth,omitempty"`
	MaxWidth     *float64 `json:"maxWidth,omitempty"`
	MinHeight    *float64 `json:"minHeight,omitempty"`
	MaxHeight    *float64 `json:"maxHeight,omitempty"`
	// Position is auto (placed by the parent's auto layout) or absolute.
	Position        string       `json:"position"`
	Offset          *Point       `json:"offset,omitempty"`
	Constraints     *Constraints `json:"constraints,omitempty"`
	Clips           bool         `json:"clips,omitempty"`
	Overflow        string       `json:"overflow,omitempty"`
	Grid            *Grid        `json:"grid,omitempty"`
	GridChild       *GridChild   `json:"gridChild,omitempty"`
	Grow            *float64     `json:"grow,omitempty"`
	LayoutAlign     string       `json:"layoutAlign,omitempty"`
	ReverseZIndex   bool         `json:"reverseZIndex,omitempty"`
	StrokesInLayout bool         `json:"strokesInLayout,omitempty"`
	// Tokens holds the variable bound to gap, rowGap, and each padding
	// side. Unbound properties are present with a null value so it is
	// clear which values are not tokenized.
	Tokens map[string]*TokenRef `json:"tokens,omitempty"`
}

// Padding is the four sides plus the CSS shorthand.
type Padding struct {
	Top       float64 `json:"top"`
	Right     float64 `json:"right"`
	Bottom    float64 `json:"bottom"`
	Left      float64 `json:"left"`
	Shorthand string  `json:"shorthand"`
}

// Sizing is the per axis sizing behavior.
type Sizing struct {
	Horizontal Axis `json:"horizontal"`
	Vertical   Axis `json:"vertical"`
}

// Axis is fixed, hug, or fill, with the current size in px.
type Axis struct {
	Mode string  `json:"mode"`
	Px   float64 `json:"px"`
}

// Constraints are the resize constraints of an absolutely placed node.
type Constraints struct {
	Horizontal string `json:"horizontal"`
	Vertical   string `json:"vertical"`
}

// Grid describes a grid layout container.
type Grid struct {
	Rows          int     `json:"rows"`
	Columns       int     `json:"columns"`
	RowGap        float64 `json:"rowGap"`
	ColumnGap     float64 `json:"columnGap"`
	RowsSizing    string  `json:"rowsSizing,omitempty"`
	ColumnsSizing string  `json:"columnsSizing,omitempty"`
}

// GridChild describes a node placed in a grid layout parent.
type GridChild struct {
	RowSpan    int    `json:"rowSpan"`
	ColumnSpan int    `json:"columnSpan"`
	Row        int    `json:"row"`
	Column     int    `json:"column"`
	AlignH     string `json:"alignHorizontal,omitempty"`
	AlignV     string `json:"alignVertical,omitempty"`
}

// Visual is the appearance of a node.
type Visual struct {
	Fills        []Fill       `json:"fills"`
	Strokes      []Stroke     `json:"strokes,omitempty"`
	StrokeWeight *Sides       `json:"strokeWeight,omitempty"`
	StrokeAlign  string       `json:"strokeAlign,omitempty"`
	StrokeDashes []float64    `json:"strokeDashes,omitempty"`
	Radius       *Radius      `json:"radius,omitempty"`
	Effects      []Effect     `json:"effects,omitempty"`
	Opacity      *float64     `json:"opacity,omitempty"`
	BlendMode    string       `json:"blendMode,omitempty"`
	LayoutGrids  []LayoutGrid `json:"layoutGrids,omitempty"`
	IsMask       bool         `json:"isMask,omitempty"`
}

// Sides are per side values, for stroke weights.
type Sides struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Uniform reports whether all four sides are equal.
func (s Sides) Uniform() bool {
	return s.Top == s.Right && s.Right == s.Bottom && s.Bottom == s.Left
}

// Radius is the corner radius: uniform, or four corners in CSS order
// (top-left, top-right, bottom-right, bottom-left).
type Radius struct {
	Uniform   *float64    `json:"uniform,omitempty"`
	Corners   *[4]float64 `json:"corners,omitempty"`
	Smoothing float64     `json:"smoothing,omitempty"`
	Token     *TokenRef   `json:"token"`
}

// Fill is a paint: solid, gradient, or image.
type Fill struct {
	Type      string    `json:"type"`
	Hex       string    `json:"hex,omitempty"`
	Hex8      string    `json:"hex8,omitempty"`
	RGBA      string    `json:"rgba,omitempty"`
	Opacity   *float64  `json:"opacity,omitempty"`
	BlendMode string    `json:"blendMode,omitempty"`
	Gradient  *Gradient `json:"gradient,omitempty"`
	ImageRef  string    `json:"imageRef,omitempty"`
	ScaleMode string    `json:"scaleMode,omitempty"`
	Token     *TokenRef `json:"token"`
	Style     *StyleRef `json:"style,omitempty"`
}

// Stroke is a paint used as a stroke.
type Stroke = Fill

// Gradient is a gradient paint converted to CSS.
type Gradient struct {
	// Type is linear, radial, angular, or diamond.
	Type        string         `json:"type"`
	CSS         string         `json:"css"`
	Angle       float64        `json:"angle"`
	Stops       []GradientStop `json:"stops"`
	Approximate bool           `json:"approximate,omitempty"`
}

// GradientStop is one stop of a gradient.
type GradientStop struct {
	Position float64   `json:"position"`
	Hex      string    `json:"hex"`
	RGBA     string    `json:"rgba"`
	CSS      string    `json:"css"`
	Token    *TokenRef `json:"token"`
}

// Effect is a shadow or blur with its CSS declaration.
type Effect struct {
	// Type is drop-shadow, inner-shadow, layer-blur, or background-blur.
	Type string `json:"type"`
	// Property is the CSS property the value belongs to: box-shadow,
	// filter, or backdrop-filter.
	Property string    `json:"property"`
	CSS      string    `json:"css"`
	Hex      string    `json:"hex,omitempty"`
	RGBA     string    `json:"rgba,omitempty"`
	Offset   *Point    `json:"offset,omitempty"`
	Radius   float64   `json:"radius"`
	Spread   float64   `json:"spread,omitempty"`
	Token    *TokenRef `json:"token"`
	Style    *StyleRef `json:"style,omitempty"`
}

// LayoutGrid is a layout grid on a frame.
type LayoutGrid struct {
	// Pattern is columns, rows, or grid.
	Pattern     string    `json:"pattern"`
	Count       float64   `json:"count,omitempty"`
	Gutter      float64   `json:"gutter,omitempty"`
	Offset      float64   `json:"offset,omitempty"`
	SectionSize float64   `json:"sectionSize"`
	Alignment   string    `json:"alignment,omitempty"`
	Visible     bool      `json:"visible"`
	Style       *StyleRef `json:"style,omitempty"`
}

// Text is the typography of a text node or a text style.
type Text struct {
	Characters         string         `json:"characters,omitempty"`
	FontFamily         string         `json:"fontFamily,omitempty"`
	FontPostScriptName string         `json:"fontPostScriptName,omitempty"`
	Weight             *float64       `json:"weight,omitempty"`
	Italic             bool           `json:"italic,omitempty"`
	Size               *float64       `json:"size,omitempty"`
	LineHeight         *LineHeight    `json:"lineHeight,omitempty"`
	LetterSpacing      *LetterSpacing `json:"letterSpacing,omitempty"`
	Case               string         `json:"case,omitempty"`
	Decoration         string         `json:"decoration,omitempty"`
	AlignH             string         `json:"alignHorizontal,omitempty"`
	AlignV             string         `json:"alignVertical,omitempty"`
	AutoResize         string         `json:"autoResize,omitempty"`
	Truncation         string         `json:"truncation,omitempty"`
	MaxLines           *int           `json:"maxLines,omitempty"`
	ParagraphSpacing   *float64       `json:"paragraphSpacing,omitempty"`
	Hyperlink          string         `json:"hyperlink,omitempty"`
	TabularNumbers     bool           `json:"tabularNumbers,omitempty"`
	Runs               []TextRun      `json:"runs,omitempty"`
	// Tokens holds the variables bound to fontFamily, fontSize, weight,
	// lineHeight, and letterSpacing; unbound entries are null.
	Tokens map[string]*TokenRef `json:"tokens,omitempty"`
	Style  *StyleRef            `json:"style,omitempty"`
}

// LineHeight in px, unitless (px / font size), and percent of font size.
type LineHeight struct {
	Px       float64  `json:"px"`
	Unitless *float64 `json:"unitless,omitempty"`
	Percent  *float64 `json:"percent,omitempty"`
	// Unit is px, percent, or auto (Figma's intrinsic line height).
	Unit string `json:"unit,omitempty"`
}

// LetterSpacing in px and em.
type LetterSpacing struct {
	Px float64  `json:"px"`
	Em *float64 `json:"em,omitempty"`
}

// TextRun is a range of characters with typography overrides.
type TextRun struct {
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Text      string `json:"text"`
	Overrides *Text  `json:"overrides,omitempty"`
	Fill      *Fill  `json:"fill,omitempty"`
}

// Component describes instances, components, and component sets.
type Component struct {
	IsInstance          bool                          `json:"isInstance,omitempty"`
	IsComponent         bool                          `json:"isComponent,omitempty"`
	IsComponentSet      bool                          `json:"isComponentSet,omitempty"`
	Key                 string                        `json:"key,omitempty"`
	MainComponentID     string                        `json:"mainComponentId,omitempty"`
	MainComponentName   string                        `json:"mainComponentName,omitempty"`
	SetID               string                        `json:"setId,omitempty"`
	SetName             string                        `json:"setName,omitempty"`
	VariantProperties   map[string]string             `json:"variantProperties,omitempty"`
	Properties          map[string]Property           `json:"properties,omitempty"`
	Overrides           []Override                    `json:"overrides,omitempty"`
	ExposedInstances    []string                      `json:"exposedInstances,omitempty"`
	Description         string                        `json:"description,omitempty"`
	DocumentationLinks  []string                      `json:"documentationLinks,omitempty"`
	PropertyDefinitions map[string]PropertyDefinition `json:"propertyDefinitions,omitempty"`
	Remote              bool                          `json:"remote,omitempty"`
}

// Property is a component property value on an instance.
type Property struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
	// Key is the raw Figma property key including the #id suffix.
	Key   string    `json:"key"`
	Token *TokenRef `json:"token,omitempty"`
}

// PropertyDefinition is a component property declaration.
type PropertyDefinition struct {
	Type           string   `json:"type"`
	DefaultValue   any      `json:"defaultValue"`
	VariantOptions []string `json:"variantOptions,omitempty"`
	Key            string   `json:"key"`
}

// Override lists the fields an instance overrides on one of its nodes.
type Override struct {
	NodeID string   `json:"nodeId"`
	Fields []string `json:"fields"`
}

// Prototype holds interactions when requested.
type Prototype struct {
	Interactions []Interaction `json:"interactions"`
}

// Interaction is one trigger and action pair.
type Interaction struct {
	Trigger       string      `json:"trigger"`
	Action        string      `json:"action"`
	Navigation    string      `json:"navigation,omitempty"`
	DestinationID string      `json:"destinationId,omitempty"`
	URL           string      `json:"url,omitempty"`
	Transition    *Transition `json:"transition,omitempty"`
}

// Transition animates a navigation.
type Transition struct {
	Type      string  `json:"type"`
	Duration  float64 `json:"duration"`
	Easing    string  `json:"easing,omitempty"`
	Direction string  `json:"direction,omitempty"`
}

// TokenRef is a resolved reference to a variable.
type TokenRef struct {
	VariableID   string            `json:"variableId"`
	Name         string            `json:"name,omitempty"`
	Collection   string            `json:"collection,omitempty"`
	ResolvedType string            `json:"resolvedType,omitempty"`
	CodeSyntax   map[string]string `json:"codeSyntax,omitempty"`
	// Values holds the resolved value per mode name; colors are hex.
	Values map[string]any `json:"values,omitempty"`
	// Chain lists the variable names followed through aliases in the
	// default mode, starting with this variable.
	Chain []string `json:"chain,omitempty"`
	// Web is var(--name) when WEB code syntax is set and was requested.
	Web string `json:"web,omitempty"`
	// Unresolved is set when the variable is bound but its definition is
	// not available (no variables access on this plan or token).
	Unresolved bool `json:"unresolved,omitempty"`
}

// StyleRef is a reference to a style.
type StyleRef struct {
	ID   string `json:"id"`
	Key  string `json:"key,omitempty"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Measurement is a distance a designer pinned between two nodes in Dev Mode,
// with the gap resolved to pixels. Figma reports only what the measurement is
// attached to, so Distance is computed from the two nodes' bounding boxes.
//
// A measurement is a spacing decision stated outright rather than inferred
// from layout, which makes it the design's own answer where auto layout and a
// screenshot can only be read for clues.
type Measurement struct {
	ID string `json:"id"`
	// Axis is horizontal or vertical, taken from the sides the measurement
	// is pinned to. It is empty when the two sides do not share an axis.
	Axis string `json:"axis,omitempty"`
	// Distance is the gap in pixels, absent when it cannot be computed
	// because a node is missing a bounding box or the sides disagree.
	Distance *float64 `json:"distance,omitempty"`
	// CSS is Distance as a length, for pasting into a rule.
	CSS string `json:"css,omitempty"`
	// Label is what the design specifies: the designer's own text when they
	// overrode the value, otherwise the measured distance.
	Label string `json:"label,omitempty"`
	// FreeText is set only when the designer replaced the measured value.
	FreeText string             `json:"freeText,omitempty"`
	Start    MeasurementPin     `json:"start"`
	End      MeasurementPin     `json:"end"`
	Offset   *MeasurementOffset `json:"offset,omitempty"`
	// Unresolved says why Distance is absent.
	Unresolved string `json:"unresolved,omitempty"`
}

// MeasurementPin is one end of a measurement: the node, its name, and the
// side the pin sits on.
type MeasurementPin struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Side   string `json:"side"`
}

// MeasurementOffset is where along the side the measurement sits.
type MeasurementOffset struct {
	Type     string   `json:"type"`
	Relative *float64 `json:"relative,omitempty"`
	Fixed    *float64 `json:"fixed,omitempty"`
}
