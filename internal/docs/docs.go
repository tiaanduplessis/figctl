// Package docs renders the figctl command tree as markdown. The generator
// in cmd/gen-docs writes the result to docs/commands.md and a test asserts
// the committed file still matches the command tree.
package docs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const header = `# figctl command reference

Generated from the command tree by ` + "`make docs`" + `. Do not edit by hand.

Every command accepts the [global flags](#global-flags). Output is JSON when
stdout is not a terminal and a table when it is; ` + "`--json`" + ` forces JSON. See
[the output contract](../README.md#output-contract) for the envelope, exit
codes, and error codes.
`

// Markdown renders the whole command tree rooted at cmd as a single
// markdown document.
func Markdown(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString(header)

	cmds := visible(root)

	b.WriteString("\n## Commands\n\n")
	b.WriteString("| Command | Description |\n| --- | --- |\n")
	for _, c := range cmds {
		path := c.CommandPath()
		fmt.Fprintf(&b, "| [`%s`](#%s) | %s |\n", path, anchor(path), escapeCell(c.Short))
	}

	writeGlobalFlags(&b, root)

	for _, c := range cmds {
		writeCommand(&b, c)
	}

	return b.String()
}

// visible returns every command in the tree except the root, hidden
// commands, and the generated help command, in a stable depth-first order.
func visible(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(parent *cobra.Command) {
		for _, c := range sortedChildren(parent) {
			if c.Hidden || c.Name() == "help" || !c.IsAvailableCommand() && !c.HasSubCommands() {
				continue
			}
			out = append(out, c)
			walk(c)
		}
	}
	walk(root)
	return out
}

func writeGlobalFlags(b *strings.Builder, root *cobra.Command) {
	b.WriteString("\n## Global flags\n\n")
	b.WriteString("These persistent flags are accepted by every command and are not repeated below.\n\n")
	writeFlagTable(b, root.PersistentFlags())
	b.WriteString(`
Configuration precedence is flags, then environment (` + "`FIGMA_TOKEN`, `FIGCTL_PROFILE`, `NO_COLOR`" + `),
then the project config ` + "`.figctl.yaml`" + `, then the user config, then the built-in defaults.
`)
}

func writeCommand(b *strings.Builder, c *cobra.Command) {
	path := c.CommandPath()
	fmt.Fprintf(b, "\n## %s\n\n", path)
	if c.Short != "" {
		fmt.Fprintf(b, "%s\n\n", c.Short)
	}

	fmt.Fprintf(b, "```\n%s\n```\n", strings.TrimRight(useLine(c), "\n"))

	if long := strings.TrimSpace(c.Long); long != "" && long != strings.TrimSpace(c.Short) {
		fmt.Fprintf(b, "\n%s\n", fence(long))
	}

	if aliases := c.Aliases; len(aliases) > 0 {
		fmt.Fprintf(b, "\nAliases: %s\n", "`"+strings.Join(aliases, "`, `")+"`")
	}

	if subs := directSubcommands(c); len(subs) > 0 {
		b.WriteString("\nSubcommands:\n\n")
		for _, s := range subs {
			fmt.Fprintf(b, "- [`%s`](#%s) %s\n", s.CommandPath(), anchor(s.CommandPath()), s.Short)
		}
	}

	if ex := dedent(c.Example); ex != "" {
		b.WriteString("\nExamples:\n\n")
		fmt.Fprintf(b, "```sh\n%s\n```\n", ex)
	}

	local := c.LocalFlags()
	if hasFlags(local) {
		b.WriteString("\nFlags:\n\n")
		writeFlagTable(b, local)
	}

	inherited := c.InheritedFlags()
	if hasFlags(inherited) {
		b.WriteString("\nPlus the [global flags](#global-flags).\n")
	}
}

// directSubcommands lists the immediate children of c that appear in this
// reference.
func directSubcommands(c *cobra.Command) []*cobra.Command {
	return sortedChildren(c)
}

// sortedChildren copies the child commands of c that belong in the
// reference and orders them by name. The copy keeps cobra's own slice
// untouched.
func sortedChildren(c *cobra.Command) []*cobra.Command {
	out := make([]*cobra.Command, 0, len(c.Commands()))
	for _, s := range c.Commands() {
		if s.Hidden || s.Name() == "help" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// dedent trims trailing space and removes the indentation cobra example
// blocks carry, so the lines land flush inside a fenced code block.
func dedent(s string) string {
	lines := strings.Split(strings.TrimRight(s, " \t\n"), "\n")
	indent := -1
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		if i == 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			// The first example line is usually already flush; the rest of
			// the block sets the indentation to remove.
			continue
		}
		if n := len(line) - len(trimmed); indent < 0 || n < indent {
			indent = n
		}
	}
	for i, line := range lines {
		if indent > 0 && len(line) >= indent && strings.TrimSpace(line[:indent]) == "" {
			lines[i] = line[indent:]
		}
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// useLine renders the usage line, adding the parent path that cobra's own
// UseLine leaves off for commands with subcommands.
func useLine(c *cobra.Command) string {
	line := c.UseLine()
	if c.HasAvailableSubCommands() && !strings.Contains(line, "[command]") {
		line += " [command]"
	}
	return line
}

func hasFlags(fs *pflag.FlagSet) bool {
	found := false
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			found = true
		}
	})
	return found
}

func writeFlagTable(b *strings.Builder, fs *pflag.FlagSet) {
	b.WriteString("| Flag | Type | Default | Description |\n| --- | --- | --- | --- |\n")
	var rows [][4]string
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "`--" + f.Name + "`"
		if f.Shorthand != "" {
			name = "`-" + f.Shorthand + "`, " + name
		}
		def := f.DefValue
		if def == "" || def == "[]" {
			def = "-"
		} else {
			def = "`" + def + "`"
		}
		rows = append(rows, [4]string{name, "`" + flagType(f) + "`", def, escapeCell(f.Usage)})
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	for _, r := range rows {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", r[0], r[1], r[2], r[3])
	}
}

// flagType names the flag value type the way the help text does, with the
// pflag slice suffixes normalized.
func flagType(f *pflag.Flag) string {
	t := f.Value.Type()
	switch t {
	case "stringArray", "stringSlice":
		return "string (repeatable)"
	case "float64":
		return "float"
	default:
		return t
	}
}

// fence wraps a long description in a code block when it contains the
// indented example blocks the help text uses, so markdown keeps the layout.
func fence(long string) string {
	for _, line := range strings.Split(long, "\n") {
		if (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) && strings.TrimSpace(line) != "" {
			return "```\n" + long + "\n```"
		}
	}
	return long
}

func escapeCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

// anchor converts a command path into the GitHub heading anchor for it.
func anchor(path string) string {
	return strings.ReplaceAll(path, " ", "-")
}
