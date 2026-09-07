package tokens

import (
	"strings"
	"unicode"
)

// Name case options for NormalizePath.
const (
	// CaseKebab lowercases every segment and joins words with "-".
	CaseKebab = "kebab"
	// CaseCamel lowercases the first word of a segment and capitalizes
	// the rest.
	CaseCamel = "camel"
	// CaseSnake lowercases every segment and joins words with "_".
	CaseSnake = "snake"
	// CaseNone keeps the Figma segment as written, minus the characters
	// DTCG forbids in names.
	CaseNone = "none"
)

// NameCases lists the accepted --name-case values.
var NameCases = []string{CaseKebab, CaseCamel, CaseSnake, CaseNone}

// ValidNameCase reports whether v is a supported name case.
func ValidNameCase(v string) bool {
	for _, c := range NameCases {
		if c == v {
			return true
		}
	}
	return false
}

// NormalizePath splits a Figma name on "/" (the group separator Figma
// uses in variable and style names) and normalizes every segment with
// the given name case. Empty segments are dropped; a name that
// normalizes to nothing yields a single "token" segment so the result is
// always addressable.
func NormalizePath(name, nameCase string) []string {
	parts := strings.Split(name, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		seg := normalizeSegment(part, nameCase)
		if seg == "" {
			continue
		}
		out = append(out, seg)
	}
	if len(out) == 0 {
		return []string{"token"}
	}
	return out
}

// normalizeSegment applies the name case to one path segment.
func normalizeSegment(s, nameCase string) string {
	s = stripInvalid(s)
	if nameCase == CaseNone {
		return strings.TrimSpace(s)
	}
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}
	switch nameCase {
	case CaseCamel:
		var b strings.Builder
		for i, w := range words {
			if i == 0 {
				b.WriteString(strings.ToLower(w))
				continue
			}
			b.WriteString(capitalize(w))
		}
		return b.String()
	case CaseSnake:
		return strings.ToLower(strings.Join(words, "_"))
	default:
		return strings.ToLower(strings.Join(words, "-"))
	}
}

// stripInvalid removes the characters DTCG forbids in token and group
// names: "{" and "}" delimit references, "." separates path segments in a
// reference, and a leading "$" marks format properties.
func stripInvalid(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '{' || r == '}' || r == '.' || r == '$':
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitWords breaks a segment into words on separators and on lower to
// upper case transitions, so "Show helper" and "showHelper" both give
// ["Show", "helper"] and ["show", "Helper"].
func splitWords(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	prev := rune(0)
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && unicode.IsLower(prev) {
				flush()
			}
			cur = append(cur, r)
		default:
			flush()
		}
		prev = r
	}
	flush()
	return words
}

func capitalize(w string) string {
	runes := []rune(strings.ToLower(w))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// kebab converts one already normalized segment to kebab case, which is
// what CSS custom property names use whatever --name-case produced.
func kebab(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(words, "-"))
}

// cssName joins a token path into a CSS custom property name without the
// leading "--".
func cssName(path []string) string {
	parts := make([]string, 0, len(path))
	for _, seg := range path {
		if k := kebab(seg); k != "" {
			parts = append(parts, k)
		}
	}
	return strings.Join(parts, "-")
}
