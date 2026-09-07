package cli

import (
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// Context is everything a command body needs: flags, streams, the printer,
// the logger, and lazily opened config and credential stores.
type Context struct {
	Flags       *globalFlags
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	StdinIsTTY  bool
	StdoutIsTTY bool
	Printer     *output.Printer
	Log         *output.Logger
	// Command is the dotted command name used in the envelope.
	Command string

	profile *output.ProfileInfo
	file    *output.FileInfo
	store   config.Store
}

func newContext(opts Options, cmd *cobra.Command, g *globalFlags) (*Context, error) {
	stdoutTTY := isTerminal(opts.Stdout)
	mode, err := output.ResolveMode(g.Output, g.JSON, stdoutTTY)
	if err != nil {
		return nil, err
	}
	log := output.NewLogger(opts.Stderr, g.Quiet, g.Verbose)
	ctx := &Context{
		Flags:       g,
		Stdin:       opts.Stdin,
		Stdout:      opts.Stdout,
		Stderr:      opts.Stderr,
		StdinIsTTY:  isTerminal(opts.Stdin),
		StdoutIsTTY: stdoutTTY,
		Log:         log,
		Command:     commandName(cmd),
	}
	ctx.Printer = &output.Printer{
		Out:    opts.Stdout,
		Err:    opts.Stderr,
		Mode:   mode,
		Fields: splitFields(g.Fields),
		Color:  output.ColorEnabled(g.NoColor, stdoutTTY, os.Getenv),
		Log:    log,
	}
	return ctx, nil
}

func splitFields(s string) []string {
	var fields []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

// Envelope builds a success envelope for the running command, including the
// active profile when one was set.
func (c *Context) Envelope(data any) *output.Envelope {
	env := output.NewEnvelope(c.Command, data)
	env.Profile = c.profile
	env.File = c.file
	return env
}

// Print renders a success envelope for data.
func (c *Context) Print(data any) error {
	return c.Printer.Print(c.Envelope(data))
}

// SetProfile records the active profile for the envelope.
func (c *Context) SetProfile(active *config.Active) {
	if active == nil {
		c.profile = nil
		return
	}
	c.profile = &output.ProfileInfo{Name: active.Name, Handle: active.Profile.Handle}
}

// SetFile records the file a command touched for the envelope.
func (c *Context) SetFile(info *output.FileInfo) {
	c.file = info
}

// Store opens the credential store once.
func (c *Context) Store() (config.Store, error) {
	if c.store != nil {
		return c.store, nil
	}
	store, err := config.OpenStore(c.Log.Warnf)
	if err != nil {
		return nil, err
	}
	c.store = store
	return store, nil
}

// Cwd returns the working directory.
func (c *Context) Cwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "reading working directory: "+err.Error())
	}
	return cwd, nil
}

// Home returns the user home directory, which is where user level agent
// skill directories live.
func (c *Context) Home() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "reading home directory: "+err.Error())
	}
	return home, nil
}

// ResolveOptions builds the profile resolution options from the flags.
func (c *Context) ResolveOptions(withStore bool) (config.Options, error) {
	cwd, err := c.Cwd()
	if err != nil {
		return config.Options{}, err
	}
	opts := config.Options{ProfileFlag: c.Flags.Profile, TokenFile: c.Flags.TokenFile, Cwd: cwd}
	if withStore {
		opts.Store, err = c.Store()
		if err != nil {
			return config.Options{}, err
		}
	}
	return opts, nil
}
