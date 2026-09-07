package assets

import (
	"regexp"
	"strings"
)

// SVGCleanOptions selects the SVG post-processing steps.
type SVGCleanOptions struct {
	// StripIDs removes id attributes that nothing in the document
	// references (clip paths, gradients, and masks keep theirs).
	StripIDs bool
	// StripDimensions removes width and height from the root element when
	// it has a viewBox, so the SVG scales with its container.
	StripDimensions bool
	// CurrentColor replaces fill and stroke colors with currentColor when
	// the icon uses a single color.
	CurrentColor bool
}

// Enabled reports whether any step is selected.
func (o SVGCleanOptions) Enabled() bool {
	return o.StripIDs || o.StripDimensions || o.CurrentColor
}

var (
	svgRootRe      = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	svgDimensionRe = regexp.MustCompile(`\s+(?:width|height)="[^"]*"`)
	svgViewBoxRe   = regexp.MustCompile(`\sviewBox="[^"]*"`)
	svgIDRe        = regexp.MustCompile(`\s+id="([^"]*)"`)
	svgRefRe       = regexp.MustCompile(`#([A-Za-z0-9_:.-]+)`)
	svgPaintRe     = regexp.MustCompile(`(\s)(fill|stroke)="([^"]*)"`)
)

// CleanSVG applies the selected steps and returns the new document.
func CleanSVG(data []byte, opts SVGCleanOptions) []byte {
	s := string(data)
	if opts.StripDimensions {
		s = stripDimensions(s)
	}
	if opts.StripIDs {
		s = stripIDs(s)
	}
	if opts.CurrentColor {
		s = currentColor(s)
	}
	return []byte(s)
}

func stripDimensions(s string) string {
	loc := svgRootRe.FindStringIndex(s)
	if loc == nil {
		return s
	}
	root := s[loc[0]:loc[1]]
	if !svgViewBoxRe.MatchString(root) {
		return s
	}
	return s[:loc[0]] + svgDimensionRe.ReplaceAllString(root, "") + s[loc[1]:]
}

func stripIDs(s string) string {
	referenced := map[string]bool{}
	for _, m := range svgRefRe.FindAllStringSubmatch(s, -1) {
		referenced[m[1]] = true
	}
	return svgIDRe.ReplaceAllStringFunc(s, func(attr string) string {
		id := svgIDRe.FindStringSubmatch(attr)[1]
		if referenced[id] {
			return attr
		}
		return ""
	})
}

// SVGColors returns the distinct fill and stroke colors used by attribute
// values, lowercased, ignoring none, currentColor, inherit, transparent,
// and paint server references.
func SVGColors(data []byte) []string {
	seen := map[string]bool{}
	var colors []string
	for _, m := range svgPaintRe.FindAllStringSubmatch(string(data), -1) {
		v := normalizePaint(m[3])
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		colors = append(colors, v)
	}
	return colors
}

func normalizePaint(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case v == "", v == "none", v == "currentcolor", v == "inherit", v == "transparent":
		return ""
	case strings.HasPrefix(v, "url("):
		return ""
	}
	return v
}

func currentColor(s string) string {
	colors := SVGColors([]byte(s))
	if len(colors) != 1 {
		return s
	}
	return svgPaintRe.ReplaceAllStringFunc(s, func(attr string) string {
		m := svgPaintRe.FindStringSubmatch(attr)
		if normalizePaint(m[3]) != colors[0] {
			return attr
		}
		return m[1] + m[2] + `="currentColor"`
	})
}
