package resolve

import (
	"strconv"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

// Typography converts a text style into the model. Line height is given
// in px, unitless (px divided by the font size, two decimals), and
// percent of the font size; letter spacing in px and em. Bound variables
// on the style become tokens; the five common properties are always
// present in Tokens, null when unbound.
func (r *Resolver) Typography(ts *figma.TypeStyle, style *model.StyleRef) *model.Text {
	if ts == nil {
		return nil
	}
	t := r.partialTypography(ts, 0)
	if t == nil {
		t = &model.Text{}
	}
	t.Style = style
	tokens := map[string]*model.TokenRef{}
	for _, key := range []string{"fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing"} {
		tokens[key] = r.AliasToken(ts.BoundVariables, key)
	}
	for key, alias := range ts.BoundVariables {
		if _, ok := tokens[key]; !ok {
			tokens[key] = r.Token(alias.ID)
		}
	}
	t.Tokens = tokens
	return t
}

// partialTypography converts only the fields present in a text style,
// which is what a run override carries. baseSize is used to derive
// unitless and em values when the override has no font size.
func (r *Resolver) partialTypography(ts *figma.TypeStyle, baseSize float64) *model.Text {
	if ts == nil {
		return nil
	}
	t := &model.Text{}
	present := false
	size := baseSize
	if ts.FontSize != nil {
		size = *ts.FontSize
		s := *ts.FontSize
		t.Size = &s
		present = true
	}
	if ts.FontFamily != "" {
		t.FontFamily = ts.FontFamily
		present = true
	}
	if ts.FontPostScriptName != nil && *ts.FontPostScriptName != "" {
		t.FontPostScriptName = *ts.FontPostScriptName
		present = true
	}
	if w := Weight(ts); w != nil {
		t.Weight = w
		present = true
	}
	if Italic(ts) {
		t.Italic = true
		present = true
	}
	if ts.LineHeightPx != nil || ts.LineHeightPercentFontSize != nil || ts.LineHeightUnit != "" {
		t.LineHeight = lineHeight(ts, size)
		present = true
	}
	if ts.LetterSpacing != nil {
		ls := &model.LetterSpacing{Px: Round(*ts.LetterSpacing, 2)}
		if size > 0 {
			em := Round(*ts.LetterSpacing/size, 3)
			ls.Em = &em
		}
		t.LetterSpacing = ls
		present = true
	}
	if ts.TextCase != "" {
		t.Case = TextCase(ts.TextCase)
		present = true
	}
	if ts.TextDecoration != "" {
		t.Decoration = TextDecoration(ts.TextDecoration)
		present = true
	}
	if ts.TextAlignHorizontal != "" {
		t.AlignH = alignH(ts.TextAlignHorizontal)
		present = true
	}
	if ts.TextAlignVertical != "" {
		t.AlignV = alignV(ts.TextAlignVertical)
		present = true
	}
	if ts.TextAutoResize != "" {
		t.AutoResize = autoResize(ts.TextAutoResize)
		present = true
	}
	if ts.TextTruncation != "" && ts.TextTruncation != "DISABLED" {
		t.Truncation = strings.ToLower(ts.TextTruncation)
		present = true
	}
	if ts.MaxLines != nil {
		n := *ts.MaxLines
		t.MaxLines = &n
		present = true
	}
	if ts.ParagraphSpacing != nil && *ts.ParagraphSpacing != 0 {
		p := *ts.ParagraphSpacing
		t.ParagraphSpacing = &p
		present = true
	}
	if ts.Hyperlink != nil {
		if ts.Hyperlink.URL != "" {
			t.Hyperlink = ts.Hyperlink.URL
		} else if ts.Hyperlink.NodeID != "" {
			t.Hyperlink = "node:" + ts.Hyperlink.NodeID
		}
		present = true
	}
	if v, ok := ts.OpentypeFlags["TNUM"]; ok && v != 0 {
		t.TabularNumbers = true
		present = true
	}
	if !present {
		return nil
	}
	return t
}

func lineHeight(ts *figma.TypeStyle, size float64) *model.LineHeight {
	lh := &model.LineHeight{}
	if ts.LineHeightPx != nil {
		lh.Px = Round(*ts.LineHeightPx, 2)
	}
	if ts.LineHeightPercentFontSize != nil {
		p := Round(*ts.LineHeightPercentFontSize, 2)
		lh.Percent = &p
	} else if ts.LineHeightPx != nil && size > 0 {
		p := Round(*ts.LineHeightPx/size*100, 2)
		lh.Percent = &p
	}
	if lh.Px > 0 && size > 0 {
		u := Round(lh.Px/size, 2)
		lh.Unitless = &u
	}
	switch ts.LineHeightUnit {
	case "PIXELS":
		lh.Unit = "px"
	case "FONT_SIZE_%":
		lh.Unit = "percent"
	case "INTRINSIC_%":
		lh.Unit = "auto"
	}
	return lh
}

// Weight returns the numeric font weight, deriving it from the font style
// or PostScript name when fontWeight is absent.
func Weight(ts *figma.TypeStyle) *float64 {
	if ts.FontWeight != nil {
		w := *ts.FontWeight
		return &w
	}
	name := ts.FontStyle
	if name == "" && ts.FontPostScriptName != nil {
		if i := strings.LastIndex(*ts.FontPostScriptName, "-"); i >= 0 {
			name = (*ts.FontPostScriptName)[i+1:]
		}
	}
	if name == "" {
		return nil
	}
	lower := strings.TrimSuffix(strings.ToLower(strings.ReplaceAll(name, " ", "")), "italic")
	if lower == "" {
		w := 400.0
		return &w
	}
	weights := []struct {
		key string
		w   float64
	}{
		{"extrablack", 950}, {"ultrablack", 950}, {"black", 900}, {"heavy", 900},
		{"extrabold", 800}, {"ultrabold", 800},
		{"semibold", 600}, {"demibold", 600},
		{"bold", 700},
		{"medium", 500},
		{"extralight", 200}, {"ultralight", 200},
		{"light", 300},
		{"thin", 100}, {"hairline", 100},
		{"regular", 400}, {"normal", 400}, {"book", 400},
	}
	for _, e := range weights {
		if strings.Contains(lower, e.key) {
			w := e.w
			return &w
		}
	}
	return nil
}

// Italic reports whether the style is italic.
func Italic(ts *figma.TypeStyle) bool {
	if ts.Italic != nil {
		return *ts.Italic
	}
	if strings.Contains(strings.ToLower(ts.FontStyle), "italic") {
		return true
	}
	return ts.FontPostScriptName != nil && strings.Contains(strings.ToLower(*ts.FontPostScriptName), "italic")
}

// TextCase converts textCase into a CSS text-transform (or font-variant
// for small caps) value.
func TextCase(v string) string {
	switch v {
	case "UPPER":
		return "uppercase"
	case "LOWER":
		return "lowercase"
	case "TITLE":
		return "capitalize"
	case "SMALL_CAPS":
		return "small-caps"
	case "SMALL_CAPS_FORCED":
		return "all-small-caps"
	case "ORIGINAL", "":
		return ""
	}
	return strings.ToLower(v)
}

// TextDecoration converts textDecoration into a CSS text-decoration.
func TextDecoration(v string) string {
	switch v {
	case "UNDERLINE":
		return "underline"
	case "STRIKETHROUGH":
		return "line-through"
	case "NONE", "":
		return ""
	}
	return strings.ToLower(v)
}

func alignH(v string) string {
	switch v {
	case "JUSTIFIED":
		return "justify"
	case "":
		return ""
	}
	return strings.ToLower(v)
}

func alignV(v string) string {
	switch v {
	case "CENTER":
		return "middle"
	case "":
		return ""
	}
	return strings.ToLower(v)
}

func autoResize(v string) string {
	switch v {
	case "NONE":
		return "fixed"
	case "HEIGHT":
		return "height"
	case "WIDTH_AND_HEIGHT":
		return "width-and-height"
	case "TRUNCATE":
		return "truncate"
	}
	return strings.ToLower(v)
}

// Runs splits mixed text into runs of consecutive characters sharing an
// override id. Runs without an override (id 0) are skipped. Ranges are
// in characters (runes), matching Figma's characterStyleOverrides.
func (r *Resolver) Runs(node *figma.Node) []model.TextRun {
	if node == nil || len(node.CharacterStyleOverrides) == 0 || len(node.StyleOverrideTable) == 0 {
		return nil
	}
	chars := []rune(node.Characters)
	baseSize := 0.0
	if node.Style != nil && node.Style.FontSize != nil {
		baseSize = *node.Style.FontSize
	}
	var runs []model.TextRun
	ids := node.CharacterStyleOverrides
	for start := 0; start < len(ids); {
		id := ids[start]
		end := start
		for end < len(ids) && ids[end] == id {
			end++
		}
		if id != 0 {
			run := model.TextRun{Start: start, End: end}
			if end <= len(chars) {
				run.Text = string(chars[start:end])
			} else if start < len(chars) {
				run.Text = string(chars[start:])
			}
			if ov, ok := node.StyleOverrideTable[strconv.Itoa(id)]; ok {
				run.Overrides = r.partialTypography(&ov, baseSize)
				for _, p := range ov.Fills {
					if !p.IsVisible() {
						continue
					}
					if f := r.Paint(p); f != nil {
						run.Fill = f
						break
					}
				}
			}
			runs = append(runs, run)
		}
		start = end
	}
	return runs
}
