// Package output implements the output contract: the JSON envelopes, the
// markdown, table and plain renderers, field projection, and the stderr
// logger.
package output

import (
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// Mode is an output format.
type Mode string

// Supported output modes.
const (
	ModeJSON     Mode = "json"
	ModeMarkdown Mode = "md"
	ModeTable    Mode = "table"
	ModePlain    Mode = "plain"
)

// ParseMode validates a mode name. The empty string is not a valid mode.
func ParseMode(s string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case ModeJSON:
		return ModeJSON, nil
	case ModeMarkdown, "markdown":
		return ModeMarkdown, nil
	case ModeTable:
		return ModeTable, nil
	case ModePlain:
		return ModePlain, nil
	}
	return "", figctl.Newf(figctl.CodeUsage, "unknown output mode %q", s).
		WithHint("Use --output json|md|table|plain.")
}

// ResolveMode picks the output mode: an explicit --output wins, then --json,
// otherwise json when stdout is not a terminal and table when it is.
func ResolveMode(flag string, jsonAlias bool, stdoutIsTTY bool) (Mode, error) {
	if flag != "" {
		return ParseMode(flag)
	}
	if jsonAlias {
		return ModeJSON, nil
	}
	if stdoutIsTTY {
		return ModeTable, nil
	}
	return ModeJSON, nil
}

// ColorEnabled reports whether ANSI color may be used on stdout. It honors
// --no-color, NO_COLOR, TERM=dumb, and requires a terminal.
func ColorEnabled(noColorFlag bool, stdoutIsTTY bool, getenv func(string) string) bool {
	if noColorFlag || !stdoutIsTTY {
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(getenv("TERM"), "dumb") {
		return false
	}
	return true
}
