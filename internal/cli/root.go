// Package cli defines the figctl commands. Each command lives in its own
// file and registers itself with rootCmd from an init function.
package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
)

const rootLong = `figctl is an agent-first Figma CLI: one binary that reads a Figma file
and hands an agent everything it needs to implement the design.

Recommended workflow:
  1. figctl file tree <ref>                sparse outline of pages and frames
  2. figctl node context <ref> --node ID   layout, styles, tokens, and a screenshot for one node
  3. Implement the node in the codebase
  4. figctl render <ref> --node ID         render the design to compare with the result
     figctl diff design.png actual.png --out diff.png
  5. figctl tokens export <ref>            variables and styles as DTCG, CSS, or Tailwind

<ref> is a file key or any Figma URL; a node-id in the URL is used automatically.
Output is JSON when piped and a table on a terminal; add --json to force JSON.
Run figctl <cmd> --help for flags and examples.`

// globalFlags holds the persistent flags every command accepts.
type globalFlags struct {
	Output      string
	JSON        bool
	Fields      string
	Limit       int
	Cursor      string
	Quiet       bool
	Verbose     bool
	NoCache     bool
	Refresh     bool
	FileVersion string
	Timeout     time.Duration
	Yes         bool
	NoColor     bool
	Profile     string
	TokenFile   string
	CacheTTL    time.Duration
	MaxFileMB   int
}

func addGlobalFlags(fs *pflag.FlagSet, g *globalFlags) {
	fs.StringVarP(&g.Output, "output", "o", "", "output format: json, md, table, or plain (default: json when piped, table on a terminal)")
	fs.BoolVar(&g.JSON, "json", false, "alias for --output json")
	fs.StringVar(&g.Fields, "fields", "", "comma separated top level fields to keep in data")
	fs.IntVar(&g.Limit, "limit", 0, "maximum items in list output (0 uses the command default)")
	fs.StringVar(&g.Cursor, "cursor", "", "pagination cursor taken from a previous nextCursor")
	fs.BoolVarP(&g.Quiet, "quiet", "q", false, "no progress or warnings on stderr")
	fs.BoolVarP(&g.Verbose, "verbose", "v", false, "HTTP trace on stderr (tokens redacted)")
	fs.BoolVar(&g.NoCache, "no-cache", false, "bypass the disk cache")
	fs.BoolVar(&g.Refresh, "refresh", false, "refresh the disk cache")
	fs.StringVar(&g.FileVersion, "file-version", "", "pin a file version id")
	fs.DurationVar(&g.Timeout, "timeout", 60*time.Second, "request timeout")
	fs.BoolVar(&g.Yes, "yes", false, "skip confirmations")
	fs.BoolVar(&g.NoColor, "no-color", false, "disable color (NO_COLOR and TERM=dumb are honored too)")
	fs.StringVar(&g.Profile, "profile", "", "profile name (overrides FIGCTL_PROFILE and .figctl.yaml)")
	fs.StringVar(&g.TokenFile, "token-file", "", "read the token from this file instead of the credential store")
	fs.DurationVar(&g.CacheTTL, "cache-ttl", cache.DefaultTTL, "how long a file meta check stays valid before the cache is re-validated")
	fs.IntVar(&g.MaxFileMB, "max-file-mb", 200, "largest file document (MB) fetched whole and cached; bigger files use per-node requests")
	_ = fs.MarkHidden("cache-ttl")
}

var flags globalFlags

var rootCmd = &cobra.Command{
	Use:           "figctl",
	Short:         "Agent-first Figma CLI",
	Long:          rootLong,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	addGlobalFlags(rootCmd.PersistentFlags(), &flags)
}

// Options describes one invocation of the CLI.
type Options struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// runState is the per-invocation state shared between Run and the command
// wrapper.
type runState struct {
	opts    Options
	started bool
	ctx     *Context
}

var state *runState

// Run executes the CLI with the given arguments and streams. It renders any
// error as the error envelope (JSON on stdout in JSON mode, text on stderr
// otherwise) and returns the process exit code.
func Run(opts Options) int {
	resetFlags(rootCmd)
	state = &runState{opts: opts}
	// A nil slice makes cobra fall back to os.Args, which would let the
	// host process's own flags leak into the command.
	args := opts.Args
	if args == nil {
		args = []string{}
	}
	rootCmd.SetArgs(args)
	rootCmd.SetIn(opts.Stdin)
	rootCmd.SetOut(opts.Stdout)
	rootCmd.SetErr(opts.Stderr)

	err := rootCmd.Execute()
	if err == nil {
		return figctl.ExitOK
	}
	if !state.started {
		err = usageError(err)
	}
	printer := state.errorPrinter()
	var printed *printedError
	if errors.As(err, &printed) {
		// The data envelope is already on stdout; only the human readable
		// summary goes to stderr, and the exit code reports the failure.
		if printer.Mode != output.ModeJSON {
			_ = printer.PrintError(printed.err)
		}
		return figctl.ExitCode(err)
	}
	if perr := printer.PrintError(err); perr != nil {
		return figctl.ExitError
	}
	return figctl.ExitCode(err)
}

func usageError(err error) error {
	if e := figctl.From(err); e.Code != figctl.CodeInternal {
		return e
	}
	return figctl.Wrap(figctl.CodeUsage, err, err.Error()).
		WithHint("Run figctl --help, or figctl <cmd> --help, for usage.")
}

// errorPrinter returns the printer of the running command, or one built
// from a best-effort scan of the global flags when no command started (for
// example on an unknown command).
func (s *runState) errorPrinter() *output.Printer {
	if s.ctx != nil {
		return s.ctx.Printer
	}
	g := globalFlags{}
	fs := pflag.NewFlagSet("prescan", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.SetOutput(io.Discard)
	addGlobalFlags(fs, &g)
	_ = fs.Parse(s.opts.Args)
	stdoutTTY := isTerminal(s.opts.Stdout)
	mode, err := output.ResolveMode(g.Output, g.JSON, stdoutTTY)
	if err != nil {
		mode = output.ModeTable
		if !stdoutTTY {
			mode = output.ModeJSON
		}
	}
	return &output.Printer{
		Out:   s.opts.Stdout,
		Err:   s.opts.Stderr,
		Mode:  mode,
		Color: output.ColorEnabled(g.NoColor, stdoutTTY, os.Getenv),
		Log:   output.NewLogger(s.opts.Stderr, g.Quiet, g.Verbose),
	}
}

// run wraps a command body so it receives a Context and marks the run as
// started, which separates usage errors from runtime errors.
func run(fn func(ctx *Context, cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		state.started = true
		ctx, err := newContext(state.opts, cmd, &flags)
		if err != nil {
			return err
		}
		state.ctx = ctx
		return fn(ctx, cmd, args)
	}
}

// resetFlags restores every flag to its default so the command tree can be
// executed more than once in one process.
func resetFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if slice, ok := f.Value.(pflag.SliceValue); ok {
			// Set appends on slice flags, so restore them explicitly.
			_ = slice.Replace([]string{})
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	cmd.PersistentFlags().VisitAll(reset)
	cmd.LocalNonPersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// commandName converts a cobra command path into the envelope command name.
func commandName(cmd *cobra.Command) string {
	path := strings.TrimPrefix(cmd.CommandPath(), rootCmd.Name())
	return strings.ReplaceAll(strings.TrimSpace(path), " ", ".")
}

// Root returns the root command with the full command tree attached. It
// exists for the documentation generator and completion tooling; the
// returned command shares process-wide flag state and is not safe to
// execute concurrently with Run.
func Root() *cobra.Command {
	return rootCmd
}
