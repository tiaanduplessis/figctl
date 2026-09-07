package tokens

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// Tailwind module formats.
const (
	TailwindESM  = "esm"
	TailwindCJS  = "cjs"
	TailwindJSON = "json"
)

// TailwindFormats lists the accepted --tailwind-format values.
var TailwindFormats = []string{TailwindESM, TailwindCJS, TailwindJSON}

// Tailwind theme keys, in the order they are written.
var tailwindKeys = []string{"colors", "spacing", "fontFamily", "fontSize", "fontWeight", "borderRadius", "boxShadow"}

// tailwindGroups are the leading path segments dropped when a token is
// placed under the matching theme key, so color/brand/500 becomes
// colors.brand.500 rather than colors.color.brand.500.
var tailwindGroups = map[string]string{
	"color":        "colors",
	"colors":       "colors",
	"space":        "spacing",
	"spacing":      "spacing",
	"radius":       "borderRadius",
	"radii":        "borderRadius",
	"borderradius": "borderRadius",
	"shadow":       "boxShadow",
	"shadows":      "boxShadow",
	"font":         "fontFamily",
	"fonts":        "fontFamily",
}

// TailwindOptions configures the Tailwind writer.
type TailwindOptions struct {
	// Format is one of the Tailwind constants; empty means TailwindESM.
	Format string
	// Vars emits var(--name) references instead of literal values, for
	// projects that also ship the CSS output.
	Vars bool
}

// Tailwind renders the document as a Tailwind theme module.
//
// Tokens are placed under the theme key their type and Figma scopes
// imply (colors, spacing, fontFamily, fontSize, fontWeight,
// borderRadius, boxShadow) and nested by group path, with a leading
// group segment that repeats the theme key dropped. Every value is a
// string, because that is what Tailwind's theme expects.
//
// Tailwind's theme has no key for typography composites, gradients,
// grids, booleans, or plain numbers, so those go under extend, grouped
// by token type, with a comment saying why.
func (d *Document) Tailwind(opts TailwindOptions) ([]Output, error) {
	format := opts.Format
	if format == "" {
		format = TailwindESM
	}
	set := d.rootSet()
	theme := map[string]map[string]any{}
	extend := map[string]map[string]any{}
	out := Output{Tokens: len(set.Tokens)}
	for _, t := range set.Tokens {
		value := tailwindValue(t, opts.Vars)
		if value == nil {
			continue
		}
		target, path := theme, tailwindPath(t)
		key := t.Category
		if key == "" {
			target, key = extend, t.Type
		}
		if target[key] == nil {
			target[key] = map[string]any{}
		}
		if warning := put(target[key], path, value, t.Source); warning != "" {
			out.Warnings = append(out.Warnings, warning)
		}
	}

	var b strings.Builder
	switch format {
	case TailwindJSON:
		body := map[string]any{}
		for k, v := range theme {
			body[k] = v
		}
		if len(extend) > 0 {
			body["extend"] = extend
		}
		content, err := marshalIndent(body)
		if err != nil {
			return nil, err
		}
		out.Name, out.Content = "tokens.json", content
		return []Output{out}, nil
	case TailwindESM, TailwindCJS:
	default:
		return nil, figctl.Newf(figctl.CodeUsage, "invalid tailwind format %q", format).
			WithHint("Use one of: %s.", strings.Join(TailwindFormats, ", "))
	}

	fmt.Fprintf(&b, "/* %s */\n\nconst theme = {\n", d.header())
	for _, key := range tailwindKeys {
		if len(theme[key]) == 0 {
			continue
		}
		if err := writeJSEntry(&b, key, theme[key], 1); err != nil {
			return nil, err
		}
	}
	if len(extend) > 0 {
		b.WriteString("  // Tailwind's theme has no key for these token types; they are kept\n")
		b.WriteString("  // under extend so a project can map them itself.\n")
		body := map[string]any{}
		for k, v := range extend {
			body[k] = v
		}
		if err := writeJSEntry(&b, "extend", body, 1); err != nil {
			return nil, err
		}
	}
	b.WriteString("};\n\n")
	if format == TailwindCJS {
		b.WriteString("module.exports = theme;\n")
		out.Name = "tokens.cjs"
	} else {
		b.WriteString("export default theme;\n")
		out.Name = "tokens.js"
	}
	out.Content = []byte(b.String())
	return []Output{out}, nil
}

// tailwindValue renders a token as the string, or object of strings,
// Tailwind's theme expects.
func tailwindValue(t Token, vars bool) any {
	if len(t.Parts) > 0 {
		body := map[string]any{}
		for _, p := range t.Parts {
			value := p.Value
			if vars && t.CSSName != "" {
				value = "var(--" + t.CSSName + "-" + p.Suffix + ")"
			}
			body[p.Suffix] = value
		}
		return body
	}
	if vars && t.CSSName != "" {
		return "var(--" + t.CSSName + ")"
	}
	if t.CSS == "" {
		return nil
	}
	return t.CSS
}

// tailwindPath drops a leading group segment that only repeats the theme
// key, and falls back to the token name when nothing is left.
func tailwindPath(t Token) []string {
	path := t.Path
	if t.Category != "" && len(path) > 1 && tailwindGroups[strings.ToLower(strings.Join(splitWords(path[0]), ""))] == t.Category {
		path = path[1:]
	}
	return path
}

// put nests a value under a path, reporting a collision when two tokens
// claim the same key with different values.
func put(root map[string]any, path []string, value any, source string) string {
	node := root
	for i, seg := range path {
		if i == len(path)-1 {
			if prev, exists := node[seg]; exists {
				if fmt.Sprint(prev) == fmt.Sprint(value) {
					return ""
				}
				return fmt.Sprintf("tailwind key %q is claimed by more than one token; kept the first and dropped %q", strings.Join(path, "."), source)
			}
			node[seg] = value
			return ""
		}
		child, exists := node[seg]
		if !exists {
			next := map[string]any{}
			node[seg] = next
			node = next
			continue
		}
		next, isGroup := child.(map[string]any)
		if !isGroup {
			return fmt.Sprintf("tailwind key %q is both a value and a group; dropped %q", strings.Join(path[:i+1], "."), source)
		}
		node = next
	}
	return ""
}

// category maps a token to its Tailwind theme key, or the empty string
// when Tailwind has no key for it.
func category(t *Token) string {
	switch t.Type {
	case TypeColor, TypeGradient:
		return "colors"
	case TypeFontFamily:
		return "fontFamily"
	case TypeShadow:
		return "boxShadow"
	case TypeDimension:
		switch {
		case scoped(t.Scopes, "CORNER_RADIUS"):
			return "borderRadius"
		case scoped(t.Scopes, "FONT_SIZE"):
			return "fontSize"
		case scoped(t.Scopes, "LINE_HEIGHT"), scoped(t.Scopes, "LETTER_SPACING"):
			return ""
		default:
			return "spacing"
		}
	case TypeNumber:
		switch {
		case scoped(t.Scopes, "FONT_WEIGHT"):
			return "fontWeight"
		case scoped(t.Scopes, "FONT_SIZE"):
			return "fontSize"
		}
	}
	return ""
}

// writeJSEntry writes one key of a JavaScript object literal, using JSON
// for the value so the output is valid JavaScript and stable.
func writeJSEntry(b *strings.Builder, key string, value any, depth int) error {
	body, err := json.MarshalIndent(value, strings.Repeat("  ", depth), "  ")
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "encoding tailwind theme: "+err.Error())
	}
	fmt.Fprintf(b, "%s%q: %s,\n", strings.Repeat("  ", depth), key, body)
	return nil
}
