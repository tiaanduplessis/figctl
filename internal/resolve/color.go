// Package resolve turns raw Figma values into resolved ones: variable
// alias chains across modes and extended collections, style values read
// from style nodes, color conversion, gradients, and typography. It is a
// library used by the inspector, the tokens exporter, and the commands.
package resolve

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

// Color is an RGBA color with channels in 0..1, as Figma stores them.
type Color figma.Color

// FromFigma converts a Figma color.
func FromFigma(c figma.Color) Color { return Color(c) }

// WithOpacity multiplies the alpha channel by an opacity (a paint or
// effect opacity).
func (c Color) WithOpacity(opacity float64) Color {
	c.A *= opacity
	return c
}

func channel(v float64) int {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return int(math.Round(v * 255))
}

// Hex returns #rrggbb in lowercase, ignoring alpha.
func (c Color) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", channel(c.R), channel(c.G), channel(c.B))
}

// Hex8 returns #rrggbbaa in lowercase when alpha is below 1, otherwise
// the empty string.
func (c Color) Hex8() string {
	if c.A >= 1 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", channel(c.R), channel(c.G), channel(c.B), channel(c.A))
}

// RGBA returns rgba(r, g, b, a) with alpha rounded to two decimals and
// trailing zeros trimmed.
func (c Color) RGBA() string {
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", channel(c.R), channel(c.G), channel(c.B), Num(c.A, 2))
}

// CSS returns the shortest CSS color: hex when opaque, rgba otherwise.
func (c Color) CSS() string {
	if c.A >= 1 {
		return c.Hex()
	}
	return c.RGBA()
}

// Opaque reports whether the alpha channel is 1.
func (c Color) Opaque() bool { return c.A >= 1 }

// ParseHex parses #rgb, #rrggbb, or #rrggbbaa (case insensitive).
func ParseHex(s string) (Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 && len(s) != 8 {
		return Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return Color{}, false
	}
	c := Color{A: 1}
	if len(s) == 8 {
		c.A = float64(v&0xff) / 255
		v >>= 8
	}
	c.B = float64(v&0xff) / 255
	c.G = float64((v>>8)&0xff) / 255
	c.R = float64((v>>16)&0xff) / 255
	return c, true
}

// Num formats a number with at most the given decimals and trailing
// zeros trimmed, for CSS values and JSON friendly output.
func Num(v float64, decimals int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	factor := math.Pow(10, float64(decimals))
	v = math.Round(v*factor) / factor
	if v == 0 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Round rounds to the given decimals.
func Round(v float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	r := math.Round(v*factor) / factor
	if r == 0 {
		return 0
	}
	return r
}

// Px formats a pixel value.
func Px(v float64) string { return Num(v, 2) + "px" }
