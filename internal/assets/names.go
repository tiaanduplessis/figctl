// Package assets renders nodes to image files, discovers exportable
// layers, icons, and image fills in a document, downloads image fills, and
// cleans SVG output. Everything is cached by (node, format, scale, version)
// so repeated exports of an unchanged file cost no image requests.
package assets

import (
	"strconv"
	"strings"
)

// NameBy values for RenderRequest.NameBy.
const (
	NameByName = "name"
	NameByID   = "id"
)

// Slug converts a node name into a filename stem: lowercase ASCII letters
// and digits, with every other run of characters collapsed into one
// hyphen. "Button/Primary" becomes "button-primary". An empty result
// falls back to "node".
func Slug(name string) string {
	var b strings.Builder
	pending := false
	for _, r := range strings.ToLower(name) {
		alnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !alnum {
			pending = b.Len() > 0
			continue
		}
		if pending {
			b.WriteByte('-')
			pending = false
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "node"
	}
	return b.String()
}

// IDSlug converts a node id into a filename fragment: "2:6" becomes
// "2-6" and "I2:5;3:21" becomes "i2-5-3-21".
func IDSlug(id string) string {
	return Slug(id)
}

// ScaleSuffix returns the filename suffix for a render scale: "" at 1x,
// "@2x" at 2x, "@1.5x" at 1.5x.
func ScaleSuffix(scale float64) string {
	if scale == 0 || scale == 1 {
		return ""
	}
	return "@" + strconv.FormatFloat(scale, 'f', -1, 64) + "x"
}

// Extension returns the file extension for a render format.
func Extension(format string) string {
	f := strings.ToLower(format)
	if f == "jpeg" {
		return "jpg"
	}
	return f
}

// fileName builds one candidate filename.
func fileName(stem, suffix, ext string) string {
	return stem + suffix + "." + ext
}

// assignNames gives every item a unique filename, in order, so the result
// is deterministic. The first choice is slug(name)+suffix+ext; a collision
// appends the node id (button-primary-2-6.png), and a further collision
// (the same node exported twice with the same suffix) appends a counter.
func assignNames(items []Item, nameBy string) []string {
	used := map[string]bool{}
	names := make([]string, len(items))
	for i, it := range items {
		stem := Slug(it.Name)
		if nameBy == NameByID || it.Name == "" {
			stem = IDSlug(it.NodeID)
		}
		suffix := it.Suffix
		if suffix == "" {
			suffix = ScaleSuffix(it.Scale)
		}
		ext := Extension(it.Format)
		name := fileName(stem, suffix, ext)
		if used[name] {
			withID := stem
			if nameBy != NameByID {
				withID = stem + "-" + IDSlug(it.NodeID)
			}
			name = fileName(withID, suffix, ext)
			for n := 2; used[name]; n++ {
				name = fileName(withID+"-"+strconv.Itoa(n), suffix, ext)
			}
		}
		used[name] = true
		names[i] = name
	}
	return names
}
