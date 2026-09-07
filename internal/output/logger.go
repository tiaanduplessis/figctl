package output

import (
	"fmt"
	"io"
)

// Logger writes progress, warnings, and debug traces to stderr. Data never
// goes through the logger. --quiet silences everything, --verbose enables
// debug output.
type Logger struct {
	w       io.Writer
	quiet   bool
	verbose bool
}

// NewLogger creates a logger writing to w.
func NewLogger(w io.Writer, quiet, verbose bool) *Logger {
	return &Logger{w: w, quiet: quiet, verbose: verbose}
}

// Infof reports progress. Silenced by --quiet.
func (l *Logger) Infof(format string, args ...any) {
	if l == nil || l.quiet {
		return
	}
	fmt.Fprintf(l.w, format+"\n", args...)
}

// Warnf reports a warning. Silenced by --quiet.
func (l *Logger) Warnf(format string, args ...any) {
	if l == nil || l.quiet {
		return
	}
	fmt.Fprintf(l.w, "warning: "+format+"\n", args...)
}

// Debugf reports a trace line. Only shown with --verbose and never with
// --quiet.
func (l *Logger) Debugf(format string, args ...any) {
	if l == nil || l.quiet || !l.verbose {
		return
	}
	fmt.Fprintf(l.w, "debug: "+format+"\n", args...)
}
